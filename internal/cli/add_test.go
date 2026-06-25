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

func TestRunEmptyArgs(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{}, d, &out, &errOut); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
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
