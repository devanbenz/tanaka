package study

import (
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestOrderedStatuses(t *testing.T) {
	src := &model.Source{ID: "s", Sections: []model.Section{
		{ID: "a", Idx: 0}, {ID: "b", Idx: 1}, {ID: "c", Idx: 2},
	}}
	statuses := map[string]string{"a": model.StatusPassed, "b": model.StatusUnlocked}
	got := OrderedStatuses(src, statuses)
	want := []string{model.StatusPassed, model.StatusUnlocked, model.StatusLocked}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCurrentSectionIdx(t *testing.T) {
	cases := []struct {
		in   []string
		want int
	}{
		{[]string{model.StatusUnlocked, model.StatusLocked}, 0},
		{[]string{model.StatusPassed, model.StatusUnlocked, model.StatusLocked}, 1},
		{[]string{model.StatusPassed, model.StatusSkipped, model.StatusPassed}, 2}, // all done -> last
		{[]string{}, 0},
	}
	for _, c := range cases {
		if got := CurrentSectionIdx(c.in); got != c.want {
			t.Fatalf("CurrentSectionIdx(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
