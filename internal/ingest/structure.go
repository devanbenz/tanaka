package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

const structureSchema = `{
  "type": "object",
  "required": ["title", "sections"],
  "properties": {
    "title": {"type": "string"},
    "sections": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["title", "markdown"],
        "properties": {
          "title": {"type": "string"},
          "markdown": {"type": "string"}
        }
      }
    }
  }
}`

func structurePrompt(content string) string {
	return "You are preparing technical content for study. " +
		"Clean the following content into Markdown and split it into ordered sections " +
		"at natural informational boundaries. Return a title and the sections.\n\n" +
		"CONTENT:\n" + content
}

type structureResult struct {
	Title    string `json:"title"`
	Sections []struct {
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
	} `json:"sections"`
}

// Structure turns raw content into a Source with ordered sections via the agent.
func Structure(ctx context.Context, inv agent.Invoker, raw *RawSource, newID func() string) (*model.Source, error) {
	job := agent.Job{Prompt: structurePrompt(string(raw.Bytes)), Schema: structureSchema}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("structure invoke: %w", err)
	}
	var res structureResult
	if err := json.Unmarshal(resp, &res); err != nil {
		return nil, fmt.Errorf("parse structure result: %w", err)
	}
	if len(res.Sections) == 0 {
		return nil, fmt.Errorf("agent returned no sections for %s", raw.Origin)
	}
	src := &model.Source{
		ID:        newID(),
		Title:     res.Title,
		Origin:    raw.Origin,
		CreatedAt: time.Now().UTC(),
	}
	for i, s := range res.Sections {
		src.Sections = append(src.Sections, model.Section{
			ID:       newID(),
			SourceID: src.ID,
			Idx:      i,
			Title:    s.Title,
			Markdown: s.Markdown,
		})
	}
	return src, nil
}
