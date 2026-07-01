package store

import (
	"context"
	"errors"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestGetSection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	sec, err := s.GetSection(ctx, "sec1")
	if err != nil {
		t.Fatalf("GetSection: %v", err)
	}
	if sec.SourceID != "src1" || sec.Markdown != "m" {
		t.Fatalf("section = %+v", sec)
	}
	if _, err := s.GetSection(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing section err = %v, want ErrNotFound", err)
	}
}

func TestSectionSatisfied(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	// No questions yet -> satisfied (auto-pass).
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); !ok {
		t.Fatal("section with no questions should be satisfied")
	}
	// Add two questions.
	err := s.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec1", Summary: "x", KeyConcepts: []string{"k"},
		Questions: []model.Question{
			{ID: "q1", SectionID: "sec1", Idx: 0, Kind: model.KindFree, Prompt: "p", Rubric: "r"},
			{ID: "q2", SectionID: "sec1", Idx: 1, Kind: model.KindMCQ, Prompt: "p", Options: []string{"a"}, CorrectIndex: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); ok {
		t.Fatal("unanswered questions -> not satisfied")
	}
	if err := s.SaveQuestionProgress(ctx, "q1", "pass", "", -1, ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); ok {
		t.Fatal("one of two answered -> not satisfied")
	}
	if err := s.SaveQuestionProgress(ctx, "q2", "partial", "", -1, ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); !ok {
		t.Fatal("all answered non-fail -> satisfied")
	}
	// A fail makes it unsatisfied.
	if err := s.SaveQuestionProgress(ctx, "q2", "fail", "", -1, ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SectionSatisfied(ctx, "sec1"); ok {
		t.Fatal("a fail verdict -> not satisfied")
	}
}
