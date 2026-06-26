package study

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

// Verdict is the result of grading one answer.
type Verdict struct {
	Verdict  string `json:"verdict"` // "pass" | "partial" | "fail"
	Feedback string `json:"feedback"`
}

const verdictSchema = `{
  "type": "object",
  "required": ["verdict", "feedback"],
  "properties": {
    "verdict": {"type": "string", "enum": ["pass", "partial", "fail"]},
    "feedback": {"type": "string"}
  }
}`

// GradeChoice grades an MCQ answer in pure Go.
func GradeChoice(q *model.Question, choice int) Verdict {
	if choice == q.CorrectIndex {
		return Verdict{Verdict: "pass", Feedback: q.Explanation}
	}
	return Verdict{Verdict: "fail", Feedback: q.Explanation}
}

func gradePrompt(q *model.Question) string {
	return "You are grading a learner's free-response answer for understanding. " +
		"The section text and the learner's answer are provided to you. " +
		"Question: " + q.Prompt + "\nRubric: " + q.Rubric + "\n" +
		"Please grade: decide pass, partial, or fail and give one or two sentences of feedback."
}

// GradeFree grades a free-response answer via one agent call.
func GradeFree(ctx context.Context, inv agent.Invoker, sectionMarkdown string, q *model.Question, answer string) (Verdict, error) {
	stdin := "SECTION:\n" + sectionMarkdown + "\n\nLEARNER ANSWER:\n" + answer
	job := agent.Job{Prompt: gradePrompt(q), Schema: verdictSchema, Stdin: []byte(stdin)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return Verdict{}, fmt.Errorf("grade invoke: %w", err)
	}
	var v Verdict
	if err := json.Unmarshal(resp, &v); err != nil {
		return Verdict{}, fmt.Errorf("parse verdict: %w", err)
	}
	return v, nil
}
