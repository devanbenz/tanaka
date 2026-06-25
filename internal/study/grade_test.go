package study

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

func TestGradeChoice(t *testing.T) {
	q := &model.Question{Kind: model.KindMCQ, CorrectIndex: 2, Explanation: "C is right"}
	if v := GradeChoice(q, 2); v.Verdict != "pass" || v.Feedback != "C is right" {
		t.Fatalf("correct choice => %+v, want pass + explanation", v)
	}
	if v := GradeChoice(q, 0); v.Verdict != "fail" {
		t.Fatalf("wrong choice => %+v, want fail", v)
	}
}

func TestGradeFreeParsesVerdict(t *testing.T) {
	fake := &agent.Fake{Responses: map[string]json.RawMessage{
		"grade": json.RawMessage(`{"verdict":"partial","feedback":"close, but mention X"}`),
	}}
	q := &model.Question{Kind: model.KindFree, Prompt: "explain X", Rubric: "must mention X"}
	v, err := GradeFree(context.Background(), fake, "section text", q, "my answer")
	if err != nil {
		t.Fatalf("GradeFree: %v", err)
	}
	if v.Verdict != "partial" || !strings.Contains(v.Feedback, "mention X") {
		t.Fatalf("verdict = %+v", v)
	}
	// The section text and the user's answer must reach the agent via Stdin.
	call := fake.Calls[0]
	if !strings.Contains(string(call.Stdin), "section text") || !strings.Contains(string(call.Stdin), "my answer") {
		t.Fatalf("stdin missing section/answer: %q", call.Stdin)
	}
	// The question prompt must be in the prompt (so Fake matches and the agent sees it).
	if !strings.Contains(call.Prompt, "explain X") {
		t.Fatalf("prompt missing question: %q", call.Prompt)
	}
}
