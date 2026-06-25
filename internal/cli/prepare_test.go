package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
)

func TestPrepareGeneratesStudy(t *testing.T) {
	d := testDeps(t)
	// Replace the invoker with one that answers both ingest (structure) and study.
	d.invoker = &agent.Fake{Responses: map[string]json.RawMessage{
		"sections":      json.RawMessage(`{"title":"Doc","sections":[{"title":"S1","markdown":"x"}]}`),
		"study package": json.RawMessage(`{"summary":"s","key_concepts":["k"],"questions":[{"kind":"free","prompt":"why","rubric":"r"}]}`),
	}}
	d.stdin = strings.NewReader("content")
	var out, errOut bytes.Buffer

	if code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut); code != 0 {
		t.Fatalf("add exit = %d; %s", code, errOut.String())
	}
	// testDeps newID makes the source id "id1".
	out.Reset()
	if code := run(context.Background(), []string{"prepare", "id1"}, d, &out, &errOut); code != 0 {
		t.Fatalf("prepare exit = %d; %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "prepared") {
		t.Fatalf("prepare output = %q, want it to confirm prepared", out.String())
	}
}

func TestPrepareRequiresArg(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"prepare"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit when prepare has no id")
	}
}

func TestPrepareUnknownID(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"prepare", "nope"}, d, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown id")
	}
	if !strings.Contains(errOut.String(), "no source with id") {
		t.Fatalf("stderr = %q, want 'no source with id'", errOut.String())
	}
}
