package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/devandbenz/tanaka/internal/model"
)

func (s *sqliteStore) SaveSectionStudy(ctx context.Context, study *model.SectionStudy) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	concepts, err := json.Marshal(study.KeyConcepts)
	if err != nil {
		return fmt.Errorf("marshal key_concepts: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO section_study (section_id, summary, key_concepts) VALUES (?, ?, ?)
		 ON CONFLICT(section_id) DO UPDATE SET summary=excluded.summary, key_concepts=excluded.key_concepts`,
		study.SectionID, study.Summary, string(concepts)); err != nil {
		return fmt.Errorf("upsert section_study: %w", err)
	}
	// Replace questions for this section.
	if _, err := tx.ExecContext(ctx, `DELETE FROM questions WHERE section_id = ?`, study.SectionID); err != nil {
		return fmt.Errorf("clear questions: %w", err)
	}
	for _, q := range study.Questions {
		var opts any
		if q.Options != nil {
			b, err := json.Marshal(q.Options)
			if err != nil {
				return fmt.Errorf("marshal options: %w", err)
			}
			opts = string(b)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO questions (id, section_id, idx, kind, prompt, options, correct_index, rubric, explanation)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			q.ID, study.SectionID, q.Idx, q.Kind, q.Prompt, opts, q.CorrectIndex, q.Rubric, q.Explanation); err != nil {
			return fmt.Errorf("insert question %s: %w", q.ID, err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) GetSectionStudy(ctx context.Context, sectionID string) (*model.SectionStudy, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT summary, key_concepts FROM section_study WHERE section_id = ?`, sectionID)
	var summary, concepts string
	if err := row.Scan(&summary, &concepts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get section_study %s: %w", sectionID, err)
	}
	study := &model.SectionStudy{SectionID: sectionID, Summary: summary}
	if err := json.Unmarshal([]byte(concepts), &study.KeyConcepts); err != nil {
		return nil, fmt.Errorf("unmarshal key_concepts: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, idx, kind, prompt, options, correct_index, rubric, explanation
		 FROM questions WHERE section_id = ? ORDER BY idx`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("query questions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var q model.Question
		var opts sql.NullString
		var correct sql.NullInt64
		var rubric, expl sql.NullString
		if err := rows.Scan(&q.ID, &q.Idx, &q.Kind, &q.Prompt, &opts, &correct, &rubric, &expl); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}
		q.SectionID = sectionID
		if opts.Valid && opts.String != "" {
			if err := json.Unmarshal([]byte(opts.String), &q.Options); err != nil {
				return nil, fmt.Errorf("unmarshal options: %w", err)
			}
		}
		q.CorrectIndex = int(correct.Int64)
		q.Rubric = rubric.String
		q.Explanation = expl.String
		study.Questions = append(study.Questions, q)
	}
	return study, rows.Err()
}

func (s *sqliteStore) IsPrepared(ctx context.Context, sourceID string) (bool, error) {
	var total, studied int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sections WHERE source_id = ?`, sourceID).Scan(&total); err != nil {
		return false, fmt.Errorf("count sections: %w", err)
	}
	if total == 0 {
		return false, nil
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM section_study st
		 JOIN sections se ON se.id = st.section_id WHERE se.source_id = ?`, sourceID).Scan(&studied); err != nil {
		return false, fmt.Errorf("count studied: %w", err)
	}
	return studied == total, nil
}

func (s *sqliteStore) GetSectionStatuses(ctx context.Context, sourceID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT se.id, COALESCE(sp.status, ?) FROM sections se
		 LEFT JOIN section_progress sp ON sp.section_id = se.id
		 WHERE se.source_id = ?`, model.StatusLocked, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query statuses: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, fmt.Errorf("scan status: %w", err)
		}
		out[id] = status
	}
	return out, rows.Err()
}

func (s *sqliteStore) SetSectionStatus(ctx context.Context, sectionID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO section_progress (section_id, status) VALUES (?, ?)
		 ON CONFLICT(section_id) DO UPDATE SET status=excluded.status`, sectionID, status)
	if err != nil {
		return fmt.Errorf("set status %s: %w", sectionID, err)
	}
	return nil
}

func (s *sqliteStore) GetQuestion(ctx context.Context, questionID string) (*model.Question, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, section_id, idx, kind, prompt, options, correct_index, rubric, explanation
		 FROM questions WHERE id = ?`, questionID)
	var q model.Question
	var opts, rubric, expl sql.NullString
	var correct sql.NullInt64
	if err := row.Scan(&q.ID, &q.SectionID, &q.Idx, &q.Kind, &q.Prompt, &opts, &correct, &rubric, &expl); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get question %s: %w", questionID, err)
	}
	if opts.Valid && opts.String != "" {
		if err := json.Unmarshal([]byte(opts.String), &q.Options); err != nil {
			return nil, fmt.Errorf("unmarshal options: %w", err)
		}
	}
	q.CorrectIndex = int(correct.Int64)
	q.Rubric = rubric.String
	q.Explanation = expl.String
	return &q, nil
}

func (s *sqliteStore) GetSection(ctx context.Context, sectionID string) (*model.Section, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, idx, title, markdown FROM sections WHERE id = ?`, sectionID)
	var sec model.Section
	if err := row.Scan(&sec.ID, &sec.SourceID, &sec.Idx, &sec.Title, &sec.Markdown); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get section %s: %w", sectionID, err)
	}
	return &sec, nil
}

func (s *sqliteStore) SaveQuestionProgress(ctx context.Context, questionID, verdict, answer string, choice int, feedback string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO question_progress (question_id, verdict, answer, choice, feedback)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(question_id) DO UPDATE SET
		   verdict=excluded.verdict, answer=excluded.answer,
		   choice=excluded.choice, feedback=excluded.feedback`,
		questionID, verdict, answer, choice, feedback)
	if err != nil {
		return fmt.Errorf("save progress %s: %w", questionID, err)
	}
	return nil
}

// GetSectionProgress returns the latest saved attempt for every answered
// question in a section, keyed by question id. Unanswered questions are absent.
func (s *sqliteStore) GetSectionProgress(ctx context.Context, sectionID string) (map[string]model.QuestionProgress, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT qp.question_id, qp.verdict, qp.answer, qp.choice, qp.feedback
		 FROM question_progress qp
		 JOIN questions q ON q.id = qp.question_id
		 WHERE q.section_id = ?`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("query progress: %w", err)
	}
	defer rows.Close()
	out := map[string]model.QuestionProgress{}
	for rows.Next() {
		var id string
		var p model.QuestionProgress
		if err := rows.Scan(&id, &p.Verdict, &p.Answer, &p.Choice, &p.Feedback); err != nil {
			return nil, fmt.Errorf("scan progress: %w", err)
		}
		out[id] = p
	}
	return out, rows.Err()
}

func (s *sqliteStore) SectionSatisfied(ctx context.Context, sectionID string) (bool, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM questions WHERE section_id = ?`, sectionID).Scan(&total); err != nil {
		return false, fmt.Errorf("count questions: %w", err)
	}
	if total == 0 {
		return true, nil
	}
	var nonFail int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM question_progress qp
		 JOIN questions q ON q.id = qp.question_id
		 WHERE q.section_id = ? AND qp.verdict != 'fail'`, sectionID).Scan(&nonFail); err != nil {
		return false, fmt.Errorf("count satisfied: %w", err)
	}
	return nonFail == total, nil
}
