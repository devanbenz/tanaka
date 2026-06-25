// Package model holds Tanaka's core domain types.
package model

import "time"

// Source is a piece of ingested technical content split into ordered sections.
type Source struct {
	ID        string
	Title     string
	Origin    string // file path, URL, or "stdin"
	CreatedAt time.Time
	Sections  []Section
}

// Section is one ordered chunk of a Source's content.
type Section struct {
	ID       string
	SourceID string
	Idx      int
	Title    string
	Markdown string
}

// Study status values (also the strings stored in section_progress).
const (
	StatusLocked   = "locked"
	StatusUnlocked = "unlocked"
	StatusPassed   = "passed"
	StatusSkipped  = "skipped"
)

// Question kinds.
const (
	KindMCQ  = "mcq"
	KindFree = "free"
)

// Question is one quiz item for a section.
type Question struct {
	ID           string
	SectionID    string
	Idx          int
	Kind         string // KindMCQ or KindFree
	Prompt       string
	Options      []string // MCQ only
	CorrectIndex int      // MCQ only
	Rubric       string   // free only
	Explanation  string   // shown after answering
}

// SectionStudy is the generated study package for one section.
type SectionStudy struct {
	SectionID   string
	Summary     string
	KeyConcepts []string
	Questions   []Question
}
