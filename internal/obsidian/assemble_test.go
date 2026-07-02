package obsidian

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

// seededStore builds a source exercising every filtering rule:
// sec0 has questions in all progress states (pass, fail, partial, unanswered),
// sec1 has no study package, sec2 has a study but no progress at all.
func seededStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if err := st.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "http://x", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "sec0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hi"},
			{ID: "sec1", SourceID: "src1", Idx: 1, Title: "No Study", Markdown: "raw"},
			{ID: "sec2", SourceID: "src1", Idx: 2, Title: "Untouched", Markdown: "later"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a"},
		Questions: []model.Question{
			{ID: "q0", SectionID: "sec0", Idx: 0, Kind: model.KindMCQ,
				Prompt: "pick", Options: []string{"x", "y"}, CorrectIndex: 1, Explanation: "y!"},
			{ID: "q1", SectionID: "sec0", Idx: 1, Kind: model.KindFree, Prompt: "flunked", Rubric: "r"},
			{ID: "q2", SectionID: "sec0", Idx: 2, Kind: model.KindFree, Prompt: "half", Rubric: "r"},
			{ID: "q3", SectionID: "sec0", Idx: 3, Kind: model.KindFree, Prompt: "untried", Rubric: "r"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec2", Summary: "s2", KeyConcepts: []string{"b"},
		Questions: []model.Question{
			{ID: "q4", SectionID: "sec2", Idx: 0, Kind: model.KindFree, Prompt: "later", Rubric: "r"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveQuestionProgress(ctx, "q0", "pass", "", 1, "nice"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveQuestionProgress(ctx, "q1", "fail", "nope", -1, "wrong"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveQuestionProgress(ctx, "q2", "partial", "half right", -1, "close"); err != nil {
		t.Fatal(err)
	}
	return st
}

// Assemble includes only questions with a pass/partial verdict, only sections
// holding at least one such question, and progress only for included questions.
func TestAssembleFiltersToCompletedQuestions(t *testing.T) {
	st := seededStore(t)
	exp, err := Assemble(context.Background(), st, "src1")
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if exp.Source.Title != "My Paper" {
		t.Fatalf("source wrong: %+v", exp.Source)
	}
	if len(exp.Source.Sections) != 1 || exp.Source.Sections[0].ID != "sec0" {
		t.Fatalf("want only sec0 included, got %+v", exp.Source.Sections)
	}
	study := exp.Studies["sec0"]
	if study == nil || study.Summary != "sum" {
		t.Fatalf("study missing: %+v", exp.Studies)
	}
	if len(study.Questions) != 2 || study.Questions[0].ID != "q0" || study.Questions[1].ID != "q2" {
		t.Fatalf("want questions [q0 q2], got %+v", study.Questions)
	}
	if _, ok := exp.Studies["sec2"]; ok {
		t.Fatal("no-progress section should be absent from Studies")
	}
	prog := exp.Progress["sec0"]
	if len(prog) != 2 {
		t.Fatalf("want progress for q0 and q2 only, got %+v", prog)
	}
	if p := prog["q0"]; p.Verdict != "pass" || p.Choice != 1 || p.Feedback != "nice" {
		t.Fatalf("q0 progress wrong: %+v", p)
	}
	if p := prog["q2"]; p.Verdict != "partial" {
		t.Fatalf("q2 progress wrong: %+v", p)
	}
	if exp.ExportedAt.IsZero() {
		t.Fatal("ExportedAt not set")
	}
}

// A source with no passing answers assembles to an empty export.
func TestAssembleNoProgress(t *testing.T) {
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if err := st.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "http://x", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "sec0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hi"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a"},
		Questions: []model.Question{
			{ID: "q0", SectionID: "sec0", Idx: 0, Kind: model.KindFree, Prompt: "why", Rubric: "r"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	exp, err := Assemble(ctx, st, "src1")
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if len(exp.Source.Sections) != 0 || len(exp.Studies) != 0 || len(exp.Progress) != 0 {
		t.Fatalf("want empty export, got sections=%+v studies=%+v progress=%+v",
			exp.Source.Sections, exp.Studies, exp.Progress)
	}
}

func TestAssembleUnknownSource(t *testing.T) {
	st := seededStore(t)
	if _, err := Assemble(context.Background(), st, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
