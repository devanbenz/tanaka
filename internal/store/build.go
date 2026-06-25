package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func (s *sqliteStore) SaveBuild(ctx context.Context, b *model.Build) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO builds (id, source_id, language, difficulty, workspace, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.SourceID, b.Language, b.Difficulty, b.Workspace, b.CreatedAt.Unix()); err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	for _, st := range b.Steps {
		files, err := json.Marshal(st.Files)
		if err != nil {
			return fmt.Errorf("marshal files: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO build_steps (id, build_id, idx, goal, files_json, status) VALUES (?, ?, ?, ?, ?, ?)`,
			st.ID, b.ID, st.Idx, st.Goal, string(files), st.Status); err != nil {
			return fmt.Errorf("insert build_step %s: %w", st.ID, err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) GetBuild(ctx context.Context, sourceID, language string) (*model.Build, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, difficulty, workspace, created_at FROM builds WHERE source_id = ? AND language = ?`,
		sourceID, language)
	var b model.Build
	var created int64
	if err := row.Scan(&b.ID, &b.Difficulty, &b.Workspace, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get build: %w", err)
	}
	b.SourceID, b.Language, b.CreatedAt = sourceID, language, time.Unix(created, 0).UTC()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, idx, goal, files_json, status FROM build_steps WHERE build_id = ? ORDER BY idx`, b.ID)
	if err != nil {
		return nil, fmt.Errorf("query build_steps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		st, err := scanStep(rows, b.ID)
		if err != nil {
			return nil, err
		}
		b.Steps = append(b.Steps, *st)
	}
	return &b, rows.Err()
}

func (s *sqliteStore) SetBuildStepStatus(ctx context.Context, stepID, status string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE build_steps SET status = ? WHERE id = ?`, status, stepID); err != nil {
		return fmt.Errorf("set build step status %s: %w", stepID, err)
	}
	return nil
}

func (s *sqliteStore) GetBuildStep(ctx context.Context, stepID string) (*model.BuildStep, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT build_id, id, idx, goal, files_json, status FROM build_steps WHERE id = ?`, stepID)
	st, err := scanStepRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get build step %s: %w", stepID, err)
	}
	return st, nil
}

// scanStep decodes a build_steps row (id, idx, goal, files_json, status) from *sql.Rows.
func scanStep(rows *sql.Rows, buildID string) (*model.BuildStep, error) {
	var st model.BuildStep
	var files string
	if err := rows.Scan(&st.ID, &st.Idx, &st.Goal, &files, &st.Status); err != nil {
		return nil, fmt.Errorf("scan build step: %w", err)
	}
	st.BuildID = buildID
	if err := json.Unmarshal([]byte(files), &st.Files); err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	return &st, nil
}

// scanStepRow decodes a build_steps row (build_id, id, idx, goal, files_json, status) from a *sql.Row.
func scanStepRow(row *sql.Row) (*model.BuildStep, error) {
	var st model.BuildStep
	var files string
	if err := row.Scan(&st.BuildID, &st.ID, &st.Idx, &st.Goal, &files, &st.Status); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(files), &st.Files); err != nil {
		return nil, fmt.Errorf("unmarshal files: %w", err)
	}
	return &st, nil
}
