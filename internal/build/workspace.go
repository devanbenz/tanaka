package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

// WriteFiles writes each file under workspace, rejecting unsafe paths.
func WriteFiles(workspace string, files []model.BuildFile) error {
	for _, f := range files {
		if err := SafeRelPath(f.Path); err != nil {
			return err
		}
		full := filepath.Join(workspace, f.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
	}
	return nil
}

// StartBuild generates a build plan, persists it, and scaffolds the workspace
// with the skeleton and the first step's files.
func StartBuild(ctx context.Context, inv agent.Invoker, st store.Store, src *model.Source, language, difficulty string, newID func() string, buildsDir string) (*model.Build, error) {
	if !model.ValidLanguage(language) {
		return nil, fmt.Errorf("unsupported language: %s", language)
	}
	if !model.ValidDifficulty(difficulty) {
		return nil, fmt.Errorf("unsupported difficulty: %s", difficulty)
	}
	var sb strings.Builder
	for _, sec := range src.Sections {
		sb.WriteString("## " + sec.Title + "\n" + sec.Markdown + "\n\n")
	}
	skeleton, steps, err := GenerateBuild(ctx, inv, sb.String(), language, difficulty)
	if err != nil {
		return nil, err
	}
	ws := filepath.Join(buildsDir, src.ID+"-"+language)
	b := &model.Build{
		ID: newID(), SourceID: src.ID, Language: language, Difficulty: difficulty,
		Workspace: ws, CreatedAt: time.Now().UTC(),
	}
	for i, sg := range steps {
		status := model.StatusLocked
		if i == 0 {
			status = model.StatusUnlocked
		}
		b.Steps = append(b.Steps, model.BuildStep{
			ID: newID(), BuildID: b.ID, Idx: i, Goal: sg.Goal, Files: sg.Files, Status: status,
		})
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	if err := WriteFiles(ws, skeleton); err != nil {
		return nil, err
	}
	if len(b.Steps) > 0 {
		if err := WriteFiles(ws, b.Steps[0].Files); err != nil {
			return nil, err
		}
	}
	if err := st.SaveBuild(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// PassStep marks step idx passed and activates the next step (write files + unlock).
func PassStep(ctx context.Context, st store.Store, b *model.Build, idx int) error {
	return advance(ctx, st, b, idx, model.StatusPassed)
}

// SkipStep marks step idx skipped and activates the next step.
func SkipStep(ctx context.Context, st store.Store, b *model.Build, idx int) error {
	return advance(ctx, st, b, idx, model.StatusSkipped)
}

func advance(ctx context.Context, st store.Store, b *model.Build, idx int, status string) error {
	if idx < 0 || idx >= len(b.Steps) {
		return fmt.Errorf("step index %d out of range", idx)
	}
	if err := st.SetBuildStepStatus(ctx, b.Steps[idx].ID, status); err != nil {
		return err
	}
	b.Steps[idx].Status = status
	next := idx + 1
	if next < len(b.Steps) {
		if err := WriteFiles(b.Workspace, b.Steps[next].Files); err != nil {
			return err
		}
		if b.Steps[next].Status == model.StatusLocked {
			if err := st.SetBuildStepStatus(ctx, b.Steps[next].ID, model.StatusUnlocked); err != nil {
				return err
			}
			b.Steps[next].Status = model.StatusUnlocked
		}
	}
	return nil
}
