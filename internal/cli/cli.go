// Package cli dispatches Tanaka subcommands.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/app"
	"github.com/devandbenz/tanaka/internal/ingest"
	"github.com/devandbenz/tanaka/internal/store"
)

const version = "0.0.1"

// deps carries injectable dependencies so commands are testable.
type deps struct {
	invoker agent.Invoker
	store   store.Store
	newID   func() string
	stdin   io.Reader
}

// Run builds real dependencies and dispatches the command.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka <command> [args]")
		return 2
	}
	// Short-circuit for version before opening the DB; run also handles version
	// for direct (test) calls that bypass Run.
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
		fmt.Fprintln(stderr, "usage: tanaka <command> [args]")
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "tanaka %s\n", version)
		return 0
	case "add":
		return cmdAdd(ctx, args[1:], d, stdout, stderr)
	case "list":
		return cmdList(ctx, d, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
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
	raw, err := ingest.Read(ctx, args[0], d.stdin)
	if err != nil {
		fmt.Fprintf(stderr, "read: %v\n", err)
		return 1
	}
	src, err := ingest.Structure(ctx, d.invoker, raw, d.newID)
	if err != nil {
		fmt.Fprintf(stderr, "structure: %v\n", err)
		return 1
	}
	if err := d.store.SaveSource(ctx, src); err != nil {
		fmt.Fprintf(stderr, "save: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "added %q (%d sections) as %s\n", src.Title, len(src.Sections), src.ID)
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
