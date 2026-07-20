package scan

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// QueuedAdmission is one locally-recorded scan attempt, waiting to be
// batch-synced to the server once the gate is back online. It mirrors the
// shape of a row in the store schema's `admissions` table, but this
// package never touches that table directly (internal/store owns it) —
// wiring the two together happens elsewhere.
type QueuedAdmission struct {
	TicketID  string
	EventID   string
	GateID    string
	DeviceID  string
	ScannedAt time.Time
	Result    Status
	Note      string
}

// SyncKey is the idempotency key for batch sync: (ticket_id, device_id,
// scanned_at). Re-uploading the exact same scan event — e.g. because a
// batch upload was retried after a dropped connection acked-but-not-seen
// by the client — must be a no-op the second time.
type SyncKey struct {
	TicketID  string
	DeviceID  string
	ScannedAt time.Time
}

func keyOf(a QueuedAdmission) SyncKey {
	return SyncKey{TicketID: a.TicketID, DeviceID: a.DeviceID, ScannedAt: a.ScannedAt.UTC()}
}

// Queue is a device-local, durable holding area for QueuedAdmissions
// recorded while offline, until they can be synced. Decide (admission.go)
// does not use Queue directly — a caller wires "record the Result of
// Decide as a QueuedAdmission" together; Queue only owns local storage and
// retrieval of the queue itself.
type Queue interface {
	// Enqueue records a is pending sync. Enqueuing the same (TicketID,
	// DeviceID, ScannedAt) twice is safe and does not create a duplicate
	// pending entry.
	Enqueue(ctx context.Context, a QueuedAdmission) error
	// Pending returns up to limit not-yet-synced admissions, oldest first.
	// limit <= 0 means "no limit".
	Pending(ctx context.Context, limit int) ([]QueuedAdmission, error)
	// MarkSynced marks the given admissions (matched by SyncKey) as synced
	// so they no longer appear in Pending.
	MarkSynced(ctx context.Context, keys []SyncKey) error
}

// --- in-memory Queue --------------------------------------------------

// MemoryQueue is an in-process Queue. Useful for tests and for scanners
// that don't need the pending queue to survive a process restart.
type MemoryQueue struct {
	mu     sync.Mutex
	byKey  map[SyncKey]QueuedAdmission
	order  []SyncKey // insertion order, for oldest-first Pending
	synced map[SyncKey]bool
}

// NewMemoryQueue returns an empty, ready-to-use MemoryQueue.
func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		byKey:  make(map[SyncKey]QueuedAdmission),
		synced: make(map[SyncKey]bool),
	}
}

func (q *MemoryQueue) Enqueue(_ context.Context, a QueuedAdmission) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	k := keyOf(a)
	if _, exists := q.byKey[k]; !exists {
		q.order = append(q.order, k)
	}
	q.byKey[k] = a
	return nil
}

func (q *MemoryQueue) Pending(_ context.Context, limit int) ([]QueuedAdmission, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []QueuedAdmission
	for _, k := range q.order {
		if q.synced[k] {
			continue
		}
		out = append(out, q.byKey[k])
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (q *MemoryQueue) MarkSynced(_ context.Context, keys []SyncKey) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, k := range keys {
		q.synced[k] = true
	}
	return nil
}

// --- SQLite-backed Queue -----------------------------------------------

// SQLiteQueue is a durable Queue backed by its own SQLite table,
// independent of internal/store. Use this on a real gate device so the
// pending-sync queue survives a reboot mid-event.
type SQLiteQueue struct {
	db *sql.DB
}

// OpenSQLiteQueue opens (creating if necessary) a SQLite database at path
// and ensures its own `scan_queue` table exists. Use ":memory:" for an
// ephemeral, isolated in-memory database.
func OpenSQLiteQueue(path string) (*SQLiteQueue, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=busy_timeout(5000)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("scan: open queue db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("scan: ping queue db: %w", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS scan_queue (
		ticket_id  TEXT NOT NULL,
		event_id   TEXT NOT NULL,
		gate_id    TEXT NOT NULL,
		device_id  TEXT NOT NULL,
		scanned_at TEXT NOT NULL,
		result     TEXT NOT NULL,
		note       TEXT NOT NULL DEFAULT '',
		synced     INTEGER NOT NULL DEFAULT 0,
		enqueued_at TEXT NOT NULL,
		PRIMARY KEY (ticket_id, device_id, scanned_at)
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("scan: create scan_queue table: %w", err)
	}

	return &SQLiteQueue{db: db}, nil
}

// Close closes the underlying database connection.
func (q *SQLiteQueue) Close() error { return q.db.Close() }

func (q *SQLiteQueue) Enqueue(ctx context.Context, a QueuedAdmission) error {
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO scan_queue (ticket_id, event_id, gate_id, device_id, scanned_at, result, note, synced, enqueued_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT (ticket_id, device_id, scanned_at) DO UPDATE SET
			event_id = excluded.event_id,
			gate_id  = excluded.gate_id,
			result   = excluded.result,
			note     = excluded.note
	`,
		a.TicketID, a.EventID, a.GateID, a.DeviceID, a.ScannedAt.UTC().Format(time.RFC3339Nano),
		string(a.Result), a.Note, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("scan: enqueue: %w", err)
	}
	return nil
}

func (q *SQLiteQueue) Pending(ctx context.Context, limit int) ([]QueuedAdmission, error) {
	query := `SELECT ticket_id, event_id, gate_id, device_id, scanned_at, result, note
	           FROM scan_queue WHERE synced = 0 ORDER BY enqueued_at ASC`
	args := []any{}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("scan: pending query: %w", err)
	}
	defer rows.Close()

	var out []QueuedAdmission
	for rows.Next() {
		var a QueuedAdmission
		var scannedAt, result string
		if err := rows.Scan(&a.TicketID, &a.EventID, &a.GateID, &a.DeviceID, &scannedAt, &result, &a.Note); err != nil {
			return nil, fmt.Errorf("scan: pending scan: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, scannedAt)
		if err != nil {
			return nil, fmt.Errorf("scan: pending parse scanned_at: %w", err)
		}
		a.ScannedAt = t
		a.Result = Status(result)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (q *SQLiteQueue) MarkSynced(ctx context.Context, keys []SyncKey) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("scan: mark synced begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	stmt, err := tx.PrepareContext(ctx, `UPDATE scan_queue SET synced = 1 WHERE ticket_id = ? AND device_id = ? AND scanned_at = ?`)
	if err != nil {
		return fmt.Errorf("scan: mark synced prepare: %w", err)
	}
	defer stmt.Close()

	for _, k := range keys {
		if _, err := stmt.ExecContext(ctx, k.TicketID, k.DeviceID, k.ScannedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("scan: mark synced exec: %w", err)
		}
	}
	return tx.Commit()
}

// --- idempotent sync sink ----------------------------------------------

// SyncSink is what a device's offline Queue eventually syncs into once
// back online — conceptually the central admissions store. This package
// defines the interface and a reference in-memory implementation for
// tests; the real, server-side implementation (backed by internal/store)
// is wired up elsewhere and must honour the same idempotency contract.
type SyncSink interface {
	// Apply idempotently applies a batch: applying an admission whose
	// SyncKey (ticket_id, device_id, scanned_at) has already been applied
	// must be a no-op — it must not create a duplicate entry or otherwise
	// change state. The returned slice reports, per input item in order,
	// whether it was newly applied (true) or already present (false).
	Apply(ctx context.Context, batch []QueuedAdmission) ([]bool, error)
}

// MemorySyncSink is a reference SyncSink implementation, keyed by SyncKey.
type MemorySyncSink struct {
	mu      sync.Mutex
	applied map[SyncKey]QueuedAdmission
	order   []SyncKey
}

// NewMemorySyncSink returns an empty, ready-to-use MemorySyncSink.
func NewMemorySyncSink() *MemorySyncSink {
	return &MemorySyncSink{applied: make(map[SyncKey]QueuedAdmission)}
}

func (s *MemorySyncSink) Apply(_ context.Context, batch []QueuedAdmission) ([]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := make([]bool, len(batch))
	for i, a := range batch {
		k := keyOf(a)
		if _, exists := s.applied[k]; exists {
			results[i] = false
			continue
		}
		s.applied[k] = a
		s.order = append(s.order, k)
		results[i] = true
	}
	return results, nil
}

// All returns every applied admission, in the order first applied
// (append-only — never overwritten by a later duplicate Apply of the same
// key). Test/diagnostic helper.
func (s *MemorySyncSink) All() []QueuedAdmission {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]QueuedAdmission, 0, len(s.order))
	for _, k := range s.order {
		out = append(out, s.applied[k])
	}
	return out
}

// --- deterministic cross-device reconciliation --------------------------

// ReconcileTicket takes every locally-recorded "admitted" attempt for a
// SINGLE ticket_id — gathered, potentially, from more than one gate device
// that was offline and had no way to know about the other's scan — and
// returns the canonical outcome: exactly one attempt is deemed the winner
// (Status stays Admitted) and every other attempt is downgraded to
// Duplicate.
//
// The winner is chosen by a fixed, total order that does not depend on
// input order or on wall-clock arrival time at the reconciler: earliest
// ScannedAt wins; ties are broken by DeviceID (lexicographically smallest)
// and then, for full determinism even in pathological test inputs, by the
// original Note field. Feeding the same set of attempts through
// ReconcileTicket in any permutation always produces the same output.
//
// Attempts with a Status other than Admitted are returned unchanged —
// reconciliation only ever resolves conflicts between attempts that each
// independently believed they were the (sole) first admission.
func ReconcileTicket(attempts []QueuedAdmission) []QueuedAdmission {
	out := make([]QueuedAdmission, len(attempts))
	copy(out, attempts)

	type indexed struct {
		idx int
		a   QueuedAdmission
	}
	var admitted []indexed
	for i, a := range out {
		if a.Result == Admitted {
			admitted = append(admitted, indexed{idx: i, a: a})
		}
	}
	if len(admitted) <= 1 {
		return out
	}

	sort.Slice(admitted, func(i, j int) bool {
		ai, aj := admitted[i].a, admitted[j].a
		if !ai.ScannedAt.Equal(aj.ScannedAt) {
			return ai.ScannedAt.Before(aj.ScannedAt)
		}
		if ai.DeviceID != aj.DeviceID {
			return ai.DeviceID < aj.DeviceID
		}
		return ai.Note < aj.Note
	})

	// admitted[0] is the deterministic winner; downgrade the rest.
	for i := 1; i < len(admitted); i++ {
		out[admitted[i].idx].Result = Duplicate
	}
	return out
}
