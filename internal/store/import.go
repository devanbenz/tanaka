package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

// ImportSheet writes sheet as a brand-new source with freshly generated IDs for
// the source, every section, and every question. All inserts run in a single
// transaction; on any error nothing is written. Learner progress is never
// created. Returns the new source id.
func (s *sqliteStore) ImportSheet(ctx context.Context, sheet *model.Sheet, newID func() string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	srcID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sources (id, title, origin, created_at) VALUES (?, ?, ?, ?)`,
		srcID, sheet.Source.Title, sheet.Source.Origin, time.Now().Unix()); err != nil {
		return "", fmt.Errorf("insert source: %w", err)
	}

	for _, sec := range sheet.Source.Sections {
		secID := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sections (id, source_id, idx, title, markdown) VALUES (?, ?, ?, ?, ?)`,
			secID, srcID, sec.Idx, sec.Title, sec.Markdown); err != nil {
			return "", fmt.Errorf("insert section: %w", err)
		}
		if sec.Study == nil {
			continue
		}
		concepts, err := json.Marshal(sec.Study.KeyConcepts)
		if err != nil {
			return "", fmt.Errorf("marshal key_concepts: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO section_study (section_id, summary, key_concepts) VALUES (?, ?, ?)`,
			secID, sec.Study.Summary, string(concepts)); err != nil {
			return "", fmt.Errorf("insert section_study: %w", err)
		}
		for _, q := range sec.Study.Questions {
			var opts any
			if q.Options != nil {
				b, err := json.Marshal(q.Options)
				if err != nil {
					return "", fmt.Errorf("marshal options: %w", err)
				}
				opts = string(b)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO questions (id, section_id, idx, kind, prompt, options, correct_index, rubric, explanation)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				newID(), secID, q.Idx, q.Kind, q.Prompt, opts, q.CorrectIndex, q.Rubric, q.Explanation); err != nil {
				return "", fmt.Errorf("insert question: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return srcID, nil
}
