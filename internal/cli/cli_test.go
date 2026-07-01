package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "tanaka") {
		t.Fatalf("stdout = %q, want it to contain %q", out.String(), "tanaka")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"frobnicate"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero for unknown command")
	}
	if !strings.Contains(errOut.String(), "unknown") {
		t.Fatalf("stderr = %q, want it to mention %q", errOut.String(), "unknown")
	}
}

func seedSource(t *testing.T, d deps) {
	t.Helper()
	ctx := context.Background()
	if err := d.store.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "http://x", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "sec0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.store.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a"},
		Questions: []model.Question{{ID: "q0", SectionID: "sec0", Idx: 0, Kind: model.KindFree, Prompt: "why", Rubric: "r"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCmdExportThenImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	d := testDeps(t)
	seedSource(t, d)

	out := filepath.Join(t.TempDir(), "sheet.tanaka")
	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"export", "src1", "-o", out}, d, &stdout, &stderr); code != 0 {
		t.Fatalf("export exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("export file missing: %v", err)
	}

	// Import into a FRESH store.
	d2 := testDeps(t)
	stdout.Reset()
	stderr.Reset()
	if code := run(ctx, []string{"import", out}, d2, &stdout, &stderr); code != 0 {
		t.Fatalf("import exit=%d stderr=%s", code, stderr.String())
	}
	srcs, err := d2.store.ListSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 || srcs[0].Title != "My Paper" {
		t.Fatalf("imported source wrong: %+v", srcs)
	}
}

func TestCmdExportUnknownID(t *testing.T) {
	d := testDeps(t)
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"export", "nope"}, d, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown id")
	}
	if !strings.Contains(stderr.String(), "no source") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCmdImportBadFile(t *testing.T) {
	d := testDeps(t)
	bad := filepath.Join(t.TempDir(), "bad.tanaka")
	os.WriteFile(bad, []byte("not a gzip"), 0o644)
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"import", bad}, d, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for corrupt file")
	}
}

func TestCmdExportObsidian(t *testing.T) {
	ctx := context.Background()
	d := testDeps(t)
	seedSource(t, d)

	out := filepath.Join(t.TempDir(), "vault")
	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"export", "src1", "--format", "obsidian", "-o", out}, d, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "My Paper.md")); err != nil {
		t.Fatalf("hub note missing: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(out, "sections", "01 Intro.md"))
	if err != nil {
		t.Fatalf("section note missing: %v", err)
	}
	if !strings.Contains(string(b), "[[My Paper]]") {
		t.Fatalf("section note missing hub link:\n%s", b)
	}
}

func TestCmdExportInvalidFormat(t *testing.T) {
	d := testDeps(t)
	seedSource(t, d)
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"export", "src1", "--format", "yaml"}, d, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestCmdExportDefaultFormatIsSheet(t *testing.T) {
	ctx := context.Background()
	d := testDeps(t)
	seedSource(t, d)
	out := filepath.Join(t.TempDir(), "s.tanaka")
	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"export", "src1", "-o", out}, d, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("sheet file missing: %v", err)
	}
}
