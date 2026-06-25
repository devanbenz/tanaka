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
	// Insert newer first to prove ordering is by created_at, not insertion order.
	if err := s.SaveSource(ctx, &model.Source{ID: "b", Title: "b", Origin: "x", CreatedAt: time.Unix(2, 0)}); err != nil {
		t.Fatalf("SaveSource b: %v", err)
	}
	if err := s.SaveSource(ctx, &model.Source{ID: "a", Title: "a", Origin: "x", CreatedAt: time.Unix(1, 0)}); err != nil {
		t.Fatalf("SaveSource a: %v", err)
	}
	list, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d sources, want 2", len(list))
	}
	// Oldest (a, Unix(1)) must come first.
	if list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("wrong order: got [%s, %s], want [a, b]", list[0].ID, list[1].ID)
	}
}

func TestDeleteSourceRemovesSourceAndSections(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	src := &model.Source{
		ID: "d1", Title: "Doomed", Origin: "x", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "ds1", SourceID: "d1", Idx: 0, Title: "S1", Markdown: "a"},
			{ID: "ds2", SourceID: "d1", Idx: 1, Title: "S2", Markdown: "b"},
		},
	}
	if err := s.SaveSource(ctx, src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	if err := s.DeleteSource(ctx, "d1"); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	if _, err := s.GetSource(ctx, "d1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete GetSource err = %v, want ErrNotFound", err)
	}
	// Re-saving with the same section IDs must succeed, proving the old sections
	// were cascade-deleted (otherwise the section PK insert would conflict).
	if err := s.SaveSource(ctx, src); err != nil {
		t.Fatalf("re-SaveSource after delete (sections not cascaded?): %v", err)
	}
}

func TestDeleteSourceNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteSource(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteSource(missing) = %v, want ErrNotFound", err)
	}
}
