package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

// seedTwoQuestions creates src1/sec1 with one free-response and one MCQ question.
func seedTwoQuestions(t *testing.T, s Store) {
	t.Helper()
	sourceWithSection(t, s, "src1", "sec1")
	err := s.SaveSectionStudy(context.Background(), &model.SectionStudy{
		SectionID: "sec1", Summary: "x", KeyConcepts: []string{"k"},
		Questions: []model.Question{
			{ID: "qf", SectionID: "sec1", Idx: 0, Kind: model.KindFree, Prompt: "p", Rubric: "r"},
			{ID: "qm", SectionID: "sec1", Idx: 1, Kind: model.KindMCQ, Prompt: "p", Options: []string{"a", "b"}, CorrectIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("SaveSectionStudy: %v", err)
	}
}

func TestSaveAndGetSectionProgress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedTwoQuestions(t, s)

	// Free-response: answer text, no choice.
	if err := s.SaveQuestionProgress(ctx, "qf", "pass", "my written answer", -1, "well reasoned"); err != nil {
		t.Fatalf("SaveQuestionProgress qf: %v", err)
	}
	// MCQ: selected choice index, no answer text.
	if err := s.SaveQuestionProgress(ctx, "qm", "fail", "", 0, "wrong, see explanation"); err != nil {
		t.Fatalf("SaveQuestionProgress qm: %v", err)
	}

	prog, err := s.GetSectionProgress(ctx, "sec1")
	if err != nil {
		t.Fatalf("GetSectionProgress: %v", err)
	}
	qf, ok := prog["qf"]
	if !ok {
		t.Fatal("qf missing from progress")
	}
	if qf.Verdict != "pass" || qf.Answer != "my written answer" || qf.Choice != -1 || qf.Feedback != "well reasoned" {
		t.Fatalf("qf progress = %+v", qf)
	}
	qm, ok := prog["qm"]
	if !ok {
		t.Fatal("qm missing from progress")
	}
	if qm.Verdict != "fail" || qm.Answer != "" || qm.Choice != 0 || qm.Feedback != "wrong, see explanation" {
		t.Fatalf("qm progress = %+v", qm)
	}
}

func TestGetSectionProgressOmitsUnanswered(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedTwoQuestions(t, s)
	if err := s.SaveQuestionProgress(ctx, "qf", "pass", "answered", -1, "ok"); err != nil {
		t.Fatal(err)
	}
	prog, err := s.GetSectionProgress(ctx, "sec1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prog["qm"]; ok {
		t.Fatal("unanswered qm should not appear in progress map")
	}
	if len(prog) != 1 {
		t.Fatalf("progress has %d entries, want 1", len(prog))
	}
}

func TestSaveQuestionProgressUpsertsLatest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedTwoQuestions(t, s)

	if err := s.SaveQuestionProgress(ctx, "qf", "fail", "first try", -1, "no"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveQuestionProgress(ctx, "qf", "pass", "second try", -1, "yes"); err != nil {
		t.Fatalf("second save should upsert: %v", err)
	}
	prog, _ := s.GetSectionProgress(ctx, "sec1")
	got := prog["qf"]
	if got.Verdict != "pass" || got.Answer != "second try" || got.Feedback != "yes" {
		t.Fatalf("latest not kept: %+v", got)
	}
}

// TestMigrateAddsAnswerColumns proves an existing database with the old
// two-column question_progress is migrated in place: new columns are added,
// and the old verdict row survives with sensible defaults.
func TestMigrateAddsAnswerColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// Simulate a pre-existing DB carrying only the old schema for the row we care about.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	seed := []string{
		`CREATE TABLE questions (id TEXT PRIMARY KEY, section_id TEXT, idx INTEGER, kind TEXT, prompt TEXT, options TEXT, correct_index INTEGER, rubric TEXT, explanation TEXT);`,
		`INSERT INTO questions (id, section_id, idx, kind, prompt) VALUES ('q1','s1',0,'free','p');`,
		`CREATE TABLE question_progress (question_id TEXT PRIMARY KEY, verdict TEXT NOT NULL);`,
		`INSERT INTO question_progress (question_id, verdict) VALUES ('q1','pass');`,
	}
	for _, st := range seed {
		if _, err := raw.Exec(st); err != nil {
			t.Fatalf("seed %q: %v", st, err)
		}
	}
	raw.Close()

	// Reopen through NewSQLite, which must migrate the existing table in place.
	s, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite (migrate): %v", err)
	}
	t.Cleanup(func() { s.Close() })

	prog, err := s.GetSectionProgress(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetSectionProgress after migrate: %v", err)
	}
	p, ok := prog["q1"]
	if !ok {
		t.Fatal("q1 progress lost after migration")
	}
	if p.Verdict != "pass" || p.Answer != "" || p.Choice != -1 || p.Feedback != "" {
		t.Fatalf("migrated row = %+v, want verdict=pass with empty defaults", p)
	}
}
