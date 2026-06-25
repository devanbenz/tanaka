// Package store persists Tanaka's domain objects.
package store

import (
	"context"
	"errors"

	"github.com/devandbenz/tanaka/internal/model"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Store persists sources and their sections.
type Store interface {
	SaveSource(ctx context.Context, s *model.Source) error
	GetSource(ctx context.Context, id string) (*model.Source, error)
	ListSources(ctx context.Context) ([]*model.Source, error)
	DeleteSource(ctx context.Context, id string) error
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
`
