package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func testDeps(t *testing.T) Deps {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "My Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "B", Markdown: "b"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return Deps{Store: st}
}

// drive runs msg through the screen and keeps executing any returned
// commands until none remain, feeding their messages back in.
func drive(t *testing.T, s screen, msg tea.Msg) screen {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		var cmd tea.Cmd
		s, cmd = s.Update(queue[0])
		queue = queue[1:]
		if cmd != nil {
			if m := cmd(); m != nil {
				queue = append(queue, m)
			}
		}
	}
	return s
}

func runes(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func loadedHome(t *testing.T, d Deps) screen {
	t.Helper()
	var s screen = newHome(d)
	s = drive(t, s, tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd := s.Init(); cmd != nil {
		s = drive(t, s, cmd())
	}
	return s
}

func TestHomeListsSourcesWithProgress(t *testing.T) {
	s := loadedHome(t, testDeps(t))
	view := s.View()
	if !strings.Contains(view, "My Paper") {
		t.Fatalf("view missing source title:\n%s", view)
	}
	if !strings.Contains(view, "0/2") {
		t.Fatalf("view missing progress 0/2:\n%s", view)
	}
}

func TestHomeEnterAnnouncesStudyComingSoon(t *testing.T) {
	s := loadedHome(t, testDeps(t))
	s = drive(t, s, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(s.View(), "next PR") {
		t.Fatalf("enter should set a coming-soon status:\n%s", s.View())
	}
}

func TestHomeDeleteConfirmAndExecute(t *testing.T) {
	d := testDeps(t)
	s := loadedHome(t, d)
	s = drive(t, s, runes("d"))
	if !strings.Contains(s.View(), "Delete") {
		t.Fatalf("d should ask for confirmation:\n%s", s.View())
	}
	s = drive(t, s, runes("y"))
	if _, err := d.Store.GetSource(context.Background(), "src1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("source should be deleted, got err = %v", err)
	}
	if strings.Contains(s.View(), "My Paper — ") {
		t.Fatalf("deleted source still listed:\n%s", s.View())
	}
}

func TestHomeDeleteCancel(t *testing.T) {
	d := testDeps(t)
	s := loadedHome(t, d)
	s = drive(t, s, runes("d"))
	s = drive(t, s, runes("n"))
	if _, err := d.Store.GetSource(context.Background(), "src1"); err != nil {
		t.Fatalf("cancelled delete removed the source: %v", err)
	}
}

func TestHomeQuit(t *testing.T) {
	s := loadedHome(t, testDeps(t))
	_, cmd := s.Update(runes("q"))
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q returned %T, want tea.QuitMsg", cmd())
	}
}
