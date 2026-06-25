# Final Fix Report — plan1-foundation-ingestion

## Changes Made

### IMPORTANT 1 — Populate CreatedAt

**`internal/ingest/structure.go`** (lines 8, 61–64)
- Added `"time"` import.
- Set `CreatedAt: time.Now().UTC()` when constructing `&model.Source{}`.

**`internal/ingest/structure_test.go`** (lines 46–48)
- Added assertion in `TestStructureBuildsSource`:
  ```go
  if src.CreatedAt.IsZero() { t.Fatal("CreatedAt not set") }
  ```

**`internal/store/sqlite_test.go`** — `TestListSources` (lines 63–82)
- Replaced same-timestamp loop with two explicit inserts using distinct timestamps (`time.Unix(2,0)` for "b" inserted first, `time.Unix(1,0)` for "a" inserted second).
- After listing, asserts `list[0].ID == "a"` and `list[1].ID == "b"` (ascending created_at, not insertion order).

**RED evidence (before fix):**
- `TestStructureBuildsSource` would fail with `"CreatedAt not set"` because `structure.go` never set `src.CreatedAt`.
- `TestListSources` (new form) would fail: without distinct timestamps, SQLite ordering was undefined and the "a first" assertion would be non-deterministic.

**GREEN:** Both pass after fix.

---

### IMPORTANT 2 — Wire startup Check for claude CLI

**`internal/agent/agent.go`** (lines 19–21, 28–30, 34–36)
- Added `Check(ctx context.Context) error` to the `Invoker` interface.
- Added `CheckErr error` field to `Fake`.
- Added `func (f *Fake) Check(_ context.Context) error { return f.CheckErr }`.

**`internal/cli/cli.go`** — `cmdAdd` (lines 83–87)
- Added as the first step (before `ingest.Read`):
  ```go
  if err := d.invoker.Check(ctx); err != nil {
      fmt.Fprintf(stderr, "claude CLI unavailable: %v\nis it installed and logged in? try: claude login\n", err)
      return 1
  }
  ```

**`internal/cli/add_test.go`** — `TestAddCheckFailure` (new, lines 64–75)
- Builds deps with `&agent.Fake{CheckErr: errors.New("not found")}`.
- Runs `add -`, asserts non-zero exit and stderr contains `"claude CLI unavailable"`.

**RED evidence (before fix):**
- `TestAddCheckFailure` would not compile: `Fake` had no `Check` method and `Invoker` interface had no `Check`, so `&agent.Fake{CheckErr: ...}` would be rejected as not satisfying the interface.

**GREEN:** Compiles and passes after fix.

---

### IMPORTANT 3 — HTTP timeout + interrupt-cancellable context

**`internal/ingest/reader.go`** (lines 13–14, 33)
- Added `"time"` import.
- Added `var httpClient = &http.Client{Timeout: 30 * time.Second}` package-level variable.
- Replaced `http.DefaultClient.Do(req)` with `httpClient.Do(req)`.

**`internal/cli/cli.go`** — `Run` (lines 53–54)
- Added `"os/signal"` import.
- Replaced `context.Background()` with:
  ```go
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
  defer stop()
  ```

---

### MINOR — Store error hygiene

**`internal/store/sqlite.go`**
- Added `"errors"` import.
- `GetSource` line 64: `if err == sql.ErrNoRows` → `if errors.Is(err, sql.ErrNoRows)`.
- `GetSource` scan error: `return nil, err` → `return nil, fmt.Errorf("scan source %s: %w", id, err)`.
- `GetSource` sections query error: `return nil, err` → `return nil, fmt.Errorf("query sections for %s: %w", id, err)`.
- `GetSource` section scan error: `return nil, err` → `return nil, fmt.Errorf("scan section for %s: %w", id, err)`.
- `ListSources` query error: `return nil, err` → `return nil, fmt.Errorf("query sources: %w", err)`.
- `ListSources` scan error: `return nil, err` → `return nil, fmt.Errorf("scan source row: %w", err)`.
- `ErrNotFound` sentinel is still returned directly (not wrapped), preserving `errors.Is(err, ErrNotFound)` in callers.

---

### MINOR — Comment intentional version duplication

**`internal/cli/cli.go`** — `Run` (line 33)
- Replaced bare `if args[0] == "version"` block comment with:
  ```go
  // Short-circuit for version before opening the DB; run also handles version
  // for direct (test) calls that bypass Run.
  ```

---

## Full `go test ./...` Output (GREEN)

```
?   	github.com/devandbenz/tanaka	[no test files]
ok  	github.com/devandbenz/tanaka/internal/agent	0.005s
ok  	github.com/devandbenz/tanaka/internal/app	(cached)
ok  	github.com/devandbenz/tanaka/internal/cli	0.007s
ok  	github.com/devandbenz/tanaka/internal/ingest	0.002s
?   	github.com/devandbenz/tanaka/internal/model	[no test files]
ok  	github.com/devandbenz/tanaka/internal/store	0.004s
```

Verbose output for changed packages:

```
=== RUN   TestStructureBuildsSource
--- PASS: TestStructureBuildsSource (0.00s)
=== RUN   TestStructureRejectsEmptySections
--- PASS: TestStructureRejectsEmptySections (0.00s)
PASS
ok  	github.com/devandbenz/tanaka/internal/ingest	0.003s

=== RUN   TestSaveAndGetSource
--- PASS: TestSaveAndGetSource (0.00s)
=== RUN   TestGetSourceNotFound
--- PASS: TestGetSourceNotFound (0.00s)
=== RUN   TestListSources
--- PASS: TestListSources (0.00s)
PASS
ok  	github.com/devandbenz/tanaka/internal/store	0.005s

=== RUN   TestAddThenList
--- PASS: TestAddThenList (0.00s)
=== RUN   TestAddRequiresArg
--- PASS: TestAddRequiresArg (0.00s)
=== RUN   TestRunEmptyArgs
--- PASS: TestRunEmptyArgs (0.00s)
=== RUN   TestAddCheckFailure
--- PASS: TestAddCheckFailure (0.00s)
=== RUN   TestRunVersion
--- PASS: TestRunVersion (0.00s)
=== RUN   TestRunUnknownCommand
--- PASS: TestRunUnknownCommand (0.00s)
PASS
ok  	github.com/devandbenz/tanaka/internal/cli	0.005s
```
