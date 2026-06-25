package study

import (
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
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
