package build

import (
	"strings"
	"testing"
)

// Build content + code reach the agent via stdin; the prompts must not name
// "stdin"/"standard input" or the model treats it as subject matter.
func TestBuildPromptsDoNotReferenceStdin(t *testing.T) {
	prompts := map[string]string{
		"build": buildPrompt("go", "spec+tests"),
		"hint":  hintPrompt("implement X"),
	}
	for name, p := range prompts {
		low := strings.ToLower(p)
		if strings.Contains(low, "stdin") || strings.Contains(low, "standard input") {
			t.Fatalf("%s prompt must not reference stdin/standard input: %q", name, p)
		}
	}
}
