package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

// seqCounter is package-level so IDs remain unique across multiple seqIDs()
// closures within the same test binary run.
var seqCounter int64

func seqIDs() func() string {
	return func() string {
		n := atomic.AddInt64(&seqCounter, 1)
		return fmt.Sprintf("new%d", n)
	}
}

func sampleSheet() *model.Sheet {
	return &model.Sheet{
		Format: model.SheetFormat, Version: model.SheetVersion,
		Source: model.SheetSource{
			Title: "Imported", Origin: "http://src",
			Sections: []model.SheetSection{
				{Idx: 0, Title: "Intro", Markdown: "# hi", Study: &model.SheetStudy{
					Summary: "sum", KeyConcepts: []string{"a", "b"},
					Questions: []model.SheetQuestion{
						{Idx: 0, Kind: model.KindMCQ, Prompt: "pick", Options: []string{"x", "y"}, CorrectIndex: 1, Explanation: "because y"},
						{Idx: 1, Kind: model.KindFree, Prompt: "explain", Rubric: "mentions a"},
					},
				}},
				{Idx: 1, Title: "No quiz", Markdown: "body", Study: nil},
			},
		},
	}
}

func TestImportSheetCreatesSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, err := s.ImportSheet(ctx, sampleSheet(), seqIDs())
	if err != nil {
		t.Fatalf("ImportSheet: %v", err)
	}
	if id == "" {
		t.Fatal("empty new source id")
	}
	src, err := s.GetSource(ctx, id)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if src.Title != "Imported" || src.Origin != "http://src" || len(src.Sections) != 2 {
		t.Fatalf("source wrong: %+v", src)
	}
	// Section 0 has a study package with 2 questions.
	study, err := s.GetSectionStudy(ctx, src.Sections[0].ID)
	if err != nil {
		t.Fatalf("GetSectionStudy sec0: %v", err)
	}
	if study.Summary != "sum" || len(study.Questions) != 2 || study.Questions[0].Options[1] != "y" {
		t.Fatalf("study wrong: %+v", study)
	}
	// Section 1 has no study package.
	if _, err := s.GetSectionStudy(ctx, src.Sections[1].ID); err == nil {
		t.Fatal("section 1 should have no study")
	}
	// No progress rows were written.
	prog, err := s.GetSectionProgress(ctx, src.Sections[0].ID)
	if err != nil {
		t.Fatalf("GetSectionProgress: %v", err)
	}
	if len(prog) != 0 {
		t.Fatalf("expected no progress, got %d rows", len(prog))
	}
}

func TestImportSheetTwiceCreatesTwoSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id1, err := s.ImportSheet(ctx, sampleSheet(), seqIDs())
	if err != nil {
		t.Fatalf("import 1: %v", err)
	}
	id2, err := s.ImportSheet(ctx, sampleSheet(), seqIDs())
	if err != nil {
		t.Fatalf("import 2: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected distinct source ids, both = %s", id1)
	}
	all, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(all))
	}
}
