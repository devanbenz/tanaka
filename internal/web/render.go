// Package web serves the Tanaka study UI.
package web

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
)

// renderMarkdown converts Markdown to HTML. Content is locally generated and
// trusted, so the result is returned as template.HTML (not re-escaped).
func renderMarkdown(md string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}
