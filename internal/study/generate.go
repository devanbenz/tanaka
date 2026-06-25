package study

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

const studySchema = `{
  "type": "object",
  "required": ["summary", "key_concepts", "questions"],
  "properties": {
    "summary": {"type": "string"},
    "key_concepts": {"type": "array", "items": {"type": "string"}},
    "questions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["kind", "prompt"],
        "properties": {
          "kind": {"type": "string", "enum": ["mcq", "free"]},
          "prompt": {"type": "string"},
          "options": {"type": "array", "items": {"type": "string"}},
          "correct_index": {"type": "integer"},
          "rubric": {"type": "string"},
          "explanation": {"type": "string"}
        }
      }
    }
  }
}`

func studyPrompt() string {
	return "You are building a study package for the section on stdin. Produce a short " +
		"summary, a list of key concepts, and a mix of quiz questions: at least one " +
		"multiple-choice (mcq, with options, a correct_index, and an explanation) and at " +
		"least one free-response (free, with a grading rubric). Return the study package."
}

type genResult struct {
	Summary     string   `json:"summary"`
	KeyConcepts []string `json:"key_concepts"`
	Questions   []struct {
		Kind         string   `json:"kind"`
		Prompt       string   `json:"prompt"`
		Options      []string `json:"options"`
		CorrectIndex int      `json:"correct_index"`
		Rubric       string   `json:"rubric"`
		Explanation  string   `json:"explanation"`
	} `json:"questions"`
}

// GenerateSection produces a study package for one section via one agent call.
// Returned questions do not yet have ID/SectionID/Idx set.
func GenerateSection(ctx context.Context, inv agent.Invoker, sectionMarkdown string) (string, []string, []model.Question, error) {
	job := agent.Job{Prompt: studyPrompt(), Schema: studySchema, Stdin: []byte(sectionMarkdown)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return "", nil, nil, fmt.Errorf("gen-study invoke: %w", err)
	}
	var res genResult
	if err := json.Unmarshal(resp, &res); err != nil {
		return "", nil, nil, fmt.Errorf("parse study result: %w", err)
	}
	var qs []model.Question
	for _, q := range res.Questions {
		qs = append(qs, model.Question{
			Kind: q.Kind, Prompt: q.Prompt, Options: q.Options,
			CorrectIndex: q.CorrectIndex, Rubric: q.Rubric, Explanation: q.Explanation,
		})
	}
	return res.Summary, res.KeyConcepts, qs, nil
}

// PrepareSource generates and stores study packages for every section that lacks
// one, then unlocks the first section. Resumable: already-studied sections are
// skipped. onSection (may be nil) is called before each generated section.
func PrepareSource(ctx context.Context, inv agent.Invoker, st store.Store, src *model.Source, newID func() string, onSection func(i, total int, title string)) error {
	total := len(src.Sections)
	for i, sec := range src.Sections {
		if _, err := st.GetSectionStudy(ctx, sec.ID); err == nil {
			continue // already prepared
		}
		if onSection != nil {
			onSection(i, total, sec.Title)
		}
		summary, concepts, qs, err := GenerateSection(ctx, inv, sec.Markdown)
		if err != nil {
			return fmt.Errorf("prepare section %d (%s): %w", i, sec.Title, err)
		}
		for j := range qs {
			qs[j].ID = newID()
			qs[j].SectionID = sec.ID
			qs[j].Idx = j
		}
		if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
			SectionID: sec.ID, Summary: summary, KeyConcepts: concepts, Questions: qs,
		}); err != nil {
			return fmt.Errorf("save study for section %d: %w", i, err)
		}
	}
	if total > 0 {
		statuses, err := st.GetSectionStatuses(ctx, src.ID)
		if err != nil {
			return err
		}
		if statuses[src.Sections[0].ID] == model.StatusLocked {
			if err := st.SetSectionStatus(ctx, src.Sections[0].ID, model.StatusUnlocked); err != nil {
				return err
			}
		}
	}
	return nil
}
