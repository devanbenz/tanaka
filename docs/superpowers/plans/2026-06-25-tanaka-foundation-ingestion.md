# Tanaka Foundation & Ingestion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundation of Tanaka — project scaffold, persistent store, the agent-invocation boundary, and content ingestion — so that `tanaka add <file|url|->` ingests technical content into structured, stored sections and `tanaka list` shows them.

**Architecture:** A single Go binary owns all state and orchestration. Ingestion reads raw bytes (file/URL/stdin), hands them to an `Invoker` boundary that shells out to `claude --bare -p` for structuring into sections, and persists the result in SQLite under `~/.tanaka/`. The `Invoker` is an interface so tests use a fake and never spend tokens.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure-Go SQLite, no cgo → single static binary), Go standard library for HTTP/CLI/JSON. The `claude` CLI is an external runtime dependency invoked as a subprocess.

## Global Constraints

- Module path: `github.com/devandbenz/tanaka` (adjust if the repo moves; used verbatim in imports below).
- Go version floor: `go 1.26`.
- Pure-Go only — no cgo. SQLite via `modernc.org/sqlite`. This keeps the binary statically linkable.
- The only third-party dependency permitted in this plan is `modernc.org/sqlite`. Everything else is the standard library.
- All agent calls go through the `agent.Invoker` interface. No code outside `internal/agent` may exec `claude` directly.
- State lives under `~/.tanaka/` (`tanaka.db` for data). Resolve via `os.UserHomeDir()`.
- Docs/README style: plain and minimal — no marketing voice, no emoji, no filler.

---

### Task 1: Project scaffold and `version` command

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `internal/cli/cli.go`
- Test: `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `cli.Run(args []string, stdout, stderr io.Writer) int` — the single CLI entrypoint dispatching subcommands; returns a process exit code. `main.go` calls it.

- [ ] **Step 1: Initialize the module**

Run:
```bash
cd /home/devan/tanaka
go mod init github.com/devandbenz/tanaka
```
Expected: creates `go.mod` containing `module github.com/devandbenz/tanaka` and `go 1.26`.

- [ ] **Step 2: Write the failing test**

Create `internal/cli/cli_test.go`:
```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "tanaka") {
		t.Fatalf("stdout = %q, want it to contain %q", out.String(), "tanaka")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"frobnicate"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero for unknown command")
	}
	if !strings.Contains(errOut.String(), "unknown") {
		t.Fatalf("stderr = %q, want it to mention %q", errOut.String(), "unknown")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/cli/`
Expected: FAIL — `undefined: Run` (package does not compile).

- [ ] **Step 4: Write minimal implementation**

Create `internal/cli/cli.go`:
```go
// Package cli dispatches Tanaka subcommands.
package cli

import (
	"fmt"
	"io"
)

const version = "0.0.1"

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka <command> [args]")
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "tanaka %s\n", version)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		return 2
	}
}
```

Create `main.go`:
```go
package main

import (
	"os"

	"github.com/devandbenz/tanaka/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/`
Expected: PASS (both tests).

- [ ] **Step 6: Verify the binary builds and runs**

Run: `go build -o /tmp/tanaka . && /tmp/tanaka version`
Expected: prints `tanaka 0.0.1`.

- [ ] **Step 7: Commit**

```bash
git add go.mod main.go internal/cli/
git commit -m "feat: project scaffold and version command"
```

---

### Task 2: Domain model and SQLite store

**Files:**
- Create: `internal/model/model.go`
- Create: `internal/store/store.go` (interface + schema)
- Create: `internal/store/sqlite.go` (implementation)
- Test: `internal/store/sqlite_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `model.Source{ID, Title, Origin string; CreatedAt time.Time; Sections []model.Section}`
  - `model.Section{ID, SourceID string; Idx int; Title, Markdown string}`
  - `store.Store` interface: `SaveSource(ctx context.Context, s *model.Source) error`, `GetSource(ctx context.Context, id string) (*model.Source, error)`, `ListSources(ctx context.Context) ([]*model.Source, error)`, `Close() error`
  - `store.NewSQLite(path string) (store.Store, error)` — opens/creates the DB and applies schema.
  - `store.ErrNotFound` sentinel error returned by `GetSource` when the id is absent.

- [ ] **Step 1: Add the SQLite dependency**

Run:
```bash
go get modernc.org/sqlite@latest
```
Expected: `go.mod` gains a `require modernc.org/sqlite ...` line; `go.sum` is updated.

- [ ] **Step 2: Write the domain types**

Create `internal/model/model.go`:
```go
// Package model holds Tanaka's core domain types.
package model

import "time"

// Source is a piece of ingested technical content split into ordered sections.
type Source struct {
	ID        string
	Title     string
	Origin    string // file path, URL, or "stdin"
	CreatedAt time.Time
	Sections  []Section
}

// Section is one ordered chunk of a Source's content.
type Section struct {
	ID       string
	SourceID string
	Idx      int
	Title    string
	Markdown string
}
```

- [ ] **Step 3: Write the failing test**

Create `internal/store/sqlite_test.go`:
```go
package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func newTestStore(t *testing.T) Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSaveAndGetSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	src := &model.Source{
		ID:        "src1",
		Title:     "A Paper",
		Origin:    "paper.pdf",
		CreatedAt: time.Unix(1000, 0).UTC(),
		Sections: []model.Section{
			{ID: "sec1", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# Intro"},
			{ID: "sec2", SourceID: "src1", Idx: 1, Title: "Method", Markdown: "# Method"},
		},
	}
	if err := s.SaveSource(ctx, src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	got, err := s.GetSource(ctx, "src1")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.Title != "A Paper" || got.Origin != "paper.pdf" {
		t.Fatalf("got %+v, want title/origin to match", got)
	}
	if len(got.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(got.Sections))
	}
	if got.Sections[0].Title != "Intro" || got.Sections[1].Idx != 1 {
		t.Fatalf("sections not round-tripped in order: %+v", got.Sections)
	}
}

func TestGetSourceNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetSource(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if err := s.SaveSource(ctx, &model.Source{ID: id, Title: id, Origin: "x", CreatedAt: time.Unix(1, 0)}); err != nil {
			t.Fatalf("SaveSource %s: %v", id, err)
		}
	}
	list, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d sources, want 2", len(list))
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/store/`
Expected: FAIL — `undefined: Store`, `undefined: NewSQLite`, `undefined: ErrNotFound`.

- [ ] **Step 5: Write the interface and schema**

Create `internal/store/store.go`:
```go
// Package store persists Tanaka's domain objects.
package store

import (
	"context"
	"errors"

	"github.com/devandbenz/tanaka/internal/model"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Store persists sources and their sections.
type Store interface {
	SaveSource(ctx context.Context, s *model.Source) error
	GetSource(ctx context.Context, id string) (*model.Source, error)
	ListSources(ctx context.Context) ([]*model.Source, error)
	Close() error
}

const schema = `
CREATE TABLE IF NOT EXISTS sources (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL,
	origin     TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sections (
	id        TEXT PRIMARY KEY,
	source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
	idx       INTEGER NOT NULL,
	title     TEXT NOT NULL,
	markdown  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sections_source ON sections(source_id, idx);
`
```

- [ ] **Step 6: Write the SQLite implementation**

Create `internal/store/sqlite.go`:
```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

// NewSQLite opens (creating if needed) the database at path and applies the schema.
func NewSQLite(path string) (Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) SaveSource(ctx context.Context, src *model.Source) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO sources (id, title, origin, created_at) VALUES (?, ?, ?, ?)`,
		src.ID, src.Title, src.Origin, src.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert source: %w", err)
	}
	for _, sec := range src.Sections {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO sections (id, source_id, idx, title, markdown) VALUES (?, ?, ?, ?, ?)`,
			sec.ID, src.ID, sec.Idx, sec.Title, sec.Markdown)
		if err != nil {
			return fmt.Errorf("insert section %s: %w", sec.ID, err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) GetSource(ctx context.Context, id string) (*model.Source, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, title, origin, created_at FROM sources WHERE id = ?`, id)
	var src model.Source
	var created int64
	if err := row.Scan(&src.ID, &src.Title, &src.Origin, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	src.CreatedAt = time.Unix(created, 0).UTC()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_id, idx, title, markdown FROM sections WHERE source_id = ? ORDER BY idx`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sec model.Section
		if err := rows.Scan(&sec.ID, &sec.SourceID, &sec.Idx, &sec.Title, &sec.Markdown); err != nil {
			return nil, err
		}
		src.Sections = append(src.Sections, sec)
	}
	return &src, rows.Err()
}

func (s *sqliteStore) ListSources(ctx context.Context) ([]*model.Source, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, origin, created_at FROM sources ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Source
	for rows.Next() {
		var src model.Source
		var created int64
		if err := rows.Scan(&src.ID, &src.Title, &src.Origin, &created); err != nil {
			return nil, err
		}
		src.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, &src)
	}
	return out, rows.Err()
}

func (s *sqliteStore) Close() error { return s.db.Close() }
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/store/`
Expected: PASS (all three tests).

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/model/ internal/store/
git commit -m "feat: domain model and SQLite store"
```

---

### Task 3: Agent invoker boundary

**Files:**
- Create: `internal/agent/agent.go` (interface, Job, fake)
- Create: `internal/agent/claude.go` (headless implementation + CLI check)
- Test: `internal/agent/claude_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `agent.Job{Prompt string; Schema string}` — a prompt plus the JSON schema the response must satisfy.
  - `agent.Invoker` interface: `Invoke(ctx context.Context, job Job) (json.RawMessage, error)` — returns the structured output object as raw JSON.
  - `agent.Fake{Responses map[string]json.RawMessage; Err error; Calls []Job}` implementing `Invoker`, matching on a substring of `job.Prompt` (used by later tasks' tests).
  - `agent.NewClaude(binary string) *Claude` — real implementation; `binary` defaults to `"claude"` when empty.
  - `agent.(*Claude).Check(ctx context.Context) error` — verifies the CLI is runnable (`claude --version`).

- [ ] **Step 1: Write the failing test**

Create `internal/agent/claude_test.go`:
```go
package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeStubClaude writes a fake `claude` executable that echoes a fixed
// --output-format json envelope with the given structured_output payload.
func writeStubClaude(t *testing.T, structuredOutput string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\n" +
		"cat <<'EOF'\n" +
		`{"result":"ok","structured_output":` + structuredOutput + "}\n" +
		"EOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func TestClaudeInvokeReturnsStructuredOutput(t *testing.T) {
	stub := writeStubClaude(t, `{"title":"T","sections":[]}`)
	c := NewClaude(stub)
	raw, err := c.Invoke(context.Background(), Job{Prompt: "hi", Schema: `{"type":"object"}`})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var got struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Title != "T" {
		t.Fatalf("title = %q, want T", got.Title)
	}
}

func TestFakeInvokerMatchesOnPromptSubstring(t *testing.T) {
	f := &Fake{Responses: map[string]json.RawMessage{
		"structure": json.RawMessage(`{"ok":true}`),
	}}
	raw, err := f.Invoke(context.Background(), Job{Prompt: "please structure this content"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("raw = %s, want {\"ok\":true}", raw)
	}
	if len(f.Calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(f.Calls))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/`
Expected: FAIL — `undefined: NewClaude`, `undefined: Job`, `undefined: Fake`.

- [ ] **Step 3: Write the interface and fake**

Create `internal/agent/agent.go`:
```go
// Package agent is the single boundary for invoking the coding agent.
// No other package may exec `claude` directly.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Job is one agent request: a prompt and the JSON schema its answer must satisfy.
type Job struct {
	Prompt string
	Schema string
}

// Invoker runs a Job and returns the structured-output object as raw JSON.
type Invoker interface {
	Invoke(ctx context.Context, job Job) (json.RawMessage, error)
}

// Fake is an in-memory Invoker for tests. It matches a Job by finding the first
// Responses key that is a substring of job.Prompt.
type Fake struct {
	Responses map[string]json.RawMessage
	Err       error
	Calls     []Job
}

// Invoke records the call and returns the matching canned response.
func (f *Fake) Invoke(_ context.Context, job Job) (json.RawMessage, error) {
	f.Calls = append(f.Calls, job)
	if f.Err != nil {
		return nil, f.Err
	}
	for key, resp := range f.Responses {
		if strings.Contains(job.Prompt, key) {
			return resp, nil
		}
	}
	return nil, fmt.Errorf("fake: no response matching prompt %q", job.Prompt)
}
```

- [ ] **Step 4: Write the headless Claude implementation**

Create `internal/agent/claude.go`:
```go
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Claude invokes the `claude` CLI in headless print mode.
type Claude struct {
	binary string
}

// NewClaude returns a Claude invoker. If binary is empty, "claude" is used.
func NewClaude(binary string) *Claude {
	if binary == "" {
		binary = "claude"
	}
	return &Claude{binary: binary}
}

// Check verifies the CLI is runnable.
func (c *Claude) Check(ctx context.Context) error {
	if err := exec.CommandContext(ctx, c.binary, "--version").Run(); err != nil {
		return fmt.Errorf("claude CLI not runnable (%q): %w; is it installed and on PATH?", c.binary, err)
	}
	return nil
}

// envelope is the subset of `claude --output-format json` output we read.
type envelope struct {
	StructuredOutput json.RawMessage `json:"structured_output"`
	Result           string          `json:"result"`
}

// Invoke runs the job and returns its structured_output payload.
func (c *Claude) Invoke(ctx context.Context, job Job) (json.RawMessage, error) {
	args := []string{"--bare", "-p", job.Prompt, "--output-format", "json"}
	if job.Schema != "" {
		args = append(args, "--json-schema", job.Schema)
	}
	cmd := exec.CommandContext(ctx, c.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude invoke: %w; stderr: %s", err, stderr.String())
	}
	var env envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return nil, fmt.Errorf("parse claude output: %w; raw: %s", err, stdout.String())
	}
	if len(env.StructuredOutput) == 0 {
		return nil, fmt.Errorf("claude returned no structured_output; result: %s", env.Result)
	}
	return env.StructuredOutput, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/agent/`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add internal/agent/
git commit -m "feat: agent invoker boundary with headless claude impl and fake"
```

---

### Task 4: Raw content reader (file / URL / stdin)

**Files:**
- Create: `internal/ingest/reader.go`
- Test: `internal/ingest/reader_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `ingest.RawSource{Origin string; Bytes []byte}` — raw ingested content plus where it came from.
  - `ingest.Read(ctx context.Context, arg string, stdin io.Reader) (*RawSource, error)` — `arg == "-"` reads `stdin`; `arg` starting with `http://`/`https://` fetches over HTTP; otherwise `arg` is a file path. Origin is set to `"stdin"`, the URL, or the file path respectively.

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/reader_test.go`:
```go
package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := Read(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(raw.Bytes) != "hello" || raw.Origin != path {
		t.Fatalf("got %q / %q", raw.Bytes, raw.Origin)
	}
}

func TestReadStdin(t *testing.T) {
	raw, err := Read(context.Background(), "-", strings.NewReader("piped"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(raw.Bytes) != "piped" || raw.Origin != "stdin" {
		t.Fatalf("got %q / %q", raw.Bytes, raw.Origin)
	}
}

func TestReadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("from web"))
	}))
	defer srv.Close()
	raw, err := Read(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(raw.Bytes) != "from web" || raw.Origin != srv.URL {
		t.Fatalf("got %q / %q", raw.Bytes, raw.Origin)
	}
}

func TestReadMissingFile(t *testing.T) {
	_, err := Read(context.Background(), filepath.Join(t.TempDir(), "nope.md"), nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ingest/`
Expected: FAIL — `undefined: Read`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/ingest/reader.go`:
```go
// Package ingest reads raw technical content and structures it into sections.
package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// RawSource is unstructured ingested content plus its origin.
type RawSource struct {
	Origin string
	Bytes  []byte
}

// Read loads content from a file path, an http(s) URL, or stdin ("-").
func Read(ctx context.Context, arg string, stdin io.Reader) (*RawSource, error) {
	switch {
	case arg == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return &RawSource{Origin: "stdin", Bytes: b}, nil
	case strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, arg, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", arg, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: status %d", arg, resp.StatusCode)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		return &RawSource{Origin: arg, Bytes: b}, nil
	default:
		b, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", arg, err)
		}
		return &RawSource{Origin: arg, Bytes: b}, nil
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ingest/`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/reader.go internal/ingest/reader_test.go
git commit -m "feat: raw content reader for file, url, and stdin"
```

---

### Task 5: Structurer (raw content → stored Source)

**Files:**
- Create: `internal/ingest/structure.go`
- Test: `internal/ingest/structure_test.go`

**Interfaces:**
- Consumes: `ingest.RawSource` (Task 4), `agent.Invoker`/`agent.Job` (Task 3), `model.Source`/`model.Section` (Task 2).
- Produces:
  - `ingest.Structure(ctx context.Context, inv agent.Invoker, raw *RawSource, newID func() string) (*model.Source, error)` — builds one `agent.Job` (prompt embeds `raw.Bytes`, schema below), parses the structured response, and returns a `model.Source` with generated IDs and `Idx` set in order. `newID` supplies unique ids (injected for deterministic tests).
  - The prompt is built by `structurePrompt(content string) string` and the schema is the package constant `structureSchema`.

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/structure_test.go`:
```go
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func seqIDer() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("id%d", n) }
}

func TestStructureBuildsSource(t *testing.T) {
	fake := &agent.Fake{Responses: map[string]json.RawMessage{
		// matches because structurePrompt contains the word "sections"
		"sections": json.RawMessage(`{
			"title":"My Paper",
			"sections":[
				{"title":"Intro","markdown":"# Intro\ntext"},
				{"title":"Method","markdown":"# Method\ntext"}
			]
		}`),
	}}
	raw := &RawSource{Origin: "paper.pdf", Bytes: []byte("raw bytes")}
	src, err := Structure(context.Background(), fake, raw, seqIDer())
	if err != nil {
		t.Fatalf("Structure: %v", err)
	}
	if src.Title != "My Paper" || src.Origin != "paper.pdf" {
		t.Fatalf("got %+v", src)
	}
	if len(src.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(src.Sections))
	}
	if src.Sections[0].Idx != 0 || src.Sections[1].Idx != 1 {
		t.Fatalf("idx not set in order: %+v", src.Sections)
	}
	if src.Sections[0].SourceID != src.ID {
		t.Fatalf("section SourceID %q != source ID %q", src.Sections[0].SourceID, src.ID)
	}
	// The raw content must reach the agent.
	if !strings.Contains(fake.Calls[0].Prompt, "raw bytes") {
		t.Fatalf("prompt did not include raw content: %q", fake.Calls[0].Prompt)
	}
}

func TestStructureRejectsEmptySections(t *testing.T) {
	fake := &agent.Fake{Responses: map[string]json.RawMessage{
		"sections": json.RawMessage(`{"title":"Empty","sections":[]}`),
	}}
	_, err := Structure(context.Background(), fake, &RawSource{Origin: "x", Bytes: []byte("y")}, seqIDer())
	if err == nil {
		t.Fatal("expected error when agent returns zero sections")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ingest/ -run TestStructure`
Expected: FAIL — `undefined: Structure`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/ingest/structure.go`:
```go
package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

const structureSchema = `{
  "type": "object",
  "required": ["title", "sections"],
  "properties": {
    "title": {"type": "string"},
    "sections": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["title", "markdown"],
        "properties": {
          "title": {"type": "string"},
          "markdown": {"type": "string"}
        }
      }
    }
  }
}`

func structurePrompt(content string) string {
	return "You are preparing technical content for study. " +
		"Clean the following content into Markdown and split it into ordered sections " +
		"at natural informational boundaries. Return a title and the sections.\n\n" +
		"CONTENT:\n" + content
}

type structureResult struct {
	Title    string `json:"title"`
	Sections []struct {
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
	} `json:"sections"`
}

// Structure turns raw content into a Source with ordered sections via the agent.
func Structure(ctx context.Context, inv agent.Invoker, raw *RawSource, newID func() string) (*model.Source, error) {
	job := agent.Job{Prompt: structurePrompt(string(raw.Bytes)), Schema: structureSchema}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("structure invoke: %w", err)
	}
	var res structureResult
	if err := json.Unmarshal(resp, &res); err != nil {
		return nil, fmt.Errorf("parse structure result: %w", err)
	}
	if len(res.Sections) == 0 {
		return nil, fmt.Errorf("agent returned no sections for %s", raw.Origin)
	}
	src := &model.Source{
		ID:     newID(),
		Title:  res.Title,
		Origin: raw.Origin,
	}
	for i, s := range res.Sections {
		src.Sections = append(src.Sections, model.Section{
			ID:       newID(),
			SourceID: src.ID,
			Idx:      i,
			Title:    s.Title,
			Markdown: s.Markdown,
		})
	}
	return src, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ingest/`
Expected: PASS (reader + structure tests).

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/structure.go internal/ingest/structure_test.go
git commit -m "feat: structurer turns raw content into stored Source via agent"
```

---

### Task 6: Shared runtime helpers (paths, IDs, time)

**Files:**
- Create: `internal/app/app.go`
- Test: `internal/app/app_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `app.DataDir() (string, error)` — returns `~/.tanaka`, creating it if absent.
  - `app.DBPath() (string, error)` — returns `<DataDir>/tanaka.db`.
  - `app.NewID() string` — returns a random 16-hex-char id (crypto/rand).

- [ ] **Step 1: Write the failing test**

Create `internal/app/app_test.go`:
```go
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirCreatesUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if dir != filepath.Join(home, ".tanaka") {
		t.Fatalf("dir = %q", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("data dir not created: %v", err)
	}
}

func TestNewIDUniqueAndHex(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b {
		t.Fatal("ids not unique")
	}
	if len(a) != 16 {
		t.Fatalf("id len = %d, want 16", len(a))
	}
	if strings.ContainsAny(a, "ghijklmnopqrstuvwxyz") {
		t.Fatalf("id %q is not hex", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/`
Expected: FAIL — `undefined: DataDir`, `undefined: NewID`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/app/app.go`:
```go
// Package app holds shared runtime helpers (paths, ids).
package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// DataDir returns ~/.tanaka, creating it if needed.
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".tanaka")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dir, nil
}

// DBPath returns the SQLite database path.
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tanaka.db"), nil
}

// NewID returns a random 16-hex-character identifier.
func NewID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/app/
git commit -m "feat: shared runtime helpers for paths and ids"
```

---

### Task 7: Wire `tanaka add` and `tanaka list` commands

**Files:**
- Modify: `internal/cli/cli.go` (add `add` and `list` subcommands + dependency wiring)
- Test: `internal/cli/add_test.go`

**Interfaces:**
- Consumes: `ingest.Read`/`ingest.Structure` (Tasks 4–5), `agent.NewClaude`/`agent.Invoker` (Task 3), `store.NewSQLite`/`store.Store` (Task 2), `app.DBPath`/`app.NewID` (Task 6).
- Produces: a testable `cli.run(args, deps, stdout, stderr) int` where `deps` carries an `agent.Invoker`, a `store.Store`, and a `newID func() string`, so tests inject fakes. `Run` builds the real `deps` (real Claude invoker + SQLite store) and calls `run`.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/add_test.go`:
```go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/store"
)

func testDeps(t *testing.T) deps {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	n := 0
	return deps{
		invoker: &agent.Fake{Responses: map[string]json.RawMessage{
			"sections": json.RawMessage(`{"title":"Doc","sections":[{"title":"S1","markdown":"x"}]}`),
		}},
		store: st,
		newID: func() string { n++; return fmt.Sprintf("id%d", n) },
		stdin: strings.NewReader("some content"),
	}
}

func TestAddThenList(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer

	if code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut); code != 0 {
		t.Fatalf("add exit = %d; stderr=%s", code, errOut.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"list"}, d, &out, &errOut); code != 0 {
		t.Fatalf("list exit = %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Doc") {
		t.Fatalf("list output %q missing title", out.String())
	}
}

func TestAddRequiresArg(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"add"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit when add has no argument")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestAdd`
Expected: FAIL — `undefined: deps`, `undefined: run`.

- [ ] **Step 3: Rewrite `cli.go` to support injected dependencies**

Replace the entire contents of `internal/cli/cli.go` with:
```go
// Package cli dispatches Tanaka subcommands.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

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

	d := deps{
		invoker: agent.NewClaude(""),
		store:   st,
		newID:   app.NewID,
		stdin:   os.Stdin,
	}
	return run(context.Background(), args, d, stdout, stderr)
}

// run dispatches subcommands using the provided dependencies.
func run(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
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
```

Note: `model` and `time` are not imported here; the existing `cli_test.go` from Task 1 still compiles because `Run` still handles `version`.

- [ ] **Step 4: Run all CLI tests to verify they pass**

Run: `go test ./internal/cli/`
Expected: PASS (`TestRunVersion`, `TestRunUnknownCommand`, `TestAddThenList`, `TestAddRequiresArg`).

- [ ] **Step 5: Run the whole suite and vet**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/
git commit -m "feat: wire tanaka add and list commands"
```

---

### Task 8: README (minimal) and end-to-end smoke check

**Files:**
- Create: `README.md`
- Modify: none (verification task)

**Interfaces:**
- Consumes: the full CLI from Task 7.
- Produces: a minimal README and a verified end-to-end path with the real `claude` CLI.

- [ ] **Step 1: Write a minimal README**

Create `README.md` (plain, no marketing, no emoji):
```markdown
# Tanaka

Turn technical content (papers, blog posts, articles) into a study-then-build
learning flow. This milestone covers ingestion: importing content into
structured sections.

## Requirements

- Go 1.26+
- The `claude` CLI, logged in (`claude login`)

## Install

    go build -o tanaka .

## Usage

Add content from a file, URL, or stdin:

    tanaka add paper.pdf
    tanaka add https://example.com/post
    cat notes.md | tanaka add -

List what you have imported:

    tanaka list

## How it works

`tanaka add` reads the raw content and asks the `claude` CLI to clean it into
Markdown and split it into ordered sections, then stores the result in
`~/.tanaka/tanaka.db`. Later milestones add the study UI and the build phase.
```

- [ ] **Step 2: Commit the README**

```bash
git add README.md
git commit -m "docs: minimal README for ingestion milestone"
```

- [ ] **Step 3: Manual end-to-end smoke test (real claude CLI)**

Run:
```bash
go build -o /tmp/tanaka .
printf '# Quicksort\nQuicksort partitions an array around a pivot, then recurses on each side. Average time is O(n log n).\n\n# Stability\nQuicksort is not stable because partitioning can reorder equal keys.' | /tmp/tanaka add -
/tmp/tanaka list
```
Expected: `add` prints `added "..." (N sections) as <id>` with N >= 1; `list` shows the new source. (This step spends a small amount of Claude usage.)

- [ ] **Step 4: Confirm persistence**

Run: `ls -la ~/.tanaka/ && /tmp/tanaka list`
Expected: `tanaka.db` exists and `list` shows the previously added source.

---

## Self-Review

**Spec coverage (Plan 1 scope = foundation + ingestion):**
- Ingestion (file/URL/stdin) — Tasks 4, 7. ✓
- Agent-driven structuring via headless `claude -p` behind an interface — Tasks 3, 5. ✓
- SQLite store under `~/.tanaka/` — Tasks 2, 6. ✓
- Single Go binary + CLI — Tasks 1, 7. ✓
- Error handling: missing file, fetch non-200, agent error, empty sections, missing CLI — Tasks 3–5, 7. ✓
- Mockable agent for token-free tests — Task 3 `Fake`. ✓
- Minimal README — Task 8. ✓
- Out of Plan 1 scope (deferred to Plans 2–3): study-package generation, web UI/server, live grading, build engine. Documented as follow-on plans.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every command has expected output. ✓

**Type consistency:** `agent.Invoker.Invoke(ctx, Job) (json.RawMessage, error)` used identically in Tasks 3, 5, 7. `agent.Job{Prompt, Schema}` consistent. `model.Source`/`model.Section` field names consistent across Tasks 2, 5, 7. `ingest.Read(ctx, arg, stdin)` and `ingest.Structure(ctx, inv, raw, newID)` signatures match their call sites in Task 7. `store.Store` methods match between Tasks 2 and 7. `app.NewID`/`app.DBPath` match usage in Task 7. ✓

---

## Follow-on Plans (not in this document)

- **Plan 2 — Study UI & grading:** study-package generator (`gen-study`), local web server + reading UI, live answer grading (`grade-answer`), progress gating/persistence.
- **Plan 3 — Build engine:** language + difficulty selection, build-plan generation (`gen-build`), shell-run acceptance tests, on-request hints.
