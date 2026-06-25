package model

import "testing"

func TestStudyConstants(t *testing.T) {
	if StatusLocked != "locked" || StatusUnlocked != "unlocked" ||
		StatusPassed != "passed" || StatusSkipped != "skipped" {
		t.Fatal("status constant values must match the schema strings")
	}
	if KindMCQ != "mcq" || KindFree != "free" {
		t.Fatal("kind constant values must match the schema strings")
	}
}

func TestSectionStudyHoldsQuestions(t *testing.T) {
	s := SectionStudy{
		SectionID:   "sec1",
		Summary:     "a summary",
		KeyConcepts: []string{"x", "y"},
		Questions: []Question{
			{ID: "q1", SectionID: "sec1", Idx: 0, Kind: KindMCQ, Prompt: "?", Options: []string{"a", "b"}, CorrectIndex: 1},
			{ID: "q2", SectionID: "sec1", Idx: 1, Kind: KindFree, Prompt: "explain", Rubric: "mentions x"},
		},
	}
	if len(s.Questions) != 2 || s.Questions[0].Options[1] != "b" || s.Questions[1].Kind != KindFree {
		t.Fatalf("unexpected: %+v", s)
	}
}
