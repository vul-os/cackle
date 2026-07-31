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
	"encoding/binary"
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
	// keys holds the unwrapped event-key vault, or nothing. A freshly
	// opened Store is LOCKED: it can read every public key in the database
	// (so scan bundles and key rings work with no key material at all) but
	// cannot create, read or use a single private signing key until
	// UnlockKeyVault succeeds. See keyvault_db.go.
	keys keyVaultState
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
//
// It delegates through idSource so that --demo can make a seeded database
// REPRODUCIBLE (see SetDeterministicIDs). Nothing else may replace it: the
// setter is guarded, single-shot, and has no un-setter, so a process that has
// not deliberately opted in is generating real ULIDs and cannot be talked out
// of it.
func NewID() string { return idSource() }

// idSource is the live generator. Never call it directly — NewID is the API.
var idSource = func() string { return ulid.Make().String() }

// deterministicIDsSet records that the opt-in already happened, so a second
// call cannot quietly re-seed a running process's key generator.
var deterministicIDsSet bool

// SetDeterministicIDs makes NewID return a fixed, reproducible sequence.
//
// WHY THIS EXISTS, and why it is not a general-purpose hook: docs/screenshots
// are regenerated from `cackle --demo`, and a run whose ticket serials and
// timestamps re-roll produces a diff on every single capture. That makes the
// question "are these screenshots current?" unanswerable — a real UI
// regression and a fresh ULID look exactly the same in `git status`, so the
// honest answer becomes "nobody can tell", and megabytes of PNG churn get
// committed to say nothing.
//
// SAFETY. IDs from this source are sequential and completely predictable, so
// anything that treats an ID as unguessable — a share link, a ticket serial
// somebody could forge — is insecure under it. It is therefore:
//
//   - callable ONCE, and only before the first NewID call in the process;
//   - wired ONLY from the --demo boot path in cmd/cackle, against an
//     in-memory database that is discarded when the process exits;
//   - never reachable from a build that has not passed --demo.
//
// It returns false if it was already called, so a caller that assumes it won
// the race cannot silently proceed on real ULIDs — or, worse, re-seed a
// generator that has already minted live keys.
func SetDeterministicIDs(seed uint64) bool {
	if deterministicIDsSet {
		return false
	}
	deterministicIDsSet = true
	idSource = DeterministicIDSource(seed)
	return true
}

// DeterministicIDSource returns the generator SetDeterministicIDs installs.
//
// Exported and pure so its behaviour can be tested without touching the
// package-global generator: SetDeterministicIDs is single-shot and has no
// un-setter (deliberately — see above), so a test that called it would leave
// every later test in this binary running on sequential keys.
func DeterministicIDSource(seed uint64) func() string {
	// A counter, not a PRNG: the point is a sequence that is identical run to
	// run AND ordered, because these IDs are sortable primary keys and demo
	// data that sorts differently every run defeats the whole exercise.
	var n uint64
	return func() string {
		n++
		var e [10]byte
		binary.BigEndian.PutUint64(e[:8], seed)
		binary.BigEndian.PutUint16(e[8:], uint16(n))
		// A fixed ULID timestamp too — the millisecond field is part of the
		// rendered string, so a wall-clock one would churn on its own.
		var id ulid.ULID
		copy(id[:6], ulidTime(demoULIDMillis))
		copy(id[6:], e[:])
		return id.String()
	}
}

// demoULIDMillis is the fixed ULID timestamp used under SetDeterministicIDs:
// 2026-01-01T00:00:00Z, chosen only because it is stable and obviously
// synthetic.
const demoULIDMillis = uint64(1767225600000)

func ulidTime(ms uint64) []byte {
	return []byte{
		byte(ms >> 40), byte(ms >> 32), byte(ms >> 24),
		byte(ms >> 16), byte(ms >> 8), byte(ms),
	}
}

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
