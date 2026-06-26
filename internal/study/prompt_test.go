package study

import (
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

// The agent receives section content via the process's stdin; the PROMPT must
// not say "stdin"/"standard input", or the model treats Unix stdin as the
// subject and emits file-descriptor lore as the study content.
func TestPromptsDoNotReferenceStdin(t *testing.T) {
	q := &model.Question{Kind: model.KindFree, Prompt: "why", Rubric: "r"}
	prompts := map[string]string{"study": studyPrompt(), "grade": gradePrompt(q)}
	for name, p := range prompts {
		low := strings.ToLower(p)
		if strings.Contains(low, "stdin") || strings.Contains(low, "standard input") {
			t.Fatalf("%s prompt must not reference stdin/standard input: %q", name, p)
		}
	}
}
