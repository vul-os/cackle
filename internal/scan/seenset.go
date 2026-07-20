package scan

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// MemorySeenSet is an in-process SeenSet backed by a mutex-guarded map.
// Suitable for tests and for a scanner process that does not need its
// dedupe state to survive a restart. For a real gate device, prefer
// SQLiteSeenSet so admission history survives a reboot mid-event.
type MemorySeenSet struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// NewMemorySeenSet returns an empty, ready-to-use MemorySeenSet.
func NewMemorySeenSet() *MemorySeenSet {
	return &MemorySeenSet{seen: make(map[string]time.Time)}
}

// MarkSeen implements SeenSet.
func (s *MemorySeenSet) MarkSeen(_ context.Context, ticketID string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[ticketID]; ok {
		return false, nil
	}
	s.seen[ticketID] = at
	return true, nil
}

// Seen implements SeenSet.
func (s *MemorySeenSet) Seen(_ context.Context, ticketID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[ticketID]
	return ok, nil
}

// FirstSeenAt returns when ticketID was first marked, if ever. Test/
// diagnostic helper.
func (s *MemorySeenSet) FirstSeenAt(ticketID string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.seen[ticketID]
	return t, ok
}

// SQLiteSeenSet is a durable SeenSet backed by its own embedded-SQLite
// table, independent of and never importing internal/store (this package
// must stay self-contained — wiring to the rest of the app's shared
// database happens elsewhere). It is safe for concurrent use.
type SQLiteSeenSet struct {
	db *sql.DB
}

// OpenSQLiteSeenSet opens (creating if necessary) a SQLite database at path
// and ensures its own `scan_seen` table exists. Use path ":memory:" for an
// ephemeral, isolated in-memory database (e.g. in tests).
func OpenSQLiteSeenSet(path string) (*SQLiteSeenSet, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		// Deliberately NOT cache=shared: with a single fixed DSN string,
		// shared-cache mode would let unrelated SQLiteSeenSet instances in
		// the same process accidentally see each other's data. A private,
		// single-connection in-memory database (the default) keeps every
		// Open call isolated, matching internal/store's convention.
		dsn = "file::memory:?_pragma=busy_timeout(5000)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("scan: open seenset db: %w", err)
	}
	// A single connection keeps semantics simple and correct for both the
	// ":memory:" case (which otherwise loses state across pooled
	// connections) and the file case (avoids SQLITE_BUSY under our own
	// concurrent test goroutines beyond what busy_timeout smooths over).
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("scan: ping seenset db: %w", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS scan_seen (
		ticket_id        TEXT PRIMARY KEY,
		first_scanned_at TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("scan: create scan_seen table: %w", err)
	}

	return &SQLiteSeenSet{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteSeenSet) Close() error { return s.db.Close() }

// MarkSeen implements SeenSet. It uses INSERT OR IGNORE so the
// check-and-record happens as a single atomic statement: exactly one
// concurrent caller for a given ticketID observes RowsAffected == 1 (and
// therefore firstSeen == true), no matter how many call MarkSeen at once.
func (s *SQLiteSeenSet) MarkSeen(ctx context.Context, ticketID string, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO scan_seen (ticket_id, first_scanned_at) VALUES (?, ?)`,
		ticketID, at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, fmt.Errorf("scan: mark seen: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("scan: mark seen rows affected: %w", err)
	}
	return n == 1, nil
}

// Seen implements SeenSet.
func (s *SQLiteSeenSet) Seen(ctx context.Context, ticketID string) (bool, error) {
	var dummy string
	err := s.db.QueryRowContext(ctx, `SELECT ticket_id FROM scan_seen WHERE ticket_id = ?`, ticketID).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("scan: seen query: %w", err)
	}
	return true, nil
}
