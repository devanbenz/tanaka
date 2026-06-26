package build

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devandbenz/tanaka/internal/agent"
)

const hintSchema = `{"type":"object","required":["hint"],"properties":{"hint":{"type":"string"}}}`

func hintPrompt(goal string) string {
	return "Give a single short hint (a nudge, not the solution) for this build step. " +
		"The learner's current code and the failing test output are provided to you. " +
		"Step goal: " + goal
}

// Hint returns one nudge for a build step via an agent call.
func Hint(ctx context.Context, inv agent.Invoker, goal, code, failingOutput string) (string, error) {
	stdin := "CURRENT CODE:\n" + code + "\n\nFAILING OUTPUT:\n" + failingOutput
	job := agent.Job{Prompt: hintPrompt(goal), Schema: hintSchema, Stdin: []byte(stdin)}
	resp, err := inv.Invoke(ctx, job)
	if err != nil {
		return "", fmt.Errorf("gen-hint invoke: %w", err)
	}
	var out struct {
		Hint string `json:"hint"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return "", fmt.Errorf("parse hint: %w", err)
	}
	return out.Hint, nil
}
