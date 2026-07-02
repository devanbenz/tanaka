package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/devandbenz/tanaka/internal/study"
)

// sourceItem is one row in the home list.
type sourceItem struct {
	id          string
	title       string
	done, total int
}

func (s sourceItem) Title() string { return s.title }
func (s sourceItem) Description() string {
	return fmt.Sprintf("%d/%d sections done", s.done, s.total)
}
func (s sourceItem) FilterValue() string { return s.title }

// Messages produced by home commands.
type (
	sourcesMsg []list.Item
	deletedMsg struct{ title string }
	homeErrMsg struct{ err error }
)

var (
	statusStyle  = lipgloss.NewStyle().Faint(true).MarginLeft(2)
	confirmStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"}).MarginLeft(2)
)

// home lists sources: enter studies, d deletes (with confirm), q quits.
type home struct {
	deps          Deps
	list          list.Model
	confirm       *sourceItem // non-nil while a delete awaits confirmation
	status        string
	width, height int
}

func newHome(d Deps) home {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Tanaka — Sources"
	l.SetShowStatusBar(false)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "study")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		}
	}
	return home{deps: d, list: l}
}

func (h home) Init() tea.Cmd { return loadSources(h.deps) }

// loadSources reads every source and its section progress, mirroring the
// web home page's rows.
func loadSources(d Deps) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		sources, err := d.Store.ListSources(ctx)
		if err != nil {
			return homeErrMsg{err}
		}
		var items []list.Item
		for _, src := range sources {
			full, err := d.Store.GetSource(ctx, src.ID)
			if err != nil {
				return homeErrMsg{err}
			}
			statuses, err := d.Store.GetSectionStatuses(ctx, src.ID)
			if err != nil {
				return homeErrMsg{err}
			}
			done := study.DoneCount(study.OrderedStatuses(full, statuses))
			items = append(items, sourceItem{id: src.ID, title: src.Title, done: done, total: len(full.Sections)})
		}
		return sourcesMsg(items)
	}
}

func deleteSource(d Deps, it sourceItem) tea.Cmd {
	return func() tea.Msg {
		if err := d.Store.DeleteSource(context.Background(), it.id); err != nil {
			return homeErrMsg{err}
		}
		return deletedMsg{title: it.title}
	}
}

func (h home) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
		h.list.SetSize(msg.Width, msg.Height-2) // room for the status line
		return h, nil
	case sourcesMsg:
		return h, h.list.SetItems([]list.Item(msg))
	case deletedMsg:
		h.status = fmt.Sprintf("deleted %q", msg.title)
		return h, loadSources(h.deps)
	case homeErrMsg:
		h.status = "error: " + msg.err.Error()
		return h, nil
	case tea.KeyMsg:
		if h.confirm != nil {
			switch msg.String() {
			case "y":
				it := *h.confirm
				h.confirm = nil
				return h, deleteSource(h.deps, it)
			case "n", "esc":
				h.confirm = nil
			}
			return h, nil
		}
		// While the list's filter input is active it owns the keyboard.
		if h.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return h, tea.Quit
		case "enter":
			if it, ok := h.list.SelectedItem().(sourceItem); ok {
				st := newStudy(h.deps, it.id)
				next, _ := st.Update(tea.WindowSizeMsg{Width: h.width, Height: h.height})
				return next, st.Init()
			}
			return h, nil
		case "d":
			if it, ok := h.list.SelectedItem().(sourceItem); ok {
				h.confirm = &it
			}
			return h, nil
		}
	}
	var cmd tea.Cmd
	h.list, cmd = h.list.Update(msg)
	return h, cmd
}

func (h home) View() string {
	footer := statusStyle.Render(h.status)
	if h.confirm != nil {
		footer = confirmStyle.Render(fmt.Sprintf("Delete %q? Its quizzes and your progress go with it. (y/n)", h.confirm.title))
	}
	return h.list.View() + "\n" + footer
}
