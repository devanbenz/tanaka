package build

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

const fileSchema = `{"type":"object","required":["path","content"],"properties":{"path":{"type":"string"},"content":{"type":"string"}}}`

const buildSchema = `{
  "type": "object",
  "required": ["skeleton_files", "steps"],
  "properties": {
    "skeleton_files": {"type": "array", "items": ` + fileSchema + `},
    "steps": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["goal", "files"],
        "properties": {
          "goal": {"type": "string"},
          "files": {"type": "array", "items": ` + fileSchema + `}
        }
      }
    }
  }
}`

// StepGen is a generated step before IDs/status are assigned.
type StepGen struct {
	Goal  string
	Files []model.BuildFile
}

func buildPrompt(language, difficulty string) string {
	return "You are designing a hands-on build plan that has the learner implement the " +
		"technical content provided to you, in " + language + ". Difficulty: " + difficulty +
		" (guided = starter code + comments; spec+tests = stubs + tests; blank-page = goal + tests only). " +
		"Produce a language-appropriate project: skeleton_files (project files like go.mod/Cargo.toml/Makefile) " +
		"and an ordered list of steps, each with a goal and files (its acceptance tests plus difficulty-scaled " +
		"scaffold). Tests must be runnable with the standard command for the language. Return the build plan."
}

type buildResult struct {
	SkeletonFiles []model.BuildFile `json:"skeleton_files"`
	Steps         []struct {
		Goal  string            `json:"goal"`
		Files []model.BuildFile `json:"files"`
	} `json:"steps"`
}

// GenerateBuild produces a build plan via one agent call. Section content goes
// via stdin; language/difficulty go in the prompt. All file paths are validated.
func GenerateBuild(ctx context.Context, inv agent.Invoker, sectionsMarkdown, language, difficulty string) ([]model.BuildFile, []StepGen, error) {
	job := agent.Job{Prompt: buildPrompt(language, difficulty), Schema: buildSchema, Stdin: []byte(sectionsMarkdown)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return nil, nil, fmt.Errorf("gen-build invoke: %w", err)
	}
	var res buildResult
	if err := json.Unmarshal(resp, &res); err != nil {
		return nil, nil, fmt.Errorf("parse build result: %w", err)
	}
	if len(res.Steps) == 0 {
		return nil, nil, fmt.Errorf("agent returned no build steps")
	}
	for _, f := range res.SkeletonFiles {
		if err := SafeRelPath(f.Path); err != nil {
			return nil, nil, fmt.Errorf("skeleton file: %w", err)
		}
	}
	var steps []StepGen
	for _, s := range res.Steps {
		for _, f := range s.Files {
			if err := SafeRelPath(f.Path); err != nil {
				return nil, nil, fmt.Errorf("step file: %w", err)
			}
		}
		steps = append(steps, StepGen{Goal: s.Goal, Files: s.Files})
	}
	return res.SkeletonFiles, steps, nil
}
