# Tanaka Feedback, Background Jobs & Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `prepare`/`build` run as background jobs (so the UI never blocks), add server-side logging, and give live feedback — including a global completion toast and animated random-cycling kaomoji in every loading state (web and `tanaka add`).

**Architecture:** A new in-memory `JobManager` on the web `Server` runs long operations in goroutines; the `prepare`/`build` POST handlers start a job and return immediately; in-progress pages and a global `jobs.js` poller read a new `GET /jobs` endpoint (toast on completion). `slog` logs requests + jobs + errors. A shared animated-kaomoji idea is implemented in `kaomoji.js` (web) and the existing `internal/ui` `Spinner` (CLI).

**Tech Stack:** Go 1.26, stdlib `log/slog`/`net/http`/`html/template`/`sync`/`math/rand`, 98.css, existing `internal/build`/`internal/study`/`internal/store`. No new third-party deps.

## Global Constraints

- Module path: `github.com/devandbenz/tanaka` (verbatim in imports).
- Go 1.26; pure-Go; deps limited to `modernc.org/sqlite` + `github.com/yuin/goldmark`.
- Server binds 127.0.0.1; retro Win95 (98.css); no marketing copy/emoji in chrome (kaomoji are allowed as loading/status indicators).
- Background jobs run with `context.Background()` (NOT the request context, which is cancelled when the handler returns); job state is in-memory only.
- One job per key (`prepare:<id>`, `build:<id>:<lang>`); a start while running is a no-op.
- Logging via `log/slog` text handler to stderr, level info; tests use a logger over `io.Discard`.
- Comments/docs plain and minimal.

---

### Task 1: JobManager

**Files:**
- Create: `internal/web/jobs.go`
- Test: `internal/web/jobs_test.go`

**Interfaces:**
- Consumes: `log/slog`.
- Produces:
  - `web.Job{Key, Kind, SourceID, Lang, Status, Progress, Err string; Done bool}` — `Status` is `"running"|"done"|"error"`.
  - `web.NewJobManager(log *slog.Logger) *JobManager`.
  - `(*JobManager).Start(key, kind, sourceID, lang string, fn func(progress func(string)) error) bool` — false if a job for `key` is already running; otherwise registers a running job, launches a goroutine running `fn` (passing a `progress` updater), records `done`/`error` on return, and returns true.
  - `(*JobManager).Get(key string) (Job, bool)` — returns a copy.
  - `(*JobManager).Snapshot() []Job` — copies of all jobs.

- [ ] **Step 1: Write the failing test**

Create `internal/web/jobs_test.go`:
```go
package web

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func quietJM() *JobManager {
	return NewJobManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func waitJob(t *testing.T, m *JobManager, key string) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := m.Get(key); ok && j.Done {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish in time", key)
	return Job{}
}

func TestJobRunsToDone(t *testing.T) {
	m := quietJM()
	got := ""
	started := m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error {
		progress("section 1/2")
		got = "ran"
		return nil
	})
	if !started {
		t.Fatal("Start should return true for a fresh key")
	}
	j := waitJob(t, m, "prepare:a")
	if j.Status != "done" || got != "ran" {
		t.Fatalf("job = %+v, got=%q", j, got)
	}
}

func TestJobError(t *testing.T) {
	m := quietJM()
	m.Start("build:a:go", "build", "a", "go", func(progress func(string)) error {
		return errors.New("boom")
	})
	j := waitJob(t, m, "build:a:go")
	if j.Status != "error" || j.Err != "boom" {
		t.Fatalf("job = %+v, want error/boom", j)
	}
}

func TestJobDedupWhileRunning(t *testing.T) {
	m := quietJM()
	release := make(chan struct{})
	m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error {
		<-release
		return nil
	})
	if m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error { return nil }) {
		t.Fatal("second Start while running should return false")
	}
	close(release)
	waitJob(t, m, "prepare:a")
	// After completion a new Start is allowed again.
	if !m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error { return nil }) {
		t.Fatal("Start after completion should return true")
	}
}

func TestSnapshotAndProgress(t *testing.T) {
	m := quietJM()
	release := make(chan struct{})
	m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error {
		progress("section 2/3")
		<-release
		return nil
	})
	// Give the goroutine a moment to set progress.
	time.Sleep(20 * time.Millisecond)
	snap := m.Snapshot()
	if len(snap) != 1 || snap[0].Key != "prepare:a" || snap[0].Progress != "section 2/3" {
		t.Fatalf("snapshot = %+v", snap)
	}
	close(release)
	waitJob(t, m, "prepare:a")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web/ -run Job`
Expected: FAIL — `undefined: NewJobManager`, `undefined: Job`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/web/jobs.go`:
```go
package web

import (
	"log/slog"
	"sync"
	"time"
)

// Job is the status of a background operation.
type Job struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	SourceID string `json:"sourceId"`
	Lang     string `json:"lang"`
	Status   string `json:"status"` // running | done | error
	Progress string `json:"progress"`
	Err      string `json:"error"`
	Done     bool   `json:"done"`
}

// JobManager runs long operations in the background and tracks their status.
type JobManager struct {
	mu   sync.Mutex
	jobs map[string]*Job
	log  *slog.Logger
}

// NewJobManager returns an empty JobManager.
func NewJobManager(log *slog.Logger) *JobManager {
	return &JobManager{jobs: map[string]*Job{}, log: log}
}

// Start launches fn in a goroutine unless a job for key is already running.
// It returns true if a new job was started.
func (m *JobManager) Start(key, kind, sourceID, lang string, fn func(progress func(string)) error) bool {
	m.mu.Lock()
	if j, ok := m.jobs[key]; ok && j.Status == "running" {
		m.mu.Unlock()
		return false
	}
	m.jobs[key] = &Job{Key: key, Kind: kind, SourceID: sourceID, Lang: lang, Status: "running"}
	m.mu.Unlock()
	m.log.Info("job.start", "kind", kind, "source", sourceID, "lang", lang)

	progress := func(s string) {
		m.mu.Lock()
		if j := m.jobs[key]; j != nil {
			j.Progress = s
		}
		m.mu.Unlock()
		m.log.Info("job.progress", "kind", kind, "source", sourceID, "step", s)
	}
	start := time.Now()
	go func() {
		err := fn(progress)
		m.mu.Lock()
		j := m.jobs[key]
		if j != nil {
			j.Done = true
			if err != nil {
				j.Status = "error"
				j.Err = err.Error()
			} else {
				j.Status = "done"
			}
		}
		m.mu.Unlock()
		if err != nil {
			m.log.Error("job.error", "kind", kind, "source", sourceID, "err", err)
		} else {
			m.log.Info("job.done", "kind", kind, "source", sourceID, "dur", time.Since(start))
		}
	}()
	return true
}

// Get returns a copy of the job for key.
func (m *JobManager) Get(key string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[key]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// Snapshot returns copies of all known jobs.
func (m *JobManager) Snapshot() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, *j)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/web/ -run Job` then `go test -race ./internal/web/ -run Job`
Expected: PASS (including under `-race`).

- [ ] **Step 5: Commit**

```bash
git add internal/web/jobs.go internal/web/jobs_test.go
git commit -m "feat: in-memory JobManager for background operations"
```

---

### Task 2: Request-logging middleware

**Files:**
- Create: `internal/web/logging.go`
- Test: `internal/web/logging_test.go`

**Interfaces:**
- Consumes: `log/slog`, `net/http`.
- Produces:
  - `web.logRequests(log *slog.Logger, next http.Handler) http.Handler` — logs one `request` line (method, path, status, dur) per request.
  - `web.statusRecorder` wrapping `http.ResponseWriter` to capture the status code (default 200).

- [ ] **Step 1: Write the failing test**

Create `internal/web/logging_test.go`:
```go
package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogRequestsEmitsLine(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	h := logRequests(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	out := buf.String()
	if !strings.Contains(out, "request") || !strings.Contains(out, "method=GET") ||
		!strings.Contains(out, "path=/x") || !strings.Contains(out, "status=418") {
		t.Fatalf("log line missing fields: %q", out)
	}
}

func TestStatusRecorderDefaults200(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: 200}
	sr.Write([]byte("hi")) // no WriteHeader -> stays 200
	if sr.status != 200 {
		t.Fatalf("status = %d, want 200", sr.status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web/ -run 'LogRequests|StatusRecorder'`
Expected: FAIL — `undefined: logRequests`, `undefined: statusRecorder`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/web/logging.go`:
```go
package web

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder captures the response status code (default 200).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests logs one line per request: method, path, status, duration.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur", time.Since(start))
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/web/ -run 'LogRequests|StatusRecorder'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/web/logging.go internal/web/logging_test.go
git commit -m "feat: request-logging middleware"
```

---

### Task 3: Wire logger + JobManager into the Server, mount logging + /jobs

**Files:**
- Modify: `internal/web/server.go` (Server fields, NewServer signature, Handler wrap, `/jobs`)
- Modify: `internal/web/server_test.go` (update `testServer`)
- Modify: `internal/cli/cli.go` (`cmdServe` builds the logger)
- Test: `internal/web/jobs_endpoint_test.go`

**Interfaces:**
- Consumes: `JobManager` (Task 1), `logRequests` (Task 2).
- Produces:
  - `Server` gains `log *slog.Logger` and `jobs *JobManager`.
  - `web.NewServer(st store.Store, inv agent.Invoker, newID func() string, runner build.Runner, buildsDir string, log *slog.Logger) (*Server, error)` — signature extended with `log`; creates `jobs = NewJobManager(log)`.
  - `Handler()` wraps the mux with `logRequests(s.log, mux)` and registers `GET /jobs` → `writeJSON(s.jobs.Snapshot())`.

- [ ] **Step 1: Update testServer to the new signature**

In `internal/web/server_test.go`, update `testServer` to pass a discard logger:
```go
	srv, err := NewServer(st, &agent.Fake{}, func() string { n++; return "id" + string(rune('0'+n)) },
		&build.FakeRunner{Result: build.Result{Passed: true}},
		t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
```
Add imports `"io"` and `"log/slog"` to `server_test.go` if missing.

- [ ] **Step 2: Write the failing test**

Create `internal/web/jobs_endpoint_test.go`:
```go
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJobsEndpointEmpty(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/jobs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var jobs []Job
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("body not JSON array: %v (%s)", err, rec.Body.String())
	}
	if len(jobs) != 0 {
		t.Fatalf("want empty, got %+v", jobs)
	}
}

func TestJobsEndpointReportsRegistered(t *testing.T) {
	srv, _ := testServer(t)
	srv.jobs.Start("prepare:x", "prepare", "x", "", func(progress func(string)) error { return nil })
	// Let it finish.
	waitJob(t, srv.jobs, "prepare:x")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/jobs", nil))
	var jobs []Job
	json.Unmarshal(rec.Body.Bytes(), &jobs)
	if len(jobs) != 1 || jobs[0].Key != "prepare:x" || jobs[0].Status != "done" {
		t.Fatalf("jobs = %+v", jobs)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/web/ -run JobsEndpoint`
Expected: FAIL — `NewServer` arity / `srv.jobs` undefined.

- [ ] **Step 4: Extend the Server**

In `internal/web/server.go`: add `"log/slog"` to imports. Extend struct + constructor + Handler:
```go
type Server struct {
	store     store.Store
	inv       agent.Invoker
	newID     func() string
	runner    build.Runner
	buildsDir string
	log       *slog.Logger
	jobs      *JobManager
	tmpl      *template.Template
}

func NewServer(st store.Store, inv agent.Invoker, newID func() string, runner build.Runner, buildsDir string, log *slog.Logger) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{store: st, inv: inv, newID: newID, runner: runner, buildsDir: buildsDir,
		log: log, jobs: NewJobManager(log), tmpl: tmpl}, nil
}
```
In `Handler()`, register the jobs route and wrap the mux. At the end of `Handler()`, change `return mux` to:
```go
	mux.HandleFunc("GET /jobs", s.handleJobs)
	return logRequests(s.log, mux)
```
Add the handler:
```go
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.jobs.Snapshot())
}
```
(`writeJSON` already exists from the build endpoints.)

- [ ] **Step 5: Build the logger in cmdServe**

In `internal/cli/cli.go` `cmdServe`, add `"log/slog"` import. Before `web.NewServer`, create the logger and pass it:
```go
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv, err := web.NewServer(d.store, d.invoker, d.newID, build.NewExecRunner(), filepath.Join(dataDir, "builds"), logger)
```
(Use the command's `stderr` writer so logs go to standard error.)

- [ ] **Step 6: Run tests + full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages (the updated `testServer` keeps existing web tests compiling).

- [ ] **Step 7: Commit**

```bash
git add internal/web/ internal/cli/cli.go
git commit -m "feat: wire slog logger + JobManager into server; /jobs + request logging"
```

---

### Task 4: Async prepare + preparing page

**Files:**
- Modify: `internal/web/server.go` (`handlePrepare`, `handleStudyEntry`)
- Create: `internal/web/templates/preparing.html`
- Test: `internal/web/preparing_test.go`

**Interfaces:**
- Consumes: `JobManager`, `study.PrepareSource`, `store.IsPrepared`/`GetSource`.
- Produces: `handlePrepare` starts a background job (`prepare:<id>`) and 303s to `/study/{id}` immediately; `handleStudyEntry` renders `preparing.html` when that job is running and the source isn't prepared yet.

- [ ] **Step 1: Write the failing test**

Create `internal/web/preparing_test.go`:
```go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrepareStartsJobAndShowsPreparing(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = studyFake() // from prepare_test.go; answers "study package"
	addSource(t, st, "src1", 1) // from prepare_test.go helper

	// POST prepare returns fast (303) and registers a job.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/prepare", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303; %s", rec.Code, rec.Body.String())
	}
	// The job exists and finishes.
	waitJob(t, srv.jobs, "prepare:src1")
}

func TestStudyEntryShowsPreparingWhileRunning(t *testing.T) {
	srv, st := testServer(t)
	addSource(t, st, "src1", 1)
	// Register a still-running prepare job by hand (no real work).
	release := make(chan struct{})
	srv.jobs.Start("prepare:src1", "prepare", "src1", "", func(progress func(string)) error {
		progress("section 1/1")
		<-release
		return nil
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Preparing") {
		t.Fatalf("expected preparing page, got %q", rec.Body.String())
	}
	close(release)
	waitJob(t, srv.jobs, "prepare:src1")
}
```
Note: `addSource`, `studyFake` are existing helpers in `prepare_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web/ -run 'PrepareStartsJob|StudyEntryShowsPreparing'`
Expected: FAIL — preparing page text absent / prepare still synchronous.

- [ ] **Step 3: Make prepare async**

In `internal/web/server.go`, replace the body of `handlePrepare` (after the `GetSource` 404 check) with a background start:
```go
func (s *Server) handlePrepare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.jobs.Start("prepare:"+id, "prepare", id, "", func(progress func(string)) error {
		return study.PrepareSource(context.Background(), s.inv, s.store, src, s.newID,
			func(i, total int, title string) { progress(fmt.Sprintf("section %d/%d", i+1, total)) })
	})
	http.Redirect(w, r, "/study/"+id, http.StatusSeeOther)
}
```
Note: the job uses `context.Background()`, not `ctx` (the request context is cancelled once this handler returns).

In `handleStudyEntry`, add a preparing branch: after computing that the source is NOT prepared, check for a running prepare job before showing the Prepare page:
```go
	if !prepared {
		if j, ok := s.jobs.Get("prepare:" + id); ok && j.Status == "running" {
			s.render(w, "preparing.html", map[string]any{"Title": src.Title, "SourceID": id, "Progress": j.Progress})
			return
		}
		s.render(w, "prepare.html", map[string]any{"Title": src.Title, "Source": src})
		return
	}
```
(Keep the existing prepared → redirect-to-current-section logic.)

- [ ] **Step 4: Write the preparing template**

Create `internal/web/templates/preparing.html`:
```html
{{define "content"}}
<div class="window" style="max-width:520px">
  <div class="title-bar"><div class="title-bar-text">Preparing - {{.Title}}</div></div>
  <div class="window-body">
    <p>Preparing your study package. You can leave this page — it keeps working,
       and a notification appears when it's ready.</p>
    <p><span class="kaomoji" data-kaomoji="1">...</span> <span id="progress">{{.Progress}}</span></p>
  </div>
</div>
<script>
  window.__pollJob = {key: "prepare:{{.SourceID}}", redirect: "/study/{{.SourceID}}"};
</script>
{{end}}
```
(The `kaomoji` span is animated by `kaomoji.js`; `__pollJob` is read by `jobs.js` to poll and redirect when done — both added in Task 7.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/web/
git commit -m "feat: async prepare with preparing page"
```

---

### Task 5: Async build + building page

**Files:**
- Modify: `internal/web/server.go` (`handleBuildStart`, `handleBuildView`)
- Create: `internal/web/templates/building.html`
- Test: `internal/web/building_test.go`

**Interfaces:**
- Consumes: `JobManager`, `build.StartBuild`, `store.GetBuild`/`GetSource`.
- Produces: `handleBuildStart` starts a background job (`build:<id>:<lang>`) and 303s to `/build/{id}/{lang}` immediately; `handleBuildView` renders `building.html` when that job is running and no build exists yet.

- [ ] **Step 1: Write the failing test**

Create `internal/web/building_test.go`:
```go
package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBuildStartIsAsyncAndShowsBuilding(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = buildFakeAgent() // from build_test.go
	addBuildSource(t, st, "src1")

	form := url.Values{"language": {"go"}, "difficulty": {"spec+tests"}}
	req := httptest.NewRequest("POST", "/build/src1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("start status = %d, want 303", rec.Code)
	}
	waitJob(t, srv.jobs, "build:src1:go")
}

func TestBuildViewShowsBuildingWhileRunning(t *testing.T) {
	srv, st := testServer(t)
	addBuildSource(t, st, "src1")
	release := make(chan struct{})
	srv.jobs.Start("build:src1:go", "build", "src1", "go", func(progress func(string)) error {
		<-release
		return nil
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/build/src1/go", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Building") {
		t.Fatalf("expected building page, got %q", rec.Body.String())
	}
	close(release)
	waitJob(t, srv.jobs, "build:src1:go")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web/ -run 'BuildStartIsAsync|BuildViewShowsBuilding'`
Expected: FAIL — building page absent / start still synchronous.

- [ ] **Step 3: Make build start async + building branch**

In `internal/web/server.go` `handleBuildStart`, replace the `build.StartBuild` call + redirect (the non-resume path) with a background start:
```go
	s.jobs.Start("build:"+id+":"+lang, "build", id, lang, func(progress func(string)) error {
		progress("generating build plan")
		_, err := build.StartBuild(context.Background(), s.inv, s.store, src, lang, diff, s.newID, s.buildsDir)
		return err
	})
	http.Redirect(w, r, "/build/"+id+"/"+lang, http.StatusSeeOther)
```
(Keep the earlier resume-if-exists and invalid-lang/diff branches unchanged. Use `context.Background()` in the job.)

In `handleBuildView`, when `GetBuild` returns `ErrNotFound`, show the building page if a build job is running before 404ing:
```go
	b, err := s.store.GetBuild(ctx, id, lang)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			if j, ok := s.jobs.Get("build:" + id + ":" + lang); ok && j.Status == "running" {
				s.render(w, "building.html", map[string]any{"Title": id, "SourceID": id, "Lang": lang, "Progress": j.Progress})
				return
			}
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
```

- [ ] **Step 4: Write the building template**

Create `internal/web/templates/building.html`:
```html
{{define "content"}}
<div class="window" style="max-width:520px">
  <div class="title-bar"><div class="title-bar-text">Building - {{.Lang}}</div></div>
  <div class="window-body">
    <p>Generating your build workspace. You can leave this page — it keeps working,
       and a notification appears when it's ready.</p>
    <p><span class="kaomoji" data-kaomoji="1">...</span> <span id="progress">{{.Progress}}</span></p>
  </div>
</div>
<script>
  window.__pollJob = {key: "build:{{.SourceID}}:{{.Lang}}", redirect: "/build/{{.SourceID}}/{{.Lang}}"};
</script>
{{end}}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/web/
git commit -m "feat: async build with building page"
```

---

### Task 6: CLI animated kaomoji set + rotation

**Files:**
- Modify: `internal/ui/progress.go`
- Test: `internal/ui/progress_test.go`

**Interfaces:**
- Consumes: `math/rand`.
- Produces:
  - `ui.kaomojiSet [][]string` — curated set; each entry is a list of animation frames.
  - `ui.nextKaomoji(cur, n int, r *rand.Rand) int` — returns an index in `[0,n)` different from `cur` when `n > 1` (returns `0` when `n <= 1`).
  - The `Spinner` (TTY path) animates the current kaomoji's frames and rotates to `nextKaomoji` every ~3s. Non-TTY behavior unchanged.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/progress_test.go`:
```go
import "math/rand" // add to the existing import block

func TestKaomojiSetNonEmpty(t *testing.T) {
	if len(kaomojiSet) == 0 {
		t.Fatal("kaomojiSet must not be empty")
	}
	for i, k := range kaomojiSet {
		if len(k) == 0 {
			t.Fatalf("kaomojiSet[%d] has no frames", i)
		}
		for j, f := range k {
			if f == "" {
				t.Fatalf("kaomojiSet[%d][%d] is empty", i, j)
			}
		}
	}
}

func TestNextKaomoji(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	// With >1 entries, never returns the current index.
	for i := 0; i < 100; i++ {
		got := nextKaomoji(2, 5, r)
		if got == 2 || got < 0 || got >= 5 {
			t.Fatalf("nextKaomoji returned %d", got)
		}
	}
	// With a single entry, returns 0.
	if got := nextKaomoji(0, 1, r); got != 0 {
		t.Fatalf("nextKaomoji(0,1) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'Kaomoji'`
Expected: FAIL — `undefined: kaomojiSet`, `undefined: nextKaomoji`.

- [ ] **Step 3: Add the set + helper and use them in Spinner**

In `internal/ui/progress.go`, replace the single `workFrames` var with the curated set and a helper, and update the animation loop. First add (near the top, replacing `workFrames`):
```go
// kaomojiSet is a curated set of animated kaomoji; each is a list of frames.
var kaomojiSet = [][]string{
	{"┌(･o･)┘", "└(･o･)┐", "┌(･o･)┐", "└(･o･)┘"},
	{"(・_・)", "(・_・ )", "( ・_・)", "(・_・)"},
	{"┐(･ω･)┌", "┌(･ω･)┐"},
	{"(>_<)", "(>ω<)", "(>﹏<)"},
	{"(๑•̀ㅂ•́)و", "(๑•̀ㅂ•́)൬"},
}

// nextKaomoji returns an index in [0,n) different from cur (when n>1).
func nextKaomoji(cur, n int, r *rand.Rand) int {
	if n <= 1 {
		return 0
	}
	for {
		k := r.Intn(n)
		if k != cur {
			return k
		}
	}
}
```
Add imports `"math/rand"` and `"time"` (time is already imported). Then update the spinner's animation goroutine in `Start` to rotate kaomoji. Replace the existing ticker loop body so it tracks the current kaomoji and frame:
```go
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		cur := r.Intn(len(kaomojiSet))
		frame := 0
		lastRotate := time.Now()
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				if time.Since(lastRotate) > 3*time.Second {
					cur = nextKaomoji(cur, len(kaomojiSet), r)
					frame = 0
					lastRotate = time.Now()
				}
				frames := kaomojiSet[cur]
				fmt.Fprint(s.w, frameLine(frames[frame%len(frames)], s.msg, time.Since(s.start)))
				frame++
			}
		}
	}()
```
Remove the now-unused `workFrames` references. Keep `frameLine`, `doneFace`, `failFace`, `tick`, and the non-TTY path unchanged. If `TestWorkFramesNonEmpty` exists in the test file and references `workFrames`, delete that test (superseded by `TestKaomojiSetNonEmpty`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/
git commit -m "feat: animated random-cycling kaomoji in CLI spinner"
```

---

### Task 7: Web kaomoji.js + jobs.js toast + button feedback + smoke

**Files:**
- Create: `internal/web/assets/kaomoji.js`
- Create: `internal/web/assets/jobs.js`
- Modify: `internal/web/templates/base.html` (load both, before app.js/build.js)
- Modify: `internal/web/templates/prepare.html`, `internal/web/templates/build_picker.html` (animated kaomoji on submit)
- Test: `internal/web/assets_test.go`

**Interfaces:**
- Consumes: the `GET /jobs` endpoint (Task 3); the `__pollJob` global set by preparing/building pages (Tasks 4-5); `.kaomoji` spans.
- Produces: `kaomoji.js` (animates any `.kaomoji` element + a `startKaomoji(el)` helper) and `jobs.js` (global poller → toast; drives `__pollJob` redirect). Tests assert both assets are served and `base.html` loads them.

- [ ] **Step 1: Write the failing test**

Create `internal/web/assets_test.go`:
```go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKaomojiAndJobsAssetsServed(t *testing.T) {
	srv, _ := testServer(t)
	for _, path := range []string{"/static/kaomoji.js", "/static/jobs.js"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("%s not served: status=%d len=%d", path, rec.Code, rec.Body.Len())
		}
	}
}

func TestBaseLoadsKaomojiAndJobs(t *testing.T) {
	srv, st := testServer(t)
	addSource(t, st, "src1", 1) // a page that renders base via a sub-page
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "/static/kaomoji.js") || !strings.Contains(body, "/static/jobs.js") {
		t.Fatalf("base.html does not load kaomoji.js + jobs.js: %q", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web/ -run 'AssetsServed|BaseLoads'`
Expected: FAIL — assets 404 / base.html missing script tags.

- [ ] **Step 3: Write kaomoji.js**

Create `internal/web/assets/kaomoji.js`:
```javascript
// Animated random-cycling kaomoji. Each kaomoji is a list of frames; the
// current kaomoji's frames advance quickly, and a different random kaomoji is
// chosen every few seconds.
const KAOMOJI = [
  ["┌(・o・)┘", "└(・o・)┐", "┌(・o・)┐", "└(・o・)┘"],
  ["(・_・)", "(・_・ )", "( ・_・)", "(・_・)"],
  ["┐(・ω・)┌", "┌(・ω・)┐"],
  ["(>_<)", "(>ω<)", "(>＿<)"],
];

function startKaomoji(el) {
  let cur = Math.floor(Math.random() * KAOMOJI.length);
  let frame = 0;
  let last = Date.now();
  function tick() {
    if (Date.now() - last > 3000) {
      let k;
      do { k = Math.floor(Math.random() * KAOMOJI.length); } while (KAOMOJI.length > 1 && k === cur);
      cur = k; frame = 0; last = Date.now();
    }
    const frames = KAOMOJI[cur];
    el.textContent = frames[frame % frames.length];
    frame++;
  }
  tick();
  return setInterval(tick, 180);
}

document.addEventListener("DOMContentLoaded", function () {
  document.querySelectorAll(".kaomoji").forEach(startKaomoji);
});
window.startKaomoji = startKaomoji;
```

- [ ] **Step 4: Write jobs.js**

Create `internal/web/assets/jobs.js`:
```javascript
// Global job poller: shows a toast when a job completes, and drives the
// in-progress pages' redirect via window.__pollJob.
function showToast(msg) {
  let host = document.getElementById("toast-host");
  if (!host) {
    host = document.createElement("div");
    host.id = "toast-host";
    host.style.cssText = "position:fixed;bottom:12px;right:12px;z-index:1000;";
    document.body.appendChild(host);
  }
  const t = document.createElement("div");
  t.className = "window";
  t.style.cssText = "margin-top:8px;max-width:260px;";
  t.innerHTML = '<div class="title-bar"><div class="title-bar-text">Tanaka</div></div>' +
    '<div class="window-body" style="margin:6px"></div>';
  t.querySelector(".window-body").textContent = msg;
  host.appendChild(t);
  setTimeout(function () { t.remove(); }, 8000);
}

function announced() {
  try { return new Set(JSON.parse(localStorage.getItem("tanakaAnnounced") || "[]")); }
  catch (e) { return new Set(); }
}
function remember(set) {
  localStorage.setItem("tanakaAnnounced", JSON.stringify(Array.from(set)));
}

async function pollJobs() {
  let jobs;
  try {
    const res = await fetch("/jobs");
    if (!res.ok) return;
    jobs = await res.json();
  } catch (e) { return; }

  const seen = announced();
  for (const j of jobs) {
    if (!j.done) continue;
    const stamp = j.key + ":" + j.status;
    if (!seen.has(stamp)) {
      seen.add(stamp);
      const what = j.kind === "build" ? "build" : "study package";
      showToast(j.status === "error" ? (j.kind + " failed: " + j.error) : (what + " ready"));
    }
  }
  remember(seen);

  // In-progress page redirect.
  if (window.__pollJob) {
    const me = jobs.find(function (x) { return x.key === window.__pollJob.key; });
    if (me) {
      const p = document.getElementById("progress");
      if (p && me.progress) p.textContent = me.progress;
      if (me.done && me.status === "done") { location.href = window.__pollJob.redirect; }
    }
  }
}

setInterval(pollJobs, 2000);
document.addEventListener("DOMContentLoaded", pollJobs);
```

- [ ] **Step 5: Load both in base.html + animate submit buttons**

In `internal/web/templates/base.html`, add the two scripts before `app.js`/`build.js`:
```html
<script src="/static/kaomoji.js"></script>
<script src="/static/jobs.js"></script>
<script src="/static/app.js"></script>
<script src="/static/build.js"></script>
```
In `internal/web/templates/prepare.html`, change the form's submit handling so the button shows an animated kaomoji:
```html
    <form method="POST" action="/study/{{.Source.ID}}/prepare" onsubmit="var b=this.querySelector('button');b.disabled=true;b.textContent='';b.classList.add('kaomoji');window.startKaomoji(b);">
      <button type="submit">Prepare this source</button>
    </form>
```
In `internal/web/templates/build_picker.html`, change the Start button similarly:
```html
      <div class="field-row"><button type="submit" onclick="var b=this;setTimeout(function(){b.disabled=true;b.textContent='';b.classList.add('kaomoji');window.startKaomoji(b);},0);">Start</button></div>
```
(The `setTimeout(...,0)` lets the form submit before the button is disabled.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 7: Run the full suite and vet**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 8: Commit**

```bash
git add internal/web/
git commit -m "feat: kaomoji.js + jobs.js toast + animated submit buttons"
```

- [ ] **Step 9: Manual smoke (real browser)**

Run:
```bash
go build -o /tmp/tanaka . && /tmp/tanaka serve --port 7777
```
Open `http://127.0.0.1:7777`, click `build` on a source, pick a language/difficulty, hit Start — the button should turn into an animated cycling kaomoji, the page shows "Building…" with a kaomoji and live progress, and you can navigate home while it runs. When it finishes a toast appears (bottom-right). Same for a source's Prepare. Check `serve`'s stderr shows `request`/`job.*` log lines. (Spends a little Claude usage.)

---

## Self-Review

**Spec coverage:**
- slog logging (requests + jobs + errors) — Tasks 1 (job logs), 2 (request middleware), 3 (wiring + cmdServe). ✓
- Background JobManager (in-memory, dedup, progress, done/error) — Task 1. ✓
- Async prepare + build (context.Background, 303 immediately) — Tasks 4, 5. ✓
- In-progress pages (preparing/building) that poll + redirect — Tasks 4, 5, 7. ✓
- `GET /jobs` status endpoint — Task 3. ✓
- Global toast (localStorage dedup) — Task 7. ✓
- Button feedback as animated kaomoji — Task 7. ✓
- Animated kaomoji set + rotation: web `kaomoji.js` (Task 7) and CLI `Spinner` (Task 6). ✓
- Testing: JobManager (+race), middleware, async handlers, /jobs, assets, ui kaomoji set/helper — Tasks 1-7. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. ✓

**Type consistency:** `NewServer(st, inv, newID, runner, buildsDir, log)` updated in Task 3 and used by testServer + cmdServe. `JobManager.Start/Get/Snapshot` + `Job` fields consistent across Tasks 1, 3, 4, 5. `study.PrepareSource`/`build.StartBuild` signatures match their goroutine call sites. `writeJSON` reused. `__pollJob`/`.kaomoji`/`#progress` contracts consistent between templates (4,5) and jobs.js/kaomoji.js (7). `nextKaomoji`/`kaomojiSet` consistent in Task 6. ✓

## Notes / Future

- In-memory jobs are lost on `serve` restart (acceptable; prepare resumes, build persists only on success).
- Polling (2s) not SSE; no cancellation; no `--log-level` — all deferred per spec §12.
