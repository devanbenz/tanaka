package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func seqIDer() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("id%d", n) }
}

func TestStructureBuildsSource(t *testing.T) {
	fake := &agent.Fake{Responses: map[string]json.RawMessage{
		// matches because structurePrompt contains the word "sections"
		"sections": json.RawMessage(`{
			"title":"My Paper",
			"sections":[
				{"title":"Intro","markdown":"# Intro\ntext"},
				{"title":"Method","markdown":"# Method\ntext"}
			]
		}`),
	}}
	raw := &RawSource{Origin: "paper.pdf", Bytes: []byte("raw bytes")}
	src, err := Structure(context.Background(), fake, raw, seqIDer())
	if err != nil {
		t.Fatalf("Structure: %v", err)
	}
	if src.Title != "My Paper" || src.Origin != "paper.pdf" {
		t.Fatalf("got %+v", src)
	}
	if len(src.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(src.Sections))
	}
	if src.Sections[0].Idx != 0 || src.Sections[1].Idx != 1 {
		t.Fatalf("idx not set in order: %+v", src.Sections)
	}
	if src.Sections[0].SourceID != src.ID {
		t.Fatalf("section SourceID %q != source ID %q", src.Sections[0].SourceID, src.ID)
	}
	if src.CreatedAt.IsZero() {
		t.Fatal("CreatedAt not set")
	}
	// The raw content must reach the agent.
	if !strings.Contains(fake.Calls[0].Prompt, "raw bytes") {
		t.Fatalf("prompt did not include raw content: %q", fake.Calls[0].Prompt)
	}
}

func TestStructureRejectsEmptySections(t *testing.T) {
	fake := &agent.Fake{Responses: map[string]json.RawMessage{
		"sections": json.RawMessage(`{"title":"Empty","sections":[]}`),
	}}
	_, err := Structure(context.Background(), fake, &RawSource{Origin: "x", Bytes: []byte("y")}, seqIDer())
	if err == nil {
		t.Fatal("expected error when agent returns zero sections")
	}
}
