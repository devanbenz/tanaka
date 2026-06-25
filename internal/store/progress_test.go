package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func twoSectionSource(t *testing.T, s Store) {
	t.Helper()
	err := s.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "B", Markdown: "b"},
		},
	})
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
}

func TestIsPrepared(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	twoSectionSource(t, s)

	ok, err := s.IsPrepared(ctx, "src1")
	if err != nil || ok {
		t.Fatalf("fresh source: ok=%v err=%v, want false/nil", ok, err)
	}
	mustStudy(t, s, "s0")
	if ok, _ := s.IsPrepared(ctx, "src1"); ok {
		t.Fatal("one of two sections studied: want not prepared")
	}
	mustStudy(t, s, "s1")
	if ok, _ := s.IsPrepared(ctx, "src1"); !ok {
		t.Fatal("all sections studied: want prepared")
	}
}

func mustStudy(t *testing.T, s Store, secID string) {
	t.Helper()
	err := s.SaveSectionStudy(context.Background(), &model.SectionStudy{
		SectionID: secID, Summary: "x", KeyConcepts: []string{"k"},
	})
	if err != nil {
		t.Fatalf("SaveSectionStudy %s: %v", secID, err)
	}
}

func TestSectionStatusesDefaultLocked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	twoSectionSource(t, s)
	if err := s.SetSectionStatus(ctx, "s0", model.StatusPassed); err != nil {
		t.Fatal(err)
	}
	statuses, err := s.GetSectionStatuses(ctx, "src1")
	if err != nil {
		t.Fatal(err)
	}
	if statuses["s0"] != model.StatusPassed {
		t.Fatalf("s0 = %q, want passed", statuses["s0"])
	}
	if statuses["s1"] != model.StatusLocked {
		t.Fatalf("s1 = %q, want locked (no row)", statuses["s1"])
	}
}

func TestSetSectionStatusUpserts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	twoSectionSource(t, s)
	if err := s.SetSectionStatus(ctx, "s0", model.StatusUnlocked); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSectionStatus(ctx, "s0", model.StatusPassed); err != nil {
		t.Fatalf("second set should upsert: %v", err)
	}
	statuses, _ := s.GetSectionStatuses(ctx, "src1")
	if statuses["s0"] != model.StatusPassed {
		t.Fatalf("s0 = %q, want passed", statuses["s0"])
	}
}

func TestGetQuestion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sourceWithSection(t, s, "src1", "sec1")
	err := s.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec1", Summary: "x", KeyConcepts: []string{"k"},
		Questions: []model.Question{{ID: "q1", SectionID: "sec1", Idx: 0, Kind: model.KindMCQ, Prompt: "p", Options: []string{"a", "b"}, CorrectIndex: 1, Explanation: "e"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.GetQuestion(ctx, "q1")
	if err != nil {
		t.Fatalf("GetQuestion: %v", err)
	}
	if q.SectionID != "sec1" || q.CorrectIndex != 1 || q.Kind != model.KindMCQ {
		t.Fatalf("question = %+v", q)
	}
	if _, err := s.GetQuestion(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing question err = %v, want ErrNotFound", err)
	}
}
