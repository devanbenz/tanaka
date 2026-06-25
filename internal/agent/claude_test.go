package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeStubClaude writes a fake `claude` executable that echoes a fixed
// --output-format json envelope with the given structured_output payload.
func writeStubClaude(t *testing.T, structuredOutput string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\n" +
		"cat <<'EOF'\n" +
		`{"result":"ok","structured_output":` + structuredOutput + "}\n" +
		"EOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func TestClaudeInvokeReturnsStructuredOutput(t *testing.T) {
	stub := writeStubClaude(t, `{"title":"T","sections":[]}`)
	c := NewClaude(stub)
	raw, err := c.Invoke(context.Background(), Job{Prompt: "hi", Schema: `{"type":"object"}`})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var got struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Title != "T" {
		t.Fatalf("title = %q, want T", got.Title)
	}
}

func TestClaudeInvokeRejectsNullStructuredOutput(t *testing.T) {
	stub := writeStubClaude(t, `null`)
	c := NewClaude(stub)
	_, err := c.Invoke(context.Background(), Job{Prompt: "hi", Schema: ""})
	if err == nil {
		t.Fatal("expected error when structured_output is null")
	}
}

// writeEchoStub writes a fake `claude` that echoes the args it received and the
// content piped to its stdin, so tests can assert how Invoke called it.
func writeEchoStub(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\n" +
		`ARGS="$*"` + "\n" +
		`IN="$(cat)"` + "\n" +
		`printf '{"result":"ok","structured_output":{"args":"%s","stdin":"%s"}}\n' "$ARGS" "$IN"` + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func TestClaudeInvokePipesStdin(t *testing.T) {
	stub := writeEchoStub(t)
	c := NewClaude(stub)
	raw, err := c.Invoke(context.Background(), Job{Prompt: "p", Stdin: []byte("hello-stdin")})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var got struct {
		Args  string `json:"args"`
		Stdin string `json:"stdin"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Stdin != "hello-stdin" {
		t.Fatalf("stdin = %q, want hello-stdin", got.Stdin)
	}
}

func TestClaudeInvokePassesAllowedTools(t *testing.T) {
	stub := writeEchoStub(t)
	c := NewClaude(stub)
	raw, err := c.Invoke(context.Background(), Job{Prompt: "p", AllowedTools: []string{"Read", "WebFetch"}})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var got struct {
		Args string `json:"args"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(got.Args, "--allowedTools Read,WebFetch") {
		t.Fatalf("args = %q, want --allowedTools Read,WebFetch", got.Args)
	}
}

func TestFakeInvokerMatchesOnPromptSubstring(t *testing.T) {
	f := &Fake{Responses: map[string]json.RawMessage{
		"structure": json.RawMessage(`{"ok":true}`),
	}}
	raw, err := f.Invoke(context.Background(), Job{Prompt: "please structure this content"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("raw = %s, want {\"ok\":true}", raw)
	}
	if len(f.Calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(f.Calls))
	}
}
