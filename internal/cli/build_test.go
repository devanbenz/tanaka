package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func TestBuildGeneratesWorkspace(t *testing.T) {
	d := testDeps(t)
	d.invoker = &agent.Fake{Responses: map[string]json.RawMessage{
		"sections":   json.RawMessage(`{"title":"Doc","sections":[{"title":"S1","markdown":"x"}]}`),
		"build plan": json.RawMessage(`{"skeleton_files":[{"path":"go.mod","content":"module x"}],"steps":[{"goal":"do the thing","files":[{"path":"a_test.go","content":"package x"}]}]}`),
	}}
	d.stdin = strings.NewReader("content")
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut); code != 0 {
		t.Fatalf("add exit = %d; %s", code, errOut.String())
	}
	out.Reset()
	// testDeps newID makes the source id "id1".
	if code := run(context.Background(), []string{"build", "id1", "--lang", "go"}, d, &out, &errOut); code != 0 {
		t.Fatalf("build exit = %d; %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "do the thing") || !strings.Contains(out.String(), "id1-go") {
		t.Fatalf("build output missing step/workspace: %q", out.String())
	}
}

func TestBuildRequiresIDAndLang(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"build"}, d, &out, &errOut); code != 2 {
		t.Fatalf("expected exit 2 when build has no id, got %d", code)
	}
	out.Reset()
	errOut.Reset()
	code := run(context.Background(), []string{"build", "id1", "--lang", "haskell"}, d, &out, &errOut)
	if code == 0 || !strings.Contains(errOut.String(), "language") {
		t.Fatalf("expected language error, code=%d stderr=%q", code, errOut.String())
	}
}

func TestBuildUnknownID(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"build", "nope", "--lang", "go"}, d, &out, &errOut)
	if code == 0 || !strings.Contains(errOut.String(), "no source with id") {
		t.Fatalf("expected unknown-id error, code=%d stderr=%q", code, errOut.String())
	}
}
