// Package tui is the terminal UI: the same study flows as the web UI,
// rendered with bubbletea for people who live in the terminal.
package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/obsidian"
	"github.com/devandbenz/tanaka/internal/store"
)

// Deps are the shared dependencies every screen draws on.
type Deps struct {
	Store  store.Store
	Inv    agent.Invoker
	NewID  func() string
	Syncer *obsidian.Syncer
}

// screen is one full-terminal view (home, study). Update returns the next
// screen, letting a screen hand control to another.
type screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (screen, tea.Cmd)
	View() string
}

// root adapts the current screen to tea.Model and swaps screens as they
// hand over control.
type root struct {
	current screen
}

func (r root) Init() tea.Cmd { return r.current.Init() }

func (r root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := r.current.Update(msg)
	r.current = next
	return r, cmd
}

func (r root) View() string { return r.current.View() }

// Run starts the TUI on the home screen and blocks until the user quits.
func Run(ctx context.Context, d Deps) error {
	p := tea.NewProgram(root{current: newHome(d)}, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
