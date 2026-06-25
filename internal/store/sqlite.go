package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

// NewSQLite opens (creating if needed) the database at path and applies the schema.
func NewSQLite(path string) (Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) SaveSource(ctx context.Context, src *model.Source) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO sources (id, title, origin, created_at) VALUES (?, ?, ?, ?)`,
		src.ID, src.Title, src.Origin, src.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert source: %w", err)
	}
	for _, sec := range src.Sections {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO sections (id, source_id, idx, title, markdown) VALUES (?, ?, ?, ?, ?)`,
			sec.ID, src.ID, sec.Idx, sec.Title, sec.Markdown)
		if err != nil {
			return fmt.Errorf("insert section %s: %w", sec.ID, err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) GetSource(ctx context.Context, id string) (*model.Source, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, title, origin, created_at FROM sources WHERE id = ?`, id)
	var src model.Source
	var created int64
	if err := row.Scan(&src.ID, &src.Title, &src.Origin, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan source %s: %w", id, err)
	}
	src.CreatedAt = time.Unix(created, 0).UTC()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_id, idx, title, markdown FROM sections WHERE source_id = ? ORDER BY idx`, id)
	if err != nil {
		return nil, fmt.Errorf("query sections for %s: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var sec model.Section
		if err := rows.Scan(&sec.ID, &sec.SourceID, &sec.Idx, &sec.Title, &sec.Markdown); err != nil {
			return nil, fmt.Errorf("scan section for %s: %w", id, err)
		}
		src.Sections = append(src.Sections, sec)
	}
	return &src, rows.Err()
}

func (s *sqliteStore) ListSources(ctx context.Context) ([]*model.Source, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, origin, created_at FROM sources ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("query sources: %w", err)
	}
	defer rows.Close()
	var out []*model.Source
	for rows.Next() {
		var src model.Source
		var created int64
		if err := rows.Scan(&src.ID, &src.Title, &src.Origin, &created); err != nil {
			return nil, fmt.Errorf("scan source row: %w", err)
		}
		src.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, &src)
	}
	return out, rows.Err()
}

func (s *sqliteStore) Close() error { return s.db.Close() }
