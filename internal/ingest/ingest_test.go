package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func seqIDer() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("id%d", n) }
}

func okFake() *agent.Fake {
	return &agent.Fake{Responses: map[string]json.RawMessage{
		"sections": json.RawMessage(`{"title":"Doc","sections":[{"title":"S1","markdown":"a"},{"title":"S2","markdown":"b"}]}`),
	}}
}

func TestIngestFileBuildsReadJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paper.md")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(path)
	f := okFake()
	src, err := Ingest(context.Background(), f, path, nil, seqIDer())
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if src.Origin != abs {
		t.Fatalf("origin = %q, want %q", src.Origin, abs)
	}
	if len(src.Sections) != 2 || src.Sections[1].Idx != 1 {
		t.Fatalf("sections wrong: %+v", src.Sections)
	}
	if src.Sections[0].SourceID != src.ID {
		t.Fatalf("section SourceID %q != source ID %q", src.Sections[0].SourceID, src.ID)
	}
	job := f.Calls[0]
	if !strings.Contains(job.Prompt, abs) {
		t.Fatalf("prompt missing abs path: %q", job.Prompt)
	}
	if len(job.AllowedTools) != 1 || job.AllowedTools[0] != "Read" {
		t.Fatalf("tools = %v, want [Read]", job.AllowedTools)
	}
	if len(job.Stdin) != 0 {
		t.Fatalf("file input must not inline content via stdin, got %q", job.Stdin)
	}
}

func TestIngestURLBuildsWebFetchJob(t *testing.T) {
	f := okFake()
	src, err := Ingest(context.Background(), f, "https://example.com/post", nil, seqIDer())
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if src.Origin != "https://example.com/post" {
		t.Fatalf("origin = %q", src.Origin)
	}
	job := f.Calls[0]
	if !strings.Contains(job.Prompt, "https://example.com/post") {
		t.Fatalf("prompt missing url: %q", job.Prompt)
	}
	if len(job.AllowedTools) != 1 || job.AllowedTools[0] != "WebFetch" {
		t.Fatalf("tools = %v, want [WebFetch]", job.AllowedTools)
	}
}

func TestIngestStdinPipesContent(t *testing.T) {
	f := okFake()
	src, err := Ingest(context.Background(), f, "-", strings.NewReader("pasted text"), seqIDer())
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if src.Origin != "stdin" {
		t.Fatalf("origin = %q, want stdin", src.Origin)
	}
	job := f.Calls[0]
	if string(job.Stdin) != "pasted text" {
		t.Fatalf("stdin = %q, want 'pasted text'", job.Stdin)
	}
	if len(job.AllowedTools) != 0 {
		t.Fatalf("stdin input needs no tools, got %v", job.AllowedTools)
	}
}

func TestIngestMissingFile(t *testing.T) {
	f := okFake()
	_, err := Ingest(context.Background(), f, filepath.Join(t.TempDir(), "nope.pdf"), nil, seqIDer())
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestIngestRejectsEmptySections(t *testing.T) {
	f := &agent.Fake{Responses: map[string]json.RawMessage{
		"sections": json.RawMessage(`{"title":"Empty","sections":[]}`),
	}}
	_, err := Ingest(context.Background(), f, "-", strings.NewReader("x"), seqIDer())
	if err == nil {
		t.Fatal("expected error when agent returns zero sections")
	}
}
