# Tanaka — Feedback, Background Jobs & Logging Design Spec

**Date:** 2026-06-26
**Status:** Approved (design); implementation plan pending
**Builds on:** the completed Tanaka app (ingest → study → build). Touches `internal/web` and `internal/cli` (serve).

## 1. Summary

Three improvements to the web server:
1. **Server-side logging** — structured `slog` (text → stderr): HTTP request lines, job lifecycle, and errors.
2. **Background jobs** — the long operations (`prepare` and `build` start) run in a background goroutine instead of blocking the HTTP handler, so the UI returns immediately and you can navigate away / do other things while they run.
3. **Feedback** — buttons give immediate feedback; in-progress pages poll and auto-advance; a global toast notifies you when a job finishes, on whatever page you're on.

## 2. Goals / Non-Goals

**Goals**
- `prepare` and `build` no longer block the request; you can browse other sources while they run.
- Immediate button feedback + live progress ("section 3/9", "building workspace…").
- A toast when a job completes/fails, visible from any page.
- Request + job + error logging to stderr.

**Non-Goals**
- Persisting job state across `serve` restarts (in-memory only; see Data/State).
- A job queue / concurrency limits / cancellation (jobs just run; duplicates per key are ignored).
- A `--log-level` flag (fixed at info).
- SSE/websockets (polling is sufficient).

## 3. Key Decisions

| Decision | Choice |
|---|---|
| Async model | Background goroutine + client polling of a `/jobs` status endpoint |
| Job state | In-memory `JobManager` on the server (lost on restart) |
| Completion UX | Global toast (polls `/jobs` on every page) + in-progress pages that auto-redirect |
| Logging | `log/slog` text handler → stderr; request middleware + operation/error logs; level info |
| Dedup | One job per key (`prepare:<id>`, `build:<id>:<lang>`); a start while running is a no-op |
| Loading indicator | Animated kaomoji — each cycles through its own frames; a *different random* kaomoji is chosen every few seconds. Shared idea across the CLI spinner and all web loading states (buttons, in-progress pages). |

## 4. Components

- `internal/web/jobs.go` — `JobManager` (in-memory, mutex-guarded) + `Job` type.
- `internal/web/logging.go` — request-logging middleware + a `statusRecorder`.
- `internal/web/server.go` — `Server` gains `*slog.Logger` and `*JobManager`; `prepare`/`build` handlers become async; new `GET /jobs`; in-progress branches in the study/build entry handlers.
- `internal/web/templates/` — `preparing.html`, `building.html` in-progress pages; toast markup is created by JS.
- `internal/web/assets/jobs.js` — global poller + toast; loaded in `base.html`.
- `internal/web/assets/kaomoji.js` — shared animated-kaomoji helper (the curated set + animate-frames + rotate-every-few-seconds); used by button loading text, the in-progress pages, and `jobs.js`. Loaded in `base.html` before the others.
- `internal/ui/progress.go` — enhance the existing CLI `Spinner` (used by `tanaka add`) to use the same curated kaomoji set: animate the current kaomoji's frames and switch to a different random kaomoji every few seconds.
- `internal/cli/cli.go` (`cmdServe`) — construct the `slog.Logger` and pass it to `NewServer`.

## 5. Logging

- `serve` builds `slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))` and passes it to `web.NewServer`. Tests pass a logger over `io.Discard`.
- **Request middleware** wraps the mux: logs `request method=… path=… status=… dur=…` using a `statusRecorder` that captures the written status code (default 200).
- **Operation logs** (where they happen): `job.start`/`job.progress`/`job.done`/`job.error` with `kind`, `source`, `lang`, and `step`/`dur`/`err`; agent-call failures logged at error.

## 6. JobManager

```go
type Job struct {
    Key      string
    Kind     string // "prepare" | "build"
    SourceID string
    Lang     string // build only
    Status   string // "running" | "done" | "error"
    Progress string
    Err      string
    Done     bool
}
type JobManager struct { /* mu sync.Mutex; jobs map[string]*Job; log *slog.Logger */ }
```
- `Start(key, kind, sourceID, lang string, fn func(progress func(string)) error) (started bool)` — if a job for `key` is already `running`, returns `false` (no-op); else registers a `running` job and launches a goroutine that runs `fn`, passing a `progress` callback that updates `Job.Progress`; on return sets `Status` `done`/`error` (+`Err`) and `Done=true`. Logs lifecycle.
- `Get(key) (*Job, bool)` and `Snapshot() []Job` (cop(ies), for the status endpoint) — all mutex-guarded.

## 7. Routes & Flow

| Method/Path | Behavior |
|---|---|
| `POST /study/{id}/prepare` | `Start("prepare:"+id, …)` running `study.PrepareSource` (its `onSection` → `progress("section i/N")`); **303** to `/study/{id}` immediately |
| `GET /study/{id}` | prepared → 303 to current section; else `Get("prepare:"+id)` running → render `preparing.html` (polls); else → Prepare page |
| `POST /build/{id}` | existing build → 303 to view; else `Start("build:"+id+":"+lang, …)` running `build.StartBuild`; **303** to `/build/{id}/{lang}` |
| `GET /build/{id}/{lang}` | build exists → build view; else build job running → render `building.html` (polls); else 404 |
| `GET /jobs` | JSON array from `Snapshot()` (key, kind, sourceID, lang, status, progress, done, error) |

## 8. UI Feedback, Polling & Toast

- **Buttons:** Prepare and Start forms disable the button on submit and replace its label with an **animated kaomoji** (per §8.1) instead of static "starting…".
- **In-progress pages** (`preparing.html`, `building.html`): show the current `Progress` next to an **animated kaomoji**, and poll `GET /jobs` (~2s); when the relevant job is `done` they reload/redirect to the ready page; on `error` they show the message + a retry link.
- **Global toast** (`jobs.js`, loaded by `base.html` on every page): polls `GET /jobs` (~2s); when a job it hasn't announced flips to `done`/`error`, render a small retro toast (98.css `window`, fixed bottom-right) — "<title>: study package ready" / "build ready" / "prepare failed". Announced job keys are remembered in `localStorage` so reloads don't re-toast. (Toast shows the source title when available; falls back to the source id.)

### 8.1 Animated kaomoji loading indicator

A small curated set of kaomoji, each defined as an ordered list of frames that read as motion, e.g.:
- `┌(･o･)┘ └(･o･)┐ ┌(･o･)┐ └(･o･)┘` (dancing)
- `(・_・) (・_・ ) ( ・_・) (・_・)` (looking around)
- `┐(･ω･)┌ ┌(･ω･)┐` (shrug)
- `(>_<) (>ω<) (>﹏<)` (effort)

Behavior anywhere a "working" state is shown: animate the **current** kaomoji by advancing its frames quickly (~150–200ms), and every few seconds (~3s) switch to a **different random** kaomoji from the set. Two implementations of the same idea:
- **Web** (`kaomoji.js`): exposes a helper that, given a target DOM element, starts this animation and returns a stop handle. Button loading and the in-progress pages use it.
- **CLI** (`internal/ui/progress.go`): the existing `Spinner` already animates one fixed kaomoji's frames; extend it to hold the curated set, animate the current kaomoji's frames, and rotate to a random different kaomoji every ~3s. Non-TTY output is unchanged (plain phase lines, no animation). Randomness uses `math/rand` (seeded per spinner); frame/rotation selection is factored into a small pure helper so it's testable.

## 9. Data / State

In-memory only. A `serve` restart drops all job state. This is acceptable: `study.PrepareSource` is resumable (skips already-studied sections), and `build.StartBuild` persists only after it fully succeeds (a dropped build simply was never created — re-click to start again). No schema changes.

## 10. Error Handling

- Job failure → `Status=error`, `Err` set, logged at error; surfaced in `/jobs`; shown as an error toast and on the in-progress page with a retry link (re-POST the start).
- Duplicate start while running → ignored (handler still 303s to the in-progress page).
- `/jobs` is read-only JSON; malformed/absent jobs simply don't appear.
- Request middleware never swallows handler panics differently than today (out of scope to add panic recovery).

## 11. Testing

Agent + runner stay faked; logger over `io.Discard`.
- **JobManager:** start runs fn and reaches `done`; error path sets `error`+`Err`; duplicate start while running returns `false`; progress callback updates `Progress`; race-clean under `-race`.
- **Async handlers:** `POST prepare`/`POST build` return 303 quickly and register a job; the job reaches `done` (poll `Get` with a short deadline, fake agent so fast); `GET /study/{id}` shows the preparing page while running and redirects once prepared; `GET /build/{id}/{lang}` shows building page while running.
- **`GET /jobs`:** returns the registered jobs as JSON with expected fields.
- **Request logging middleware:** capture slog output (text handler over a buffer), assert a `request …status=… dur=…` line.
- **Templates:** `preparing.html`/`building.html` render.
- **CLI kaomoji (`internal/ui`):** the curated kaomoji set is non-empty and every kaomoji has ≥1 non-empty frame; the pure frame/rotation-selection helper behaves (advances frames; rotation picks an in-range, different index); non-TTY `Spinner` output is unchanged (still plain phase lines). `kaomoji.js` is client JS and is exercised manually, not unit-tested in Go.

## 12. Open Questions / Future

- Persist jobs (survive restart) and show a jobs panel.
- Cancellation of a running prepare/build.
- SSE/streamed progress instead of polling.
- `--log-level` flag.
