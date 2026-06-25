package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/store"
)

func testDeps(t *testing.T) deps {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	n := 0
	return deps{
		invoker: &agent.Fake{Responses: map[string]json.RawMessage{
			"sections": json.RawMessage(`{"title":"Doc","sections":[{"title":"S1","markdown":"x"}]}`),
		}},
		store: st,
		newID: func() string { n++; return fmt.Sprintf("id%d", n) },
		stdin: strings.NewReader("some content"),
	}
}

func TestAddThenList(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer

	if code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut); code != 0 {
		t.Fatalf("add exit = %d; stderr=%s", code, errOut.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"list"}, d, &out, &errOut); code != 0 {
		t.Fatalf("list exit = %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Doc") {
		t.Fatalf("list output %q missing title", out.String())
	}
}

func TestAddRequiresArg(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"add"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit when add has no argument")
	}
}

func TestRunEmptyArgsShowsHelp(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{}, d, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Usage:") || !strings.Contains(out.String(), "add") {
		t.Fatalf("no-args output %q does not look like help", out.String())
	}
}

func TestRunHelpCommand(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"help"}, d, &out, &errOut)
	if code != 0 {
		t.Fatalf("help exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"Usage:", "add", "list", "version", "help"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output %q missing %q", got, want)
		}
	}
}

func TestRunNoArgsTopLevelShowsHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{}, &out, &errOut)
	if code != 0 {
		t.Fatalf("Run([]) exit = %d, want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("Run([]) output %q does not look like help", out.String())
	}
}

func TestRemoveDeletesSource(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	// testDeps' newID yields id1 (source), id2 (section); source id is "id1".
	if code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut); code != 0 {
		t.Fatalf("add exit = %d; stderr=%s", code, errOut.String())
	}
	if code := run(context.Background(), []string{"remove", "id1"}, d, &out, &errOut); code != 0 {
		t.Fatalf("remove exit = %d; stderr=%s", code, errOut.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"list"}, d, &out, &errOut); code != 0 {
		t.Fatalf("list exit = %d", code)
	}
	if !strings.Contains(out.String(), "no sources yet") {
		t.Fatalf("after remove, list = %q, want it empty", out.String())
	}
}

func TestRemoveRequiresArg(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"remove"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit when remove has no id")
	}
}

func TestRemoveUnknownID(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"remove", "nope"}, d, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown id")
	}
	if !strings.Contains(errOut.String(), "no source with id") {
		t.Fatalf("stderr = %q, want 'no source with id'", errOut.String())
	}
}

func TestAddCheckFailure(t *testing.T) {
	d := testDeps(t)
	d.invoker = &agent.Fake{CheckErr: errors.New("not found")}
	d.stdin = strings.NewReader("some content")
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"add", "-"}, d, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit when Check fails")
	}
	if !strings.Contains(errOut.String(), "claude CLI unavailable") {
		t.Fatalf("stderr %q does not mention 'claude CLI unavailable'", errOut.String())
	}
}
