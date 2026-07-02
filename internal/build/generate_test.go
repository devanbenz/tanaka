package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func TestSafeRelPath(t *testing.T) {
	for _, ok := range []string{"main.go", "src/lib.rs", "tests/a_test.go"} {
		if err := SafeRelPath(ok); err != nil {
			t.Fatalf("SafeRelPath(%q) = %v, want nil", ok, err)
		}
	}
	// Windows-shaped escapes must be rejected on every platform: agent
	// paths are slash-separated and relative, so none of these are ever
	// legitimate, and on Windows they root outside the workspace.
	for _, bad := range []string{"", "/etc/passwd", "../escape", "a/../../b", "/abs",
		`..\escape`, `\etc\passwd`, `C:\evil`, "C:evil", "C:/evil", `\\srv\share`} {
		if err := SafeRelPath(bad); err == nil {
			t.Fatalf("SafeRelPath(%q) = nil, want error", bad)
		}
	}
}

func buildFake() *agent.Fake {
	return &agent.Fake{Responses: map[string]json.RawMessage{
		"build plan": json.RawMessage(`{
			"skeleton_files":[{"path":"go.mod","content":"module x"}],
			"steps":[
				{"goal":"parse input","files":[{"path":"parse_test.go","content":"package x"}]},
				{"goal":"compute","files":[{"path":"compute_test.go","content":"package x"}]}
			]
		}`),
	}}
}

func TestGenerateBuild(t *testing.T) {
	f := buildFake()
	skeleton, steps, err := GenerateBuild(context.Background(), f, "the paper sections", "go", "spec+tests")
	if err != nil {
		t.Fatalf("GenerateBuild: %v", err)
	}
	if len(skeleton) != 1 || skeleton[0].Path != "go.mod" {
		t.Fatalf("skeleton = %+v", skeleton)
	}
	if len(steps) != 2 || steps[0].Goal != "parse input" || steps[1].Files[0].Path != "compute_test.go" {
		t.Fatalf("steps = %+v", steps)
	}
	// Content + language/difficulty must reach the agent: section text via stdin, lang/difficulty in prompt.
	call := f.Calls[0]
	if !strings.Contains(string(call.Stdin), "the paper sections") {
		t.Fatalf("section text must go via stdin: %q", call.Stdin)
	}
	if !strings.Contains(call.Prompt, "in go") || !strings.Contains(call.Prompt, "spec+tests") {
		t.Fatalf("prompt missing language/difficulty: %q", call.Prompt)
	}
}

func TestGenerateBuildRejectsUnsafePath(t *testing.T) {
	f := &agent.Fake{Responses: map[string]json.RawMessage{
		"build plan": json.RawMessage(`{"skeleton_files":[{"path":"../evil","content":"x"}],"steps":[{"goal":"g","files":[]}]}`),
	}}
	if _, _, err := GenerateBuild(context.Background(), f, "x", "go", "spec+tests"); err == nil {
		t.Fatal("expected error for unsafe skeleton path")
	}
}

func TestGenerateBuildRejectsEmptySteps(t *testing.T) {
	f := &agent.Fake{Responses: map[string]json.RawMessage{
		"build plan": json.RawMessage(`{"skeleton_files":[],"steps":[]}`),
	}}
	if _, _, err := GenerateBuild(context.Background(), f, "x", "go", "spec+tests"); err == nil {
		t.Fatal("expected error for zero steps")
	}
}
