// Package ingest turns a content reference (file path, URL, or stdin) into a
// stored Source. Files and URLs are read by the agent itself via its tools so
// that binary or large content never travels through argv; stdin content is
// piped to the agent.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

const studyInstruction = "Clean it into Markdown and split it into ordered sections " +
	"at natural informational boundaries. Return a title and the sections."

func filePrompt(path string) string {
	return "Read the file at " + path + " using your Read tool. " + studyInstruction
}

func urlPrompt(url string) string {
	return "Fetch " + url + " using your WebFetch tool. " + studyInstruction
}

func stdinPrompt() string {
	return "The technical content to study is provided on your standard input. " + studyInstruction
}

type structureResult struct {
	Title    string `json:"title"`
	Sections []struct {
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
	} `json:"sections"`
}

// Ingest reads the input (file path, http(s) URL, or stdin "-"), has the agent
// structure it into ordered sections, and returns a Source.
func Ingest(ctx context.Context, inv agent.Invoker, arg string, stdin io.Reader, newID func() string) (*model.Source, error) {
	job, origin, err := buildJob(arg, stdin)
	if err != nil {
		return nil, err
	}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("ingest invoke: %w", err)
	}
	var res structureResult
	if err := json.Unmarshal(resp, &res); err != nil {
		return nil, fmt.Errorf("parse ingest result: %w", err)
	}
	if len(res.Sections) == 0 {
		return nil, fmt.Errorf("agent returned no sections for %s", origin)
	}
	src := &model.Source{
		ID:        newID(),
		Title:     res.Title,
		Origin:    origin,
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

// buildJob classifies the input and constructs the agent Job plus the origin
// string to record on the Source.
func buildJob(arg string, stdin io.Reader) (agent.Job, string, error) {
	switch {
	case arg == "-":
		content, err := io.ReadAll(stdin)
		if err != nil {
			return agent.Job{}, "", fmt.Errorf("read stdin: %w", err)
		}
		return agent.Job{Prompt: stdinPrompt(), Schema: structureSchema, Stdin: content}, "stdin", nil
	case strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://"):
		return agent.Job{Prompt: urlPrompt(arg), Schema: structureSchema, AllowedTools: []string{"WebFetch"}}, arg, nil
	default:
		abs, err := filepath.Abs(arg)
		if err != nil {
			return agent.Job{}, "", fmt.Errorf("resolve path %s: %w", arg, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return agent.Job{}, "", fmt.Errorf("file not found: %s", abs)
		}
		return agent.Job{Prompt: filePrompt(abs), Schema: structureSchema, AllowedTools: []string{"Read"}}, abs, nil
	}
}
