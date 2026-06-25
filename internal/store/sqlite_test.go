package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func newTestStore(t *testing.T) Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSaveAndGetSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	src := &model.Source{
		ID:        "src1",
		Title:     "A Paper",
		Origin:    "paper.pdf",
		CreatedAt: time.Unix(1000, 0).UTC(),
		Sections: []model.Section{
			{ID: "sec1", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# Intro"},
			{ID: "sec2", SourceID: "src1", Idx: 1, Title: "Method", Markdown: "# Method"},
		},
	}
	if err := s.SaveSource(ctx, src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	got, err := s.GetSource(ctx, "src1")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.Title != "A Paper" || got.Origin != "paper.pdf" {
		t.Fatalf("got %+v, want title/origin to match", got)
	}
	if len(got.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(got.Sections))
	}
	if got.Sections[0].Title != "Intro" || got.Sections[1].Idx != 1 {
		t.Fatalf("sections not round-tripped in order: %+v", got.Sections)
	}
}

func TestGetSourceNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetSource(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if err := s.SaveSource(ctx, &model.Source{ID: id, Title: id, Origin: "x", CreatedAt: time.Unix(1, 0)}); err != nil {
			t.Fatalf("SaveSource %s: %v", id, err)
		}
	}
	list, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d sources, want 2", len(list))
	}
}
