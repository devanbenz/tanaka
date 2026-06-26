// Package cli dispatches Tanaka subcommands.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/app"
	"github.com/devandbenz/tanaka/internal/build"
	"github.com/devandbenz/tanaka/internal/ingest"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
	"github.com/devandbenz/tanaka/internal/study"
	"github.com/devandbenz/tanaka/internal/ui"
	"github.com/devandbenz/tanaka/internal/web"
)

const version = "0.0.1"

const helpText = `Tanaka turns technical content into a study-then-build learning flow.

Usage:
  tanaka <command> [args]

Commands:
  add <file|url|->   Import content from a file, a URL, or stdin (-)
  list               List imported sources
  remove <id>        Remove an imported source (and its sections)
  prepare <id>       Generate the study package for a source (quizzes etc.)
  build <id> --lang L [--difficulty D]   Scaffold a build workspace for a source
  serve [--port N]   Start the local study web UI (default 127.0.0.1:7777)
  version            Print the version
  help               Show this help

Run with no command to show this help.
`

func printHelp(w io.Writer) { fmt.Fprint(w, helpText) }

// deps carries injectable dependencies so commands are testable.
type deps struct {
	invoker agent.Invoker
	store   store.Store
	newID   func() string
	stdin   io.Reader
}

// Run builds real dependencies and dispatches the command.
func Run(args []string, stdout, stderr io.Writer) int {
	// Handle commands that need no DB before opening one. run also handles these
	// for direct (test) calls that bypass Run.
	if len(args) == 0 || args[0] == "help" {
		printHelp(stdout)
		return 0
	}
	if args[0] == "version" {
		fmt.Fprintf(stdout, "tanaka %s\n", version)
		return 0
	}

	dbPath, err := app.DBPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	st, err := store.NewSQLite(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	d := deps{
		invoker: agent.NewClaude(""),
		store:   st,
		newID:   app.NewID,
		stdin:   os.Stdin,
	}
	return run(ctx, args, d, stdout, stderr)
}

// run dispatches subcommands using the provided dependencies.
func run(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	switch args[0] {
	case "help":
		printHelp(stdout)
		return 0
	case "version":
		fmt.Fprintf(stdout, "tanaka %s\n", version)
		return 0
	case "add":
		return cmdAdd(ctx, args[1:], d, stdout, stderr)
	case "list":
		return cmdList(ctx, d, stdout, stderr)
	case "remove":
		return cmdRemove(ctx, args[1:], d, stdout, stderr)
	case "prepare":
		return cmdPrepare(ctx, args[1:], d, stdout, stderr)
	case "build":
		return cmdBuild(ctx, args[1:], d, stdout, stderr)
	case "serve":
		return cmdServe(ctx, args[1:], d, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\nrun 'tanaka help' for usage\n", args[0])
		return 2
	}
}

func cmdAdd(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka add <file|url|->")
		return 2
	}
	if err := d.invoker.Check(ctx); err != nil {
		fmt.Fprintf(stderr, "claude CLI unavailable: %v\nis it installed and logged in? try: claude login\n", err)
		return 1
	}
	label := args[0]
	if label == "-" {
		label = "stdin"
	}
	sp := ui.NewSpinner(stderr, fmt.Sprintf("reading & structuring %s", label))
	sp.Start()
	src, err := ingest.Ingest(ctx, d.invoker, args[0], d.stdin, d.newID)
	if err != nil {
		sp.Fail("could not structure the source")
		fmt.Fprintf(stderr, "ingest: %v\n", err)
		return 1
	}
	sp.Stop(fmt.Sprintf("structured %q into %d sections", src.Title, len(src.Sections)))
	if err := d.store.SaveSource(ctx, src); err != nil {
		fmt.Fprintf(stderr, "save: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "added %q (%d sections) as %s\n", src.Title, len(src.Sections), src.ID)
	return 0
}

func cmdRemove(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka remove <id>")
		return 2
	}
	if err := d.store.DeleteSource(ctx, args[0]); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fmt.Fprintf(stderr, "no source with id %s (use 'tanaka list' to see ids)\n", args[0])
			return 1
		}
		fmt.Fprintf(stderr, "remove: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "removed %s\n", args[0])
	return 0
}

func cmdPrepare(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka prepare <id>")
		return 2
	}
	if err := d.invoker.Check(ctx); err != nil {
		fmt.Fprintf(stderr, "claude CLI unavailable: %v\nis it installed and logged in? try: claude login\n", err)
		return 1
	}
	src, err := d.store.GetSource(ctx, args[0])
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fmt.Fprintf(stderr, "no source with id %s (use 'tanaka list' to see ids)\n", args[0])
			return 1
		}
		fmt.Fprintf(stderr, "prepare: %v\n", err)
		return 1
	}
	onSection := func(i, total int, title string) {
		fmt.Fprintf(stderr, "preparing section %d/%d: %s\n", i+1, total, title)
	}
	if err := study.PrepareSource(ctx, d.invoker, d.store, src, d.newID, onSection); err != nil {
		fmt.Fprintf(stderr, "prepare: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "prepared %q (%d sections)\n", src.Title, len(src.Sections))
	return 0
}

func cmdList(ctx context.Context, d deps, stdout, stderr io.Writer) int {
	sources, err := d.store.ListSources(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return 1
	}
	if len(sources) == 0 {
		fmt.Fprintln(stdout, "no sources yet; add one with: tanaka add <file|url|->")
		return 0
	}
	for _, s := range sources {
		fmt.Fprintf(stdout, "%s  %s  (%s)\n", s.ID, s.Title, s.Origin)
	}
	return 0
}

func cmdBuild(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language: rust|go|cpp|c|python")
	diff := fs.String("difficulty", model.DiffSpecTests, "difficulty: guided|spec+tests|blank-page")
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka build <id> --lang <L> [--difficulty <D>]")
		return 2
	}
	id := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if !model.ValidLanguage(*lang) {
		fmt.Fprintf(stderr, "invalid or missing --language (use rust|go|cpp|c|python)\n")
		return 2
	}
	if !model.ValidDifficulty(*diff) {
		fmt.Fprintf(stderr, "invalid --difficulty (use guided|spec+tests|blank-page)\n")
		return 2
	}
	if err := d.invoker.Check(ctx); err != nil {
		fmt.Fprintf(stderr, "claude CLI unavailable: %v\nis it installed and logged in? try: claude login\n", err)
		return 1
	}
	src, err := d.store.GetSource(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fmt.Fprintf(stderr, "no source with id %s (use 'tanaka list' to see ids)\n", id)
			return 1
		}
		fmt.Fprintf(stderr, "build: %v\n", err)
		return 1
	}
	dataDir, err := app.DataDir()
	if err != nil {
		fmt.Fprintf(stderr, "build: %v\n", err)
		return 1
	}
	buildsDir := filepath.Join(dataDir, "builds")
	b, err := build.StartBuild(ctx, d.invoker, d.store, src, *lang, *diff, d.newID, buildsDir)
	if err != nil {
		fmt.Fprintf(stderr, "build: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "build workspace: %s\n", b.Workspace)
	fmt.Fprintf(stdout, "open it in your editor. steps:\n")
	for _, st := range b.Steps {
		fmt.Fprintf(stdout, "  %d. %s\n", st.Idx+1, st.Goal)
	}
	return 0
}

func cmdServe(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", 7777, "port to listen on")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *port <= 0 || *port > 65535 {
		fmt.Fprintf(stderr, "invalid port %d (must be 1-65535)\n", *port)
		return 2
	}
	srv, err := web.NewServer(d.store, d.invoker, d.newID)
	if err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 1
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	fmt.Fprintf(stdout, "Tanaka study UI on http://%s  (Ctrl-C to stop)\n", addr)
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutCtx)
	}()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}
