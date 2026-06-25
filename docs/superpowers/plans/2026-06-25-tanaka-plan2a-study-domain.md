# Tanaka Plan 2a — Study Domain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the study-phase data and domain layer — study-package storage, per-section generation (`gen-study`), answer grading (MCQ in Go, free-response via the agent), and section gating — plus a `tanaka prepare <id>` command, so the web UI (Plan 2b) can be built on a tested foundation.

**Architecture:** New domain types live in `internal/model`. `internal/store` gains study/questions/progress tables and methods. `internal/study` orchestrates the agent (`gen-study`, `grade-answer`), grades MCQs in pure Go, computes gating, and exposes `PrepareSource`. `internal/cli` gains a `prepare` command. No HTTP in this plan.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, standard library, the existing `agent.Invoker`. No new third-party dependencies.

## Global Constraints

- Module path: `github.com/devandbenz/tanaka` (verbatim in imports).
- Go version floor: `go 1.26`.
- Pure-Go, no cgo. No new third-party dependencies in this plan — only `modernc.org/sqlite` and the standard library.
- All agent calls go through `agent.Invoker`. Content travels via `Job.Stdin`, never argv.
- New tables are additive with `ON DELETE CASCADE` from `sections` so `remove` still wipes everything.
- Status values are exactly `"unlocked"`, `"passed"`, `"skipped"`; a section with no progress row is treated as `"locked"`.
- Docs/comments plain and minimal — no marketing voice, no emoji.

---

### Task 1: Study domain types

**Files:**
- Modify: `internal/model/model.go`
- Test: `internal/model/model_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `model.Question{ID, SectionID string; Idx int; Kind string /*"mcq"|"free"*/; Prompt string; Options []string; CorrectIndex int; Rubric string; Explanation string}`
  - `model.SectionStudy{SectionID, Summary string; KeyConcepts []string; Questions []Question}`
  - Status constants: `model.StatusLocked = "locked"`, `model.StatusUnlocked = "unlocked"`, `model.StatusPassed = "passed"`, `model.StatusSkipped = "skipped"`.
  - Question kind constants: `model.KindMCQ = "mcq"`, `model.KindFree = "free"`.

- [ ] **Step 1: Write the failing test**

Create `internal/model/model_test.go`:
```go
package model

import "testing"

func TestStudyConstants(t *testing.T) {
	if StatusLocked != "locked" || StatusUnlocked != "unlocked" ||
		StatusPassed != "passed" || StatusSkipped != "skipped" {
		t.Fatal("status constant values must match the schema strings")
	}
	if KindMCQ != "mcq" || KindFree != "free" {
		t.Fatal("kind constant values must match the schema strings")
	}
}

func TestSectionStudyHoldsQuestions(t *testing.T) {
	s := SectionStudy{
		SectionID:   "sec1",
		Summary:     "a summary",
		KeyConcepts: []string{"x", "y"},
		Questions: []Question{
			{ID: "q1", SectionID: "sec1", Idx: 0, Kind: KindMCQ, Prompt: "?", Options: []string{"a", "b"}, CorrectIndex: 1},
			{ID: "q2", SectionID: "sec1", Idx: 1, Kind: KindFree, Prompt: "explain", Rubric: "mentions x"},
		},
	}
	if len(s.Questions) != 2 || s.Questions[0].Options[1] != "b" || s.Questions[1].Kind != KindFree {
		t.Fatalf("unexpected: %+v", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/`
Expected: FAIL — `undefined: StatusLocked` etc.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/model/model.go` (keep the existing `Source`/`Section` types):
```go
// Study status values (also the strings stored in section_progress).
const (
	StatusLocked   = "locked"
	StatusUnlocked = "unlocked"
	StatusPassed   = "passed"
	StatusSkipped  = "skipped"
)

// Question kinds.
const (
	KindMCQ  = "mcq"
	KindFree = "free"
)

// Question is one quiz item for a section.
type Question struct {
	ID           string
	SectionID    string
	Idx          int
	Kind         string // KindMCQ or KindFree
	Prompt       string
	Options      []string // MCQ only
	CorrectIndex int      // MCQ only
	Rubric       string   // free only
	Explanation  string   // shown after answering
}

// SectionStudy is the generated study package for one section.
type SectionStudy struct {
	SectionID   string
	Summary     string
	KeyConcepts []string
	Questions   []Question
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat: study domain types (Question, SectionStudy, status/kind constants)"
```

---

### Task 2: Store schema + study package persistence

**Files:**
- Modify: `internal/store/store.go` (schema + interface)
- Modify: `internal/store/sqlite.go` (implementation)
- Test: `internal/store/study_test.go`

**Interfaces:**
- Consumes: `model.Question`, `model.SectionStudy` (Task 1); existing `store.Store`, `store.ErrNotFound`.
- Produces (added to the `store.Store` interface):
  - `SaveSectionStudy(ctx context.Context, s *model.SectionStudy) error` — upsert: replaces any existing study + questions for that section (idempotent re-prepare).
  - `GetSectionStudy(ctx context.Context, sectionID string) (*model.SectionStudy, error)` — `ErrNotFound` if absent.

- [ ] **Step 1: Add the new tables to the schema**

In `internal/store/store.go`, append to the `schema` constant (before the closing backtick):
```sql
CREATE TABLE IF NOT EXISTS section_study (
	section_id   TEXT PRIMARY KEY REFERENCES sections(id) ON DELETE CASCADE,
	summary      TEXT NOT NULL,
	key_concepts TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS questions (
	id            TEXT PRIMARY KEY,
	section_id    TEXT NOT NULL REFERENCES sections(id) ON DELETE CASCADE,
	idx           INTEGER NOT NULL,
	kind          TEXT NOT NULL,
	prompt        TEXT NOT NULL,
	options       TEXT,
	correct_index INTEGER,
	rubric        TEXT,
	explanation   TEXT
);
CREATE INDEX IF NOT EXISTS idx_questions_section ON questions(section_id, idx);
CREATE TABLE IF NOT EXISTS section_progress (
	section_id TEXT PRIMARY KEY REFERENCES sections(id) ON DELETE CASCADE,
	status     TEXT NOT NULL
);
```

- [ ] **Step 2: Add the two methods to the Store interface**

In `internal/store/store.go`, add to the `Store` interface (after `DeleteSource`):
```go
	SaveSectionStudy(ctx context.Context, s *model.SectionStudy) error
	GetSectionStudy(ctx context.Context, sectionID string) (*model.SectionStudy, error)
```

- [ ] **Step 3: Write the failing test**

Create `internal/store/study_test.go`:
```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func sourceWithSection(t *testing.T, s Store, srcID, secID string) {
	t.Helper()
	err := s.SaveSource(context.Background(), &model.Source{
		ID: srcID, Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: secID, SourceID: srcID, Idx: 0, Title: "S", Markdown: "m"}},
	})
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
}

func TestSaveAndGetSectionStudy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")

	study := &model.SectionStudy{
		SectionID:   "sec1",
		Summary:     "summary text",
		KeyConcepts: []string{"alpha", "beta"},
		Questions: []model.Question{
			{ID: "q1", SectionID: "sec1", Idx: 0, Kind: model.KindMCQ, Prompt: "pick", Options: []string{"a", "b"}, CorrectIndex: 1, Explanation: "because b"},
			{ID: "q2", SectionID: "sec1", Idx: 1, Kind: model.KindFree, Prompt: "explain", Rubric: "mentions alpha"},
		},
	}
	if err := s.SaveSectionStudy(ctx, study); err != nil {
		t.Fatalf("SaveSectionStudy: %v", err)
	}
	got, err := s.GetSectionStudy(ctx, "sec1")
	if err != nil {
		t.Fatalf("GetSectionStudy: %v", err)
	}
	if got.Summary != "summary text" || len(got.KeyConcepts) != 2 || got.KeyConcepts[0] != "alpha" {
		t.Fatalf("study not round-tripped: %+v", got)
	}
	if len(got.Questions) != 2 || got.Questions[0].Options[1] != "b" || got.Questions[0].CorrectIndex != 1 {
		t.Fatalf("questions not round-tripped: %+v", got.Questions)
	}
	if got.Questions[1].Kind != model.KindFree || got.Questions[1].Rubric != "mentions alpha" {
		t.Fatalf("free question wrong: %+v", got.Questions[1])
	}
}

func TestSaveSectionStudyIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	mk := func() *model.SectionStudy {
		return &model.SectionStudy{SectionID: "sec1", Summary: "x", KeyConcepts: []string{"k"},
			Questions: []model.Question{{ID: "q1", SectionID: "sec1", Idx: 0, Kind: model.KindFree, Prompt: "p", Rubric: "r"}}}
	}
	if err := s.SaveSectionStudy(ctx, mk()); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSectionStudy(ctx, mk()); err != nil {
		t.Fatalf("second save should replace, not conflict: %v", err)
	}
	got, _ := s.GetSectionStudy(ctx, "sec1")
	if len(got.Questions) != 1 {
		t.Fatalf("expected 1 question after re-save, got %d", len(got.Questions))
	}
}

func TestGetSectionStudyNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetSectionStudy(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/store/ -run SectionStudy`
Expected: FAIL — `*sqliteStore` does not implement `Store` (missing methods).

- [ ] **Step 5: Write the implementation**

Create `internal/store/study.go`:
```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/model"
)

func (s *sqliteStore) SaveSectionStudy(ctx context.Context, study *model.SectionStudy) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	concepts, err := json.Marshal(study.KeyConcepts)
	if err != nil {
		return fmt.Errorf("marshal key_concepts: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO section_study (section_id, summary, key_concepts) VALUES (?, ?, ?)
		 ON CONFLICT(section_id) DO UPDATE SET summary=excluded.summary, key_concepts=excluded.key_concepts`,
		study.SectionID, study.Summary, string(concepts)); err != nil {
		return fmt.Errorf("upsert section_study: %w", err)
	}
	// Replace questions for this section.
	if _, err := tx.ExecContext(ctx, `DELETE FROM questions WHERE section_id = ?`, study.SectionID); err != nil {
		return fmt.Errorf("clear questions: %w", err)
	}
	for _, q := range study.Questions {
		var opts any
		if q.Options != nil {
			b, err := json.Marshal(q.Options)
			if err != nil {
				return fmt.Errorf("marshal options: %w", err)
			}
			opts = string(b)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO questions (id, section_id, idx, kind, prompt, options, correct_index, rubric, explanation)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			q.ID, study.SectionID, q.Idx, q.Kind, q.Prompt, opts, q.CorrectIndex, q.Rubric, q.Explanation); err != nil {
			return fmt.Errorf("insert question %s: %w", q.ID, err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) GetSectionStudy(ctx context.Context, sectionID string) (*model.SectionStudy, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT summary, key_concepts FROM section_study WHERE section_id = ?`, sectionID)
	var summary, concepts string
	if err := row.Scan(&summary, &concepts); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get section_study %s: %w", sectionID, err)
	}
	study := &model.SectionStudy{SectionID: sectionID, Summary: summary}
	if err := json.Unmarshal([]byte(concepts), &study.KeyConcepts); err != nil {
		return nil, fmt.Errorf("unmarshal key_concepts: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, idx, kind, prompt, options, correct_index, rubric, explanation
		 FROM questions WHERE section_id = ? ORDER BY idx`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("query questions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var q model.Question
		var opts sql.NullString
		var correct sql.NullInt64
		var rubric, expl sql.NullString
		if err := rows.Scan(&q.ID, &q.Idx, &q.Kind, &q.Prompt, &opts, &correct, &rubric, &expl); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}
		q.SectionID = sectionID
		if opts.Valid && opts.String != "" {
			if err := json.Unmarshal([]byte(opts.String), &q.Options); err != nil {
				return nil, fmt.Errorf("unmarshal options: %w", err)
			}
		}
		q.CorrectIndex = int(correct.Int64)
		q.Rubric = rubric.String
		q.Explanation = expl.String
		study.Questions = append(study.Questions, q)
	}
	return study, rows.Err()
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/store/`
Expected: PASS (existing + new study tests).

- [ ] **Step 7: Commit**

```bash
git add internal/store/
git commit -m "feat: persist section study packages and questions"
```

---

### Task 3: Store progress + question lookup

**Files:**
- Modify: `internal/store/store.go` (interface)
- Modify: `internal/store/study.go` (implementation)
- Test: `internal/store/progress_test.go`

**Interfaces:**
- Consumes: Task 2 store, `model` types.
- Produces (added to `store.Store`):
  - `IsPrepared(ctx context.Context, sourceID string) (bool, error)` — true iff every section of the source has a `section_study` row (and the source has at least one section).
  - `GetSectionStatuses(ctx context.Context, sourceID string) (map[string]string, error)` — sectionID → status; sections with no `section_progress` row map to `model.StatusLocked`.
  - `SetSectionStatus(ctx context.Context, sectionID, status string) error` — upsert.
  - `GetQuestion(ctx context.Context, questionID string) (*model.Question, error)` — `ErrNotFound` if absent (carries `SectionID`, `CorrectIndex`, `Rubric`, `Explanation`).

- [ ] **Step 1: Add methods to the Store interface**

In `internal/store/store.go`, add to the `Store` interface:
```go
	IsPrepared(ctx context.Context, sourceID string) (bool, error)
	GetSectionStatuses(ctx context.Context, sourceID string) (map[string]string, error)
	SetSectionStatus(ctx context.Context, sectionID, status string) error
	GetQuestion(ctx context.Context, questionID string) (*model.Question, error)
```

- [ ] **Step 2: Write the failing test**

Create `internal/store/progress_test.go`:
```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func twoSectionSource(t *testing.T, s Store) {
	t.Helper()
	err := s.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "B", Markdown: "b"},
		},
	})
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
}

func TestIsPrepared(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	twoSectionSource(t, s)

	ok, err := s.IsPrepared(ctx, "src1")
	if err != nil || ok {
		t.Fatalf("fresh source: ok=%v err=%v, want false/nil", ok, err)
	}
	mustStudy(t, s, "s0")
	if ok, _ := s.IsPrepared(ctx, "src1"); ok {
		t.Fatal("one of two sections studied: want not prepared")
	}
	mustStudy(t, s, "s1")
	if ok, _ := s.IsPrepared(ctx, "src1"); !ok {
		t.Fatal("all sections studied: want prepared")
	}
}

func mustStudy(t *testing.T, s Store, secID string) {
	t.Helper()
	err := s.SaveSectionStudy(context.Background(), &model.SectionStudy{
		SectionID: secID, Summary: "x", KeyConcepts: []string{"k"},
	})
	if err != nil {
		t.Fatalf("SaveSectionStudy %s: %v", secID, err)
	}
}

func TestSectionStatusesDefaultLocked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	twoSectionSource(t, s)
	if err := s.SetSectionStatus(ctx, "s0", model.StatusPassed); err != nil {
		t.Fatal(err)
	}
	statuses, err := s.GetSectionStatuses(ctx, "src1")
	if err != nil {
		t.Fatal(err)
	}
	if statuses["s0"] != model.StatusPassed {
		t.Fatalf("s0 = %q, want passed", statuses["s0"])
	}
	if statuses["s1"] != model.StatusLocked {
		t.Fatalf("s1 = %q, want locked (no row)", statuses["s1"])
	}
}

func TestSetSectionStatusUpserts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	twoSectionSource(t, s)
	if err := s.SetSectionStatus(ctx, "s0", model.StatusUnlocked); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSectionStatus(ctx, "s0", model.StatusPassed); err != nil {
		t.Fatalf("second set should upsert: %v", err)
	}
	statuses, _ := s.GetSectionStatuses(ctx, "src1")
	if statuses["s0"] != model.StatusPassed {
		t.Fatalf("s0 = %q, want passed", statuses["s0"])
	}
}

func TestGetQuestion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	err := s.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec1", Summary: "x", KeyConcepts: []string{"k"},
		Questions: []model.Question{{ID: "q1", SectionID: "sec1", Idx: 0, Kind: model.KindMCQ, Prompt: "p", Options: []string{"a", "b"}, CorrectIndex: 1, Explanation: "e"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.GetQuestion(ctx, "q1")
	if err != nil {
		t.Fatalf("GetQuestion: %v", err)
	}
	if q.SectionID != "sec1" || q.CorrectIndex != 1 || q.Kind != model.KindMCQ {
		t.Fatalf("question = %+v", q)
	}
	if _, err := s.GetQuestion(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing question err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'IsPrepared|SectionStatus|GetQuestion'`
Expected: FAIL — interface methods undefined.

- [ ] **Step 4: Write the implementation**

Append to `internal/store/study.go`:
```go
func (s *sqliteStore) IsPrepared(ctx context.Context, sourceID string) (bool, error) {
	var total, studied int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sections WHERE source_id = ?`, sourceID).Scan(&total); err != nil {
		return false, fmt.Errorf("count sections: %w", err)
	}
	if total == 0 {
		return false, nil
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM section_study st
		 JOIN sections se ON se.id = st.section_id WHERE se.source_id = ?`, sourceID).Scan(&studied); err != nil {
		return false, fmt.Errorf("count studied: %w", err)
	}
	return studied == total, nil
}

func (s *sqliteStore) GetSectionStatuses(ctx context.Context, sourceID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT se.id, COALESCE(sp.status, ?) FROM sections se
		 LEFT JOIN section_progress sp ON sp.section_id = se.id
		 WHERE se.source_id = ?`, model.StatusLocked, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query statuses: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, fmt.Errorf("scan status: %w", err)
		}
		out[id] = status
	}
	return out, rows.Err()
}

func (s *sqliteStore) SetSectionStatus(ctx context.Context, sectionID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO section_progress (section_id, status) VALUES (?, ?)
		 ON CONFLICT(section_id) DO UPDATE SET status=excluded.status`, sectionID, status)
	if err != nil {
		return fmt.Errorf("set status %s: %w", sectionID, err)
	}
	return nil
}

func (s *sqliteStore) GetQuestion(ctx context.Context, questionID string) (*model.Question, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, section_id, idx, kind, prompt, options, correct_index, rubric, explanation
		 FROM questions WHERE id = ?`, questionID)
	var q model.Question
	var opts, rubric, expl sql.NullString
	var correct sql.NullInt64
	if err := row.Scan(&q.ID, &q.SectionID, &q.Idx, &q.Kind, &q.Prompt, &opts, &correct, &rubric, &expl); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get question %s: %w", questionID, err)
	}
	if opts.Valid && opts.String != "" {
		if err := json.Unmarshal([]byte(opts.String), &q.Options); err != nil {
			return nil, fmt.Errorf("unmarshal options: %w", err)
		}
	}
	q.CorrectIndex = int(correct.Int64)
	q.Rubric = rubric.String
	q.Explanation = expl.String
	return &q, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat: store section progress and question lookup"
```

---

### Task 4: Gating (pure function)

**Files:**
- Create: `internal/study/gating.go`
- Test: `internal/study/gating_test.go`

**Interfaces:**
- Consumes: `model` status constants.
- Produces: `study.ComputeUnlocked(statuses []string) []bool` — given section statuses in section order, returns a parallel slice of whether each section is reachable. Section 0 is always reachable; section `i>0` is reachable iff section `i-1` is `passed` or `skipped`. A section already `passed`/`skipped`/`unlocked` is also reachable.

- [ ] **Step 1: Write the failing test**

Create `internal/study/gating_test.go`:
```go
package study

import (
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestComputeUnlocked(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []bool
	}{
		{"first always open", []string{model.StatusLocked, model.StatusLocked}, []bool{true, false}},
		{"passed unlocks next", []string{model.StatusPassed, model.StatusLocked, model.StatusLocked}, []bool{true, true, false}},
		{"skipped unlocks next", []string{model.StatusSkipped, model.StatusLocked}, []bool{true, true}},
		{"unlocked-but-unfinished does not unlock next", []string{model.StatusUnlocked, model.StatusLocked}, []bool{true, false}},
		{"all passed", []string{model.StatusPassed, model.StatusPassed}, []bool{true, true}},
		{"empty", []string{}, []bool{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeUnlocked(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d", len(got), len(c.want))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("[%d] = %v, want %v (%v)", i, got[i], c.want[i], c.in)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/study/`
Expected: FAIL — `undefined: ComputeUnlocked`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/study/gating.go`:
```go
// Package study generates study packages, grades answers, and computes gating.
package study

import "github.com/devandbenz/tanaka/internal/model"

// ComputeUnlocked returns, for each section in order, whether it is reachable.
// Section 0 is always reachable; section i>0 is reachable iff section i-1 is
// passed or skipped.
func ComputeUnlocked(statuses []string) []bool {
	out := make([]bool, len(statuses))
	for i := range statuses {
		if i == 0 {
			out[i] = true
			continue
		}
		prev := statuses[i-1]
		out[i] = prev == model.StatusPassed || prev == model.StatusSkipped
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/study/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/study/gating.go internal/study/gating_test.go
git commit -m "feat: section gating pure function"
```

---

### Task 5: Grading — MCQ (pure Go) and free-response (agent)

**Files:**
- Create: `internal/study/grade.go`
- Test: `internal/study/grade_test.go`

**Interfaces:**
- Consumes: `model.Question`, `agent.Invoker`/`agent.Job` (Plan 1).
- Produces:
  - `study.Verdict{Verdict string /*"pass"|"partial"|"fail"*/; Feedback string}`
  - `study.GradeChoice(q *model.Question, choice int) study.Verdict` — pure; `pass` if `choice == q.CorrectIndex` else `fail`; `Feedback` = `q.Explanation`.
  - `study.GradeFree(ctx context.Context, inv agent.Invoker, sectionMarkdown string, q *model.Question, answer string) (study.Verdict, error)` — one `grade-answer` agent call; section + answer via `Stdin`.
  - Package constant `verdictSchema` (string) used by `GradeFree`.

- [ ] **Step 1: Write the failing test**

Create `internal/study/grade_test.go`:
```go
package study

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

func TestGradeChoice(t *testing.T) {
	q := &model.Question{Kind: model.KindMCQ, CorrectIndex: 2, Explanation: "C is right"}
	if v := GradeChoice(q, 2); v.Verdict != "pass" || v.Feedback != "C is right" {
		t.Fatalf("correct choice => %+v, want pass + explanation", v)
	}
	if v := GradeChoice(q, 0); v.Verdict != "fail" {
		t.Fatalf("wrong choice => %+v, want fail", v)
	}
}

func TestGradeFreeParsesVerdict(t *testing.T) {
	fake := &agent.Fake{Responses: map[string]json.RawMessage{
		"grade": json.RawMessage(`{"verdict":"partial","feedback":"close, but mention X"}`),
	}}
	q := &model.Question{Kind: model.KindFree, Prompt: "explain X", Rubric: "must mention X"}
	v, err := GradeFree(context.Background(), fake, "section text", q, "my answer")
	if err != nil {
		t.Fatalf("GradeFree: %v", err)
	}
	if v.Verdict != "partial" || !strings.Contains(v.Feedback, "mention X") {
		t.Fatalf("verdict = %+v", v)
	}
	// The section text and the user's answer must reach the agent via Stdin.
	call := fake.Calls[0]
	if !strings.Contains(string(call.Stdin), "section text") || !strings.Contains(string(call.Stdin), "my answer") {
		t.Fatalf("stdin missing section/answer: %q", call.Stdin)
	}
	// The question prompt must be in the prompt (so Fake matches and the agent sees it).
	if !strings.Contains(call.Prompt, "explain X") {
		t.Fatalf("prompt missing question: %q", call.Prompt)
	}
}
```

Note: the fake matches on the substring `"grade"`; ensure `gradePrompt` contains the word "grade".

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/study/ -run Grade`
Expected: FAIL — `undefined: GradeChoice`, `undefined: GradeFree`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/study/grade.go`:
```go
package study

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

// Verdict is the result of grading one answer.
type Verdict struct {
	Verdict  string `json:"verdict"` // "pass" | "partial" | "fail"
	Feedback string `json:"feedback"`
}

const verdictSchema = `{
  "type": "object",
  "required": ["verdict", "feedback"],
  "properties": {
    "verdict": {"type": "string", "enum": ["pass", "partial", "fail"]},
    "feedback": {"type": "string"}
  }
}`

// GradeChoice grades an MCQ answer in pure Go.
func GradeChoice(q *model.Question, choice int) Verdict {
	if choice == q.CorrectIndex {
		return Verdict{Verdict: "pass", Feedback: q.Explanation}
	}
	return Verdict{Verdict: "fail", Feedback: q.Explanation}
}

func gradePrompt(q *model.Question) string {
	return "You are grading a learner's free-response answer for understanding. " +
		"The section text and the learner's answer are on stdin. " +
		"Question: " + q.Prompt + "\nRubric: " + q.Rubric + "\n" +
		"Grade pass, partial, or fail and give one or two sentences of feedback."
}

// GradeFree grades a free-response answer via one agent call.
func GradeFree(ctx context.Context, inv agent.Invoker, sectionMarkdown string, q *model.Question, answer string) (Verdict, error) {
	stdin := "SECTION:\n" + sectionMarkdown + "\n\nLEARNER ANSWER:\n" + answer
	job := agent.Job{Prompt: gradePrompt(q), Schema: verdictSchema, Stdin: []byte(stdin)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return Verdict{}, fmt.Errorf("grade invoke: %w", err)
	}
	var v Verdict
	if err := json.Unmarshal(resp, &v); err != nil {
		return Verdict{}, fmt.Errorf("parse verdict: %w", err)
	}
	return v, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/study/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/study/grade.go internal/study/grade_test.go
git commit -m "feat: MCQ and free-response grading"
```

---

### Task 6: Study generation + PrepareSource

**Files:**
- Create: `internal/study/generate.go`
- Test: `internal/study/generate_test.go`

**Interfaces:**
- Consumes: `agent.Invoker`, `store.Store`, `model` types.
- Produces:
  - `study.GenerateSection(ctx context.Context, inv agent.Invoker, sectionMarkdown string) (summary string, concepts []string, questions []model.Question, err error)` — one `gen-study` call; section text via `Stdin`. Returned questions have `Kind/Prompt/Options/CorrectIndex/Rubric/Explanation` set but NOT `ID`/`SectionID`/`Idx` (the caller assigns those).
  - `study.PrepareSource(ctx context.Context, inv agent.Invoker, st store.Store, src *model.Source, newID func() string, onSection func(i, total int, title string)) error` — for each section lacking a stored study package, generate one, assign ids + idx, save it; then set section 0's status to `unlocked` if it has no status yet. `onSection` (may be nil) is called before generating each section for progress feedback.
  - Package constant `studySchema` (string).

- [ ] **Step 1: Write the failing test**

Create `internal/study/generate_test.go`:
```go
package study

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func genFake() *agent.Fake {
	return &agent.Fake{Responses: map[string]json.RawMessage{
		"study package": json.RawMessage(`{
			"summary":"s",
			"key_concepts":["k1","k2"],
			"questions":[
				{"kind":"mcq","prompt":"pick","options":["a","b"],"correct_index":1,"explanation":"b"},
				{"kind":"free","prompt":"explain","rubric":"mention k1"}
			]
		}`),
	}}
}

func TestGenerateSection(t *testing.T) {
	f := genFake()
	summary, concepts, qs, err := GenerateSection(context.Background(), f, "the section markdown")
	if err != nil {
		t.Fatalf("GenerateSection: %v", err)
	}
	if summary != "s" || len(concepts) != 2 || len(qs) != 2 {
		t.Fatalf("got summary=%q concepts=%v qs=%d", summary, concepts, len(qs))
	}
	if qs[0].Kind != model.KindMCQ || qs[0].CorrectIndex != 1 || qs[1].Kind != model.KindFree {
		t.Fatalf("questions wrong: %+v", qs)
	}
	if !strings.Contains(string(f.Calls[0].Stdin), "the section markdown") {
		t.Fatalf("section text must go via stdin: %q", f.Calls[0].Stdin)
	}
}

func newStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPrepareSourceGeneratesAndUnlocksFirst(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := &model.Source{
		ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "B", Markdown: "b"},
		},
	}
	if err := st.SaveSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	n := 0
	newID := func() string { n++; return fmt.Sprintf("q%d", n) }
	calls := 0
	err := PrepareSource(ctx, genFake(), st, src, newID, func(i, total int, title string) { calls++ })
	if err != nil {
		t.Fatalf("PrepareSource: %v", err)
	}
	ok, _ := st.IsPrepared(ctx, "src1")
	if !ok {
		t.Fatal("source should be prepared")
	}
	statuses, _ := st.GetSectionStatuses(ctx, "src1")
	if statuses["s0"] != model.StatusUnlocked {
		t.Fatalf("s0 status = %q, want unlocked", statuses["s0"])
	}
	study, _ := st.GetSectionStudy(ctx, "s0")
	if study.Questions[0].SectionID != "s0" || study.Questions[0].ID == "" {
		t.Fatalf("questions must have ids and section id: %+v", study.Questions[0])
	}
	if calls != 2 {
		t.Fatalf("onSection called %d times, want 2", calls)
	}
}

func TestPrepareSourceSkipsAlreadyStudied(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := &model.Source{
		ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"}},
	}
	st.SaveSource(ctx, src)
	st.SaveSectionStudy(ctx, &model.SectionStudy{SectionID: "s0", Summary: "pre", KeyConcepts: []string{"k"}})
	calls := 0
	err := PrepareSource(ctx, genFake(), st, src, func() string { return "x" }, func(i, total int, title string) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("already-studied section should not be regenerated, calls=%d", calls)
	}
}
```

Note: the fake matches on `"study package"`; ensure `studyPrompt` contains that phrase.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/study/ -run 'Generate|Prepare'`
Expected: FAIL — `undefined: GenerateSection`, `undefined: PrepareSource`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/study/generate.go`:
```go
package study

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

const studySchema = `{
  "type": "object",
  "required": ["summary", "key_concepts", "questions"],
  "properties": {
    "summary": {"type": "string"},
    "key_concepts": {"type": "array", "items": {"type": "string"}},
    "questions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["kind", "prompt"],
        "properties": {
          "kind": {"type": "string", "enum": ["mcq", "free"]},
          "prompt": {"type": "string"},
          "options": {"type": "array", "items": {"type": "string"}},
          "correct_index": {"type": "integer"},
          "rubric": {"type": "string"},
          "explanation": {"type": "string"}
        }
      }
    }
  }
}`

func studyPrompt() string {
	return "You are building a study package for the section on stdin. Produce a short " +
		"summary, a list of key concepts, and a mix of quiz questions: at least one " +
		"multiple-choice (mcq, with options, a correct_index, and an explanation) and at " +
		"least one free-response (free, with a grading rubric). Return the study package."
}

type genResult struct {
	Summary     string   `json:"summary"`
	KeyConcepts []string `json:"key_concepts"`
	Questions   []struct {
		Kind         string   `json:"kind"`
		Prompt       string   `json:"prompt"`
		Options      []string `json:"options"`
		CorrectIndex int      `json:"correct_index"`
		Rubric       string   `json:"rubric"`
		Explanation  string   `json:"explanation"`
	} `json:"questions"`
}

// GenerateSection produces a study package for one section via one agent call.
// Returned questions do not yet have ID/SectionID/Idx set.
func GenerateSection(ctx context.Context, inv agent.Invoker, sectionMarkdown string) (string, []string, []model.Question, error) {
	job := agent.Job{Prompt: studyPrompt(), Schema: studySchema, Stdin: []byte(sectionMarkdown)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return "", nil, nil, fmt.Errorf("gen-study invoke: %w", err)
	}
	var res genResult
	if err := json.Unmarshal(resp, &res); err != nil {
		return "", nil, nil, fmt.Errorf("parse study result: %w", err)
	}
	var qs []model.Question
	for _, q := range res.Questions {
		qs = append(qs, model.Question{
			Kind: q.Kind, Prompt: q.Prompt, Options: q.Options,
			CorrectIndex: q.CorrectIndex, Rubric: q.Rubric, Explanation: q.Explanation,
		})
	}
	return res.Summary, res.KeyConcepts, qs, nil
}

// PrepareSource generates and stores study packages for every section that lacks
// one, then unlocks the first section. Resumable: already-studied sections are
// skipped. onSection (may be nil) is called before each generated section.
func PrepareSource(ctx context.Context, inv agent.Invoker, st store.Store, src *model.Source, newID func() string, onSection func(i, total int, title string)) error {
	total := len(src.Sections)
	for i, sec := range src.Sections {
		if _, err := st.GetSectionStudy(ctx, sec.ID); err == nil {
			continue // already prepared
		}
		if onSection != nil {
			onSection(i, total, sec.Title)
		}
		summary, concepts, qs, err := GenerateSection(ctx, inv, sec.Markdown)
		if err != nil {
			return fmt.Errorf("prepare section %d (%s): %w", i, sec.Title, err)
		}
		for j := range qs {
			qs[j].ID = newID()
			qs[j].SectionID = sec.ID
			qs[j].Idx = j
		}
		if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
			SectionID: sec.ID, Summary: summary, KeyConcepts: concepts, Questions: qs,
		}); err != nil {
			return fmt.Errorf("save study for section %d: %w", i, err)
		}
	}
	if total > 0 {
		statuses, err := st.GetSectionStatuses(ctx, src.ID)
		if err != nil {
			return err
		}
		if statuses[src.Sections[0].ID] == model.StatusLocked {
			if err := st.SetSectionStatus(ctx, src.Sections[0].ID, model.StatusUnlocked); err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/study/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/study/generate.go internal/study/generate_test.go
git commit -m "feat: study generation and resumable PrepareSource"
```

---

### Task 7: `tanaka prepare <id>` command + end-to-end smoke

**Files:**
- Modify: `internal/cli/cli.go` (dispatch + `cmdPrepare` + help text)
- Test: `internal/cli/prepare_test.go`

**Interfaces:**
- Consumes: `store.GetSource` (Plan 1), `study.PrepareSource` (Task 6), `agent.Invoker`, `ui.Spinner` (Plan 1), `app.NewID`.
- Produces: a `prepare` subcommand: `tanaka prepare <id>` loads the source, runs `study.PrepareSource` with per-section phase feedback, and prints a final summary. Errors: missing arg → exit 2; unknown id → friendly message, exit 1; agent/check failure → exit 1.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/prepare_test.go`:
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

func TestPrepareGeneratesStudy(t *testing.T) {
	d := testDeps(t)
	// Replace the invoker with one that answers both ingest (structure) and study.
	d.invoker = &agent.Fake{Responses: map[string]json.RawMessage{
		"sections":      json.RawMessage(`{"title":"Doc","sections":[{"title":"S1","markdown":"x"}]}`),
		"study package": json.RawMessage(`{"summary":"s","key_concepts":["k"],"questions":[{"kind":"free","prompt":"why","rubric":"r"}]}`),
	}}
	d.stdin = strings.NewReader("content")
	var out, errOut bytes.Buffer

	if code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut); code != 0 {
		t.Fatalf("add exit = %d; %s", code, errOut.String())
	}
	// testDeps newID makes the source id "id1".
	out.Reset()
	if code := run(context.Background(), []string{"prepare", "id1"}, d, &out, &errOut); code != 0 {
		t.Fatalf("prepare exit = %d; %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "prepared") {
		t.Fatalf("prepare output = %q, want it to confirm prepared", out.String())
	}
}

func TestPrepareRequiresArg(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"prepare"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit when prepare has no id")
	}
}

func TestPrepareUnknownID(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"prepare", "nope"}, d, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown id")
	}
	if !strings.Contains(errOut.String(), "no source with id") {
		t.Fatalf("stderr = %q, want 'no source with id'", errOut.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run Prepare`
Expected: FAIL — unknown command `prepare` (exit 2 from default), so `TestPrepareGeneratesStudy` fails at the prepare step.

- [ ] **Step 3: Add the dispatch case and help line**

In `internal/cli/cli.go`, add to the `run` switch (after the `remove` case):
```go
	case "prepare":
		return cmdPrepare(ctx, args[1:], d, stdout, stderr)
```
And add to `helpText` (after the `remove` line):
```go
//   prepare <id>       Generate the study package for a source (quizzes etc.)
```
(Match the existing two-space-aligned help format; it is plain text inside the const.)

- [ ] **Step 4: Implement cmdPrepare**

Add to `internal/cli/cli.go` (near `cmdRemove`):
```go
func cmdPrepare(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka prepare <id>")
		return 2
	}
	if err := d.invoker.Check(ctx); err != nil {
		fmt.Fprintf(stderr, "claude CLI unavailable: %v\nis it installed and logged in? try: claude login\n", err)
		return 1
	}
	src, err := d.store.GetSource(ctx, args[0])
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fmt.Fprintf(stderr, "no source with id %s (use 'tanaka list' to see ids)\n", args[0])
			return 1
		}
		fmt.Fprintf(stderr, "prepare: %v\n", err)
		return 1
	}
	onSection := func(i, total int, title string) {
		fmt.Fprintf(stderr, "preparing section %d/%d: %s\n", i+1, total, title)
	}
	if err := study.PrepareSource(ctx, d.invoker, d.store, src, d.newID, onSection); err != nil {
		fmt.Fprintf(stderr, "prepare: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "prepared %q (%d sections)\n", src.Title, len(src.Sections))
	return 0
}
```
Add imports `"github.com/devandbenz/tanaka/internal/study"` to `internal/cli/cli.go` (`errors`, `store` are already imported from the remove command).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/`
Expected: PASS (prepare tests + existing).

- [ ] **Step 6: Run the full suite and vet**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/
git commit -m "feat: tanaka prepare command for study generation"
```

- [ ] **Step 8: Manual end-to-end smoke (real claude)**

Run (uses an already-added source; run `tanaka list` first to get an id):
```bash
go build -o /tmp/tanaka .
/tmp/tanaka list
/tmp/tanaka prepare <id-of-a-small-source>
```
Expected: prints `preparing section i/N` lines, then `prepared "..." (N sections)`. (Spends a small amount of Claude usage per section.)

---

## Self-Review

**Spec coverage (Plan 2a scope = study data + domain):**
- Study tables (section_study, questions, section_progress) — Tasks 2, 3. ✓
- `gen-study` per section, content via stdin — Task 6. ✓
- `grade-answer` free-response + MCQ pure-Go grading — Task 5. ✓
- Gating pure function — Task 4. ✓
- Prepared detection + progress + question lookup — Task 3. ✓
- Lazy/resumable preparation (`PrepareSource` skips studied sections) — Task 6. ✓
- `remove` cascade still wipes study rows (FK cascade in schema) — Task 2. ✓
- Domain types — Task 1. ✓
- Out of scope (deferred to Plan 2b): `serve`, HTTP handlers, templates, 98.css, markdown rendering, the in-UI prepare/grade/skip flow. The `prepare` CLI command (Task 7) is the 2a end-to-end check; the web layer reuses `study.PrepareSource`, `study.GradeChoice`, `study.GradeFree`, `study.ComputeUnlocked`, and the new store methods.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. ✓

**Type consistency:** `model.Question`/`model.SectionStudy` fields and status/kind constants used identically across store, study, and cli tasks. `store.Store` additions (`SaveSectionStudy`, `GetSectionStudy`, `IsPrepared`, `GetSectionStatuses`, `SetSectionStatus`, `GetQuestion`) match their call sites. `study.GenerateSection`/`PrepareSource`/`GradeChoice`/`GradeFree`/`ComputeUnlocked` signatures match their callers and tests. `agent.Job{Stdin, Schema, Prompt}` reused as defined in Plan 1. ✓

---

## Follow-on Plan (not in this document)

- **Plan 2b — serve & study UI:** `internal/web` (server, routing, embedded `html/template` + `98.css` + `app.js`), markdown→HTML (add `goldmark`, pure-Go), the `serve` command, source-list and section pages, the in-UI prepare/grade/skip flow and gating, all reusing the Plan 2a domain. Note: Plan 2b relaxes the dependency constraint to also allow `github.com/yuin/goldmark`.
