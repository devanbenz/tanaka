package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func newStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestWriteFilesRejectsUnsafe(t *testing.T) {
	ws := t.TempDir()
	if err := WriteFiles(ws, []model.BuildFile{{Path: "../evil", Content: "x"}}); err == nil {
		t.Fatal("expected error for unsafe path")
	}
	if err := WriteFiles(ws, []model.BuildFile{{Path: "src/a.go", Content: "hello"}}); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(ws, "src", "a.go"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("file not written: %v / %q", err, got)
	}
}

func seqIDer() func() string {
	n := 0
	return func() string { n++; return "id" + string(rune('a'+n)) }
}

func srcWith2Sections(t *testing.T, st store.Store) *model.Source {
	t.Helper()
	src := &model.Source{ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{
			{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "alpha"},
			{ID: "s1", SourceID: "src1", Idx: 1, Title: "B", Markdown: "beta"},
		}}
	if err := st.SaveSource(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestStartBuildScaffoldsAndPersists(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := srcWith2Sections(t, st)
	buildsDir := t.TempDir()
	b, err := StartBuild(ctx, buildFake(), st, src, "go", "spec+tests", seqIDer(), buildsDir)
	if err != nil {
		t.Fatalf("StartBuild: %v", err)
	}
	// Persisted and retrievable.
	got, err := st.GetBuild(ctx, "src1", "go")
	if err != nil {
		t.Fatalf("GetBuild: %v", err)
	}
	if len(got.Steps) != 2 || got.Steps[0].Status != model.StatusUnlocked || got.Steps[1].Status != model.StatusLocked {
		t.Fatalf("steps wrong: %+v", got.Steps)
	}
	// Workspace scaffolded with skeleton + step 0 files, but NOT step 1 files yet.
	if _, err := os.Stat(filepath.Join(b.Workspace, "go.mod")); err != nil {
		t.Fatalf("skeleton not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.Workspace, "parse_test.go")); err != nil {
		t.Fatalf("step 0 files not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.Workspace, "compute_test.go")); err == nil {
		t.Fatal("step 1 files should NOT be written until step 1 activates")
	}
	if b.Workspace != filepath.Join(buildsDir, "src1-go") {
		t.Fatalf("workspace = %q", b.Workspace)
	}
}

func TestStartBuildRejectsBadLanguage(t *testing.T) {
	st := newStore(t)
	src := srcWith2Sections(t, st)
	if _, err := StartBuild(context.Background(), buildFake(), st, src, "haskell", "spec+tests", seqIDer(), t.TempDir()); err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestPassStepWritesNextAndUnlocks(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := srcWith2Sections(t, st)
	buildsDir := t.TempDir()
	b, err := StartBuild(ctx, buildFake(), st, src, "go", "spec+tests", seqIDer(), buildsDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := PassStep(ctx, st, b, 0); err != nil {
		t.Fatalf("PassStep: %v", err)
	}
	got, _ := st.GetBuild(ctx, "src1", "go")
	if got.Steps[0].Status != model.StatusPassed || got.Steps[1].Status != model.StatusUnlocked {
		t.Fatalf("statuses after pass: %+v", got.Steps)
	}
	// In-memory struct must reflect the change (Fix 1).
	if b.Steps[0].Status != model.StatusPassed || b.Steps[1].Status != model.StatusUnlocked {
		t.Fatalf("in-memory stale: %+v", b.Steps)
	}
	if _, err := os.Stat(filepath.Join(b.Workspace, "compute_test.go")); err != nil {
		t.Fatalf("step 1 files not written on pass: %v", err)
	}
}

func TestPassStepOutOfRange(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := srcWith2Sections(t, st)
	b, err := StartBuild(ctx, buildFake(), st, src, "go", "spec+tests", seqIDer(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := PassStep(ctx, st, b, 99); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestSkipStepMarksSkippedAndAdvances(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src := srcWith2Sections(t, st)
	b, err := StartBuild(ctx, buildFake(), st, src, "go", "spec+tests", seqIDer(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := SkipStep(ctx, st, b, 0); err != nil {
		t.Fatal(err)
	}
	// In-memory struct must reflect the change (Fix 1).
	if b.Steps[0].Status != model.StatusSkipped || b.Steps[1].Status != model.StatusUnlocked {
		t.Fatalf("in-memory statuses stale: %+v", b.Steps)
	}
	got, _ := st.GetBuild(ctx, "src1", "go")
	if got.Steps[0].Status != model.StatusSkipped || got.Steps[1].Status != model.StatusUnlocked {
		t.Fatalf("persisted statuses wrong: %+v", got.Steps)
	}
}
