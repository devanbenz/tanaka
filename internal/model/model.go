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

// QuestionProgress is a learner's latest saved attempt at one question.
type QuestionProgress struct {
	Verdict  string // "pass" | "partial" | "fail"
	Answer   string // free-response text ("" for MCQ)
	Choice   int    // MCQ selected option index (-1 for free-response)
	Feedback string // grader feedback / MCQ explanation
}

// Build languages (also the strings stored in builds.language).
const (
	LangRust   = "rust"
	LangGo     = "go"
	LangCPP    = "cpp"
	LangC      = "c"
	LangPython = "python"
)

// Build difficulties (stored in builds.difficulty).
const (
	DiffGuided    = "guided"
	DiffSpecTests = "spec+tests"
	DiffBlank     = "blank-page"
)

// ValidLanguage reports whether s is a supported build language.
func ValidLanguage(s string) bool {
	switch s {
	case LangRust, LangGo, LangCPP, LangC, LangPython:
		return true
	}
	return false
}

// ValidDifficulty reports whether s is a supported build difficulty.
func ValidDifficulty(s string) bool {
	switch s {
	case DiffGuided, DiffSpecTests, DiffBlank:
		return true
	}
	return false
}

// BuildFile is one file the agent generates into the build workspace.
type BuildFile struct {
	Path    string
	Content string
}

// BuildStep is one ordered step of a build plan.
type BuildStep struct {
	ID      string
	BuildID string
	Idx     int
	Goal    string
	Files   []BuildFile // written into the workspace when the step activates
	Status  string
}

// Build is a per-source, per-language implementation exercise.
type Build struct {
	ID         string
	SourceID   string
	Language   string
	Difficulty string
	Workspace  string
	CreatedAt  time.Time
	Steps      []BuildStep
}
