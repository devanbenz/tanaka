# Tanaka Plan 2b — Study Web UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `tanaka serve` — a local, retro Windows-95 web UI (98.css) for the study phase: browse sources, lazily prepare a source, read sections, answer mixed MCQ/free-response quizzes graded server-side, and advance through pass-gated, skippable sections — reusing the Plan 2a domain layer.

**Architecture:** A new `internal/web` package serves server-rendered `html/template` pages styled with embedded 98.css + a little vanilla JS for live grading. The Go server owns routing and calls the Plan 2a domain (`study.PrepareSource`, `study.GradeChoice`, `study.GradeFree`, `study.ComputeUnlocked`) and store. Markdown is rendered to HTML with goldmark. Per-question results are persisted so a section can be marked passed when all its questions are satisfied.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `github.com/yuin/goldmark` (pure-Go markdown), Go stdlib `net/http` (ServeMux with method+wildcard patterns) + `html/template` + `embed`, vendored `98.css`, the Plan 2a `internal/study`/`internal/store`/`internal/agent`/`internal/model`.

## Global Constraints

- Module path: `github.com/devandbenz/tanaka` (verbatim in imports).
- Go version floor: `go 1.26`.
- Pure-Go, no cgo. Third-party deps allowed in this plan: `modernc.org/sqlite` and `github.com/yuin/goldmark` (markdown). No others.
- All agent calls go through `agent.Invoker`; content via `Job.Stdin`, never argv.
- Server binds `127.0.0.1` only; default port `7777`, overridable with `--port`.
- UI aesthetic is retro Windows-95 via 98.css — NOT the default LLM/SaaS look. No marketing copy, no emoji in UI chrome (kaomoji feedback is allowed in verdict text).
- New store tables additive with `ON DELETE CASCADE` from `sections`.
- Status strings exactly `locked`/`unlocked`/`passed`/`skipped`; question kinds `mcq`/`free`; verdicts `pass`/`partial`/`fail`.
- Docs/comments plain and minimal.

---

### Task 1: Markdown rendering (goldmark)

**Files:**
- Create: `internal/web/render.go`
- Test: `internal/web/render_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `web.renderMarkdown(md string) (template.HTML, error)` — converts Markdown to sanitized-enough HTML for our trusted, locally-generated content, returning `html/template.HTML` so templates don't escape it.

- [ ] **Step 1: Add the goldmark dependency**

Run:
```bash
cd /home/devan/tanaka
go get github.com/yuin/goldmark@latest
```
Expected: `go.mod` gains `require github.com/yuin/goldmark ...`.

- [ ] **Step 2: Write the failing test**

Create `internal/web/render_test.go`:
```go
package web

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	html, err := renderMarkdown("# Title\n\nSome **bold** text and `code`.")
	if err != nil {
		t.Fatalf("renderMarkdown: %v", err)
	}
	s := string(html)
	if !strings.Contains(s, "<h1") || !strings.Contains(s, "Title") {
		t.Fatalf("expected an h1 with Title, got %q", s)
	}
	if !strings.Contains(s, "<strong>bold</strong>") {
		t.Fatalf("expected bold, got %q", s)
	}
	if !strings.Contains(s, "<code>code</code>") {
		t.Fatalf("expected code, got %q", s)
	}
}

func TestRenderMarkdownEmpty(t *testing.T) {
	html, err := renderMarkdown("")
	if err != nil {
		t.Fatalf("renderMarkdown empty: %v", err)
	}
	if strings.TrimSpace(string(html)) != "" {
		t.Fatalf("empty markdown should render empty, got %q", html)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/web/`
Expected: FAIL — `undefined: renderMarkdown`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/web/render.go`:
```go
// Package web serves the Tanaka study UI.
package web

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
)

// renderMarkdown converts Markdown to HTML. Content is locally generated and
// trusted, so the result is returned as template.HTML (not re-escaped).
func renderMarkdown(md string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/web/render.go internal/web/render_test.go
git commit -m "feat: markdown rendering with goldmark"
```

---

### Task 2: Store + study helpers for the UI

**Files:**
- Modify: `internal/store/store.go` (schema + interface)
- Modify: `internal/store/study.go` (implementation)
- Create: `internal/study/progress.go`
- Test: `internal/store/ui_test.go`, `internal/study/progress_test.go`

**Interfaces:**
- Consumes: Plan 2a store + `model`.
- Produces:
  - Added to `store.Store`:
    - `GetSection(ctx context.Context, sectionID string) (*model.Section, error)` — `ErrNotFound` if absent (carries `SourceID`, `Idx`, `Title`, `Markdown`).
    - `SetQuestionVerdict(ctx context.Context, questionID, verdict string) error` — upsert into new `question_progress` table.
    - `SectionSatisfied(ctx context.Context, sectionID string) (bool, error)` — true iff the section has zero questions, OR every question has a recorded verdict that is not `fail`.
  - New `question_progress(question_id PK → questions, verdict TEXT)` table.
  - `study.OrderedStatuses(src *model.Source, statuses map[string]string) []string` — statuses in `src.Sections` order (uses `model.StatusLocked` for any missing key).
  - `study.CurrentSectionIdx(statuses []string) int` — index of the first section not `passed`/`skipped`; if all are done, the last index; `0` for empty.

- [ ] **Step 1: Add the table and interface methods**

In `internal/store/store.go`, append to the `schema` const (before the closing backtick):
```sql
CREATE TABLE IF NOT EXISTS question_progress (
	question_id TEXT PRIMARY KEY REFERENCES questions(id) ON DELETE CASCADE,
	verdict     TEXT NOT NULL
);
```
And add to the `Store` interface:
```go
	GetSection(ctx context.Context, sectionID string) (*model.Section, error)
	SetQuestionVerdict(ctx context.Context, questionID, verdict string) error
	SectionSatisfied(ctx context.Context, sectionID string) (bool, error)
```

- [ ] **Step 2: Write the failing store test**

Create `internal/store/ui_test.go`:
```go
package store

import (
	"context"
	"errors"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestGetSection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	sec, err := s.GetSection(ctx, "sec1")
	if err != nil {
		t.Fatalf("GetSection: %v", err)
	}
	if sec.SourceID != "src1" || sec.Markdown != "m" {
		t.Fatalf("section = %+v", sec)
	}
	if _, err := s.GetSection(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing section err = %v, want ErrNotFound", err)
	}
}

func TestSectionSatisfied(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	// No questions yet -> satisfied (auto-pass).
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); !ok {
		t.Fatal("section with no questions should be satisfied")
	}
	// Add two questions.
	err := s.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec1", Summary: "x", KeyConcepts: []string{"k"},
		Questions: []model.Question{
			{ID: "q1", SectionID: "sec1", Idx: 0, Kind: model.KindFree, Prompt: "p", Rubric: "r"},
			{ID: "q2", SectionID: "sec1", Idx: 1, Kind: model.KindMCQ, Prompt: "p", Options: []string{"a"}, CorrectIndex: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); ok {
		t.Fatal("unanswered questions -> not satisfied")
	}
	if err := s.SetQuestionVerdict(ctx, "q1", "pass"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); ok {
		t.Fatal("one of two answered -> not satisfied")
	}
	if err := s.SetQuestionVerdict(ctx, "q2", "partial"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); !ok {
		t.Fatal("all answered non-fail -> satisfied")
	}
	// A fail makes it unsatisfied.
	if err := s.SetQuestionVerdict(ctx, "q2", "fail"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); ok {
		t.Fatal("a fail verdict -> not satisfied")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'GetSection|SectionSatisfied'`
Expected: FAIL — interface methods undefined.

- [ ] **Step 4: Implement the store methods**

Append to `internal/store/study.go`:
```go
func (s *sqliteStore) GetSection(ctx context.Context, sectionID string) (*model.Section, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, idx, title, markdown FROM sections WHERE id = ?`, sectionID)
	var sec model.Section
	if err := row.Scan(&sec.ID, &sec.SourceID, &sec.Idx, &sec.Title, &sec.Markdown); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get section %s: %w", sectionID, err)
	}
	return &sec, nil
}

func (s *sqliteStore) SetQuestionVerdict(ctx context.Context, questionID, verdict string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO question_progress (question_id, verdict) VALUES (?, ?)
		 ON CONFLICT(question_id) DO UPDATE SET verdict=excluded.verdict`, questionID, verdict)
	if err != nil {
		return fmt.Errorf("set verdict %s: %w", questionID, err)
	}
	return nil
}

func (s *sqliteStore) SectionSatisfied(ctx context.Context, sectionID string) (bool, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM questions WHERE section_id = ?`, sectionID).Scan(&total); err != nil {
		return false, fmt.Errorf("count questions: %w", err)
	}
	if total == 0 {
		return true, nil
	}
	var nonFail int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM question_progress qp
		 JOIN questions q ON q.id = qp.question_id
		 WHERE q.section_id = ? AND qp.verdict != 'fail'`, sectionID).Scan(&nonFail); err != nil {
		return false, fmt.Errorf("count satisfied: %w", err)
	}
	return nonFail == total, nil
}
```
Ensure `internal/store/study.go` imports `errors` (added in Plan 2a final fix) — if missing, add it.

- [ ] **Step 5: Run store tests to verify they pass**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 6: Write the failing study-helper test**

Create `internal/study/progress_test.go`:
```go
package study

import (
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestOrderedStatuses(t *testing.T) {
	src := &model.Source{ID: "s", Sections: []model.Section{
		{ID: "a", Idx: 0}, {ID: "b", Idx: 1}, {ID: "c", Idx: 2},
	}}
	statuses := map[string]string{"a": model.StatusPassed, "b": model.StatusUnlocked}
	got := OrderedStatuses(src, statuses)
	want := []string{model.StatusPassed, model.StatusUnlocked, model.StatusLocked}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCurrentSectionIdx(t *testing.T) {
	cases := []struct {
		in   []string
		want int
	}{
		{[]string{model.StatusUnlocked, model.StatusLocked}, 0},
		{[]string{model.StatusPassed, model.StatusUnlocked, model.StatusLocked}, 1},
		{[]string{model.StatusPassed, model.StatusSkipped, model.StatusPassed}, 2}, // all done -> last
		{[]string{}, 0},
	}
	for _, c := range cases {
		if got := CurrentSectionIdx(c.in); got != c.want {
			t.Fatalf("CurrentSectionIdx(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 7: Run test to verify it fails**

Run: `go test ./internal/study/ -run 'OrderedStatuses|CurrentSectionIdx'`
Expected: FAIL — undefined functions.

- [ ] **Step 8: Implement the study helpers**

Create `internal/study/progress.go`:
```go
package study

import "github.com/devandbenz/tanaka/internal/model"

// OrderedStatuses projects the status map onto the source's section order,
// defaulting missing entries to locked.
func OrderedStatuses(src *model.Source, statuses map[string]string) []string {
	out := make([]string, len(src.Sections))
	for i, sec := range src.Sections {
		if st, ok := statuses[sec.ID]; ok {
			out[i] = st
		} else {
			out[i] = model.StatusLocked
		}
	}
	return out
}

// CurrentSectionIdx returns the index of the first section that is not passed or
// skipped. If all are done, it returns the last index. Empty input returns 0.
func CurrentSectionIdx(statuses []string) int {
	for i, s := range statuses {
		if s != model.StatusPassed && s != model.StatusSkipped {
			return i
		}
	}
	if len(statuses) == 0 {
		return 0
	}
	return len(statuses) - 1
}
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./internal/store/ ./internal/study/`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/store/ internal/study/progress.go internal/study/progress_test.go
git commit -m "feat: store GetSection/question verdicts/SectionSatisfied + ui status helpers"
```

---

### Task 3: Server scaffold, embedded retro assets, and home page

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/assets/98.css` (vendored)
- Create: `internal/web/assets/app.css`
- Create: `internal/web/assets/app.js`
- Create: `internal/web/templates/base.html`
- Create: `internal/web/templates/home.html`
- Test: `internal/web/server_test.go`

**Interfaces:**
- Consumes: `store.Store`, `agent.Invoker`, `study.OrderedStatuses`, `renderMarkdown` (Task 1).
- Produces:
  - `web.NewServer(st store.Store, inv agent.Invoker, newID func() string) (*Server, error)` — parses embedded templates.
  - `(*Server) Handler() http.Handler` — a `*http.ServeMux` with all routes; this task registers `GET /` and `GET /static/`.
  - Home page lists sources with a `done/total` progress count (passed+skipped over total sections).

- [ ] **Step 1: Vendor 98.css and add app assets**

Run:
```bash
cd /home/devan/tanaka
mkdir -p internal/web/assets internal/web/templates
curl -fsSL https://unpkg.com/98.css@0.1.20/dist/98.css -o internal/web/assets/98.css
test -s internal/web/assets/98.css && echo "98.css vendored"
```
Expected: prints `98.css vendored` (file is non-empty). If the network is unavailable, STOP and report — do not hand-write a stub.

Create `internal/web/assets/app.css`:
```css
body { background: #008080; padding: 16px; font-family: Arial, sans-serif; }
.layout { display: flex; gap: 12px; align-items: flex-start; }
.sidebar { width: 220px; flex: none; }
.content { flex: 1; }
.section-list { list-style: none; margin: 0; padding: 0; }
.section-list li { padding: 2px 4px; }
.section-list .current { font-weight: bold; }
.section-list .locked { color: gray; }
.markdown { background: #fff; border: 1px solid #808080; padding: 8px; }
.quiz { margin-top: 12px; }
.verdict { margin-top: 6px; }
```

Create `internal/web/assets/app.js`:
```javascript
// Live quiz grading: POST an answer to /grade and show the verdict.
async function grade(form) {
  const data = {
    questionId: form.dataset.qid,
    choice: form.querySelector('input[type=radio]:checked')
      ? parseInt(form.querySelector('input[type=radio]:checked').value, 10)
      : -1,
    answer: form.querySelector('textarea') ? form.querySelector('textarea').value : "",
  };
  const out = form.querySelector('.verdict');
  out.textContent = 'grading...';
  try {
    const res = await fetch('/grade', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    if (!res.ok) { out.textContent = 'grading unavailable - try again'; return; }
    const v = await res.json();
    out.textContent = v.verdict + (v.feedback ? ' - ' + v.feedback : '');
    if (v.sectionPassed) {
      const next = document.getElementById('next-btn');
      if (next) next.disabled = false;
    }
  } catch (e) {
    out.textContent = 'grading unavailable - try again';
  }
}
document.addEventListener('submit', function (e) {
  if (e.target.classList.contains('quiz-form')) { e.preventDefault(); grade(e.target); }
});
```

- [ ] **Step 2: Write the templates**

Create `internal/web/templates/base.html`:
```html
{{define "base"}}<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Tanaka{{if .Title}} - {{.Title}}{{end}}</title>
  <link rel="stylesheet" href="/static/98.css">
  <link rel="stylesheet" href="/static/app.css">
</head>
<body>
{{template "content" .}}
<script src="/static/app.js"></script>
</body>
</html>{{end}}
```

Create `internal/web/templates/home.html`:
```html
{{define "content"}}
<div class="window" style="max-width:640px">
  <div class="title-bar"><div class="title-bar-text">Tanaka - Sources</div></div>
  <div class="window-body">
    {{if not .Sources}}
      <p>No sources yet. Add one from the CLI: <code>tanaka add &lt;file|url|-&gt;</code></p>
    {{else}}
    <ul class="tree-view">
      {{range .Sources}}
      <li><a href="/study/{{.ID}}">{{.Title}}</a> &mdash; {{.Done}}/{{.Total}}</li>
      {{end}}
    </ul>
    {{end}}
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Write the failing test**

Create `internal/web/server_test.go`:
```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func testServer(t *testing.T) (*Server, store.Store) {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	n := 0
	srv, err := NewServer(st, &agent.Fake{}, func() string { n++; return "id" + string(rune('0'+n)) })
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, st
}

func TestHomeListsSources(t *testing.T) {
	srv, st := testServer(t)
	st.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "My Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"}},
	})
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "My Paper") || !strings.Contains(body, "0/1") {
		t.Fatalf("home body missing source/progress: %q", body)
	}
}

func TestStaticAssetsServed(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest("GET", "/static/98.css", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("98.css not served: status=%d len=%d", rec.Code, rec.Body.Len())
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/web/ -run 'Home|Static'`
Expected: FAIL — `undefined: NewServer`, `undefined: Server`.

- [ ] **Step 5: Implement the server**

Create `internal/web/server.go`:
```go
package web

import (
	"context"
	"embed"
	"html/template"
	"net/http"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
	"github.com/devandbenz/tanaka/internal/study"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed assets/*
var assetsFS embed.FS

// Server holds dependencies for the study UI.
type Server struct {
	store store.Store
	inv   agent.Invoker
	newID func() string
	tmpl  *template.Template
}

// NewServer parses the embedded templates and returns a Server.
func NewServer(st store.Store, inv agent.Invoker, newID func() string) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{store: st, inv: inv, newID: newID, tmpl: tmpl}, nil
}

// Handler returns the HTTP router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(assetsFS))
	mux.HandleFunc("GET /{$}", s.handleHome)
	return mux
}

// render executes the base template with the named content block and data.
// The content block name is selected by ParseFS (each page file defines
// "content"); we clone per-render to bind the right content template.
func (s *Server) render(w http.ResponseWriter, page string, data map[string]any) {
	t, err := s.tmpl.Clone()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// page file (e.g. "home.html") provides "content"; parse just that file last.
	if _, err := t.ParseFS(templatesFS, "templates/"+page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type sourceRow struct {
	ID, Title    string
	Done, Total  int
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sources, err := s.store.ListSources(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var rows []sourceRow
	for _, src := range sources {
		full, err := s.store.GetSource(ctx, src.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		statuses, err := s.store.GetSectionStatuses(ctx, src.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		done := 0
		for _, st := range study.OrderedStatuses(full, statuses) {
			if st == model.StatusPassed || st == model.StatusSkipped {
				done++
			}
		}
		rows = append(rows, sourceRow{ID: src.ID, Title: src.Title, Done: done, Total: len(full.Sections)})
	}
	s.render(w, "home.html", map[string]any{"Title": "", "Sources": rows})
}

var _ = context.Background // keep context import if unused elsewhere
```
Note: remove the `var _ = context.Background` line if `context` is otherwise used; it's only there to avoid an unused-import error if you trim code. The handler uses `r.Context()`, so `context` may be unused — in that case delete both the import and this line.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS (render + home + static).

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/web/
git commit -m "feat: web server scaffold, embedded 98.css assets, home page"
```

---

### Task 4: Study entry + prepare flow

**Files:**
- Modify: `internal/web/server.go` (routes + handlers)
- Create: `internal/web/templates/prepare.html`
- Test: `internal/web/prepare_test.go`

**Interfaces:**
- Consumes: `store.IsPrepared`, `store.GetSource`, `store.GetSectionStatuses`, `study.PrepareSource`, `study.OrderedStatuses`, `study.CurrentSectionIdx`.
- Produces: routes `GET /study/{id}` and `POST /study/{id}/prepare`.
  - `GET /study/{id}`: 404 if no source; if not prepared → render prepare page; else redirect (303) to `/study/{id}/{current}` where current = `CurrentSectionIdx(ordered statuses)`.
  - `POST /study/{id}/prepare`: run `study.PrepareSource` synchronously, then redirect (303) to `/study/{id}/0`. On error, 500 with a message.

- [ ] **Step 1: Add routes**

In `internal/web/server.go` `Handler()`, add before `return mux`:
```go
	mux.HandleFunc("GET /study/{id}", s.handleStudyEntry)
	mux.HandleFunc("POST /study/{id}/prepare", s.handlePrepare)
```

- [ ] **Step 2: Write the prepare template**

Create `internal/web/templates/prepare.html`:
```html
{{define "content"}}
<div class="window" style="max-width:520px">
  <div class="title-bar"><div class="title-bar-text">Prepare - {{.Source.Title}}</div></div>
  <div class="window-body">
    <p>This source has not been prepared yet. Preparing generates a summary and
       quiz for each of its {{len .Source.Sections}} sections. This can take a
       minute and uses your Claude subscription.</p>
    <form method="POST" action="/study/{{.Source.ID}}/prepare" onsubmit="this.querySelector('button').disabled=true;this.querySelector('button').textContent='preparing...'">
      <button type="submit">Prepare this source</button>
    </form>
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Write the failing test**

Create `internal/web/prepare_test.go`:
```go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func addSource(t *testing.T, st store.Store, id string, nSections int) {
	t.Helper()
	src := &model.Source{ID: id, Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0)}
	for i := 0; i < nSections; i++ {
		src.Sections = append(src.Sections, model.Section{
			ID: id + "-s" + string(rune('0'+i)), SourceID: id, Idx: i, Title: "S", Markdown: "body",
		})
	}
	if err := st.SaveSource(context.Background(), src); err != nil {
		t.Fatal(err)
	}
}

func studyFake() *agent.Fake {
	return &agent.Fake{Responses: map[string]json.RawMessage{
		"study package": json.RawMessage(`{"summary":"s","key_concepts":["k"],"questions":[{"kind":"free","prompt":"why","rubric":"r"}]}`),
	}}
}

func TestStudyEntryUnpreparedShowsPreparePage(t *testing.T) {
	srv, st := testServer(t)
	addSource(t, st, "src1", 2)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Prepare this source") {
		t.Fatalf("expected prepare page, got %q", rec.Body.String())
	}
}

func TestStudyEntryUnknownIs404(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPrepareThenEntryRedirects(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = studyFake() // server uses the fake invoker for PrepareSource
	addSource(t, st, "src1", 2)
	// Prepare.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/prepare", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/study/src1/0" {
		t.Fatalf("prepare redirect = %q, want /study/src1/0", loc)
	}
	// Now entry redirects to current section instead of the prepare page.
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/study/src1", nil))
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("entry status = %d, want 303", rec2.Code)
	}
	if loc := rec2.Header().Get("Location"); !strings.HasPrefix(loc, "/study/src1/") {
		t.Fatalf("entry redirect = %q", loc)
	}
}
```
Note: `srv.inv` is an unexported field; the test is in package `web` so it can set it.

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/web/ -run 'StudyEntry|Prepare'`
Expected: FAIL — `undefined: s.handleStudyEntry`.

- [ ] **Step 5: Implement the handlers**

Add to `internal/web/server.go`:
```go
func (s *Server) handleStudyEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	prepared, err := s.store.IsPrepared(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !prepared {
		s.render(w, "prepare.html", map[string]any{"Title": src.Title, "Source": src})
		return
	}
	statuses, err := s.store.GetSectionStatuses(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idx := study.CurrentSectionIdx(study.OrderedStatuses(src, statuses))
	http.Redirect(w, r, "/study/"+id+"/"+itoa(idx), http.StatusSeeOther)
}

func (s *Server) handlePrepare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := study.PrepareSource(ctx, s.inv, s.store, src, s.newID, nil); err != nil {
		http.Error(w, "prepare failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/study/"+id+"/0", http.StatusSeeOther)
}
```
Add a small int-to-string helper at the bottom of `server.go` (avoid importing strconv repeatedly is fine; you may use strconv instead):
```go
import "strconv" // add to the import block

func itoa(i int) string { return strconv.Itoa(i) }
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/web/
git commit -m "feat: study entry and synchronous prepare flow"
```

---

### Task 5: Section page (reading + quiz, with gating)

**Files:**
- Modify: `internal/web/server.go` (route + handler)
- Create: `internal/web/templates/section.html`
- Create: `internal/web/templates/locked.html`
- Test: `internal/web/section_test.go`

**Interfaces:**
- Consumes: `store.GetSource`, `store.GetSectionStudy`, `store.GetSectionStatuses`, `study.ComputeUnlocked`, `study.OrderedStatuses`, `renderMarkdown`.
- Produces: route `GET /study/{id}/{idx}` rendering the section page: section nav (with status marks + lock), rendered markdown, key concepts, and the quiz (MCQ radios / free textarea). If the requested section is not unlocked (per `ComputeUnlocked`), render the locked notice instead. 404 for unknown source or out-of-range idx.

- [ ] **Step 1: Add the route**

In `Handler()` add:
```go
	mux.HandleFunc("GET /study/{id}/{idx}", s.handleSection)
```

- [ ] **Step 2: Write the templates**

Create `internal/web/templates/section.html`:
```html
{{define "content"}}
<div class="layout">
  <div class="sidebar window">
    <div class="title-bar"><div class="title-bar-text">{{.Source.Title}}</div></div>
    <div class="window-body">
      <ul class="section-list">
        {{range .Nav}}
        <li class="{{if .Current}}current{{end}} {{if not .Unlocked}}locked{{end}}">
          {{if .Unlocked}}<a href="/study/{{$.Source.ID}}/{{.Idx}}">{{.Mark}} {{.Title}}</a>
          {{else}}{{.Mark}} {{.Title}}{{end}}
        </li>
        {{end}}
      </ul>
      <p class="status-bar"><span class="status-bar-field">progress {{.Done}}/{{.Total}}</span></p>
    </div>
  </div>
  <div class="content window">
    <div class="title-bar"><div class="title-bar-text">{{.Section.Title}}</div></div>
    <div class="window-body">
      <div class="markdown">{{.Body}}</div>
      {{if .Concepts}}
      <fieldset><legend>Key concepts</legend>
        <ul>{{range .Concepts}}<li>{{.}}</li>{{end}}</ul>
      </fieldset>
      {{end}}
      <div class="quiz">
        <fieldset><legend>Quiz</legend>
        {{range .Questions}}
          <form class="quiz-form" data-qid="{{.ID}}">
            <p>{{.Prompt}}</p>
            {{if eq .Kind "mcq"}}
              {{range $i, $opt := .Options}}
                <div class="field-row"><input type="radio" id="{{$.Section.ID}}-{{$.Section.Idx}}-{{$opt}}" name="opt-{{.}}" value="{{$i}}"><label>{{$opt}}</label></div>
              {{end}}
            {{else}}
              <textarea rows="4" cols="50"></textarea>
            {{end}}
            <div class="field-row"><button type="submit">Check</button></div>
            <div class="verdict"></div>
          </form>
        {{end}}
        </fieldset>
      </div>
      <div class="field-row" style="justify-content:space-between">
        <form method="POST" action="/study/{{.Source.ID}}/{{.Section.Idx}}/skip"><button type="submit">Skip section</button></form>
        {{if .HasNext}}<a href="/study/{{.Source.ID}}/{{.NextIdx}}"><button id="next-btn" {{if not .NextUnlocked}}disabled{{end}}>Next</button></a>{{end}}
      </div>
    </div>
  </div>
</div>
{{end}}
```

Create `internal/web/templates/locked.html`:
```html
{{define "content"}}
<div class="window" style="max-width:480px">
  <div class="title-bar"><div class="title-bar-text">Locked</div></div>
  <div class="window-body">
    <p>Finish the previous section first.</p>
    <a href="/study/{{.SourceID}}/{{.PrevIdx}}"><button>Go back</button></a>
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Write the failing test**

Create `internal/web/section_test.go`:
```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func prep(t *testing.T, srv *Server) {
	t.Helper()
	srv.inv = studyFake()
	addSource(t, srv.store, "src1", 2)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/prepare", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSectionPageRendersReadingAndQuiz(t *testing.T) {
	srv, _ := testServer(t)
	prep(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/0", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "quiz-form") || !strings.Contains(body, "why") {
		t.Fatalf("section page missing quiz: %q", body)
	}
}

func TestLockedSectionShowsNotice(t *testing.T) {
	srv, _ := testServer(t)
	prep(t, srv)
	// Section 1 is locked (section 0 only unlocked after prepare).
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Finish the previous section first") {
		t.Fatalf("expected locked notice, got %q", rec.Body.String())
	}
}

func TestSectionOutOfRangeIs404(t *testing.T) {
	srv, _ := testServer(t)
	prep(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/9", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	_ = context.Background
	_ = model.StatusLocked
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/web/ -run 'Section|Locked'`
Expected: FAIL — `undefined: s.handleSection`.

- [ ] **Step 5: Implement the handler**

Add to `internal/web/server.go`:
```go
type navItem struct {
	Idx      int
	Title    string
	Mark     string
	Current  bool
	Unlocked bool
}

func mark(status string) string {
	switch status {
	case model.StatusPassed:
		return "[x]"
	case model.StatusSkipped:
		return "[-]"
	default:
		return "[ ]"
	}
}

func (s *Server) handleSection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if idx < 0 || idx >= len(src.Sections) {
		http.NotFound(w, r)
		return
	}
	statuses, err := s.store.GetSectionStatuses(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ordered := study.OrderedStatuses(src, statuses)
	unlocked := study.ComputeUnlocked(ordered)
	if !unlocked[idx] {
		s.render(w, "locked.html", map[string]any{"Title": "Locked", "SourceID": id, "PrevIdx": idx - 1})
		return
	}
	sec := src.Sections[idx]
	stud, err := s.store.GetSectionStudy(ctx, sec.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := renderMarkdown(sec.Markdown)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var nav []navItem
	done := 0
	for i, seci := range src.Sections {
		nav = append(nav, navItem{Idx: i, Title: seci.Title, Mark: mark(ordered[i]), Current: i == idx, Unlocked: unlocked[i]})
		if ordered[i] == model.StatusPassed || ordered[i] == model.StatusSkipped {
			done++
		}
	}
	hasNext := idx+1 < len(src.Sections)
	data := map[string]any{
		"Title": src.Title, "Source": src, "Section": sec, "Body": body,
		"Concepts": stud.KeyConcepts, "Questions": stud.Questions, "Nav": nav,
		"Done": done, "Total": len(src.Sections),
		"HasNext": hasNext, "NextIdx": idx + 1,
		"NextUnlocked": ordered[idx] == model.StatusPassed || ordered[idx] == model.StatusSkipped,
	}
	s.render(w, "section.html", data)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/web/
git commit -m "feat: section page with reading, quiz, and gating"
```

---

### Task 6: Grade endpoint

**Files:**
- Modify: `internal/web/server.go` (route + handler)
- Test: `internal/web/grade_test.go`

**Interfaces:**
- Consumes: `store.GetQuestion`, `store.GetSection`, `store.SetQuestionVerdict`, `store.SectionSatisfied`, `store.SetSectionStatus`, `store.GetSource`, `store.GetSectionStatuses`, `study.GradeChoice`, `study.GradeFree`.
- Produces: route `POST /grade` accepting JSON `{questionId string, choice int, answer string}` and returning JSON `{verdict, feedback, sectionPassed bool}`. MCQ → `GradeChoice`; free → `GradeFree` (needs the section markdown via `GetSection`). Persists the verdict; if the verdict is not `fail` and the section is now satisfied, sets the section `passed` and unlocks the next section (if currently locked).

- [ ] **Step 1: Add the route**

In `Handler()` add:
```go
	mux.HandleFunc("POST /grade", s.handleGrade)
```

- [ ] **Step 2: Write the failing test**

Create `internal/web/grade_test.go`:
```go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func gradeReq(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/grade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// Build a prepared source with one free question we control directly.
func preppedWithFreeQ(t *testing.T, srv *Server) string {
	t.Helper()
	ctx := context.Background()
	addSource(t, srv.store, "src1", 2)
	err := srv.store.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "src1-s0", Summary: "s", KeyConcepts: []string{"k"},
		Questions: []model.Question{{ID: "q1", SectionID: "src1-s0", Idx: 0, Kind: model.KindFree, Prompt: "why", Rubric: "r"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Second section too, so "unlock next" is observable.
	if err := srv.store.SaveSectionStudy(ctx, &model.SectionStudy{SectionID: "src1-s1", Summary: "s2", KeyConcepts: []string{"k"}}); err != nil {
		t.Fatal(err)
	}
	srv.store.SetSectionStatus(ctx, "src1-s0", model.StatusUnlocked)
	return "q1"
}

func TestGradeFreePassUnlocksNext(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = studyFake() // its grade-answer fake below
	srv.inv = &fakeGrader{verdict: "pass", feedback: "good"}
	preppedWithFreeQ(t, srv)
	rec := gradeReq(t, srv, `{"questionId":"q1","answer":"because"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Verdict       string `json:"verdict"`
		SectionPassed bool   `json:"sectionPassed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Verdict != "pass" || !resp.SectionPassed {
		t.Fatalf("resp = %+v, want pass + sectionPassed", resp)
	}
	// Section 0 should now be passed and section 1 unlocked.
	statuses, _ := st.GetSectionStatuses(context.Background(), "src1")
	if statuses["src1-s0"] != model.StatusPassed {
		t.Fatalf("s0 = %q, want passed", statuses["src1-s0"])
	}
	if statuses["src1-s1"] != model.StatusUnlocked {
		t.Fatalf("s1 = %q, want unlocked", statuses["src1-s1"])
	}
}

func TestGradeUnknownQuestion404(t *testing.T) {
	srv, _ := testServer(t)
	rec := gradeReq(t, srv, `{"questionId":"nope","answer":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

Also add a small grading fake in the test file (the package `agent.Fake` matches on prompt; a dedicated grader fake is simpler here):
```go
type fakeGrader struct {
	verdict, feedback string
}

func (f *fakeGrader) Check(ctx context.Context) error { return nil }
func (f *fakeGrader) Invoke(ctx context.Context, job interface{ }) (interface{}, error) { panic("unused") }
```
NOTE: that stub signature will not satisfy `agent.Invoker`. Instead implement it against the real interface:
```go
// replace the stub above with this:
// fakeGrader implements agent.Invoker, always returning a fixed verdict JSON.
```
Implement it properly in Step 3's guidance.

- [ ] **Step 3: Implement a proper grading fake in the test**

Replace the `fakeGrader` stub with this (delete the panic version):
```go
type fakeGrader struct {
	verdict, feedback string
}

func (f *fakeGrader) Check(ctx context.Context) error { return nil }
func (f *fakeGrader) Invoke(ctx context.Context, job agent.Job) (json.RawMessage, error) {
	return json.RawMessage(`{"verdict":"` + f.verdict + `","feedback":"` + f.feedback + `"}`), nil
}
```
Add `"github.com/devandbenz/tanaka/internal/agent"` to the test imports.

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/web/ -run Grade`
Expected: FAIL — `undefined: s.handleGrade`.

- [ ] **Step 5: Implement the handler**

Add to `internal/web/server.go`:
```go
type gradeRequest struct {
	QuestionID string `json:"questionId"`
	Choice     int    `json:"choice"`
	Answer     string `json:"answer"`
}

type gradeResponse struct {
	Verdict       string `json:"verdict"`
	Feedback      string `json:"feedback"`
	SectionPassed bool   `json:"sectionPassed"`
}

func (s *Server) handleGrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req gradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	q, err := s.store.GetQuestion(ctx, req.QuestionID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var v study.Verdict
	if q.Kind == model.KindMCQ {
		v = study.GradeChoice(q, req.Choice)
	} else {
		sec, err := s.store.GetSection(ctx, q.SectionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		v, err = study.GradeFree(ctx, s.inv, sec.Markdown, q, req.Answer)
		if err != nil {
			http.Error(w, "grading unavailable", http.StatusBadGateway)
			return
		}
	}
	if err := s.store.SetQuestionVerdict(ctx, q.ID, v.Verdict); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := gradeResponse{Verdict: v.Verdict, Feedback: v.Feedback}
	if v.Verdict != "fail" {
		satisfied, err := s.store.SectionSatisfied(ctx, q.SectionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if satisfied {
			if err := s.passAndUnlockNext(ctx, q.SectionID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp.SectionPassed = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// passAndUnlockNext marks the section passed and unlocks the following section
// (by source order) if it is currently locked.
func (s *Server) passAndUnlockNext(ctx context.Context, sectionID string) error {
	sec, err := s.store.GetSection(ctx, sectionID)
	if err != nil {
		return err
	}
	if err := s.store.SetSectionStatus(ctx, sectionID, model.StatusPassed); err != nil {
		return err
	}
	src, err := s.store.GetSource(ctx, sec.SourceID)
	if err != nil {
		return err
	}
	next := sec.Idx + 1
	if next < len(src.Sections) {
		statuses, err := s.store.GetSectionStatuses(ctx, src.ID)
		if err != nil {
			return err
		}
		nextID := src.Sections[next].ID
		if statuses[nextID] == model.StatusLocked {
			return s.store.SetSectionStatus(ctx, nextID, model.StatusUnlocked)
		}
	}
	return nil
}
```
Add `"encoding/json"` to the import block.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/web/
git commit -m "feat: grade endpoint with section pass and next-unlock"
```

---

### Task 7: Skip endpoint + `tanaka serve` command + smoke

**Files:**
- Modify: `internal/web/server.go` (route + skip handler)
- Modify: `internal/cli/cli.go` (serve command + dispatch + help)
- Test: `internal/web/skip_test.go`, `internal/cli/serve_test.go`

**Interfaces:**
- Consumes: `store.GetSource`, `store.SetSectionStatus`, `web.NewServer`, `web.(*Server).Handler`.
- Produces:
  - route `POST /study/{id}/{idx}/skip`: set the section `skipped`, unlock the next section if locked, redirect (303) to the next section (or back to the same if last).
  - `tanaka serve [--port N]`: builds a `web.Server` from the real store + Claude invoker and serves on `127.0.0.1:N` (default 7777). On bind error, exit 1 with a clear message.

- [ ] **Step 1: Add the skip route + handler**

In `Handler()` add:
```go
	mux.HandleFunc("POST /study/{id}/{idx}/skip", s.handleSkip)
```
Add to `server.go`:
```go
func (s *Server) handleSkip(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if idx < 0 || idx >= len(src.Sections) {
		http.NotFound(w, r)
		return
	}
	if err := s.store.SetSectionStatus(ctx, src.Sections[idx].ID, model.StatusSkipped); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	next := idx + 1
	if next < len(src.Sections) {
		statuses, err := s.store.GetSectionStatuses(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if statuses[src.Sections[next].ID] == model.StatusLocked {
			if err := s.store.SetSectionStatus(ctx, src.Sections[next].ID, model.StatusUnlocked); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		http.Redirect(w, r, "/study/"+id+"/"+itoa(next), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/study/"+id+"/"+itoa(idx), http.StatusSeeOther)
}
```

- [ ] **Step 2: Write the failing skip test**

Create `internal/web/skip_test.go`:
```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestSkipMarksSkippedAndUnlocksNext(t *testing.T) {
	srv, st := testServer(t)
	prep(t, srv) // src1 with 2 sections, section 0 unlocked
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/0/skip", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/study/src1/1" {
		t.Fatalf("redirect = %q, want /study/src1/1", loc)
	}
	statuses, _ := st.GetSectionStatuses(context.Background(), "src1")
	if statuses["src1-s0"] != model.StatusSkipped {
		t.Fatalf("s0 = %q, want skipped", statuses["src1-s0"])
	}
	if statuses["src1-s1"] != model.StatusUnlocked {
		t.Fatalf("s1 = %q, want unlocked", statuses["src1-s1"])
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/web/ -run Skip`
Expected: FAIL — `undefined: s.handleSkip`.

- [ ] **Step 4: Implement (done in Step 1) and verify**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 5: Write the failing serve-command test**

Create `internal/cli/serve_test.go`:
```go
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestServeRejectsBadPort(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	// Port 0 with our flag parsing is invalid usage in our command (we require >0).
	if code := run(context.Background(), []string{"serve", "--port", "-1"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit for invalid port")
	}
	if !strings.Contains(errOut.String(), "port") {
		t.Fatalf("stderr = %q, want it to mention port", errOut.String())
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/cli/ -run Serve`
Expected: FAIL — unknown command `serve` (default branch returns 2 but no "port" message), so the assertion fails.

- [ ] **Step 7: Implement the serve command**

In `internal/cli/cli.go`, add the dispatch case:
```go
	case "serve":
		return cmdServe(ctx, args[1:], d, stdout, stderr)
```
Add to `helpText` after the `prepare` line:
```
  serve [--port N]   Start the local study web UI (default 127.0.0.1:7777)
```
Add the command (uses `flag`, `net/http`, `web`):
```go
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
	go func() { <-ctx.Done(); httpSrv.Close() }()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}
```
Add imports to `internal/cli/cli.go`: `"flag"`, `"net/http"`, and `"github.com/devandbenz/tanaka/internal/web"`.

- [ ] **Step 8: Run tests + full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 9: Commit**

```bash
git add internal/web/ internal/cli/
git commit -m "feat: skip endpoint and tanaka serve command"
```

- [ ] **Step 10: Manual end-to-end smoke (real browser, optional claude)**

Run:
```bash
go build -o /tmp/tanaka .
/tmp/tanaka serve --port 7777 &
SERVE_PID=$!
sleep 1
curl -fsS http://127.0.0.1:7777/ | grep -q "Tanaka - Sources" && echo "home OK"
curl -fsS http://127.0.0.1:7777/static/98.css | head -c 40
kill $SERVE_PID
```
Expected: `home OK` and the first bytes of 98.css. Then, for a full manual check, run `tanaka serve` and open `http://127.0.0.1:7777` in a browser: pick a prepared source (or click Prepare), read section 1, answer the quiz, and confirm the Next button enables after the section is satisfied. (Grading a free answer uses a little Claude usage.)

---

## Self-Review

**Spec coverage (Plan 2 spec, web portion):**
- `tanaka serve`, localhost, `--port` default 7777 — Task 7. ✓
- Routes `GET /`, `GET /study/{id}`, `POST /study/{id}/prepare`, `GET /study/{id}/{idx}`, `POST /grade`, `POST /study/{id}/{idx}/skip`, `GET /static/*` — Tasks 3–7. ✓
- Lazy prepare with synchronous loading state — Task 4 (template disables button + "preparing..."). ✓
- Retro Win95 via embedded 98.css — Task 3. ✓
- Source list with progress — Task 3. ✓
- Section page: markdown + key concepts + quiz, gating/lock — Task 5. ✓
- Mixed MCQ (server check) + free (agent) grading via `/grade` — Task 6. ✓
- Pass-gated, skippable; partial advances — Tasks 5–7 (SectionSatisfied treats partial as non-fail; skip unlocks next). ✓
- Markdown rendering — Task 1 (goldmark). ✓
- Error handling: unknown id/idx → 404; locked → notice; grading failure → 502 + "grading unavailable" (JS shows retry); prepare failure → 500; bad port → exit 2 — Tasks 4–7. ✓
- Per-question result persistence to compute section pass — Task 2 (`question_progress`, `SectionSatisfied`). ✓
- `remove` cascade covers new `question_progress` (FK cascade) — Task 2. ✓

**Placeholder scan:** No TBD/TODO; every step has complete code. The one stub-in-progress in Task 6 Step 2 is explicitly replaced in Step 3 (the proper `fakeGrader`). ✓

**Type consistency:** `study.Verdict{Verdict, Feedback}`, `study.GradeChoice(*model.Question,int)`, `study.GradeFree(ctx,inv,string,*model.Question,string)`, `study.ComputeUnlocked([]string)`, `study.OrderedStatuses`, `study.CurrentSectionIdx` match Plan 2a + Task 2. New store methods `GetSection`, `SetQuestionVerdict`, `SectionSatisfied` match their call sites. `agent.Invoker`/`agent.Job` reused as defined. `r.PathValue("id"|"idx")` matches the `{id}`/`{idx}` route patterns. ✓

---

## Notes / Future

- Synchronous prepare can block for a minute on large sources; SSE/streamed progress is a later enhancement (spec §14).
- No answer-history beyond the latest verdict per question (sufficient for gating).
- Auto-open browser on `serve` deferred.
