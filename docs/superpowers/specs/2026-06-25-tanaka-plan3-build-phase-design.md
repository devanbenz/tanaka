# Tanaka Plan 3 — Build Phase Design Spec

**Date:** 2026-06-25
**Status:** Approved (design); implementation plan pending
**Builds on:** Plan 1 (ingest), Plan 2a (study domain), Plan 2b (study web UI / `serve`).

## 1. Summary

Plan 3 adds the **build phase** to the web UI: after studying a source, you implement its ideas in a language of your choice, guided by an ordered, pass-gated set of steps with generated acceptance tests. You pick a **language** (Rust, Go, C++, C, Python) and a **difficulty** (guided / spec+tests / blank-page). Tanaka scaffolds a workspace on disk; you write code in your own editor; the server runs the language's test command and gates progression on passing. Hints are available per step on request. It is deliberately not hand-holdy — the difficulty dial controls how much scaffolding you get.

## 2. Goals / Non-Goals

**Goals**
- Web-UI build flow (extends `serve`): pick language + difficulty → generated ordered steps → code → run tests → advance.
- Tanaka-scaffolded workspace under `~/.tanaka/builds/`; the server runs tests there.
- Test execution is server-side, no LLM (a plain subprocess per language).
- Difficulty dial scales scaffolding; hints per step on request.
- Runs on the Claude subscription (agent only for `gen-build` and `gen-hint`).

**Non-Goals (Plan 3)**
- Installing or detecting language toolchains beyond running the command and reporting a clear error if it is missing.
- Auto-grading code quality/style beyond the generated acceptance tests passing.
- In-browser code editing (you use your own editor).
- Sandboxing/containerizing test execution (v1 runs tests locally; single-user, local tool).

## 3. Key Decisions

| Decision | Choice |
|---|---|
| Build UX | Web-UI-driven (extends `serve`) |
| Workspace | Tanaka-scaffolded `~/.tanaka/builds/<sourceID>-<lang>/` |
| Test execution | Server-side subprocess, per-language command, no LLM |
| Structure | Ordered steps, pass-gated, skippable (reuses study gating) |
| File delivery | `gen-build` returns structured JSON; the server writes files |
| Difficulty | guided / spec+tests / blank-page (default spec+tests) |
| Languages | Rust, Go, C++, C, Python |

## 4. Architecture

```
  Browser ──HTTP──> internal/web (build routes, picker + build view templates)
                      ├── internal/store  (builds, build_steps tables)
                      ├── internal/build  (gen-build, gen-hint over agent.Invoker; TestRunner)
                      │      │ gen-build/gen-hint headless (content via stdin)
                      │      ▼   claude -p
                      └── TestRunner.Run(workspace, lang)  ── exec ──> cargo/go/pytest/make
```

The server owns state, file scaffolding, and test execution. The agent is used only to generate the build plan and hints. Test running is a plain subprocess behind an injectable `TestRunner` interface.

## 5. Package Layout

- `internal/build` — `gen-build` (plan generation), `gen-hint` (per-step hints) over `agent.Invoker`; the `TestRunner` interface + real (exec) and fake impls; per-language command map; gating reuse.
- `internal/store` — `builds` + `build_steps` tables and methods.
- `internal/web` — build routes, handlers, picker + build-view templates, JS for run-tests/hint.
- Reuses `study.ComputeUnlocked` for step gating.

## 6. Routes

| Method/Path | Purpose |
|---|---|
| `GET /build/{id}` | Build exists → redirect to its view; else language + difficulty picker |
| `POST /build/{id}` | `{language, difficulty}` → `gen-build` → store build+steps, scaffold workspace + step 1, redirect to view |
| `GET /build/{id}/{lang}` | Build view |
| `POST /build/{id}/{lang}/test` | Run tests → JSON `{passed, output, runError}`; on pass → mark step passed, write next step's files, unlock next |
| `POST /build/{id}/{lang}/hint` | `gen-hint` → JSON `{hint}` |
| `POST /build/{id}/{lang}/skip` | Skip step, write next files, unlock |

## 7. Screens (retro 98.css)

- **Picker** window: language radios, difficulty radios, Start button.
- **Build view**: left = step nav (locked / ▶ current / ✔ passed); right = current step goal, the **workspace path** ("open this folder in your editor"), a **Run tests** button + output pane, a **Hint** button + hint pane, and **Skip / Next**. Finishing the last step shows `\(^_^)/ build complete`.

## 8. Data Model (additive)

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
  files_json TEXT NOT NULL,   -- [{path, content}] written when the step activates
  status     TEXT NOT NULL    -- 'locked' | 'unlocked' | 'passed' | 'skipped'
);
CREATE INDEX IF NOT EXISTS idx_build_steps_build ON build_steps(build_id, idx);
```
- A source can have one build per language (UNIQUE). `remove` cascades sources → builds → build_steps.
- Step gating: step `idx` unlocked iff `idx==0` or the previous step is passed/skipped (`study.ComputeUnlocked`).

## 9. Agent Calls

Both reuse `agent.Invoker`; content via `Job.Stdin`, never argv.

- **`gen-build`**: `Stdin` = the source's concatenated section markdown; prompt carries language + difficulty + instructions. Output schema → `{skeleton_files: [{path, content}], steps: [{goal, files: [{path, content}]}]}`. `skeleton_files` are the language project files (e.g. Cargo.toml/go.mod/Makefile); each step's `files` are its acceptance tests + difficulty-scaled scaffold. Difficulty: guided = starter code + comments; spec+tests = stubs + tests; blank-page = goal + tests only.
- **`gen-hint`**: `Stdin` = step goal + the user's current workspace code + the latest failing test output; prompt asks for a nudge, not a solution. Output → `{hint}`.

## 10. Test Runner

`TestRunner` interface: `Run(ctx context.Context, workspace, language string) (Result, error)` where `Result{Passed bool, Output string, RunError bool}`.
- Real impl execs the per-language command in `workspace` with a bounded timeout (90s):
  | Lang | Command |
  |---|---|
  | rust | `cargo test` |
  | go | `go test ./...` |
  | python | `pytest` |
  | c / cpp | `make test` |
- `RunError = true` when the command itself can't run (binary missing / build error distinct from test assertion failure — at minimum, executable-not-found). A normal non-zero exit with test output is `Passed=false, RunError=false`.
- A fake `TestRunner` is injected in handler/domain tests so they need no toolchains.

## 11. Flow

1. Pick language + difficulty → `POST /build/{id}` → `gen-build` → persist `builds` + `build_steps` (step 0 `unlocked`, rest `locked`) → scaffold workspace: write `skeleton_files` + step 0's `files` → redirect to build view.
2. Build view shows step 0 goal + workspace path. You code in your editor.
3. **Run tests** → server `TestRunner.Run` → on `Passed` → mark step `passed`, write the next step's `files` into the workspace, unlock the next step. On failure → show output (step stays active). On `RunError` → show the install/run-error message.
4. **Hint** → `gen-hint` with current code + last failing output → show nudge.
5. **Skip** → mark `skipped`, write next files, unlock next.
6. Last step passed → "build complete".

## 12. Store Methods (new)

- `SaveBuild(ctx, *model.Build) error` — inserts the build + its steps (transaction).
- `GetBuild(ctx, sourceID, language string) (*model.Build, error)` — `ErrNotFound` if absent; loads steps ordered by idx.
- `SetBuildStepStatus(ctx, stepID, status string) error`.
- `GetBuildStep(ctx, stepID string) (*model.BuildStep, error)` — for writing its files / status.
- (Listing builds per source for the home/study page is optional v1.)

New model types: `model.Build{ID, SourceID, Language, Difficulty, Workspace string; CreatedAt time.Time; Steps []BuildStep}`, `model.BuildStep{ID, BuildID string; Idx int; Goal string; Files []BuildFile; Status string}`, `model.BuildFile{Path, Content string}`.

## 13. Error Handling

- `gen-build` failure → error page + Retry; the build row is persisted only on success, so retry is clean.
- Toolchain missing / un-runnable → `RunError` surfaced in the output pane ("install `cargo`/`go`/… to run tests"); step not advanced.
- Test timeout (90s) → reported as a run error, not a hang.
- Workspace write failure → 500. Unknown source/language → 404.
- Path safety: file paths from `gen-build` are written under the workspace only; reject any path that escapes it (no absolute paths or `..`).
- `remove` cascades through `builds`/`build_steps`.

## 14. Testing Strategy

TDD-first; agent and test-runner both faked.
- **store:** builds/build_steps round-trip; cascade from source; `UNIQUE(source,language)`.
- **build domain:** `gen-build` parse + path-safety rejection; `gen-hint` parse; step pass→unlock logic — fake agent.
- **test runner:** real impl against a trivial passing/failing command (e.g. a tiny shell project) or a stubbed command; fake used elsewhere.
- **web handlers:** picker; start (fake agent → files written to a temp workspace, build persisted, redirect); `/test` (fake runner: pass advances + writes next files; fail/runError don't); hint; skip; gating — `httptest` + fakes + real SQLite.

## 15. Open Questions / Future

- Sandbox/containerized test execution.
- Toolchain detection/setup guidance per language.
- Multiple builds/languages surfaced on the home page.
- Regenerating a step or resetting a build.
