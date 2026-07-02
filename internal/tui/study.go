package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/study"
)

const sidebarWidth = 26

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	currentStyle  = lipgloss.NewStyle().Bold(true)
	lockedStyle   = lipgloss.NewStyle().Faint(true)
	sidebarStyle  = lipgloss.NewStyle().Width(sidebarWidth).Border(lipgloss.RoundedBorder(), false, true, false, false).PaddingRight(1)
	quizStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), true, false, false, false).PaddingTop(0)
	passStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})
	partialStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"})
	failStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"})
	faintStyle    = lipgloss.NewStyle().Faint(true)
	cursorMarker  = "› "
	noCursorSpace = "  "
)

// Messages produced by study commands.
type (
	studyDataMsg struct {
		src      *model.Source
		prepared bool
		ordered  []string
		idx      int
		stud     *model.SectionStudy
		progress map[string]model.QuestionProgress
	}
	studyErrMsg struct{ err error }
	gradedMsg   struct {
		qid           string
		answer        string
		choice        int
		verdict       study.Verdict
		sectionPassed bool
		err           error
	}
	prepTickMsg string
	prepDoneMsg struct{ err error }
)

// studyScreen mirrors the web study page: a section sidebar, the section
// content, and the quiz — one question focused at a time.
type studyScreen struct {
	deps     Deps
	sourceID string

	width, height int

	src      *model.Source
	prepared bool
	ordered  []string
	unlocked []bool
	idx      int
	stud     *model.SectionStudy
	progress map[string]model.QuestionProgress

	vp   viewport.Model
	ta   textarea.Model
	spin spinner.Model

	qIdx      int // focused question
	cursor    int // MCQ option cursor
	mainWidth int // columns available right of the sidebar

	preparing bool
	prepNote  string
	prepCh    chan tea.Msg
	grading   bool
	status    string
}

func newStudy(d Deps, sourceID string) studyScreen {
	ta := textarea.New()
	ta.Placeholder = "your answer…"
	ta.SetHeight(3)
	return studyScreen{deps: d, sourceID: sourceID, ta: ta, spin: spinner.New(spinner.WithSpinner(spinner.Dot))}
}

func (s studyScreen) Init() tea.Cmd { return loadStudy(s.deps, s.sourceID, -1) }

// loadStudy gathers everything the screen needs. idx < 0 means "the current
// section" (first not yet passed/skipped), mirroring the web study entry.
func loadStudy(d Deps, id string, idx int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		src, err := d.Store.GetSource(ctx, id)
		if err != nil {
			return studyErrMsg{err}
		}
		prepared, err := d.Store.IsPrepared(ctx, id)
		if err != nil {
			return studyErrMsg{err}
		}
		if !prepared {
			return studyDataMsg{src: src, prepared: false}
		}
		statuses, err := d.Store.GetSectionStatuses(ctx, id)
		if err != nil {
			return studyErrMsg{err}
		}
		ordered := study.OrderedStatuses(src, statuses)
		if idx < 0 {
			idx = study.CurrentSectionIdx(ordered)
		}
		if idx >= len(src.Sections) {
			idx = len(src.Sections) - 1
		}
		stud, err := d.Store.GetSectionStudy(ctx, src.Sections[idx].ID)
		if err != nil {
			return studyErrMsg{err}
		}
		progress, err := d.Store.GetSectionProgress(ctx, src.Sections[idx].ID)
		if err != nil {
			return studyErrMsg{err}
		}
		return studyDataMsg{src: src, prepared: true, ordered: ordered, idx: idx, stud: stud, progress: progress}
	}
}

// grade grades one answer and advances gating, exactly like the web's
// /grade handler: save progress; on a non-failing verdict sync Obsidian and,
// when the section is satisfied, mark it passed and unlock the next.
func grade(d Deps, sourceID string, q model.Question, choice int, answer, sectionMarkdown string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var v study.Verdict
		if q.Kind == model.KindMCQ {
			v = study.GradeChoice(&q, choice)
		} else {
			var err error
			v, err = study.GradeFree(ctx, d.Inv, sectionMarkdown, &q, answer)
			if err != nil {
				return gradedMsg{qid: q.ID, err: err}
			}
		}
		if err := d.Store.SaveQuestionProgress(ctx, q.ID, v.Verdict, answer, choice, v.Feedback); err != nil {
			return gradedMsg{qid: q.ID, err: err}
		}
		passed := false
		if v.Verdict != "fail" {
			satisfied, err := d.Store.SectionSatisfied(ctx, q.SectionID)
			if err != nil {
				return gradedMsg{qid: q.ID, err: err}
			}
			if satisfied {
				if err := study.PassAndUnlockNext(ctx, d.Store, q.SectionID); err != nil {
					return gradedMsg{qid: q.ID, err: err}
				}
				passed = true
			}
			if d.Syncer != nil {
				d.Syncer.Sync(sourceID)
			}
		}
		return gradedMsg{qid: q.ID, answer: answer, choice: choice, verdict: v, sectionPassed: passed}
	}
}

func skipSection(d Deps, sectionID string) tea.Cmd {
	return func() tea.Msg {
		if err := study.SkipAndUnlockNext(context.Background(), d.Store, sectionID); err != nil {
			return studyErrMsg{err}
		}
		return prepDoneMsg{} // reuse: triggers a reload at the next section
	}
}

// startPrepare generates studies for every section, streaming progress into
// a channel the update loop subscribes to.
func (s *studyScreen) startPrepare() tea.Cmd {
	ch := make(chan tea.Msg, 16)
	s.prepCh = ch
	src := s.src
	d := s.deps
	go func() {
		err := study.PrepareSource(context.Background(), d.Inv, d.Store, src, d.NewID,
			func(i, total int, title string) {
				ch <- prepTickMsg(fmt.Sprintf("section %d/%d: %s", i+1, total, title))
			})
		ch <- prepDoneMsg{err: err}
		close(ch)
	}()
	return tea.Batch(waitPrep(ch), s.spin.Tick)
}

func waitPrep(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (s studyScreen) currentQuestion() *model.Question {
	if s.stud == nil || len(s.stud.Questions) == 0 || s.qIdx >= len(s.stud.Questions) {
		return nil
	}
	return &s.stud.Questions[s.qIdx]
}

// focusQuestion points the quiz at question i, restoring any saved answer.
func (s *studyScreen) focusQuestion(i int) {
	if s.stud == nil || len(s.stud.Questions) == 0 {
		return
	}
	s.qIdx = (i + len(s.stud.Questions)) % len(s.stud.Questions)
	s.cursor = 0
	q := s.stud.Questions[s.qIdx]
	if p, ok := s.progress[q.ID]; ok && q.Kind == model.KindMCQ && p.Choice >= 0 {
		s.cursor = p.Choice
	}
	if q.Kind == model.KindFree {
		s.ta.SetValue("")
		if p, ok := s.progress[q.ID]; ok {
			s.ta.SetValue(p.Answer)
		}
		s.ta.Focus()
	} else {
		s.ta.Blur()
	}
}

func (s studyScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		s.layout()
		return s, nil
	case studyDataMsg:
		s.src = msg.src
		s.prepared = msg.prepared
		s.ordered = msg.ordered
		s.unlocked = study.ComputeUnlocked(msg.ordered)
		s.idx = msg.idx
		s.stud = msg.stud
		s.progress = msg.progress
		s.layout()
		s.focusQuestion(0)
		return s, nil
	case studyErrMsg:
		s.status = "error: " + msg.err.Error()
		return s, nil
	case gradedMsg:
		s.grading = false
		if msg.err != nil {
			s.status = "grading unavailable — try again (" + msg.err.Error() + ")"
			return s, nil
		}
		s.progress[msg.qid] = model.QuestionProgress{
			Verdict: msg.verdict.Verdict, Feedback: msg.verdict.Feedback, Answer: msg.answer, Choice: msg.choice}
		if msg.sectionPassed {
			s.status = "section passed — next unlocked"
			return s, loadStudy(s.deps, s.sourceID, s.idx)
		}
		return s, nil
	case prepTickMsg:
		s.prepNote = string(msg)
		return s, waitPrep(s.prepCh)
	case prepDoneMsg:
		s.preparing = false
		if msg.err != nil {
			s.status = "prepare failed: " + msg.err.Error()
			return s, nil
		}
		return s, loadStudy(s.deps, s.sourceID, -1)
	case spinner.TickMsg:
		if !s.preparing && !s.grading {
			return s, nil
		}
		var cmd tea.Cmd
		s.spin, cmd = s.spin.Update(msg)
		return s, cmd
	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

func (s studyScreen) handleKey(msg tea.KeyMsg) (screen, tea.Cmd) {
	if s.preparing || s.grading {
		if msg.String() == "ctrl+c" {
			return s, tea.Quit
		}
		return s, nil
	}
	q := s.currentQuestion()
	inTextarea := q != nil && q.Kind == model.KindFree

	switch msg.String() {
	case "ctrl+c":
		return s, tea.Quit
	case "esc":
		return s.goHome()
	case "tab":
		s.focusQuestion(s.qIdx + 1)
		return s, nil
	case "shift+tab":
		s.focusQuestion(s.qIdx - 1)
		return s, nil
	case "ctrl+s":
		if inTextarea {
			s.grading = true
			s.status = ""
			return s, tea.Batch(grade(s.deps, s.sourceID, *q, -1, s.ta.Value(), s.src.Sections[s.idx].Markdown), s.spin.Tick)
		}
		return s, nil
	}

	if inTextarea {
		var cmd tea.Cmd
		s.ta, cmd = s.ta.Update(msg)
		return s, cmd
	}

	switch msg.String() {
	case "q":
		return s, tea.Quit
	case "p":
		if !s.prepared {
			s.preparing = true
			s.status = ""
			return s, s.startPrepare()
		}
		return s, nil
	case "s":
		if s.prepared {
			return s, skipSection(s.deps, s.src.Sections[s.idx].ID)
		}
		return s, nil
	case "up", "k":
		if q != nil && s.cursor > 0 {
			s.cursor--
		}
		return s, nil
	case "down", "j":
		if q != nil && s.cursor < len(q.Options)-1 {
			s.cursor++
		}
		return s, nil
	case "left":
		s.focusQuestion(s.qIdx - 1)
		return s, nil
	case "right":
		s.focusQuestion(s.qIdx + 1)
		return s, nil
	case "[":
		return s.switchSection(s.idx - 1)
	case "]":
		return s.switchSection(s.idx + 1)
	case "enter":
		if q != nil && q.Kind == model.KindMCQ {
			s.status = ""
			return s, grade(s.deps, s.sourceID, *q, s.cursor, "", s.src.Sections[s.idx].Markdown)
		}
		return s, nil
	}

	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	return s, cmd
}

func (s studyScreen) switchSection(i int) (screen, tea.Cmd) {
	if !s.prepared || i < 0 || i >= len(s.src.Sections) || !s.unlocked[i] {
		return s, nil
	}
	return s, loadStudy(s.deps, s.sourceID, i)
}

func (s studyScreen) goHome() (screen, tea.Cmd) {
	h := newHome(s.deps)
	next, _ := h.Update(tea.WindowSizeMsg{Width: s.width, Height: s.height})
	return next, h.Init()
}

// layout sizes the viewport and re-renders the section markdown. The
// viewport shrinks to its content so short sections keep the quiz directly
// below the text instead of stranding it at the bottom of a tall terminal;
// long content scrolls within the space left above the quiz.
func (s *studyScreen) layout() {
	if s.width == 0 || s.src == nil || !s.prepared {
		return
	}
	s.mainWidth = s.width - sidebarWidth - 4
	if s.mainWidth < 20 {
		s.mainWidth = 20
	}
	s.ta.SetWidth(s.mainWidth - 4)
	md := s.src.Sections[s.idx].Markdown
	if s.stud != nil && s.stud.Summary != "" {
		md = "**Summary:** " + s.stud.Summary + "\n\n---\n\n" + md
	}
	content := md
	if r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(s.mainWidth-2)); err == nil {
		if out, err := r.Render(md); err == nil {
			content = out
		}
	}
	const quizHeight = 12 // quiz box + title + help + status
	maxVP := s.height - quizHeight
	if maxVP < 3 {
		maxVP = 3
	}
	vpHeight := lipgloss.Height(strings.TrimRight(content, "\n"))
	if vpHeight > maxVP {
		vpHeight = maxVP
	}
	s.vp = viewport.New(s.mainWidth, vpHeight)
	s.vp.SetContent(content)
}

func verdictLine(p model.QuestionProgress) string {
	text := p.Verdict
	if p.Feedback != "" {
		text += " — " + p.Feedback
	}
	switch p.Verdict {
	case "pass":
		return passStyle.Render(text)
	case "partial":
		return partialStyle.Render(text)
	default:
		return failStyle.Render(text)
	}
}

func (s studyScreen) sidebar() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(s.src.Title) + "\n\n")
	done := study.DoneCount(s.ordered)
	for i, sec := range s.src.Sections {
		line := fmt.Sprintf("%s %s", mark(s.ordered[i]), sec.Title)
		switch {
		case i == s.idx:
			line = currentStyle.Render(line)
		case !s.unlocked[i]:
			line = lockedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + faintStyle.Render(fmt.Sprintf("%d/%d done", done, len(s.src.Sections))))
	return sidebarStyle.Render(b.String())
}

func mark(status string) string {
	switch status {
	case model.StatusPassed:
		return "[x]"
	case model.StatusSkipped:
		return "[-]"
	default:
		return "[ ]"
	}
}

func (s studyScreen) quiz() string {
	if s.stud == nil || len(s.stud.Questions) == 0 {
		return faintStyle.Render("no questions for this section")
	}
	q := s.stud.Questions[s.qIdx]
	var b strings.Builder
	submit := "enter to check"
	if q.Kind == model.KindFree {
		submit = "ctrl+s to submit"
	}
	b.WriteString(faintStyle.Render(fmt.Sprintf("question %d/%d — %s", s.qIdx+1, len(s.stud.Questions), submit)) + "\n")
	b.WriteString(q.Prompt + "\n")
	if q.Kind == model.KindMCQ {
		for i, opt := range q.Options {
			prefix := noCursorSpace
			if i == s.cursor {
				prefix = cursorMarker
			}
			b.WriteString(prefix + opt + "\n")
		}
	} else {
		b.WriteString(s.ta.View() + "\n")
	}
	if s.grading {
		b.WriteString(s.spin.View() + " grading…\n")
	} else if p, ok := s.progress[q.ID]; ok {
		b.WriteString(verdictLine(p) + "\n")
	}
	// Width both wraps long prompts/options/feedback and stops them from
	// overflowing the pane (the terminal cuts overflowing lines off).
	return quizStyle.Width(s.mainWidth).Render(b.String())
}

func (s studyScreen) View() string {
	if s.src == nil {
		return "loading…"
	}
	if s.preparing {
		return fmt.Sprintf("\n  %s preparing quizzes… %s\n", s.spin.View(), s.prepNote)
	}
	if !s.prepared {
		return fmt.Sprintf("\n  %s\n\n  %s\n\n  %s\n",
			titleStyle.Render(s.src.Title),
			"No quizzes yet — press p to prepare this source.",
			faintStyle.Render("p prepare • esc home • q quit"))
	}
	main := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Width(s.mainWidth).Render(s.src.Sections[s.idx].Title),
		s.vp.View(),
		s.quiz(),
		faintStyle.Width(s.mainWidth).Render("tab/←→ question • ↑↓ option • enter check • ctrl+s submit • [ ] section • s skip • esc home"),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, s.sidebar(), main)
	if s.status != "" {
		body += "\n" + statusStyle.Render(s.status)
	}
	return body
}
