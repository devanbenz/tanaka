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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

// migrate applies additive schema changes to databases created before those
// columns existed. CREATE TABLE IF NOT EXISTS never alters an existing table, so
// pre-existing databases need ADD COLUMN. Each addition is guarded by inspecting
// the current columns, making migrate idempotent and safe to run on every open.
func migrate(db *sql.DB) error {
	adds := []struct{ column, ddl string }{
		{"answer", `ALTER TABLE question_progress ADD COLUMN answer TEXT NOT NULL DEFAULT ''`},
		{"choice", `ALTER TABLE question_progress ADD COLUMN choice INTEGER NOT NULL DEFAULT -1`},
		{"feedback", `ALTER TABLE question_progress ADD COLUMN feedback TEXT NOT NULL DEFAULT ''`},
	}
	existing, err := tableColumns(db, "question_progress")
	if err != nil {
		return err
	}
	for _, a := range adds {
		if existing[a.column] {
			continue
		}
		if _, err := db.Exec(a.ddl); err != nil {
			return fmt.Errorf("add column %s: %w", a.column, err)
		}
	}
	return nil
}

// tableColumns returns the set of column names on a table via PRAGMA table_info.
func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("table_info %s: %w", table, err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info: %w", err)
		}
		cols[name] = true
	}
	return cols, rows.Err()
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

func (s *sqliteStore) DeleteSource(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete source %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete source %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *sqliteStore) Close() error { return s.db.Close() }
