package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func buildSource(t *testing.T, s Store) {
	t.Helper()
	if err := s.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func sampleBuild() *model.Build {
	return &model.Build{
		ID: "b1", SourceID: "src1", Language: model.LangGo, Difficulty: model.DiffSpecTests,
		Workspace: "/tmp/ws", CreatedAt: time.Unix(5, 0).UTC(),
		Steps: []model.BuildStep{
			{ID: "st0", BuildID: "b1", Idx: 0, Goal: "step zero", Status: model.StatusUnlocked,
				Files: []model.BuildFile{{Path: "go.mod", Content: "module x"}, {Path: "a_test.go", Content: "package x"}}},
			{ID: "st1", BuildID: "b1", Idx: 1, Goal: "step one", Status: model.StatusLocked,
				Files: []model.BuildFile{{Path: "b_test.go", Content: "package x"}}},
		},
	}
}

func TestSaveAndGetBuild(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	buildSource(t, s)
	if err := s.SaveBuild(ctx, sampleBuild()); err != nil {
		t.Fatalf("SaveBuild: %v", err)
	}
	got, err := s.GetBuild(ctx, "src1", model.LangGo)
	if err != nil {
		t.Fatalf("GetBuild: %v", err)
	}
	if got.Difficulty != model.DiffSpecTests || got.Workspace != "/tmp/ws" {
		t.Fatalf("build = %+v", got)
	}
	if len(got.Steps) != 2 || got.Steps[1].Idx != 1 {
		t.Fatalf("steps not ordered: %+v", got.Steps)
	}
	if len(got.Steps[0].Files) != 2 || got.Steps[0].Files[1].Path != "a_test.go" {
		t.Fatalf("files not round-tripped: %+v", got.Steps[0].Files)
	}
	if got.Steps[0].Status != model.StatusUnlocked || got.Steps[1].Status != model.StatusLocked {
		t.Fatalf("status not round-tripped: %+v", got.Steps)
	}
}

func TestGetBuildNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetBuild(context.Background(), "src1", model.LangRust); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetBuildStepStatusAndGetStep(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	buildSource(t, s)
	if err := s.SaveBuild(ctx, sampleBuild()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBuildStepStatus(ctx, "st0", model.StatusPassed); err != nil {
		t.Fatal(err)
	}
	step, err := s.GetBuildStep(ctx, "st0")
	if err != nil {
		t.Fatalf("GetBuildStep: %v", err)
	}
	if step.Status != model.StatusPassed || step.Goal != "step zero" || len(step.Files) != 2 {
		t.Fatalf("step = %+v", step)
	}
	if _, err := s.GetBuildStep(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing step err = %v, want ErrNotFound", err)
	}
}

func TestUniqueBuildPerSourceLanguage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	buildSource(t, s)
	if err := s.SaveBuild(ctx, sampleBuild()); err != nil {
		t.Fatal(err)
	}
	dup := sampleBuild()
	dup.ID = "b2"
	dup.Steps = nil
	if err := s.SaveBuild(ctx, dup); err == nil {
		t.Fatal("expected UNIQUE(source_id, language) violation on second build")
	}
}
