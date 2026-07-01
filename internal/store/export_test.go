package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestExportSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	err := s.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "http://x", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "sec0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hi"},
			{ID: "sec1", SourceID: "src1", Idx: 1, Title: "No quiz", Markdown: "body"},
		},
	})
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	// Study for sec0 only; sec1 stays without a quiz.
	if err := s.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a", "b"},
		Questions: []model.Question{
			{ID: "q0", SectionID: "sec0", Idx: 0, Kind: model.KindMCQ, Prompt: "pick", Options: []string{"x", "y"}, CorrectIndex: 1, Explanation: "because y"},
			{ID: "q1", SectionID: "sec0", Idx: 1, Kind: model.KindFree, Prompt: "explain", Rubric: "mentions a"},
		},
	}); err != nil {
		t.Fatalf("SaveSectionStudy: %v", err)
	}

	sheet, err := s.ExportSource(ctx, "src1")
	if err != nil {
		t.Fatalf("ExportSource: %v", err)
	}
	if sheet.Format != model.SheetFormat || sheet.Version != model.SheetVersion {
		t.Fatalf("envelope not set: %+v", sheet)
	}
	if sheet.Source.Title != "My Paper" || sheet.Source.Origin != "http://x" {
		t.Fatalf("source meta wrong: %+v", sheet.Source)
	}
	if len(sheet.Source.Sections) != 2 {
		t.Fatalf("want 2 sections, got %d", len(sheet.Source.Sections))
	}
	s0 := sheet.Source.Sections[0]
	if s0.Title != "Intro" || s0.Markdown != "# hi" || s0.Study == nil {
		t.Fatalf("section 0 wrong: %+v", s0)
	}
	if s0.Study.Summary != "sum" || len(s0.Study.KeyConcepts) != 2 || len(s0.Study.Questions) != 2 {
		t.Fatalf("study wrong: %+v", s0.Study)
	}
	if s0.Study.Questions[0].Options[1] != "y" || s0.Study.Questions[0].CorrectIndex != 1 {
		t.Fatalf("mcq wrong: %+v", s0.Study.Questions[0])
	}
	if sheet.Source.Sections[1].Study != nil {
		t.Fatalf("section 1 should have no study: %+v", sheet.Source.Sections[1].Study)
	}
}

func TestExportSourceNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ExportSource(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
