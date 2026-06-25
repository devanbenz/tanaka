# Tanaka — Design Spec

**Date:** 2026-06-25
**Status:** Approved (design); implementation plan pending

> *"Well, Mr. Average, do you really have time to be hanging your head?"* — Tanaka Ryūnosuke, Haikyū!!
>
> Tanaka exists to take someone who thinks they are average and push them toward great heights. It does not coddle. It makes you struggle productively.

## 1. Summary

Tanaka is a local single-binary application that turns **any technical content** (academic paper, blog post, article, docs — as a file, URL, or pasted text) into a two-phase learning experience:

1. **Study phase** — the content is chunked into sections; at natural informational points you are quizzed to verify understanding before you can advance. Reading and answering happen in a browser UI; answers are graded by a coding agent.
2. **Build phase** — after finishing the reading, Tanaka generates a hands-on tutorial that has you implement the ideas in a language of your choice (Rust, Go, C++, C, Python). It is deliberately **not** hand-holding; a difficulty dial controls how much scaffolding you get.

The pedagogy borrows from active-recall / productive-struggle approaches (prediction, teach-it-back, trace-the-path, retrieval practice): the tool poses a question and **waits for your answer** rather than answering itself.

## 2. Goals / Non-Goals

**Goals**
- Accept arbitrary technical content as input (file / URL / pasted text).
- Verify understanding via interactive, graded comprehension checks before advancing.
- Produce a build tutorial that translates the content into code in the user's chosen language, with tunable difficulty.
- Run entirely on the user's Claude subscription (no API key required).
- Ship as a single Go binary with a local web UI and a small CLI.

**Non-Goals (for v1)**
- Multi-user / hosted service. Tanaka is local and single-user.
- Spaced-repetition scheduling across many sources over time (possible later).
- Auto-detecting or installing language toolchains. Test-running relies on the agent's shell environment.
- A pluggable multi-provider LLM layer. The agent is Claude Code via headless invocation, behind an interface (see §7).

## 3. Form Factor & Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Form factor | New standalone app (CLI + local web UI) | Full control over the study→build flow |
| Implementation language | Go | Single static binary, easy local web server + CLI, simple distribution |
| LLM integration | Agent-driven via headless `claude -p` | Uses existing Claude subscription, no API key |
| Interaction model | UI-driven; agent grades | Best reading UX; quizzing/build-grading happen live |
| Orchestration | App-orchestrated (Approach A) + batch pre-gen of static study content (borrowed from Approach C) | Smooth UI-driven loop without a long-lived agent session; snappy reading |
| Build rigor | Difficulty dial: guided → spec+tests → blank-page (default spec+tests) | Adjustable productive struggle |

### Billing / auth note
`claude -p` (headless print mode) draws from the user's Pro/Max **subscription**, identically to interactive mode — no API key needed. Anthropic's planned June 15, 2026 move of `claude -p` / Agent SDK / third-party usage to a separate credit pool was **paused, not cancelled**. Because it could return, the agent integration sits behind an interface (§7) so it can be swapped to the API later without touching the rest of Tanaka.

## 4. Architecture

```
            ┌──────────────────────────────────────────────┐
  Browser ──┤  Go binary (`tanaka`)                          │
   (UI)     │   • local web server + UI (reading, quizzing)  │
            │   • CLI (add, serve, status)                   │
            │   • store: ~/.tanaka/ (SQLite + files)         │
            │   • Agent Invoker ──── shells out ───┐         │
            └──────────────────────────────────────┼─────────┘
                                                    ▼
                                       `claude -p ...`
                                  --output-format json --json-schema
                                  (structure, gen-study, grade-answer,
                                   gen-build, run-tests)
```

The Go binary owns all state and sequencing. Every "smart" task is a stateless headless agent call with a JSON-schema'd response. The agent is a worker, not a driver.

## 5. Components

Each component has one clear purpose and is independently testable.

1. **Ingestion** — Accepts a file (PDF/HTML/Markdown/txt), a URL, or stdin (`-`). The agent reads the source itself so content never passes through `argv` (argv cannot hold NUL bytes or huge payloads — this crashed on binary PDFs): a **file** is read by the agent's `Read` tool (handles PDFs/large files), a **URL** by its `WebFetch` tool, and **stdin/pasted text** is piped to the agent's stdin. The agent normalizes to clean Markdown, splits into ordered sections, and marks natural informational points. Output: `Source{id, title, origin, sections[]}` (origin = absolute file path, URL, or "stdin"). Verified e2e 2026-06-25: a 720K binary PDF ingests into sections.
2. **Study-package generator** (batch) — For each section, a `gen-study` call produces `{summary, key_concepts[], questions[]}`. Question types: recall, prediction, teach-it-back, trace-the-path. Stored statically; pre-generated so reading is snappy.
3. **Web UI + server** — Browse sources; read a section; answer questions inline; see progress. The next section is gated until the current section's quiz is passed (skippable). Also hosts the build-phase UI (language + difficulty selection, step view, run-tests, hints).
   - **Aesthetic (hard requirement):** retro desktop-GUI look in the spirit of Windows 95 — beveled/3D widgets, title bars, system/pixel fonts, solid colors, chunky tactile buttons. Explicitly **not** the default "LLM UI" style (gradient heroes, rounded cards, heavy whitespace, Inter/SaaS look). A library like 98.css/7.css or hand-rolled CSS is acceptable; keep it simple and pleasing.
4. **Live grading** — On answer submit: `POST /grade` → `grade-answer` call → `{verdict: pass|partial|fail, feedback, followup?}`. On pass, unlock next section and persist progress.
5. **Build engine** — On build start: `gen-build` call → `BuildPlan{steps[]: {goal, scaffold, acceptance_tests}}`, scaffolding scaled to difficulty. The user writes code in their own repo. "Run tests" → agent runs the chosen language's tests via shell within the call (no backgrounding) → `{pass/fail, output}`. Hints provided only on request.
6. **Agent Invoker** — The single chokepoint. Builds prompts from owned templates, calls `claude -p ... --output-format json --json-schema [--allowedTools ...]`, pipes any large/binary content via the process's **stdin** (never `argv`), validates/parses JSON, retries on transient failure, surfaces clean errors. An interface; mockable in tests. `agent.Job` carries `{Prompt, Schema, Stdin, AllowedTools}`.
   - **`--bare` note:** despite the docs recommending `--bare` for scripts, end-to-end testing (2026-06-25) showed `--bare` forces API-key auth and breaks Claude subscription/OAuth. It is intentionally omitted; `--output-format json` alone yields a single machine-readable JSON object. Trade-off: without `--bare`, the user's hooks/plugins/MCP load, which is slightly less deterministic — acceptable to keep subscription auth working.

## 6. Data Flow

**Ingest**
```
tanaka add <file|url|->
  → classify input: file → agent Read tool | url → agent WebFetch | stdin → piped bytes
  → Agent Invoker "structure" (content read by agent or piped; never via argv)
      → clean markdown + ordered sections + info-points
  → store Source in ~/.tanaka/
  → for each section: "gen-study" → {summary, key_concepts, questions}  (cached)
```

**Study**
```
tanaka serve  → browser:
  read section → answer question
    → POST /grade → "grade-answer" → {verdict, feedback, followup?}
    → pass → unlock next section; persist progress
  finish last section → Source marked "studied"
```

**Build**
```
in UI: pick language + difficulty (default spec+tests)
  → "gen-build" → BuildPlan{steps[]}
  → user writes code in their own repo
  → "Run tests" → agent runs tests via shell → {pass/fail, output}
  → fail → "Hint" (on request) ; pass → next step
```

## 7. Interfaces & Boundaries

- **`AgentInvoker`** (Go interface): `Invoke(ctx, job) (json, error)` where `job` carries a template id, inputs, and the expected JSON schema. Concrete impl shells out to `claude`. Fake impl returns scripted JSON in tests. This boundary is also the swap point if billing rules force an API-key path later.
- **`Store`** (Go interface): persistence for sources, sections, progress, build state. Backed by SQLite under `~/.tanaka/`; generated markdown and the linked build-repo path live in a files dir.
- **Prompt templates**: owned by the app (not external skills), versioned, covered by golden tests.

## 8. State / Storage

Everything under `~/.tanaka/`:
- `tanaka.db` (SQLite): sources, sections, study packages, progress, build plans + step state.
- `files/`: generated Markdown per section; references to the user's build repo path.

Single binary; no external services.

## 9. Error Handling

- **Ingestion failures** (unfetchable URL, paywalled/encrypted PDF, bad extraction): fail loudly with the reason; always offer `tanaka add -` to paste cleaned text. Never half-import.
- **Agent call failures** (CLI missing, not logged in, timeout, malformed JSON): validate against schema, bounded retries on parse/timeout, then a clean specific error (e.g. "not logged in — run `claude login`"). Missing `claude` binary detected at startup.
- **Resumable jobs**: pre-generation writes per-section as it completes; interrupted `add` resumes rather than regenerating.
- **Build test-runs**: distinguish *your code failed the tests* (expected — show output) from *the run errored* (toolchain missing / agent error). Agent waits on the test process within the call, because headless background tasks are killed ~5s after the call returns.
- **Graceful degradation**: if live grading is unavailable, allow self-marking a section to continue rather than hard-blocking.

## 10. Testing Strategy

TDD-first. The agent is mockable to keep tests fast and token-free.

- **Agent Invoker**: interface with an injected fake returning scripted JSON; a tiny stub `claude` binary covers the subprocess path.
- **Ingestion**: unit tests over fixtures (sample PDF, HTML page, Markdown) for normalization and section splitting.
- **Store**: SQLite round-trip tests (source → progress → build state).
- **Server handlers**: table-driven tests for `/grade`, `/build`, and progress gating (next section locked until pass).
- **Prompt templates**: golden tests so prompt/schema changes are intentional.
- **End-to-end**: one happy-path test (add fixture → study → build) through the stub agent.

## 11. README Style (hard requirement)

The README must be plain and minimal — no marketing voice, no emoji, no "powerful/seamless/effortless" filler, no LLM tells. Contents: one-line description, install, a couple of real command examples, and a short "how it works." A skeleton the user can edit, not prose to delete.

## 12. Open Questions / Future

- Spaced-repetition across sources over time.
- Exporting a finished study+build package to share.
- Optional Claude Code skills as a second entry point for power users already inside an agent.
