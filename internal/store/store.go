// Package store persists Tanaka's domain objects.
package store

import (
	"context"
	"errors"

	"github.com/devandbenz/tanaka/internal/model"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Store persists sources, sections, study packages, questions, and progress.
type Store interface {
	SaveSource(ctx context.Context, s *model.Source) error
	GetSource(ctx context.Context, id string) (*model.Source, error)
	ListSources(ctx context.Context) ([]*model.Source, error)
	DeleteSource(ctx context.Context, id string) error
	SaveSectionStudy(ctx context.Context, s *model.SectionStudy) error
	GetSectionStudy(ctx context.Context, sectionID string) (*model.SectionStudy, error)
	IsPrepared(ctx context.Context, sourceID string) (bool, error)
	GetSectionStatuses(ctx context.Context, sourceID string) (map[string]string, error)
	SetSectionStatus(ctx context.Context, sectionID, status string) error
	GetQuestion(ctx context.Context, questionID string) (*model.Question, error)
	Close() error
}

const schema = `
CREATE TABLE IF NOT EXISTS sources (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL,
	origin     TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sections (
	id        TEXT PRIMARY KEY,
	source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
	idx       INTEGER NOT NULL,
	title     TEXT NOT NULL,
	markdown  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sections_source ON sections(source_id, idx);
CREATE TABLE IF NOT EXISTS section_study (
	section_id   TEXT PRIMARY KEY REFERENCES sections(id) ON DELETE CASCADE,
	summary      TEXT NOT NULL,
	key_concepts TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS questions (
	id            TEXT PRIMARY KEY,
	section_id    TEXT NOT NULL REFERENCES sections(id) ON DELETE CASCADE,
	idx           INTEGER NOT NULL,
	kind          TEXT NOT NULL,
	prompt        TEXT NOT NULL,
	options       TEXT,
	correct_index INTEGER,
	rubric        TEXT,
	explanation   TEXT
);
CREATE INDEX IF NOT EXISTS idx_questions_section ON questions(section_id, idx);
CREATE TABLE IF NOT EXISTS section_progress (
	section_id TEXT PRIMARY KEY REFERENCES sections(id) ON DELETE CASCADE,
	status     TEXT NOT NULL
);
`
