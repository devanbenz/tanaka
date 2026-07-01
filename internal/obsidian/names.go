// Package obsidian renders a Tanaka source as a folder of wikilinked markdown
// notes; Obsidian's graph view draws the concept web from the links.
package obsidian

import (
	"fmt"
	"strings"
)

// forbidden are characters Obsidian disallows in filenames and link targets.
const forbidden = `/\:*?"<>|#^[]`

// Sanitize strips characters Obsidian disallows in filenames/wikilinks and
// collapses whitespace runs to single spaces.
func Sanitize(name string) string {
	mapped := strings.Map(func(r rune) rune {
		if strings.ContainsRune(forbidden, r) {
			return ' '
		}
		return r
	}, name)
	return strings.Join(strings.Fields(mapped), " ")
}

// SourceName returns the hub note name (no extension) for a source title.
func SourceName(title string) string {
	t := Sanitize(title)
	if t == "" {
		t = "Untitled"
	}
	return t
}

// SectionName returns the note name (no extension) for a section: a
// zero-padded 1-based index prefix plus the sanitized title.
func SectionName(idx int, title string) string {
	t := Sanitize(title)
	if t == "" {
		t = "Untitled"
	}
	return fmt.Sprintf("%02d %s", idx+1, t)
}

// QuestionName returns the note name (no extension) for a question within a
// section; n is 1-based in the name.
func QuestionName(sectionIdx int, sectionTitle string, questionIdx int) string {
	return fmt.Sprintf("%s Q%d", SectionName(sectionIdx, sectionTitle), questionIdx+1)
}
