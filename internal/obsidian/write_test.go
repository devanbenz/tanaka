package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

// fixture mirrors Assemble's output shape: every question present has a
// pass/partial verdict and every section has at least one such question.
func fixture() *Export {
	return &Export{
		ExportedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Source: &model.Source{
			ID: "src1", Title: "My: Paper?", Origin: "http://x",
			Sections: []model.Section{
				{ID: "sec0", Idx: 0, Title: "Intro", Markdown: "# hello"},
				{ID: "sec1", Idx: 1, Title: "Deep Dive", Markdown: "body"},
			},
		},
		Studies: map[string]*model.SectionStudy{
			"sec0": {SectionID: "sec0", Summary: "s0", KeyConcepts: []string{"Self-Attention", "gradients"},
				Questions: []model.Question{
					{ID: "q0", Idx: 0, Kind: model.KindMCQ, Prompt: "What uses self-attention?",
						Options: []string{"CNN", "Transformer"}, CorrectIndex: 1, Explanation: "Transformers do."},
				}},
			"sec1": {SectionID: "sec1", Summary: "s1", KeyConcepts: []string{"self-attention"},
				Questions: []model.Question{
					{ID: "q2", Idx: 0, Kind: model.KindFree, Prompt: "Explain gradients.",
						Rubric: "mentions slope", Explanation: "The slope."},
				}},
		},
		Progress: map[string]map[string]model.QuestionProgress{
			"sec0": {"q0": {Verdict: "pass", Choice: 1, Feedback: "correct"}},
			"sec1": {"q2": {Verdict: "partial", Answer: "slopes", Choice: -1, Feedback: "close"}},
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
	for _, want := range []string{"origin:", "exported: 2026-07-01", "[[01 Intro]]", "[[02 Deep Dive]]"} {
		if !strings.Contains(hub, want) {
			t.Errorf("hub missing %q:\n%s", want, hub)
		}
	}

	sec0 := read(t, filepath.Join(dir, "sections", "01 Intro.md"))
	for _, want := range []string{`source: "[[My Paper]]"`, "## Summary", "s0",
		"[[Self-Attention]]", "[[gradients]]", "[[01 Intro Q1]]"} {
		if !strings.Contains(sec0, want) {
			t.Errorf("section 01 missing %q:\n%s", want, sec0)
		}
	}
	// Section notes must not embed the original source markdown.
	if strings.Contains(sec0, "## Content") || strings.Contains(sec0, "# hello") {
		t.Errorf("section 01 embeds source content:\n%s", sec0)
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

	// Free question with a partial verdict: rubric in callout, attempt present.
	q2 := read(t, filepath.Join(dir, "questions", "02 Deep Dive Q1.md"))
	for _, want := range []string{"kind: free", "verdict: partial",
		"> **Rubric:** mentions slope", "**Answer:** slopes", "**Verdict:** partial"} {
		if !strings.Contains(q2, want) {
			t.Errorf("q2 missing %q:\n%s", want, q2)
		}
	}

	// Section 02: concept links use canonical casing from first-seen dedup.
	sec1 := read(t, filepath.Join(dir, "sections", "02 Deep Dive.md"))
	if !strings.Contains(sec1, "[[Self-Attention]]") {
		t.Errorf("section 02 missing canonical [[Self-Attention]]:\n%s", sec1)
	}
	if strings.Contains(sec1, "[[self-attention]]") {
		t.Errorf("section 02 has non-canonical [[self-attention]]:\n%s", sec1)
	}

	// Concept dedup: sec1's "self-attention" merged into first-seen casing.
	// Compare directory entries by exact name — os.Stat would match the
	// canonical file case-insensitively on macOS (APFS).
	entries, err := os.ReadDir(filepath.Join(dir, "concepts"))
	if err != nil {
		t.Fatalf("read concepts dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() == "self-attention.md" {
			t.Error("lowercase duplicate concept file exists")
		}
	}
	con := read(t, filepath.Join(dir, "concepts", "Self-Attention.md"))
	for _, want := range []string{"Appears in:", "[[01 Intro]]", "[[02 Deep Dive]]"} {
		if !strings.Contains(con, want) {
			t.Errorf("concept missing %q:\n%s", want, con)
		}
	}
}

// An export with no sections (nothing completed yet) writes nothing at all.
func TestWriteEmptyExport(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	exp := &Export{
		ExportedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Source:     &model.Source{ID: "src1", Title: "My Paper", Origin: "http://x"},
		Studies:    map[string]*model.SectionStudy{},
		Progress:   map[string]map[string]model.QuestionProgress{},
	}
	if err := Write(dir, exp); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("empty export should create nothing, stat err = %v", err)
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
