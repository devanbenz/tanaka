# Tanaka Plan 3a — Build Domain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the build-phase data and domain layer — build/step storage, `gen-build` plan generation, `gen-hint`, a server-side `TestRunner`, path-safe workspace scaffolding, and step progression — plus a `tanaka build <id> --lang L --difficulty D` command, so the web build UI (Plan 3b) can be built on a tested foundation.

**Architecture:** New domain types in `internal/model`. `internal/store` gains `builds`/`build_steps` tables and methods. A new `internal/build` package orchestrates the agent (`gen-build`, `gen-hint`), runs tests via an injectable `TestRunner`, scaffolds the workspace path-safely, and advances steps (reusing `study.ComputeUnlocked` semantics via per-step status). `internal/cli` gains a `build` command. No HTTP in this plan.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `github.com/yuin/goldmark` (already present), standard library, the existing `agent.Invoker`. No new third-party dependencies.

## Global Constraints

- Module path: `github.com/devandbenz/tanaka` (verbatim in imports).
- Go version floor: `go 1.26`.
- Pure-Go, no cgo. No new third-party dependencies in this plan.
- All agent calls go through `agent.Invoker`; content via `Job.Stdin`, never argv.
- New tables additive with `ON DELETE CASCADE` so `remove` still wipes everything.
- Languages: exactly `rust`, `go`, `cpp`, `c`, `python`. Difficulties: exactly `guided`, `spec+tests`, `blank-page`.
- Step status values reuse `model.StatusLocked`/`StatusUnlocked`/`StatusPassed`/`StatusSkipped`.
- Workspace file paths from the agent must be relative and stay within the workspace (reject absolute or `..`).
- Docs/comments plain and minimal — no marketing voice, no emoji.

---

### Task 1: Build domain types + language/difficulty validation

**Files:**
- Modify: `internal/model/model.go`
- Test: `internal/model/build_test.go`

**Interfaces:**
- Consumes: existing status constants.
- Produces:
  - `model.BuildFile{Path, Content string}`
  - `model.BuildStep{ID, BuildID string; Idx int; Goal string; Files []BuildFile; Status string}`
  - `model.Build{ID, SourceID, Language, Difficulty, Workspace string; CreatedAt time.Time; Steps []BuildStep}`
  - Language constants `model.LangRust="rust"`, `LangGo="go"`, `LangCPP="cpp"`, `LangC="c"`, `LangPython="python"`.
  - Difficulty constants `model.DiffGuided="guided"`, `DiffSpecTests="spec+tests"`, `DiffBlank="blank-page"`.
  - `model.ValidLanguage(s string) bool` and `model.ValidDifficulty(s string) bool`.

- [ ] **Step 1: Write the failing test**

Create `internal/model/build_test.go`:
```go
package model

import "testing"

func TestBuildConstants(t *testing.T) {
	if LangRust != "rust" || LangGo != "go" || LangCPP != "cpp" || LangC != "c" || LangPython != "python" {
		t.Fatal("language constant values must match the spec strings")
	}
	if DiffGuided != "guided" || DiffSpecTests != "spec+tests" || DiffBlank != "blank-page" {
		t.Fatal("difficulty constant values must match the spec strings")
	}
}

func TestValidLanguageAndDifficulty(t *testing.T) {
	for _, l := range []string{"rust", "go", "cpp", "c", "python"} {
		if !ValidLanguage(l) {
			t.Fatalf("ValidLanguage(%q) = false", l)
		}
	}
	if ValidLanguage("haskell") {
		t.Fatal("ValidLanguage(haskell) should be false")
	}
	for _, d := range []string{"guided", "spec+tests", "blank-page"} {
		if !ValidDifficulty(d) {
			t.Fatalf("ValidDifficulty(%q) = false", d)
		}
	}
	if ValidDifficulty("impossible") {
		t.Fatal("ValidDifficulty(impossible) should be false")
	}
}

func TestBuildHoldsSteps(t *testing.T) {
	b := Build{ID: "b1", SourceID: "s1", Language: LangGo, Difficulty: DiffSpecTests, Workspace: "/tmp/x",
		Steps: []BuildStep{{ID: "st1", BuildID: "b1", Idx: 0, Goal: "do it", Status: StatusUnlocked,
			Files: []BuildFile{{Path: "main.go", Content: "package main"}}}}}
	if len(b.Steps) != 1 || b.Steps[0].Files[0].Path != "main.go" {
		t.Fatalf("unexpected: %+v", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/`
Expected: FAIL — `undefined: LangRust` etc.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/model/model.go`:
```go
// Build languages (also the strings stored in builds.language).
const (
	LangRust   = "rust"
	LangGo     = "go"
	LangCPP    = "cpp"
	LangC      = "c"
	LangPython = "python"
)

// Build difficulties (stored in builds.difficulty).
const (
	DiffGuided    = "guided"
	DiffSpecTests = "spec+tests"
	DiffBlank     = "blank-page"
)

// ValidLanguage reports whether s is a supported build language.
func ValidLanguage(s string) bool {
	switch s {
	case LangRust, LangGo, LangCPP, LangC, LangPython:
		return true
	}
	return false
}

// ValidDifficulty reports whether s is a supported build difficulty.
func ValidDifficulty(s string) bool {
	switch s {
	case DiffGuided, DiffSpecTests, DiffBlank:
		return true
	}
	return false
}

// BuildFile is one file the agent generates into the build workspace.
type BuildFile struct {
	Path    string
	Content string
}

// BuildStep is one ordered step of a build plan.
type BuildStep struct {
	ID      string
	BuildID string
	Idx     int
	Goal    string
	Files   []BuildFile // written into the workspace when the step activates
	Status  string
}

// Build is a per-source, per-language implementation exercise.
type Build struct {
	ID         string
	SourceID   string
	Language   string
	Difficulty string
	Workspace  string
	CreatedAt  time.Time
	Steps      []BuildStep
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat: build domain types and language/difficulty validation"
```

---

### Task 2: Store builds + build_steps

**Files:**
- Modify: `internal/store/store.go` (schema + interface)
- Create: `internal/store/build.go`
- Test: `internal/store/build_test.go`

**Interfaces:**
- Consumes: `model.Build`, `model.BuildStep`, `model.BuildFile`; existing `store.Store`, `store.ErrNotFound`.
- Produces (added to `store.Store`):
  - `SaveBuild(ctx context.Context, b *model.Build) error` — inserts the build + its steps in one transaction (each step's `Files` JSON-encoded into `files_json`).
  - `GetBuild(ctx context.Context, sourceID, language string) (*model.Build, error)` — `ErrNotFound` if absent; loads steps ordered by `idx` with `Files` decoded.
  - `SetBuildStepStatus(ctx context.Context, stepID, status string) error`.
  - `GetBuildStep(ctx context.Context, stepID string) (*model.BuildStep, error)` — `ErrNotFound` if absent; `Files` decoded.

- [ ] **Step 1: Add the tables and interface methods**

In `internal/store/store.go`, append to the `schema` const (before the closing backtick):
```sql
CREATE TABLE IF NOT EXISTS builds (
	id          TEXT PRIMARY KEY,
	source_id   TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
	language    TEXT NOT NULL,
	difficulty  TEXT NOT NULL,
	workspace   TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	UNIQUE(source_id, language)
);
CREATE TABLE IF NOT EXISTS build_steps (
	id         TEXT PRIMARY KEY,
	build_id   TEXT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
	idx        INTEGER NOT NULL,
	goal       TEXT NOT NULL,
	files_json TEXT NOT NULL,
	status     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_build_steps_build ON build_steps(build_id, idx);
```
And add to the `Store` interface (after the Plan 2 methods, before `Close`):
```go
	SaveBuild(ctx context.Context, b *model.Build) error
	GetBuild(ctx context.Context, sourceID, language string) (*model.Build, error)
	SetBuildStepStatus(ctx context.Context, stepID, status string) error
	GetBuildStep(ctx context.Context, stepID string) (*model.BuildStep, error)
```

- [ ] **Step 2: Write the failing test**

Create `internal/store/build_test.go`:
```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func buildSource(t *testing.T, s Store) {
	t.Helper()
	if err := s.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func sampleBuild() *model.Build {
	return &model.Build{
		ID: "b1", SourceID: "src1", Language: model.LangGo, Difficulty: model.DiffSpecTests,
		Workspace: "/tmp/ws", CreatedAt: time.Unix(5, 0).UTC(),
		Steps: []model.BuildStep{
			{ID: "st0", BuildID: "b1", Idx: 0, Goal: "step zero", Status: model.StatusUnlocked,
				Files: []model.BuildFile{{Path: "go.mod", Content: "module x"}, {Path: "a_test.go", Content: "package x"}}},
			{ID: "st1", BuildID: "b1", Idx: 1, Goal: "step one", Status: model.StatusLocked,
				Files: []model.BuildFile{{Path: "b_test.go", Content: "package x"}}},
		},
	}
}

func TestSaveAndGetBuild(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	buildSource(t, s)
	if err := s.SaveBuild(ctx, sampleBuild()); err != nil {
		t.Fatalf("SaveBuild: %v", err)
	}
	got, err := s.GetBuild(ctx, "src1", model.LangGo)
	if err != nil {
		t.Fatalf("GetBuild: %v", err)
	}
	if got.Difficulty != model.DiffSpecTests || got.Workspace != "/tmp/ws" {
		t.Fatalf("build = %+v", got)
	}
	if len(got.Steps) != 2 || got.Steps[1].Idx != 1 {
		t.Fatalf("steps not ordered: %+v", got.Steps)
	}
	if len(got.Steps[0].Files) != 2 || got.Steps[0].Files[1].Path != "a_test.go" {
		t.Fatalf("files not round-tripped: %+v", got.Steps[0].Files)
	}
	if got.Steps[0].Status != model.StatusUnlocked || got.Steps[1].Status != model.StatusLocked {
		t.Fatalf("status not round-tripped: %+v", got.Steps)
	}
}

func TestGetBuildNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetBuild(context.Background(), "src1", model.LangRust); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetBuildStepStatusAndGetStep(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	buildSource(t, s)
	if err := s.SaveBuild(ctx, sampleBuild()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBuildStepStatus(ctx, "st0", model.StatusPassed); err != nil {
		t.Fatal(err)
	}
	step, err := s.GetBuildStep(ctx, "st0")
	if err != nil {
		t.Fatalf("GetBuildStep: %v", err)
	}
	if step.Status != model.StatusPassed || step.Goal != "step zero" || len(step.Files) != 2 {
		t.Fatalf("step = %+v", step)
	}
	if _, err := s.GetBuildStep(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing step err = %v, want ErrNotFound", err)
	}
}

func TestUniqueBuildPerSourceLanguage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	buildSource(t, s)
	if err := s.SaveBuild(ctx, sampleBuild()); err != nil {
		t.Fatal(err)
	}
	dup := sampleBuild()
	dup.ID = "b2"
	dup.Steps = nil
	if err := s.SaveBuild(ctx, dup); err == nil {
		t.Fatal("expected UNIQUE(source_id, language) violation on second build")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -run Build`
Expected: FAIL — `*sqliteStore` does not implement `Store` (missing methods).

- [ ] **Step 4: Write the implementation**

Create `internal/store/build.go`:
```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func (s *sqliteStore) SaveBuild(ctx context.Context, b *model.Build) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO builds (id, source_id, language, difficulty, workspace, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.SourceID, b.Language, b.Difficulty, b.Workspace, b.CreatedAt.Unix()); err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	for _, st := range b.Steps {
		files, err := json.Marshal(st.Files)
		if err != nil {
			return fmt.Errorf("marshal files: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO build_steps (id, build_id, idx, goal, files_json, status) VALUES (?, ?, ?, ?, ?, ?)`,
			st.ID, b.ID, st.Idx, st.Goal, string(files), st.Status); err != nil {
			return fmt.Errorf("insert build_step %s: %w", st.ID, err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) GetBuild(ctx context.Context, sourceID, language string) (*model.Build, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, difficulty, workspace, created_at FROM builds WHERE source_id = ? AND language = ?`,
		sourceID, language)
	var b model.Build
	var created int64
	if err := row.Scan(&b.ID, &b.Difficulty, &b.Workspace, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get build: %w", err)
	}
	b.SourceID, b.Language, b.CreatedAt = sourceID, language, time.Unix(created, 0).UTC()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, idx, goal, files_json, status FROM build_steps WHERE build_id = ? ORDER BY idx`, b.ID)
	if err != nil {
		return nil, fmt.Errorf("query build_steps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		st, err := scanStep(rows, b.ID)
		if err != nil {
			return nil, err
		}
		b.Steps = append(b.Steps, *st)
	}
	return &b, rows.Err()
}

func (s *sqliteStore) SetBuildStepStatus(ctx context.Context, stepID, status string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE build_steps SET status = ? WHERE id = ?`, status, stepID); err != nil {
		return fmt.Errorf("set build step status %s: %w", stepID, err)
	}
	return nil
}

func (s *sqliteStore) GetBuildStep(ctx context.Context, stepID string) (*model.BuildStep, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT build_id, id, idx, goal, files_json, status FROM build_steps WHERE id = ?`, stepID)
	var buildID string
	if err := row.Scan(&buildID, new(string), new(int), new(string), new(string), new(string)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get build step: %w", err)
	}
	// Re-query through the shared scanner for consistent decoding.
	r := s.db.QueryRowContext(ctx,
		`SELECT id, idx, goal, files_json, status FROM build_steps WHERE id = ?`, stepID)
	return scanStepRow(r, buildID)
}

// scanStep decodes a build_steps row (id, idx, goal, files_json, status) from *sql.Rows.
func scanStep(rows *sql.Rows, buildID string) (*model.BuildStep, error) {
	var st model.BuildStep
	var files string
	if err := rows.Scan(&st.ID, &st.Idx, &st.Goal, &files, &st.Status); err != nil {
		return nil, fmt.Errorf("scan build step: %w", err)
	}
	st.BuildID = buildID
	if err := json.Unmarshal([]byte(files), &st.Files); err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	return &st, nil
}

// scanStepRow decodes the same columns from a *sql.Row.
func scanStepRow(row *sql.Row, buildID string) (*model.BuildStep, error) {
	var st model.BuildStep
	var files string
	if err := row.Scan(&st.ID, &st.Idx, &st.Goal, &files, &st.Status); err != nil {
		return nil, fmt.Errorf("scan build step: %w", err)
	}
	st.BuildID = buildID
	if err := json.Unmarshal([]byte(files), &st.Files); err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	return &st, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat: persist builds and build steps"
```

---

### Task 3: build package — path safety + gen-build

**Files:**
- Create: `internal/build/build.go` (package doc + path safety)
- Create: `internal/build/generate.go` (gen-build)
- Test: `internal/build/generate_test.go`

**Interfaces:**
- Consumes: `agent.Invoker`/`agent.Job`, `model.BuildFile`.
- Produces:
  - `build.SafeRelPath(p string) error` — error if `p` is empty, absolute, or escapes the workspace (`..`).
  - `build.StepGen{Goal string; Files []model.BuildFile}` — a generated step before IDs are assigned.
  - `build.GenerateBuild(ctx context.Context, inv agent.Invoker, sectionsMarkdown, language, difficulty string) (skeleton []model.BuildFile, steps []StepGen, err error)` — one `gen-build` agent call; content via `Stdin`; validates every returned file path with `SafeRelPath`; errors if zero steps.
  - Package constant `buildSchema`.

- [ ] **Step 1: Write the failing test**

Create `internal/build/generate_test.go`:
```go
package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func TestSafeRelPath(t *testing.T) {
	for _, ok := range []string{"main.go", "src/lib.rs", "tests/a_test.go"} {
		if err := SafeRelPath(ok); err != nil {
			t.Fatalf("SafeRelPath(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "/etc/passwd", "../escape", "a/../../b", "/abs"} {
		if err := SafeRelPath(bad); err == nil {
			t.Fatalf("SafeRelPath(%q) = nil, want error", bad)
		}
	}
}

func buildFake() *agent.Fake {
	return &agent.Fake{Responses: map[string]json.RawMessage{
		"build plan": json.RawMessage(`{
			"skeleton_files":[{"path":"go.mod","content":"module x"}],
			"steps":[
				{"goal":"parse input","files":[{"path":"parse_test.go","content":"package x"}]},
				{"goal":"compute","files":[{"path":"compute_test.go","content":"package x"}]}
			]
		}`),
	}}
}

func TestGenerateBuild(t *testing.T) {
	f := buildFake()
	skeleton, steps, err := GenerateBuild(context.Background(), f, "the paper sections", "go", "spec+tests")
	if err != nil {
		t.Fatalf("GenerateBuild: %v", err)
	}
	if len(skeleton) != 1 || skeleton[0].Path != "go.mod" {
		t.Fatalf("skeleton = %+v", skeleton)
	}
	if len(steps) != 2 || steps[0].Goal != "parse input" || steps[1].Files[0].Path != "compute_test.go" {
		t.Fatalf("steps = %+v", steps)
	}
	// Content + language/difficulty must reach the agent: section text via stdin, lang/difficulty in prompt.
	call := f.Calls[0]
	if !strings.Contains(string(call.Stdin), "the paper sections") {
		t.Fatalf("section text must go via stdin: %q", call.Stdin)
	}
	if !strings.Contains(call.Prompt, "go") || !strings.Contains(call.Prompt, "spec+tests") {
		t.Fatalf("prompt missing language/difficulty: %q", call.Prompt)
	}
}

func TestGenerateBuildRejectsUnsafePath(t *testing.T) {
	f := &agent.Fake{Responses: map[string]json.RawMessage{
		"build plan": json.RawMessage(`{"skeleton_files":[{"path":"../evil","content":"x"}],"steps":[{"goal":"g","files":[]}]}`),
	}}
	if _, _, err := GenerateBuild(context.Background(), f, "x", "go", "spec+tests"); err == nil {
		t.Fatal("expected error for unsafe skeleton path")
	}
}

func TestGenerateBuildRejectsEmptySteps(t *testing.T) {
	f := &agent.Fake{Responses: map[string]json.RawMessage{
		"build plan": json.RawMessage(`{"skeleton_files":[],"steps":[]}`),
	}}
	if _, _, err := GenerateBuild(context.Background(), f, "x", "go", "spec+tests"); err == nil {
		t.Fatal("expected error for zero steps")
	}
}
```
Note: the fake matches the substring `"build plan"`; ensure `buildPrompt` contains that phrase.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/build/`
Expected: FAIL — `undefined: SafeRelPath`, `undefined: GenerateBuild`.

- [ ] **Step 3: Write the path-safety helper**

Create `internal/build/build.go`:
```go
// Package build generates implementation exercises (build plans), grades
// progress by running tests, and provides hints.
package build

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafeRelPath rejects paths that are empty, absolute, or escape the workspace.
func SafeRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("absolute path not allowed: %s", p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace: %s", p)
	}
	return nil
}
```

- [ ] **Step 4: Write gen-build**

Create `internal/build/generate.go`:
```go
package build

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

const fileSchema = `{"type":"object","required":["path","content"],"properties":{"path":{"type":"string"},"content":{"type":"string"}}}`

var buildSchema = `{
  "type": "object",
  "required": ["skeleton_files", "steps"],
  "properties": {
    "skeleton_files": {"type": "array", "items": ` + fileSchema + `},
    "steps": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["goal", "files"],
        "properties": {
          "goal": {"type": "string"},
          "files": {"type": "array", "items": ` + fileSchema + `}
        }
      }
    }
  }
}`

// StepGen is a generated step before IDs/status are assigned.
type StepGen struct {
	Goal  string
	Files []model.BuildFile
}

func buildPrompt(language, difficulty string) string {
	return "You are designing a hands-on build plan that has the learner implement the " +
		"technical content on stdin in " + language + ". Difficulty: " + difficulty +
		" (guided = starter code + comments; spec+tests = stubs + tests; blank-page = goal + tests only). " +
		"Produce a language-appropriate project: skeleton_files (project files like go.mod/Cargo.toml/Makefile) " +
		"and an ordered list of steps, each with a goal and files (its acceptance tests plus difficulty-scaled " +
		"scaffold). Tests must be runnable with the standard command for the language. Return the build plan."
}

type buildResult struct {
	SkeletonFiles []model.BuildFile `json:"skeleton_files"`
	Steps         []struct {
		Goal  string            `json:"goal"`
		Files []model.BuildFile `json:"files"`
	} `json:"steps"`
}

// GenerateBuild produces a build plan via one agent call. Section content goes
// via stdin; language/difficulty go in the prompt. All file paths are validated.
func GenerateBuild(ctx context.Context, inv agent.Invoker, sectionsMarkdown, language, difficulty string) ([]model.BuildFile, []StepGen, error) {
	job := agent.Job{Prompt: buildPrompt(language, difficulty), Schema: buildSchema, Stdin: []byte(sectionsMarkdown)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return nil, nil, fmt.Errorf("gen-build invoke: %w", err)
	}
	var res buildResult
	if err := json.Unmarshal(resp, &res); err != nil {
		return nil, nil, fmt.Errorf("parse build result: %w", err)
	}
	if len(res.Steps) == 0 {
		return nil, nil, fmt.Errorf("agent returned no build steps")
	}
	for _, f := range res.SkeletonFiles {
		if err := SafeRelPath(f.Path); err != nil {
			return nil, nil, fmt.Errorf("skeleton file: %w", err)
		}
	}
	var steps []StepGen
	for _, s := range res.Steps {
		for _, f := range s.Files {
			if err := SafeRelPath(f.Path); err != nil {
				return nil, nil, fmt.Errorf("step file: %w", err)
			}
		}
		steps = append(steps, StepGen{Goal: s.Goal, Files: s.Files})
	}
	return res.SkeletonFiles, steps, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/build/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/build/build.go internal/build/generate.go internal/build/generate_test.go
git commit -m "feat: gen-build plan generation with path safety"
```

---

### Task 4: build package — gen-hint

**Files:**
- Create: `internal/build/hint.go`
- Test: `internal/build/hint_test.go`

**Interfaces:**
- Consumes: `agent.Invoker`/`agent.Job`.
- Produces: `build.Hint(ctx context.Context, inv agent.Invoker, goal, code, failingOutput string) (string, error)` — one `gen-hint` call; `goal` in prompt, `code` + `failingOutput` via `Stdin`; returns the hint text.

- [ ] **Step 1: Write the failing test**

Create `internal/build/hint_test.go`:
```go
package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func TestHint(t *testing.T) {
	f := &agent.Fake{Responses: map[string]json.RawMessage{
		"hint": json.RawMessage(`{"hint":"think about the base case"}`),
	}}
	h, err := Hint(context.Background(), f, "implement recursion", "func f(){}", "FAIL: stack overflow")
	if err != nil {
		t.Fatalf("Hint: %v", err)
	}
	if !strings.Contains(h, "base case") {
		t.Fatalf("hint = %q", h)
	}
	call := f.Calls[0]
	if !strings.Contains(string(call.Stdin), "func f(){}") || !strings.Contains(string(call.Stdin), "stack overflow") {
		t.Fatalf("stdin missing code/output: %q", call.Stdin)
	}
	if !strings.Contains(call.Prompt, "implement recursion") {
		t.Fatalf("prompt missing goal: %q", call.Prompt)
	}
}
```
Note: the fake matches the substring `"hint"`; ensure `hintPrompt` contains "hint".

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/build/ -run Hint`
Expected: FAIL — `undefined: Hint`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/build/hint.go`:
```go
package build

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
)

const hintSchema = `{"type":"object","required":["hint"],"properties":{"hint":{"type":"string"}}}`

func hintPrompt(goal string) string {
	return "Give a single short hint (a nudge, not the solution) for this build step. " +
		"The learner's current code and the failing test output are on stdin. " +
		"Step goal: " + goal
}

// Hint returns one nudge for a build step via an agent call.
func Hint(ctx context.Context, inv agent.Invoker, goal, code, failingOutput string) (string, error) {
	stdin := "CURRENT CODE:\n" + code + "\n\nFAILING OUTPUT:\n" + failingOutput
	job := agent.Job{Prompt: hintPrompt(goal), Schema: hintSchema, Stdin: []byte(stdin)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return "", fmt.Errorf("gen-hint invoke: %w", err)
	}
	var out struct {
		Hint string `json:"hint"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return "", fmt.Errorf("parse hint: %w", err)
	}
	return out.Hint, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/build/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/build/hint.go internal/build/hint_test.go
git commit -m "feat: gen-hint for build steps"
```

---

### Task 5: build package — TestRunner

**Files:**
- Create: `internal/build/runner.go`
- Test: `internal/build/runner_test.go`

**Interfaces:**
- Consumes: nothing beyond stdlib.
- Produces:
  - `build.Result{Passed bool; Output string; RunError bool}`
  - `build.Runner` interface: `Run(ctx context.Context, workspace, language string) (Result, error)`.
  - `build.ExecRunner{Timeout time.Duration}` implementing `Runner` by exec'ing the per-language command in `workspace`; `RunError=true` when the command binary is not found (or the context deadline is exceeded); `Passed` = exit code 0.
  - `build.NewExecRunner() *ExecRunner` (default 90s timeout).
  - `build.FakeRunner{Result Result; Err error; Calls int}` implementing `Runner` for tests.
  - `build.commandFor(language string) ([]string, bool)` — the per-language command.

- [ ] **Step 1: Write the failing test**

Create `internal/build/runner_test.go`:
```go
package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCommandFor(t *testing.T) {
	cases := map[string]string{"rust": "cargo", "go": "go", "python": "pytest", "c": "make", "cpp": "make"}
	for lang, first := range cases {
		cmd, ok := commandFor(lang)
		if !ok || cmd[0] != first {
			t.Fatalf("commandFor(%q) = %v,%v want first %q", lang, cmd, ok, first)
		}
	}
	if _, ok := commandFor("haskell"); ok {
		t.Fatal("commandFor(haskell) should be false")
	}
}

func TestExecRunnerRunErrorOnMissingBinary(t *testing.T) {
	r := &ExecRunner{Timeout: 5 * time.Second, override: []string{"definitely-not-a-real-binary-xyz"}}
	res, err := r.Run(context.Background(), t.TempDir(), "go")
	if err != nil {
		t.Fatalf("Run returned err: %v", err)
	}
	if !res.RunError {
		t.Fatalf("expected RunError for missing binary, got %+v", res)
	}
}

func TestExecRunnerPassAndFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	ws := t.TempDir()
	// Passing command.
	pass := &ExecRunner{Timeout: 5 * time.Second, override: []string{"sh", "-c", "echo ok; exit 0"}}
	res, err := pass.Run(context.Background(), ws, "go")
	if err != nil || !res.Passed || res.RunError {
		t.Fatalf("pass: res=%+v err=%v", res, err)
	}
	if !contains(res.Output, "ok") {
		t.Fatalf("output missing stdout: %q", res.Output)
	}
	// Failing command (non-zero exit, but ran fine).
	fail := &ExecRunner{Timeout: 5 * time.Second, override: []string{"sh", "-c", "echo boom; exit 1"}}
	res, err = fail.Run(context.Background(), ws, "go")
	if err != nil || res.Passed || res.RunError {
		t.Fatalf("fail: res=%+v err=%v", res, err)
	}
	_ = os.Stat // keep os import
	_ = filepath.Join
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/build/ -run 'CommandFor|ExecRunner'`
Expected: FAIL — `undefined: commandFor`, `undefined: ExecRunner`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/build/runner.go`:
```go
package build

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Result is the outcome of running a build's tests.
type Result struct {
	Passed   bool
	Output   string
	RunError bool // the command could not run (e.g. toolchain missing), distinct from a test failure
}

// Runner runs a build's tests in a workspace.
type Runner interface {
	Run(ctx context.Context, workspace, language string) (Result, error)
}

var commands = map[string][]string{
	"rust":   {"cargo", "test"},
	"go":     {"go", "test", "./..."},
	"python": {"pytest"},
	"c":      {"make", "test"},
	"cpp":    {"make", "test"},
}

func commandFor(language string) ([]string, bool) {
	c, ok := commands[language]
	return c, ok
}

// ExecRunner runs the per-language test command as a subprocess.
type ExecRunner struct {
	Timeout  time.Duration
	override []string // test hook: when set, used instead of the language command
}

// NewExecRunner returns an ExecRunner with a 90s timeout.
func NewExecRunner() *ExecRunner { return &ExecRunner{Timeout: 90 * time.Second} }

func (r *ExecRunner) Run(ctx context.Context, workspace, language string) (Result, error) {
	cmdParts := r.override
	if cmdParts == nil {
		var ok bool
		cmdParts, ok = commandFor(language)
		if !ok {
			return Result{RunError: true, Output: "unsupported language: " + language}, nil
		}
	}
	to := r.Timeout
	if to == 0 {
		to = 90 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	cmd := exec.CommandContext(cctx, cmdParts[0], cmdParts[1:]...)
	cmd.Dir = workspace
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err == nil {
		return Result{Passed: true, Output: out}, nil
	}
	// Distinguish "could not run" from "ran and failed tests".
	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return Result{RunError: true, Output: out + "\n(test run timed out)"}, nil
	}
	var notFound *exec.Error
	if errors.As(err, &notFound) {
		return Result{RunError: true, Output: "could not run tests: " + err.Error()}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{Passed: false, Output: out}, nil
	}
	return Result{RunError: true, Output: out + "\n" + err.Error()}, nil
}

// FakeRunner is a Runner for tests.
type FakeRunner struct {
	Result Result
	Err    error
	Calls  int
}

func (f *FakeRunner) Run(_ context.Context, _, _ string) (Result, error) {
	f.Calls++
	return f.Result, f.Err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/build/`
Expected: PASS (note: `TestExecRunnerPassAndFail` uses `sh`; it is skipped on Windows).

- [ ] **Step 5: Commit**

```bash
git add internal/build/runner.go internal/build/runner_test.go
git commit -m "feat: TestRunner (exec + fake) with per-language commands"
```

---

### Task 6: build package — workspace scaffolding + StartBuild + step progression

**Files:**
- Create: `internal/build/workspace.go`
- Test: `internal/build/workspace_test.go`

**Interfaces:**
- Consumes: `agent.Invoker`, `store.Store`, `model` types, `GenerateBuild` (Task 3), `SafeRelPath` (Task 3).
- Produces:
  - `build.WriteFiles(workspace string, files []model.BuildFile) error` — path-safe write (creating parent dirs), rejecting unsafe paths.
  - `build.StartBuild(ctx context.Context, inv agent.Invoker, st store.Store, src *model.Source, language, difficulty string, newID func() string, buildsDir string) (*model.Build, error)` — validates language/difficulty; concatenates section markdown; `GenerateBuild`; builds the `model.Build` (workspace = `buildsDir/<sourceID>-<language>`, step 0 `unlocked`, rest `locked`); `SaveBuild`; scaffolds the workspace (skeleton + step 0 files); returns the build.
  - `build.PassStep(ctx context.Context, st store.Store, b *model.Build, idx int) error` — mark step `idx` `passed`; if a next step exists, write its files into the workspace and unlock it if currently `locked`.
  - `build.SkipStep(ctx context.Context, st store.Store, b *model.Build, idx int) error` — same as PassStep but marks `skipped`.

- [ ] **Step 1: Write the failing test**

Create `internal/build/workspace_test.go`:
```go
package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func newStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestWriteFilesRejectsUnsafe(t *testing.T) {
	ws := t.TempDir()
	if err := WriteFiles(ws, []model.BuildFile{{Path: "../evil", Content: "x"}}); err == nil {
		t.Fatal("expected error for unsafe path")
	}
	if err := WriteFiles(ws, []model.BuildFile{{Path: "src/a.go", Content: "hello"}}); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(ws, "src", "a.go"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("file not written: %v / %q", err, got)
	}
}

func seqIDer() func() string {
	n := 0
	return func() string { n++; return "id" + string(rune('a'+n)) }
}

func srcWith2Sections(t *testing.T, st store.Store) *model.Source {
	t.Helper()
	src := &model.Source{ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "alpha"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "B", Markdown: "beta"},
		}}
	if err := st.SaveSource(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestStartBuildScaffoldsAndPersists(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := srcWith2Sections(t, st)
	buildsDir := t.TempDir()
	b, err := StartBuild(ctx, buildFake(), st, src, "go", "spec+tests", seqIDer(), buildsDir)
	if err != nil {
		t.Fatalf("StartBuild: %v", err)
	}
	// Persisted and retrievable.
	got, err := st.GetBuild(ctx, "src1", "go")
	if err != nil {
		t.Fatalf("GetBuild: %v", err)
	}
	if len(got.Steps) != 2 || got.Steps[0].Status != model.StatusUnlocked || got.Steps[1].Status != model.StatusLocked {
		t.Fatalf("steps wrong: %+v", got.Steps)
	}
	// Workspace scaffolded with skeleton + step 0 files, but NOT step 1 files yet.
	if _, err := os.Stat(filepath.Join(b.Workspace, "go.mod")); err != nil {
		t.Fatalf("skeleton not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.Workspace, "parse_test.go")); err != nil {
		t.Fatalf("step 0 files not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.Workspace, "compute_test.go")); err == nil {
		t.Fatal("step 1 files should NOT be written until step 1 activates")
	}
	if b.Workspace != filepath.Join(buildsDir, "src1-go") {
		t.Fatalf("workspace = %q", b.Workspace)
	}
}

func TestStartBuildRejectsBadLanguage(t *testing.T) {
	st := newStore(t)
	src := srcWith2Sections(t, st)
	if _, err := StartBuild(context.Background(), buildFake(), st, src, "haskell", "spec+tests", seqIDer(), t.TempDir()); err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestPassStepWritesNextAndUnlocks(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := srcWith2Sections(t, st)
	buildsDir := t.TempDir()
	b, err := StartBuild(ctx, buildFake(), st, src, "go", "spec+tests", seqIDer(), buildsDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := PassStep(ctx, st, b, 0); err != nil {
		t.Fatalf("PassStep: %v", err)
	}
	got, _ := st.GetBuild(ctx, "src1", "go")
	if got.Steps[0].Status != model.StatusPassed || got.Steps[1].Status != model.StatusUnlocked {
		t.Fatalf("statuses after pass: %+v", got.Steps)
	}
	if _, err := os.Stat(filepath.Join(b.Workspace, "compute_test.go")); err != nil {
		t.Fatalf("step 1 files not written on pass: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/build/ -run 'WriteFiles|StartBuild|PassStep'`
Expected: FAIL — `undefined: WriteFiles`, `undefined: StartBuild`, `undefined: PassStep`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/build/workspace.go`:
```go
package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

// WriteFiles writes each file under workspace, rejecting unsafe paths.
func WriteFiles(workspace string, files []model.BuildFile) error {
	for _, f := range files {
		if err := SafeRelPath(f.Path); err != nil {
			return err
		}
		full := filepath.Join(workspace, filepath.Clean(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
	}
	return nil
}

// StartBuild generates a build plan, persists it, and scaffolds the workspace
// with the skeleton and the first step's files.
func StartBuild(ctx context.Context, inv agent.Invoker, st store.Store, src *model.Source, language, difficulty string, newID func() string, buildsDir string) (*model.Build, error) {
	if !model.ValidLanguage(language) {
		return nil, fmt.Errorf("unsupported language: %s", language)
	}
	if !model.ValidDifficulty(difficulty) {
		return nil, fmt.Errorf("unsupported difficulty: %s", difficulty)
	}
	var sb strings.Builder
	for _, sec := range src.Sections {
		sb.WriteString("## " + sec.Title + "\n" + sec.Markdown + "\n\n")
	}
	skeleton, steps, err := GenerateBuild(ctx, inv, sb.String(), language, difficulty)
	if err != nil {
		return nil, err
	}
	ws := filepath.Join(buildsDir, src.ID+"-"+language)
	b := &model.Build{
		ID: newID(), SourceID: src.ID, Language: language, Difficulty: difficulty,
		Workspace: ws, CreatedAt: time.Now().UTC(),
	}
	for i, sg := range steps {
		status := model.StatusLocked
		if i == 0 {
			status = model.StatusUnlocked
		}
		b.Steps = append(b.Steps, model.BuildStep{
			ID: newID(), BuildID: b.ID, Idx: i, Goal: sg.Goal, Files: sg.Files, Status: status,
		})
	}
	if err := st.SaveBuild(ctx, b); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	if err := WriteFiles(ws, skeleton); err != nil {
		return nil, err
	}
	if len(b.Steps) > 0 {
		if err := WriteFiles(ws, b.Steps[0].Files); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// PassStep marks step idx passed and activates the next step (write files + unlock).
func PassStep(ctx context.Context, st store.Store, b *model.Build, idx int) error {
	return advance(ctx, st, b, idx, model.StatusPassed)
}

// SkipStep marks step idx skipped and activates the next step.
func SkipStep(ctx context.Context, st store.Store, b *model.Build, idx int) error {
	return advance(ctx, st, b, idx, model.StatusSkipped)
}

func advance(ctx context.Context, st store.Store, b *model.Build, idx int, status string) error {
	if idx < 0 || idx >= len(b.Steps) {
		return fmt.Errorf("step index %d out of range", idx)
	}
	if err := st.SetBuildStepStatus(ctx, b.Steps[idx].ID, status); err != nil {
		return err
	}
	next := idx + 1
	if next < len(b.Steps) {
		if err := WriteFiles(b.Workspace, b.Steps[next].Files); err != nil {
			return err
		}
		if b.Steps[next].Status == model.StatusLocked {
			if err := st.SetBuildStepStatus(ctx, b.Steps[next].ID, model.StatusUnlocked); err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/build/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/build/workspace.go internal/build/workspace_test.go
git commit -m "feat: workspace scaffolding, StartBuild, and step progression"
```

---

### Task 7: `tanaka build <id> --lang L --difficulty D` command + smoke

**Files:**
- Modify: `internal/cli/cli.go` (dispatch + `cmdBuild` + help line)
- Test: `internal/cli/build_test.go`

**Interfaces:**
- Consumes: `store.GetSource` (ErrNotFound), `build.StartBuild`, `agent.Invoker`, `app.DataDir`, `app.NewID`, `model.ValidLanguage`/`ValidDifficulty`.
- Produces: a `build` subcommand: `tanaka build <id> --lang <L> --difficulty <D>` (difficulty default `spec+tests`). Loads the source, runs `build.StartBuild` with `buildsDir = <DataDir>/builds`, prints the workspace path + the step goals. Errors: missing id → exit 2; bad/missing lang → exit 2 mentioning "language"; unknown id → friendly message exit 1; agent/check failure → exit 1.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/build_test.go`:
```go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func TestBuildGeneratesWorkspace(t *testing.T) {
	d := testDeps(t)
	d.invoker = &agent.Fake{Responses: map[string]json.RawMessage{
		"sections":   json.RawMessage(`{"title":"Doc","sections":[{"title":"S1","markdown":"x"}]}`),
		"build plan": json.RawMessage(`{"skeleton_files":[{"path":"go.mod","content":"module x"}],"steps":[{"goal":"do the thing","files":[{"path":"a_test.go","content":"package x"}]}]}`),
	}}
	d.stdin = strings.NewReader("content")
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut); code != 0 {
		t.Fatalf("add exit = %d; %s", code, errOut.String())
	}
	out.Reset()
	// testDeps newID makes the source id "id1".
	if code := run(context.Background(), []string{"build", "id1", "--lang", "go"}, d, &out, &errOut); code != 0 {
		t.Fatalf("build exit = %d; %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "do the thing") || !strings.Contains(out.String(), "id1-go") {
		t.Fatalf("build output missing step/workspace: %q", out.String())
	}
}

func TestBuildRequiresIDAndLang(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"build"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit when build has no id")
	}
	out.Reset()
	errOut.Reset()
	code := run(context.Background(), []string{"build", "id1", "--lang", "haskell"}, d, &out, &errOut)
	if code == 0 || !strings.Contains(errOut.String(), "language") {
		t.Fatalf("expected language error, code=%d stderr=%q", code, errOut.String())
	}
}

func TestBuildUnknownID(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"build", "nope", "--lang", "go"}, d, &out, &errOut)
	if code == 0 || !strings.Contains(errOut.String(), "no source with id") {
		t.Fatalf("expected unknown-id error, code=%d stderr=%q", code, errOut.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run Build`
Expected: FAIL — unknown command `build` (so `TestBuildGeneratesWorkspace` fails at the build step).

- [ ] **Step 3: Add dispatch + help line**

In `internal/cli/cli.go`, add to the `run` switch (after `prepare`):
```go
	case "build":
		return cmdBuild(ctx, args[1:], d, stdout, stderr)
```
Add to `helpText` after the `prepare` line:
```
  build <id> --lang L [--difficulty D]   Scaffold a build workspace for a source
```

- [ ] **Step 4: Implement cmdBuild**

Add to `internal/cli/cli.go` (near `cmdPrepare`):
```go
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
```
Add imports to `internal/cli/cli.go`: `"path/filepath"`, `"github.com/devandbenz/tanaka/internal/build"`, `"github.com/devandbenz/tanaka/internal/model"` (`flag`, `errors`, `store`, `app` are already imported from earlier commands).

- [ ] **Step 5: Run CLI tests + full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/
git commit -m "feat: tanaka build command to scaffold a build workspace"
```

- [ ] **Step 7: Manual end-to-end smoke (real claude)**

Run (use a small prepared/added source id from `tanaka list`; prefer a stdin source to limit tokens):
```bash
go build -o /tmp/tanaka .
/tmp/tanaka list
/tmp/tanaka build <id-of-a-small-source> --lang go --difficulty spec+tests
ls -R ~/.tanaka/builds/<id>-go | head -40
```
Expected: prints the workspace path + numbered step goals; the workspace contains the skeleton (e.g. `go.mod`) and the first step's files. (Spends a little Claude usage.)

---

## Self-Review

**Spec coverage (Plan 3a scope = build data + domain):**
- builds/build_steps tables + cascade — Task 2. ✓
- `gen-build` (content via stdin, lang/difficulty in prompt, path safety, zero-step rejection) — Task 3. ✓
- `gen-hint` — Task 4. ✓
- `TestRunner` interface + exec impl + per-language command map + RunError distinction + fake — Task 5. ✓
- Path-safe workspace scaffolding, `StartBuild`, step progression (`PassStep`/`SkipStep` write next + unlock) — Tasks 3, 6. ✓
- Languages/difficulty validation — Task 1. ✓
- `remove` cascade reaches builds/build_steps (FK cascade) — Task 2. ✓
- Domain types — Task 1. ✓
- Out of scope (Plan 3b): the web build routes/handlers/templates, the in-browser run-tests/hint/skip flow. The `build` CLI command (Task 7) is the 3a end-to-end check; the web layer reuses `build.StartBuild`/`PassStep`/`SkipStep`/`Hint`/`Runner` and the store methods.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. ✓

**Type consistency:** `model.Build`/`BuildStep`/`BuildFile` and lang/difficulty constants used identically across model, store, build, cli. `store` additions (`SaveBuild`, `GetBuild`, `SetBuildStepStatus`, `GetBuildStep`) match call sites. `build.GenerateBuild`/`Hint`/`Runner`/`Result`/`StartBuild`/`PassStep`/`SkipStep`/`WriteFiles`/`SafeRelPath` signatures match their callers and tests. `agent.Job{Stdin, Schema, Prompt}` reused as defined. ✓

---

## Follow-on Plan (not in this document)

- **Plan 3b — build web UI:** `internal/web` build routes (picker, build view), templates, run-tests/hint/skip JS, the in-browser flow, wiring `build.StartBuild`/`PassStep`/`SkipStep`/`Hint` and an injected `build.Runner` into the server, reusing the Plan 3a domain.
