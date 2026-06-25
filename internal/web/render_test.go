package web

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	html, err := renderMarkdown("# Title\n\nSome **bold** text and `code`.")
	if err != nil {
		t.Fatalf("renderMarkdown: %v", err)
	}
	s := string(html)
	if !strings.Contains(s, "<h1") || !strings.Contains(s, "Title") {
		t.Fatalf("expected an h1 with Title, got %q", s)
	}
	if !strings.Contains(s, "<strong>bold</strong>") {
		t.Fatalf("expected bold, got %q", s)
	}
	if !strings.Contains(s, "<code>code</code>") {
		t.Fatalf("expected code, got %q", s)
	}
}

func TestRenderMarkdownEmpty(t *testing.T) {
	html, err := renderMarkdown("")
	if err != nil {
		t.Fatalf("renderMarkdown empty: %v", err)
	}
	if strings.TrimSpace(string(html)) != "" {
		t.Fatalf("empty markdown should render empty, got %q", html)
	}
}
