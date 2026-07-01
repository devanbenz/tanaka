// Package store persists Tanaka's domain objects.
package store

import (
	"context"
	"errors"

	"github.com/devandbenz/tanaka/internal/model"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Store persists sources, sections, study packages, questions, progress, and builds.
type Store interface {
	SaveSource(ctx context.Context, s *model.Source) error
	GetSource(ctx context.Context, id string) (*model.Source, error)
	ListSources(ctx context.Context) ([]*model.Source, error)
	DeleteSource(ctx context.Context, id string) error
	ExportSource(ctx context.Context, id string) (*model.Sheet, error)
	ImportSheet(ctx context.Context, sheet *model.Sheet, newID func() string) (string, error)
	SaveSectionStudy(ctx context.Context, s *model.SectionStudy) error
	GetSectionStudy(ctx context.Context, sectionID string) (*model.SectionStudy, error)
	IsPrepared(ctx context.Context, sourceID string) (bool, error)
	GetSectionStatuses(ctx context.Context, sourceID string) (map[string]string, error)
	SetSectionStatus(ctx context.Context, sectionID, status string) error
	GetQuestion(ctx context.Context, questionID string) (*model.Question, error)
	GetSection(ctx context.Context, sectionID string) (*model.Section, error)
	SaveQuestionProgress(ctx context.Context, questionID, verdict, answer string, choice int, feedback string) error
	GetSectionProgress(ctx context.Context, sectionID string) (map[string]model.QuestionProgress, error)
	SectionSatisfied(ctx context.Context, sectionID string) (bool, error)
	SaveBuild(ctx context.Context, b *model.Build) error
	GetBuild(ctx context.Context, sourceID, language string) (*model.Build, error)
	SetBuildStepStatus(ctx context.Context, stepID, status string) error
	GetBuildStep(ctx context.Context, stepID string) (*model.BuildStep, error)
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
CREATE TABLE IF NOT EXISTS question_progress (
	question_id TEXT PRIMARY KEY REFERENCES questions(id) ON DELETE CASCADE,
	verdict     TEXT    NOT NULL,
	answer      TEXT    NOT NULL DEFAULT '',
	choice      INTEGER NOT NULL DEFAULT -1,
	feedback    TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS builds (
	id          TEXT PRIMARY KEY,
	source_id   TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
	language    TEXT NOT NULL,
	difficulty  TEXT NOT NULL,
	workspace   TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	UNIQUE(source_id, language)
);
CREATE TABLE IF NOT EXISTS build_steps (
	id         TEXT PRIMARY KEY,
	build_id   TEXT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
	idx        INTEGER NOT NULL,
	goal       TEXT NOT NULL,
	files_json TEXT NOT NULL,
	status     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_build_steps_build ON build_steps(build_id, idx);
`
