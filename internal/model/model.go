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
