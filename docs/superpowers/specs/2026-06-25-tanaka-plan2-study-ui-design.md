# Tanaka Plan 2 — Study UI Design Spec

**Date:** 2026-06-25
**Status:** Approved (design); implementation plan pending
**Builds on:** Plan 1 (foundation & ingestion) — `add`/`list`/`remove`, SQLite store, `agent.Invoker`.

## 1. Summary

Plan 2 adds the **study phase**: `tanaka serve` starts a local web server with a retro Windows-95 UI where you read an imported source section by section and answer quizzes to verify understanding before advancing. The build phase (Plan 3) is out of scope.

Study packages (per-section summaries + quiz questions) are generated **lazily on first open**, cached in SQLite. The UI is **server-rendered** Go `html/template` styled with **98.css**, plus a little vanilla JS for live grading. The server orchestrates headless `claude` calls (Plan 1's app-orchestrated model); MCQ grading is pure Go, free-response grading is an agent call.

## 2. Goals / Non-Goals

**Goals**
- `tanaka serve` → browse sources, read sections, answer quizzes, track progress, all in a local web UI.
- Lazy per-source study-package generation, cached.
- Mixed quizzes: MCQ (graded server-side, no agent) + free-response (agent-graded).
- Pass-gated, skippable progression.
- Retro Windows-95 aesthetic (98.css), single Go binary, runs on the Claude subscription.

**Non-Goals (Plan 2)**
- The build phase (Plan 3).
- Streamed/SSE prepare progress (v1 uses a synchronous prepare with a loading state; upgrade later).
- Multi-user / remote hosting. Server binds localhost only.
- Editing generated study content in the UI.

## 3. Key Decisions

| Decision | Choice |
|---|---|
| Scope | Study phase only (read + quiz + progress) |
| UI stack | Server-rendered Go `html/template` + embedded **98.css** + vanilla JS (`fetch`) |
| Generation timing | **Lazy on first open**, cached in SQLite |
| Quiz model | **Mixed**: MCQ (server-side check) + free-response (agent-graded) |
| Gating | **Pass-gated, skippable** |
| Prepare UX (v1) | Synchronous "Prepare" action with a loading state; resumable per section |
| Serve bind | `127.0.0.1:7777` default, `--port` to override; localhost only |

## 4. Architecture

```
  Browser ──HTTP──> internal/web (Go server: templates + 98.css + app.js)
                      │  uses
                      ├── internal/store  (SQLite: sources, sections, study, questions, progress)
                      └── internal/study  (gen-study, grade-answer over agent.Invoker)
                                              │ headless
                                              ▼
                                        claude -p (Plan 1 agent.Invoker)
```

The Go binary owns state and orchestration. Free-response grading and study generation are headless `claude` calls (content via **stdin**, never argv — Plan 1's rule). MCQ grading is pure Go.

## 5. Package Layout

- `internal/web` — HTTP server, routing, handlers, embedded `html/template` + `98.css` + `app.js`. One responsibility: HTTP ↔ domain.
- `internal/study` — `gen-study` (summary + questions per section) and `grade-answer` (free-response verdict) over `agent.Invoker`; MCQ grading helper. Owns the prompts/schemas.
- `internal/store` — extended with study/questions/progress tables + methods (below).
- `internal/cli` — new `serve` subcommand wiring store + invoker into `web.Server`.

## 6. Routes

| Method/Path | Purpose |
|---|---|
| `GET /` | Source list with progress (e.g. `3/9`) |
| `GET /study/{id}` | Unprepared → Prepare page; else redirect to current section |
| `POST /study/{id}/prepare` | Generate all sections' study package (sync, loading state) → section 0 |
| `GET /study/{id}/{idx}` | Section page (reading + key concepts + quiz); locked notice if gated |
| `POST /grade` | JSON `{sectionId, questionId, answer\|choice}` → `{verdict, feedback, explanation}` |
| `POST /study/{id}/{idx}/skip` | Record skip, unlock next |
| `GET /static/*` | Embedded `98.css`, `app.js` |

## 7. Screens (retro Win95)

**Source list (`/`):** a window listing each source with title, origin, and progress (`passed+skipped / total`); click opens `/study/{id}`.

**Section page (`/study/{id}/{idx}`):**
```
+--------------------------------------------------------------+
| ▤ Tanaka — Selection Pushdown…            [_][□][X]          |
+-------------------+------------------------------------------+
| Sections          |  ## 3. Bit Manipulation Instructions     |
|  1 Intro      ✔   |  (rendered markdown of this section)      |
|  2 Background ✔   |                                          |
| ▶3 BMI            |  ── Key concepts ──                       |
|  4 …      (lock)  |  • pext  • pdep  • parallel bit extract   |
|  …                |                                          |
|                   |  ── Quiz ──                              |
| [ progress 2/9 ]  |  Q1  ( )a ( )b ( )c     [ Check ]         |
|                   |  Q2  [ textarea........ ] [ Submit ]      |
|                   |  > \(^_^)/ pass — nice, because…         |
|                   |  [ Skip section ]        [ Next ▶ ]       |
+-------------------+------------------------------------------+
```
Left = section nav (✔ passed, ▶ current, lock = gated). Right = rendered section markdown + key concepts + quiz. "Next" enables once the section is `passed` or `skipped`.

## 8. Data Model (additive)

New tables, all `ON DELETE CASCADE` from `sections` so `remove` still wipes everything:
```sql
CREATE TABLE IF NOT EXISTS section_study (
  section_id   TEXT PRIMARY KEY REFERENCES sections(id) ON DELETE CASCADE,
  summary      TEXT NOT NULL,
  key_concepts TEXT NOT NULL            -- JSON array of strings
);
CREATE TABLE IF NOT EXISTS questions (
  id            TEXT PRIMARY KEY,
  section_id    TEXT NOT NULL REFERENCES sections(id) ON DELETE CASCADE,
  idx           INTEGER NOT NULL,
  kind          TEXT NOT NULL,          -- 'mcq' | 'free'
  prompt        TEXT NOT NULL,
  options       TEXT,                   -- JSON array (mcq only)
  correct_index INTEGER,               -- mcq only
  rubric        TEXT,                   -- free only
  explanation   TEXT                    -- shown after answering
);
CREATE TABLE IF NOT EXISTS section_progress (
  section_id TEXT PRIMARY KEY REFERENCES sections(id) ON DELETE CASCADE,
  status     TEXT NOT NULL              -- 'unlocked' | 'passed' | 'skipped'
);
```
- **Prepared?** A source is prepared when every one of its sections has a `section_study` row.
- **Gating (pure function):** section `idx` is unlocked iff `idx == 0` or the previous section's status is `passed`/`skipped`.

## 9. Agent Calls

Both reuse Plan 1's `agent.Invoker`; section text travels via **Stdin** (never argv).

- **`gen-study`** (per section, on prepare): `Stdin` = section markdown; prompt asks for a study package. Schema → `{summary, key_concepts: [string], questions: [{kind, prompt, options?, correct_index?, rubric?, explanation?}]}`. No tools.
- **`grade-answer`** (free-response only): `Stdin` = section markdown + the user's answer; prompt carries the question + rubric. Schema → `{verdict: "pass"|"partial"|"fail", feedback}`.
- **MCQ grading**: pure Go — compare submitted choice to stored `correct_index` → pass/fail + `explanation`. The correct answer never reaches the browser.

## 10. Flow

- **Prepare:** for each section in order → `gen-study` → save `section_study` + `questions`; set section 0 `unlocked`. Sequential, with a loading state. Saved per section → **resumable**: re-running regenerates only sections lacking a `section_study` row.
- **Open section:** render markdown + key concepts + questions (MCQ options shown, correct answer withheld) + lock state.
- **Answer:** MCQ → `POST /grade` server check; free → `POST /grade` → `grade-answer`. When every question in the section is satisfied (MCQ correct; free `pass`/`partial`), set section `passed` and unlock the next. A section with no questions auto-passes.
- **Skip:** `POST .../skip` → set `skipped`, unlock next.

## 11. Store Methods (new)

- `SaveSectionStudy(ctx, sectionID, summary string, keyConcepts []string, questions []Question) error`
- `GetSectionStudy(ctx, sectionID) (summary string, keyConcepts []string, questions []Question, error)`
- `IsPrepared(ctx, sourceID) (bool, error)`
- `GetSectionProgress(ctx, sourceID) (map[sectionID]status, error)` and `SetSectionStatus(ctx, sectionID, status) error`
- `GetQuestion(ctx, questionID) (Question, sectionID string, error)` — for server-side grading (carries `correct_index`/`rubric`/`explanation`).

## 12. Error Handling

- **Prepare fails mid-way:** per-section saves make it resumable; UI shows the failing section + "Retry prepare". Never half-stuck.
- **Grading fails** (agent error/timeout, free-response): quiz panel shows "grading unavailable — try again"; **Skip stays available** (graceful degradation).
- **`claude` missing / not logged in:** `serve` still starts (browse existing sources); prepare/grade surface the friendly "run `claude login`" message.
- **Locked section:** polite "finish the previous section first" notice, not a raw 403.
- **Unknown id / bad route:** 404 page. **Port in use:** clear startup error.
- **Malformed agent JSON:** handled at the `agent.Invoker` boundary (schema + bounded retry); surfaces as a prepare/grade error.

## 13. Testing Strategy

TDD-first; the agent is mockable (`agent.Fake`), so tests are token-free.

- **store:** round-trip `section_study`/`questions`/`section_progress`; `remove` cascade still wipes them; `IsPrepared` logic.
- **gating:** pure function, table-driven (locked/unlocked/passed/skipped).
- **study:** `gen-study` prompt-build + parse and `grade-answer` parse via fake invoker; MCQ grading pure function (correct/incorrect).
- **web handlers:** `httptest` + fake invoker + real SQLite — `GET /` lists with progress; locked section → notice; `POST /grade` MCQ correct/incorrect; free answer advances on fake `pass`; skip unlocks next; prepare generates + redirects.
- **assets/templates:** embedded `98.css`/`app.js` served; templates render without error.

## 14. Open Questions / Future

- Streamed/SSE prepare progress (replace synchronous prepare).
- Regenerate / re-quiz a section on demand.
- Spaced-repetition review across sources.
- Auto-open the browser on `serve`.
