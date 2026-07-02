package study

import (
	"context"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func TestComputeUnlocked(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []bool
	}{
		{"first always open", []string{model.StatusLocked, model.StatusLocked}, []bool{true, false}},
		{"passed unlocks next", []string{model.StatusPassed, model.StatusLocked, model.StatusLocked}, []bool{true, true, false}},
		{"skipped unlocks next", []string{model.StatusSkipped, model.StatusLocked}, []bool{true, true}},
		{"unlocked-but-unfinished does not unlock next", []string{model.StatusUnlocked, model.StatusLocked}, []bool{true, false}},
		{"all passed", []string{model.StatusPassed, model.StatusPassed}, []bool{true, true}},
		{"empty", []string{}, []bool{}},
		{"already-passed section reachable despite locked predecessor", []string{model.StatusLocked, model.StatusPassed}, []bool{true, true}},
		{"already-unlocked section reachable despite locked predecessor", []string{model.StatusLocked, model.StatusUnlocked}, []bool{true, true}},
		{"already-skipped section reachable despite locked predecessor", []string{model.StatusLocked, model.StatusSkipped}, []bool{true, true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeUnlocked(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d", len(got), len(c.want))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("[%d] = %v, want %v (%v)", i, got[i], c.want[i], c.in)
				}
			}
		})
	}
}

func gatingStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "B", Markdown: "b"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestPassAndUnlockNext(t *testing.T) {
	ctx := context.Background()
	st := gatingStore(t)
	if err := PassAndUnlockNext(ctx, st, "s0"); err != nil {
		t.Fatalf("PassAndUnlockNext: %v", err)
	}
	statuses, err := st.GetSectionStatuses(ctx, "src1")
	if err != nil {
		t.Fatal(err)
	}
	if statuses["s0"] != model.StatusPassed || statuses["s1"] != model.StatusUnlocked {
		t.Fatalf("statuses = %v, want s0 passed and s1 unlocked", statuses)
	}
}

func TestSkipAndUnlockNext(t *testing.T) {
	ctx := context.Background()
	st := gatingStore(t)
	if err := SkipAndUnlockNext(ctx, st, "s0"); err != nil {
		t.Fatalf("SkipAndUnlockNext: %v", err)
	}
	statuses, err := st.GetSectionStatuses(ctx, "src1")
	if err != nil {
		t.Fatal(err)
	}
	if statuses["s0"] != model.StatusSkipped || statuses["s1"] != model.StatusUnlocked {
		t.Fatalf("statuses = %v, want s0 skipped and s1 unlocked", statuses)
	}
}

// The last section has no successor to unlock.
func TestPassAndUnlockNextLastSection(t *testing.T) {
	ctx := context.Background()
	st := gatingStore(t)
	if err := PassAndUnlockNext(ctx, st, "s1"); err != nil {
		t.Fatalf("PassAndUnlockNext on last section: %v", err)
	}
}
