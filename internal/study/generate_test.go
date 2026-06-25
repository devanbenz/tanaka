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
