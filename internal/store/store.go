// Package store owns Cackle's SQLite persistence: opening the database,
// applying embedded migrations, and typed query methods.
//
// SQLite access is via modernc.org/sqlite — a pure-Go driver, no cgo, so the
// whole product stays a single static binary.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNotFound is returned by single-row lookups when no row matches.
var ErrNotFound = errors.New("store: not found")

// Store wraps the database connection pool and exposes typed query methods.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and applies
// all pending migrations. Use path ":memory:" for an ephemeral database
// (each Open call gets its own isolated in-memory database).
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}

	if path == ":memory:" {
		// An in-memory database only exists on one connection; force the
		// pool down to a single connection or pooled queries would each
		// see their own empty database.
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(8)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// buildDSN builds a modernc.org/sqlite DSN with foreign keys enforced, a
// generous busy timeout (so concurrent writers block briefly instead of
// failing), and WAL journaling for file-backed databases.
func buildDSN(path string) string {
	if path == ":memory:" {
		return "file::memory:?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}
	return "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// DB returns the underlying *sql.DB so other internal packages (events,
// tickets, orders, payments, scan) can implement query methods against the
// same connection pool and schema.
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the underlying database connection pool.
func (s *Store) Close() error { return s.db.Close() }

// NewID generates a new sortable ULID string, suitable for any primary key
// in this schema.
func NewID() string { return ulid.Make().String() }

// Migrate applies every embedded migration that has not yet been recorded
// in schema_migrations, in numeric order, each inside its own transaction.
// It is idempotent: running it again is a no-op once every migration has
// been applied.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := versionFromName(name)
		if err != nil {
			return err
		}

		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		if err := applyMigration(db, name, version); err != nil {
			return err
		}
	}

	return nil
}

func applyMigration(db *sql.DB, name string, version int) error {
	sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}

	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		version, name, timeToText(time.Now()),
	); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

// versionFromName extracts the leading integer version from a migration
// filename such as "0001_init.sql" -> 1.
func versionFromName(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration filename %q missing '_' version separator", name)
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration filename %q has non-numeric version: %w", name, err)
	}
	return v, nil
}

// AppliedMigrations reports which migration versions have been applied, for
// diagnostics.
func (s *Store) AppliedMigrations(ctx context.Context) ([]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- time helpers: timestamps are stored as RFC3339 TEXT throughout. ---

func timeToText(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func textToTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func nullTimeToText(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: timeToText(*t), Valid: true}
}

func textToNullTime(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid || ns.String == "" {
		return nil, nil
	}
	t, err := textToTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ensure io/fs stays imported even if migrationsFS usage above ever changes.
var _ fs.ReadDirFS = migrationsFS
