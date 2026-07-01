package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func fixture() *Export {
	return &Export{
		ExportedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Source: &model.Source{
			ID: "src1", Title: "My: Paper?", Origin: "http://x",
			Sections: []model.Section{
				{ID: "sec0", Idx: 0, Title: "Intro", Markdown: "# hello"},
				{ID: "sec1", Idx: 1, Title: "Deep Dive", Markdown: "body"},
				{ID: "sec2", Idx: 2, Title: "No Study", Markdown: "raw"},
			},
		},
		Studies: map[string]*model.SectionStudy{
			"sec0": {SectionID: "sec0", Summary: "s0", KeyConcepts: []string{"Self-Attention", "gradients"},
				Questions: []model.Question{
					{ID: "q0", Idx: 0, Kind: model.KindMCQ, Prompt: "What uses self-attention?",
						Options: []string{"CNN", "Transformer"}, CorrectIndex: 1, Explanation: "Transformers do."},
					{ID: "q1", Idx: 1, Kind: model.KindFree, Prompt: "Explain gradients.",
						Rubric: "mentions slope", Explanation: "The slope."},
				}},
			"sec1": {SectionID: "sec1", Summary: "s1", KeyConcepts: []string{"self-attention"},
				Questions: []model.Question{
					{ID: "q2", Idx: 0, Kind: model.KindFree, Prompt: "Why?", Rubric: "r"},
				}},
		},
		Progress: map[string]map[string]model.QuestionProgress{
			"sec0": {"q0": {Verdict: "pass", Choice: 1, Feedback: "correct"}},
		},
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestWriteTreeAndLinks(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, fixture()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	hub := read(t, filepath.Join(dir, "My Paper.md"))
	for _, want := range []string{"origin:", "exported: 2026-07-01", "[[01 Intro]]", "[[02 Deep Dive]]", "[[03 No Study]]"} {
		if !strings.Contains(hub, want) {
			t.Errorf("hub missing %q:\n%s", want, hub)
		}
	}

	sec0 := read(t, filepath.Join(dir, "sections", "01 Intro.md"))
	for _, want := range []string{`source: "[[My Paper]]"`, "## Summary", "s0",
		"[[Self-Attention]]", "[[gradients]]", "## Content", "# hello",
		"[[01 Intro Q1]]", "[[01 Intro Q2]]"} {
		if !strings.Contains(sec0, want) {
			t.Errorf("section 01 missing %q:\n%s", want, sec0)
		}
	}

	// Section without a study package: content only.
	sec2 := read(t, filepath.Join(dir, "sections", "03 No Study.md"))
	if strings.Contains(sec2, "## Summary") || strings.Contains(sec2, "## Questions") {
		t.Errorf("no-study section has study headings:\n%s", sec2)
	}
	if !strings.Contains(sec2, "raw") {
		t.Errorf("no-study section missing content:\n%s", sec2)
	}

	// MCQ question: options unmarked, answer in collapsed callout, attempt present.
	q0 := read(t, filepath.Join(dir, "questions", "01 Intro Q1.md"))
	for _, want := range []string{`section: "[[01 Intro]]"`, "kind: mcq", "verdict: pass",
		"1. CNN\n2. Transformer", "> [!success]- Answer", "> **Correct:** Transformer",
		"> Transformers do.", "## My attempt", "**Answer:** Transformer",
		"**Verdict:** pass", "**Feedback:** correct",
		"## Related concepts", "[[Self-Attention]]"} {
		if !strings.Contains(q0, want) {
			t.Errorf("q0 missing %q:\n%s", want, q0)
		}
	}
	// The correct answer must not be marked in the visible options list.
	if strings.Contains(strings.Split(q0, "> [!success]")[0], "Correct") {
		t.Errorf("answer leaked outside callout:\n%s", q0)
	}

	// Free question, unanswered: no verdict, no attempt, rubric in callout.
	q1 := read(t, filepath.Join(dir, "questions", "01 Intro Q2.md"))
	if strings.Contains(q1, "verdict:") || strings.Contains(q1, "## My attempt") {
		t.Errorf("unanswered question has progress artifacts:\n%s", q1)
	}
	for _, want := range []string{"kind: free", "> **Rubric:** mentions slope", "[[gradients]]"} {
		if !strings.Contains(q1, want) {
			t.Errorf("q1 missing %q:\n%s", want, q1)
		}
	}

	// Concept dedup: sec1's "self-attention" merged into first-seen casing.
	if _, err := os.Stat(filepath.Join(dir, "concepts", "self-attention.md")); err == nil {
		t.Error("lowercase duplicate concept file exists")
	}
	con := read(t, filepath.Join(dir, "concepts", "Self-Attention.md"))
	for _, want := range []string{"Appears in:", "[[01 Intro]]", "[[02 Deep Dive]]"} {
		if !strings.Contains(con, want) {
			t.Errorf("concept missing %q:\n%s", want, con)
		}
	}
}

func TestWriteIdempotent(t *testing.T) {
	dir := t.TempDir()
	exp := fixture()
	if err := Write(dir, exp); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	first := read(t, filepath.Join(dir, "questions", "01 Intro Q1.md"))
	if err := Write(dir, exp); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	second := read(t, filepath.Join(dir, "questions", "01 Intro Q1.md"))
	if first != second {
		t.Error("Write is not idempotent")
	}
}
