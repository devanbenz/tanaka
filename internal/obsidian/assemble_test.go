package obsidian

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

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
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a"},
		Questions: []model.Question{
			{ID: "q0", SectionID: "sec0", Idx: 0, Kind: model.KindMCQ,
				Prompt: "pick", Options: []string{"x", "y"}, CorrectIndex: 1, Explanation: "y!"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveQuestionProgress(ctx, "q0", "pass", "", 1, "nice"); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestAssemble(t *testing.T) {
	st := seededStore(t)
	exp, err := Assemble(context.Background(), st, "src1")
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if exp.Source.Title != "My Paper" || len(exp.Source.Sections) != 2 {
		t.Fatalf("source wrong: %+v", exp.Source)
	}
	if exp.Studies["sec0"] == nil || exp.Studies["sec0"].Summary != "sum" {
		t.Fatalf("study missing: %+v", exp.Studies)
	}
	if _, ok := exp.Studies["sec1"]; ok {
		t.Fatal("no-study section should be absent from Studies")
	}
	p := exp.Progress["sec0"]["q0"]
	if p.Verdict != "pass" || p.Choice != 1 || p.Feedback != "nice" {
		t.Fatalf("progress wrong: %+v", p)
	}
	if exp.ExportedAt.IsZero() {
		t.Fatal("ExportedAt not set")
	}
}

func TestAssembleUnknownSource(t *testing.T) {
	st := seededStore(t)
	if _, err := Assemble(context.Background(), st, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
