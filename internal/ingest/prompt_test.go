package ingest

import (
	"strings"
	"testing"
)

// stdinPrompt feeds the agent content via the process stdin; the prompt text
// must not name "stdin"/"standard input" or the model treats it as the subject.
func TestStdinPromptDoesNotReferenceStdin(t *testing.T) {
	low := strings.ToLower(stdinPrompt())
	if strings.Contains(low, "stdin") || strings.Contains(low, "standard input") {
		t.Fatalf("stdinPrompt must not reference stdin/standard input: %q", stdinPrompt())
	}
}
