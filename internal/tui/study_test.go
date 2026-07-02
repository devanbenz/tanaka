package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/obsidian"
	"github.com/devandbenz/tanaka/internal/store"
)

// preparedDeps seeds a two-section source whose studies are generated:
// s0 has an MCQ (correct answer 1) and a free question; s1 has one MCQ.
// s0 starts unlocked, matching PrepareSource's behavior.
func preparedDeps(t *testing.T) Deps {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if err := st.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hello\n\nwords"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "Deep Dive", Markdown: "body"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "s0", Summary: "sum", KeyConcepts: []string{"gradients"},
		Questions: []model.Question{
			{ID: "q0", SectionID: "s0", Idx: 0, Kind: model.KindMCQ,
				Prompt: "What uses self-attention?", Options: []string{"CNN", "Transformer"}, CorrectIndex: 1, Explanation: "Transformers do."},
			{ID: "q1", SectionID: "s0", Idx: 1, Kind: model.KindFree, Prompt: "Explain gradients.", Rubric: "mentions slope"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "s1", Summary: "s1", KeyConcepts: nil,
		Questions: []model.Question{
			{ID: "q2", SectionID: "s1", Idx: 0, Kind: model.KindMCQ,
				Prompt: "Why?", Options: []string{"Because", "No"}, CorrectIndex: 0, Explanation: "yes"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSectionStatus(ctx, "s0", model.StatusUnlocked); err != nil {
		t.Fatal(err)
	}
	inv := &agent.Fake{Responses: map[string]json.RawMessage{
		// "grading a learner" is unique to the grade prompt; a bare "grading"
		// would also match the generation prompt's "grading rubric".
		"grading a learner": json.RawMessage(`{"verdict":"partial","feedback":"close"}`),
		"study package":     json.RawMessage(`{"summary":"s","key_concepts":["k"],"questions":[{"kind":"mcq","prompt":"pick","options":["a","b"],"correct_index":0,"explanation":"a!"}]}`),
	}}
	return Deps{Store: st, Inv: inv, NewID: newSeq(), Syncer: obsidian.NewSyncer(st, t.TempDir(), discardTestLog())}
}

func newSeq() func() string {
	n := 0
	return func() string { n++; return "id" + string(rune('0'+n)) }
}

func loadedStudy(t *testing.T, d Deps) screen {
	t.Helper()
	var s screen = newStudy(d, "src1")
	s = drive(t, s, tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd := s.Init(); cmd != nil {
		s = drive(t, s, cmd())
	}
	return s
}

func TestStudyShowsSectionAndSidebar(t *testing.T) {
	s := loadedStudy(t, preparedDeps(t))
	view := s.View()
	for _, want := range []string{"Intro", "Deep Dive", "What uses self-attention?", "CNN", "Transformer", "1/2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("study view missing %q:\n%s", want, view)
		}
	}
}

func TestStudyMCQWrongThenRightAnswer(t *testing.T) {
	d := preparedDeps(t)
	s := loadedStudy(t, d)
	// Wrong answer: cursor starts at option 0 (CNN), grade it.
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(s.View(), "fail") {
		t.Fatalf("wrong MCQ answer should show fail:\n%s", s.View())
	}
	// Right answer: move to option 1 and grade.
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyDown})
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(s.View(), "pass") {
		t.Fatalf("correct MCQ answer should show pass:\n%s", s.View())
	}
	p, err := d.Store.GetSectionProgress(context.Background(), "s0")
	if err != nil {
		t.Fatal(err)
	}
	if p["q0"].Verdict != "pass" || p["q0"].Choice != 1 {
		t.Fatalf("progress not saved: %+v", p["q0"])
	}
}

func TestStudyFreeResponseGrading(t *testing.T) {
	d := preparedDeps(t)
	s := loadedStudy(t, d)
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyRight}) // focus question 2 (free)
	s = drive(t, s, runes("slopes"))                // type into the textarea
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyCtrlS}) // submit
	if !strings.Contains(s.View(), "partial") || !strings.Contains(s.View(), "close") {
		t.Fatalf("free grading verdict missing:\n%s", s.View())
	}
	p, err := d.Store.GetSectionProgress(context.Background(), "s0")
	if err != nil {
		t.Fatal(err)
	}
	if p["q1"].Verdict != "partial" || p["q1"].Answer != "slopes" {
		t.Fatalf("free progress not saved: %+v", p["q1"])
	}
}

// Passing every question in a section marks it passed, unlocks the next, and
// syncs the Obsidian vault.
func TestStudySectionPassUnlocksNext(t *testing.T) {
	d := preparedDeps(t)
	s := loadedStudy(t, d)
	// q0: pick Transformer (option 1) -> pass.
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyDown})
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyEnter})
	// q1 (free): answer and submit -> partial (non-failing).
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyRight})
	s = drive(t, s, runes("slopes"))
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyCtrlS})

	statuses, err := d.Store.GetSectionStatuses(context.Background(), "src1")
	if err != nil {
		t.Fatal(err)
	}
	if statuses["s0"] != model.StatusPassed || statuses["s1"] != model.StatusUnlocked {
		t.Fatalf("statuses = %v, want s0 passed and s1 unlocked", statuses)
	}
	if !strings.Contains(s.View(), "[x]") {
		t.Fatalf("sidebar should mark passed section:\n%s", s.View())
	}
	d.Syncer.Drain()
}

func TestStudySkipMovesToNextSection(t *testing.T) {
	d := preparedDeps(t)
	s := loadedStudy(t, d)
	s = drive(t, s, runes("s"))
	statuses, err := d.Store.GetSectionStatuses(context.Background(), "src1")
	if err != nil {
		t.Fatal(err)
	}
	if statuses["s0"] != model.StatusSkipped {
		t.Fatalf("s0 = %q, want skipped", statuses["s0"])
	}
	if !strings.Contains(s.View(), "Why?") {
		t.Fatalf("skip should land on the next section's quiz:\n%s", s.View())
	}
}

func TestStudyEscReturnsHome(t *testing.T) {
	s := loadedStudy(t, preparedDeps(t))
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := s.(home); !ok {
		t.Fatalf("esc should return the home screen, got %T", s)
	}
}

// An unprepared source offers p to generate quizzes; generation runs through
// the fake agent and lands on the first section.
func TestStudyPrepareFlow(t *testing.T) {
	d := preparedDeps(t)
	ctx := context.Background()
	if err := d.Store.SaveSource(ctx, &model.Source{
		ID: "src2", Title: "Fresh", Origin: "o", CreatedAt: time.Unix(2, 0),
		Sections: []model.Section{{ID: "f0", SourceID: "src2", Idx: 0, Title: "Only", Markdown: "m"}},
	}); err != nil {
		t.Fatal(err)
	}
	var s screen = newStudy(d, "src2")
	s = drive(t, s, tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd := s.Init(); cmd != nil {
		s = drive(t, s, cmd())
	}
	if !strings.Contains(s.View(), "press p") {
		t.Fatalf("unprepared source should offer prepare:\n%s", s.View())
	}
	s = drive(t, s, runes("p"))
	if !strings.Contains(s.View(), "pick") {
		t.Fatalf("after prepare the generated quiz should show:\n%s", s.View())
	}
}

// Short content must not strand the quiz at the bottom of a tall terminal:
// the viewport shrinks to its content instead of holding a fixed height.
func TestStudyQuizSitsBelowShortContent(t *testing.T) {
	d := preparedDeps(t)
	var s screen = newStudy(d, "src1")
	s = drive(t, s, tea.WindowSizeMsg{Width: 100, Height: 60})
	if cmd := s.Init(); cmd != nil {
		s = drive(t, s, cmd())
	}
	blanks, maxBlanks := 0, 0
	for _, line := range strings.Split(s.View(), "\n") {
		if strings.TrimSpace(line) == "" {
			blanks++
			if blanks > maxBlanks {
				maxBlanks = blanks
			}
		} else {
			blanks = 0
		}
	}
	if maxBlanks > 6 {
		t.Fatalf("quiz stranded below %d blank lines:\n%s", maxBlanks, s.View())
	}
}

// Long prompts wrap within the pane instead of being cut off.
func TestStudyLongPromptWraps(t *testing.T) {
	d := preparedDeps(t)
	ctx := context.Background()
	long := "Compare singly linked lists and doubly linked lists, describe the structural difference, one operation that is faster on each, and the memory tradeoff involved ENDMARKER"
	if err := d.Store.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "s0", Summary: "sum", KeyConcepts: []string{"gradients"},
		Questions: []model.Question{
			{ID: "q0", SectionID: "s0", Idx: 0, Kind: model.KindFree, Prompt: long, Rubric: "r"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	s := loadedStudy(t, d)
	// The prompt wraps: every word stays visible and no line overflows the
	// window width (overflowing lines are what the terminal cuts off).
	view := s.View()
	if !strings.Contains(view, "ENDMARKER") {
		t.Fatalf("long prompt missing entirely:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Fatalf("line overflows 100-column window (%d cols): %q", w, line)
		}
	}
}
