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

func TestDeleteSourceCascadesStudyTables(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	if err := s.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec1", Summary: "x", KeyConcepts: []string{"k"},
		Questions: []model.Question{{ID: "q1", SectionID: "sec1", Idx: 0, Kind: model.KindFree, Prompt: "p", Rubric: "r"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSectionStatus(ctx, "sec1", model.StatusPassed); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSource(ctx, "src1"); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	if _, err := s.GetSectionStudy(ctx, "sec1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("section_study should be gone, err = %v", err)
	}
	if _, err := s.GetQuestion(ctx, "q1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("question should be gone, err = %v", err)
	}
	statuses, err := s.GetSectionStatuses(ctx, "src1")
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 0 {
		t.Fatalf("section_progress should be gone, got %d rows", len(statuses))
	}
}
