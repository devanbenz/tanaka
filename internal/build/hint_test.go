package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func TestHint(t *testing.T) {
	f := &agent.Fake{Responses: map[string]json.RawMessage{
		"hint": json.RawMessage(`{"hint":"think about the base case"}`),
	}}
	h, err := Hint(context.Background(), f, "implement recursion", "func f(){}", "FAIL: stack overflow")
	if err != nil {
		t.Fatalf("Hint: %v", err)
	}
	if !strings.Contains(h, "base case") {
		t.Fatalf("hint = %q", h)
	}
	call := f.Calls[0]
	if !strings.Contains(string(call.Stdin), "func f(){}") || !strings.Contains(string(call.Stdin), "stack overflow") {
		t.Fatalf("stdin missing code/output: %q", call.Stdin)
	}
	if !strings.Contains(call.Prompt, "implement recursion") {
		t.Fatalf("prompt missing goal: %q", call.Prompt)
	}
}
