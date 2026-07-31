package scan

import (
	"context"
	"testing"
	"time"
)

func queueFactories(t *testing.T) map[string]func() Queue {
	return map[string]func() Queue{
		"memory": func() Queue { return NewMemoryQueue() },
		"sqlite": func() Queue {
			q, err := OpenSQLiteQueue(":memory:")
			if err != nil {
				t.Fatalf("OpenSQLiteQueue: %v", err)
			}
			t.Cleanup(func() { q.Close() })
			return q
		},
	}
}

func sampleAdmission(ticketID, deviceID string, at time.Time, status Status) QueuedAdmission {
	return QueuedAdmission{
		TicketID:  ticketID,
		EventID:   "event-1",
		GateID:    "gate-1",
		DeviceID:  deviceID,
		ScannedAt: at,
		Result:    status,
		Note:      "",
	}
}

func TestQueue_EnqueueAndPending(t *testing.T) {
	for name, factory := range queueFactories(t) {
		t.Run(name, func(t *testing.T) {
			q := factory()
			ctx := context.Background()
			now := time.Unix(1000, 0)

			a1 := sampleAdmission("t1", "d1", now, Admitted)
			a2 := sampleAdmission("t2", "d1", now.Add(time.Second), Admitted)

			if err := q.Enqueue(ctx, a1); err != nil {
				t.Fatalf("Enqueue a1: %v", err)
			}
			if err := q.Enqueue(ctx, a2); err != nil {
				t.Fatalf("Enqueue a2: %v", err)
			}

			pending, err := q.Pending(ctx, 0)
			if err != nil {
				t.Fatalf("Pending: %v", err)
			}
			if len(pending) != 2 {
				t.Fatalf("expected 2 pending, got %d: %+v", len(pending), pending)
			}
			// oldest first
			if pending[0].TicketID != "t1" || pending[1].TicketID != "t2" {
				t.Fatalf("expected oldest-first order, got %+v", pending)
			}
		})
	}
}

func TestQueue_EnqueueSameKeyTwice_NotDuplicated(t *testing.T) {
	for name, factory := range queueFactories(t) {
		t.Run(name, func(t *testing.T) {
			q := factory()
			ctx := context.Background()
			at := time.Unix(1000, 0)

			a := sampleAdmission("t1", "d1", at, Admitted)
			if err := q.Enqueue(ctx, a); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			if err := q.Enqueue(ctx, a); err != nil {
				t.Fatalf("Enqueue (again): %v", err)
			}

			pending, err := q.Pending(ctx, 0)
			if err != nil {
				t.Fatalf("Pending: %v", err)
			}
			if len(pending) != 1 {
				t.Fatalf("expected exactly 1 pending entry after enqueuing the same key twice, got %d", len(pending))
			}
		})
	}
}

func TestQueue_MarkSynced_RemovesFromPending(t *testing.T) {
	for name, factory := range queueFactories(t) {
		t.Run(name, func(t *testing.T) {
			q := factory()
			ctx := context.Background()
			at := time.Unix(1000, 0)

			a := sampleAdmission("t1", "d1", at, Admitted)
			if err := q.Enqueue(ctx, a); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			if err := q.MarkSynced(ctx, []SyncKey{keyOf(a)}); err != nil {
				t.Fatalf("MarkSynced: %v", err)
			}
			pending, err := q.Pending(ctx, 0)
			if err != nil {
				t.Fatalf("Pending: %v", err)
			}
			if len(pending) != 0 {
				t.Fatalf("expected 0 pending after MarkSynced, got %d", len(pending))
			}
		})
	}
}

func TestQueue_PendingLimit(t *testing.T) {
	for name, factory := range queueFactories(t) {
		t.Run(name, func(t *testing.T) {
			q := factory()
			ctx := context.Background()
			for i := 0; i < 5; i++ {
				a := sampleAdmission("t"+string(rune('0'+i)), "d1", time.Unix(int64(1000+i), 0), Admitted)
				if err := q.Enqueue(ctx, a); err != nil {
					t.Fatalf("Enqueue: %v", err)
				}
			}
			pending, err := q.Pending(ctx, 2)
			if err != nil {
				t.Fatalf("Pending: %v", err)
			}
			if len(pending) != 2 {
				t.Fatalf("expected limit=2 to cap results at 2, got %d", len(pending))
			}
		})
	}
}

func TestSQLiteQueue_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/queue.db"

	q1, err := OpenSQLiteQueue(path)
	if err != nil {
		t.Fatalf("OpenSQLiteQueue: %v", err)
	}
	a := sampleAdmission("t1", "d1", time.Unix(1000, 0), Admitted)
	if err := q1.Enqueue(context.Background(), a); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	q2, err := OpenSQLiteQueue(path)
	if err != nil {
		t.Fatalf("re-OpenSQLiteQueue: %v", err)
	}
	defer q2.Close()
	pending, err := q2.Pending(context.Background(), 0)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 1 || pending[0].TicketID != "t1" {
		t.Fatalf("expected queue to survive reopen, got %+v", pending)
	}
}

// --- idempotent sync sink ------------------------------------------------

func TestMemorySyncSink_ApplyIsIdempotent(t *testing.T) {
	sink := NewMemorySyncSink()
	ctx := context.Background()
	at := time.Unix(1000, 0)
	batch := []QueuedAdmission{sampleAdmission("t1", "d1", at, Admitted)}

	results1, err := sink.Apply(ctx, batch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(results1) != 1 || !results1[0] {
		t.Fatalf("expected first Apply to report newly-applied=true, got %v", results1)
	}

	// Re-upload the SAME batch (e.g. retried after a dropped ack).
	results2, err := sink.Apply(ctx, batch)
	if err != nil {
		t.Fatalf("Apply (retry): %v", err)
	}
	if len(results2) != 1 || results2[0] {
		t.Fatalf("expected retried Apply to report newly-applied=false, got %v", results2)
	}

	if got := len(sink.All()); got != 1 {
		t.Fatalf("expected exactly 1 admission stored after duplicate sync, got %d", got)
	}
}

func TestMemorySyncSink_DifferentDevicesSameTicketBothApplied(t *testing.T) {
	// Two different devices scanning the same ticket at different times
	// are DIFFERENT SyncKeys (device_id differs) — both get applied to the
	// sink; reconciling which one is the "true" admission is
	// ReconcileTicket's job, not Apply's.
	sink := NewMemorySyncSink()
	ctx := context.Background()
	batch := []QueuedAdmission{
		sampleAdmission("t1", "device-A", time.Unix(1000, 0), Admitted),
		sampleAdmission("t1", "device-B", time.Unix(1001, 0), Admitted),
	}
	results, err := sink.Apply(ctx, batch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(results) != 2 || !results[0] || !results[1] {
		t.Fatalf("expected both distinct-device admissions to be newly applied, got %v", results)
	}
	if got := len(sink.All()); got != 2 {
		t.Fatalf("expected 2 stored admissions, got %d", got)
	}
}

func TestMemorySyncSink_AppendOnly_NeverOverwrites(t *testing.T) {
	sink := NewMemorySyncSink()
	ctx := context.Background()
	at := time.Unix(1000, 0)

	original := sampleAdmission("t1", "d1", at, Admitted)
	original.Note = "original"
	if _, err := sink.Apply(ctx, []QueuedAdmission{original}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Same SyncKey, different Note — a well-behaved sync client would
	// never do this, but Apply must still refuse to overwrite the
	// original rather than silently accept mutated data under the same key.
	mutated := original
	mutated.Note = "mutated-attempt"
	results, err := sink.Apply(ctx, []QueuedAdmission{mutated})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if results[0] {
		t.Fatalf("expected Apply to treat same-key input as a duplicate, not apply the mutated version")
	}

	all := sink.All()
	if len(all) != 1 || all[0].Note != "original" {
		t.Fatalf("expected stored admission to remain the ORIGINAL, unmodified; got %+v", all)
	}
}

// --- deterministic cross-device reconciliation ---------------------------

func TestReconcileTicket_SingleAdmission_Unchanged(t *testing.T) {
	in := []QueuedAdmission{sampleAdmission("t1", "d1", time.Unix(1000, 0), Admitted)}
	out := ReconcileTicket(in)
	if len(out) != 1 || out[0].Result != Admitted {
		t.Fatalf("expected single admission to stay Admitted, got %+v", out)
	}
}

func TestReconcileTicket_TwoDevices_EarliestWins(t *testing.T) {
	early := sampleAdmission("t1", "device-B", time.Unix(1000, 0), Admitted)
	late := sampleAdmission("t1", "device-A", time.Unix(2000, 0), Admitted)

	out := ReconcileTicket([]QueuedAdmission{late, early}) // note: late passed first
	var earlyResult, lateResult Status
	for _, a := range out {
		if a.DeviceID == "device-B" {
			earlyResult = a.Result
		}
		if a.DeviceID == "device-A" {
			lateResult = a.Result
		}
	}
	if earlyResult != Admitted {
		t.Fatalf("expected earliest scan (device-B) to remain Admitted, got %s", earlyResult)
	}
	if lateResult != Duplicate {
		t.Fatalf("expected later scan (device-A) to become Duplicate, got %s", lateResult)
	}
}

// TestReconcileTicket_DeterministicAcrossPermutations is the key property
// requested: feeding the exact same set of cross-device admission attempts
// through ReconcileTicket, in every possible order, must always produce
// the same winner.
func TestReconcileTicket_DeterministicAcrossPermutations(t *testing.T) {
	base := []QueuedAdmission{
		sampleAdmission("t1", "device-C", time.Unix(1000, 0), Admitted),
		sampleAdmission("t1", "device-A", time.Unix(1000, 0), Admitted), // same timestamp as above, tie-break on device id
		sampleAdmission("t1", "device-B", time.Unix(1500, 0), Admitted),
	}

	permutations := [][]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	}

	var canonicalWinner string
	for i, perm := range permutations {
		permuted := make([]QueuedAdmission, len(perm))
		for j, idx := range perm {
			permuted[j] = base[idx]
		}
		out := ReconcileTicket(permuted)

		var winner string
		admittedCount := 0
		for _, a := range out {
			if a.Result == Admitted {
				admittedCount++
				winner = a.DeviceID
			}
		}
		if admittedCount != 1 {
			t.Fatalf("permutation %d: expected exactly 1 Admitted, got %d in %+v", i, admittedCount, out)
		}
		if i == 0 {
			canonicalWinner = winner
		} else if winner != canonicalWinner {
			t.Fatalf("permutation %d: winner %q differs from canonical winner %q — reconciliation is not order-independent", i, winner, canonicalWinner)
		}
	}

	// device-A and device-C tie on ScannedAt; device-A sorts first
	// lexicographically, so it must be the deterministic winner over both
	// device-C (same timestamp) and device-B (later timestamp).
	if canonicalWinner != "device-A" {
		t.Fatalf("expected device-A to win the deterministic tie-break, got %q", canonicalWinner)
	}
}

func TestReconcileTicket_NonAdmittedAttemptsUnaffected(t *testing.T) {
	in := []QueuedAdmission{
		sampleAdmission("t1", "device-A", time.Unix(1000, 0), Admitted),
		sampleAdmission("t1", "device-B", time.Unix(1001, 0), Invalid),
		sampleAdmission("t1", "device-C", time.Unix(1002, 0), WrongEvent),
	}
	out := ReconcileTicket(in)
	for _, a := range out {
		switch a.DeviceID {
		case "device-A":
			if a.Result != Admitted {
				t.Fatalf("expected device-A to remain Admitted, got %s", a.Result)
			}
		case "device-B":
			if a.Result != Invalid {
				t.Fatalf("expected device-B to remain Invalid, got %s", a.Result)
			}
		case "device-C":
			if a.Result != WrongEvent {
				t.Fatalf("expected device-C to remain WrongEvent, got %s", a.Result)
			}
		}
	}
}

// TestFullOfflineToSyncFlow exercises the whole local lifecycle: two
// devices independently admit the same ticket while both offline, each
// enqueues its own local admission, both eventually sync (idempotently)
// into a shared sink, and reconciliation deterministically picks one
// winner regardless of sync arrival order.
func TestFullOfflineToSyncFlow_CrossDeviceDuplicateResolvesDeterministically(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-shared", 1000, 5000)

	deviceA := NewMemoryQueue()
	deviceB := NewMemoryQueue()
	seenA := NewMemorySeenSet()
	seenB := NewMemorySeenSet()
	ctx := context.Background()

	atA := time.Unix(1100, 0)
	atB := time.Unix(1200, 0) // device B scanned slightly later

	resA := Decide(ctx, token, ring, "event-1", seenA, atA)
	resB := Decide(ctx, token, ring, "event-1", seenB, atB)
	// Both devices are offline and isolated, so BOTH locally believe they
	// admitted this ticket first.
	if resA.Status != Admitted || resB.Status != Admitted {
		t.Fatalf("expected both isolated devices to locally admit, got A=%s B=%s", resA.Status, resB.Status)
	}

	if err := deviceA.Enqueue(ctx, QueuedAdmission{TicketID: "ticket-shared", EventID: "event-1", GateID: "gate-1", DeviceID: "device-A", ScannedAt: atA, Result: Admitted}); err != nil {
		t.Fatalf("enqueue A: %v", err)
	}
	if err := deviceB.Enqueue(ctx, QueuedAdmission{TicketID: "ticket-shared", EventID: "event-1", GateID: "gate-1", DeviceID: "device-B", ScannedAt: atB, Result: Admitted}); err != nil {
		t.Fatalf("enqueue B: %v", err)
	}

	pendingA, _ := deviceA.Pending(ctx, 0)
	pendingB, _ := deviceB.Pending(ctx, 0)

	// Sync in one order...
	sink1 := NewMemorySyncSink()
	sink1.Apply(ctx, pendingB) //nolint:errcheck
	sink1.Apply(ctx, pendingA) //nolint:errcheck
	reconciled1 := ReconcileTicket(sink1.All())

	// ...and the reverse order.
	sink2 := NewMemorySyncSink()
	sink2.Apply(ctx, pendingA) //nolint:errcheck
	sink2.Apply(ctx, pendingB) //nolint:errcheck
	reconciled2 := ReconcileTicket(sink2.All())

	winner1 := winnerDevice(t, reconciled1)
	winner2 := winnerDevice(t, reconciled2)

	if winner1 != winner2 {
		t.Fatalf("sync order affected the reconciled winner: %q vs %q", winner1, winner2)
	}
	if winner1 != "device-A" {
		t.Fatalf("expected device-A (earlier ScannedAt) to win, got %q", winner1)
	}
}

func winnerDevice(t *testing.T, admissions []QueuedAdmission) string {
	t.Helper()
	var winner string
	count := 0
	for _, a := range admissions {
		if a.Result == Admitted {
			count++
			winner = a.DeviceID
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 admitted winner, got %d in %+v", count, admissions)
	}
	return winner
}

// --- Pending's order must be total, chronological, and stable -------------
//
// The single edit that turns the three deterministic guards below red is
// restoring `ORDER BY enqueued_at ASC` in SQLiteQueue.Pending in place of
// `ORDER BY rowid ASC`. Each test names what that costs.

// setEnqueuedAt rewrites one row's enqueued_at directly, which is the only way
// to force a specific value: Enqueue stamps time.Now() itself. Every value fed
// in here is a value time.Now().UTC().Format(time.RFC3339Nano) can genuinely
// produce, so nothing below is a synthetic shape the real writer never emits.
func setEnqueuedAt(t *testing.T, q *SQLiteQueue, ticketID string, at time.Time) {
	t.Helper()
	res, err := q.db.Exec(`UPDATE scan_queue SET enqueued_at = ? WHERE ticket_id = ?`,
		at.UTC().Format(time.RFC3339Nano), ticketID)
	if err != nil {
		t.Fatalf("setEnqueuedAt(%s): %v", ticketID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("setEnqueuedAt(%s) rows affected: %v", ticketID, err)
	}
	if n != 1 {
		t.Fatalf("setEnqueuedAt(%s): expected to update exactly 1 row, updated %d", ticketID, n)
	}
}

func pendingTicketIDs(t *testing.T, q Queue, limit int) []string {
	t.Helper()
	pending, err := q.Pending(context.Background(), limit)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	ids := make([]string, 0, len(pending))
	for _, a := range pending {
		ids = append(ids, a.TicketID)
	}
	return ids
}

func newSQLiteQueue(t *testing.T) *SQLiteQueue {
	t.Helper()
	q, err := OpenSQLiteQueue(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteQueue: %v", err)
	}
	t.Cleanup(func() { q.Close() })
	return q
}

// enqueueTickets enqueues one admission per ticket id, in the order given.
func enqueueTickets(t *testing.T, q Queue, ids ...string) {
	t.Helper()
	ctx := context.Background()
	for i, id := range ids {
		a := sampleAdmission(id, "d1", time.Unix(int64(1000+i), 0), Admitted)
		if err := q.Enqueue(ctx, a); err != nil {
			t.Fatalf("Enqueue %s: %v", id, err)
		}
	}
}

// TestSQLiteQueue_PendingOrderSurvivesEqualEnqueuedAt forces the collision the
// contract has no answer for: several rows stamped with the SAME enqueued_at.
// `ORDER BY enqueued_at ASC` alone leaves their relative order up to SQLite,
// so the batch a gate uploads — and therefore which claim reaches an already
// contested ticket first — is unspecified. Pending's documented order is
// "oldest first", and for rows that were enqueued at the same instant the only
// honest reading of "oldest" is "inserted first".
//
// Stated so this test is not credited with more than it does: it did NOT fail
// against the un-tiebroken `ORDER BY enqueued_at ASC`. This SQLite build's
// sorter happens to preserve insertion order for equal keys, at every size
// tried up to 20,000 rows. That is incidental, not promised — SQLite defines
// the order of rows with equal ORDER BY keys as undefined — so what this test
// buys is a lock: a planner change, an index on enqueued_at, a dependency bump
// or a queue large enough to spill the sorter cannot quietly change which
// claim a gate uploads first. The tests below are the ones that were red.
func TestSQLiteQueue_PendingOrderSurvivesEqualEnqueuedAt(t *testing.T) {
	q := newSQLiteQueue(t)
	want := []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8"}
	enqueueTickets(t, q, want...)

	same := time.Date(2026, 7, 31, 12, 0, 0, 250_000_000, time.UTC)
	for _, id := range want {
		setEnqueuedAt(t, q, id, same)
	}

	// Repeated because an unstable sort is allowed to be accidentally right;
	// the property under test is that it is right EVERY time.
	for i := 0; i < 50; i++ {
		got := pendingTicketIDs(t, q, 0)
		if !equalStrings(got, want) {
			t.Fatalf("iteration %d: rows sharing an enqueued_at came back as %v, want insertion order %v", i, got, want)
		}
	}

	// A bounded page must be a PREFIX of that same order, not an arbitrary
	// subset of the tied rows.
	for i := 0; i < 50; i++ {
		got := pendingTicketIDs(t, q, 3)
		if !equalStrings(got, want[:3]) {
			t.Fatalf("iteration %d: Pending(limit=3) over tied rows returned %v, want the first three %v", i, got, want[:3])
		}
	}
}

// TestSQLiteQueue_PendingOrderIsChronologicalNotLexicographic is the case that
// actually bites in production, and it needs no collision at all.
//
// enqueued_at is written with time.RFC3339Nano, which STRIPS trailing zeros
// from the fractional second. The column is TEXT, so SQLite compares it
// lexicographically — and lexicographic order disagrees with chronological
// order whenever one timestamp's fraction is a prefix of the other's:
//
//	12:00:00Z    is BEFORE 12:00:00.5Z     but sorts after  ('Z' > '.')
//	12:00:00.1Z  is BEFORE 12:00:00.10001Z but sorts after  ('Z' > '0')
//
// Both left-hand values are ones time.Now() produces on its own — any instant
// whose nanoseconds happen to end in zeros. So the queue does not merely
// return an arbitrary order for a tie; it returns a definitively WRONG order,
// intermittently, with no tie involved.
//
// MUTATION: restore `ORDER BY enqueued_at ASC` and all three cases fail,
// deterministically, returning [second first].
func TestSQLiteQueue_PendingOrderIsChronologicalNotLexicographic(t *testing.T) {
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		firstDelta time.Duration // enqueued first, and genuinely earlier
		nextDelta  time.Duration
	}{
		// "…:00Z" vs "…:00.5Z" — the whole fraction is absent.
		{"absent fraction sorts after a present one", 0, 500 * time.Millisecond},
		// "…:00.1Z" vs "…:00.10001Z" — one fraction is a prefix of the other.
		{"shorter fraction is a prefix of the longer", 100 * time.Millisecond, 100_010 * time.Microsecond},
		// "…:00.25Z" vs "…:00.250001Z" — same shape, deeper.
		{"prefix collision several digits in", 250 * time.Millisecond, 250_001 * time.Microsecond},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !base.Add(tc.firstDelta).Before(base.Add(tc.nextDelta)) {
				t.Fatalf("test setup is wrong: %v is not before %v", tc.firstDelta, tc.nextDelta)
			}
			q := newSQLiteQueue(t)
			enqueueTickets(t, q, "first", "second")
			setEnqueuedAt(t, q, "first", base.Add(tc.firstDelta))
			setEnqueuedAt(t, q, "second", base.Add(tc.nextDelta))

			want := []string{"first", "second"}
			got := pendingTicketIDs(t, q, 0)
			if !equalStrings(got, want) {
				t.Fatalf("enqueued_at %q then %q: Pending returned %v, want %v",
					base.Add(tc.firstDelta).Format(time.RFC3339Nano),
					base.Add(tc.nextDelta).Format(time.RFC3339Nano), got, want)
			}
		})
	}
}

// TestSQLiteQueue_PendingOrderSurvivesClockStepBack pins the third way a
// timestamp column fails as an order: a gate device's clock is not monotonic.
// An NTP correction mid-event stamps a LATER enqueue with an EARLIER
// enqueued_at, and any ordering that trusts that column reorders the queue
// around it. Insertion order cannot be walked backwards this way.
//
// MUTATION: restore `ORDER BY enqueued_at ASC` and this fails
// deterministically.
func TestSQLiteQueue_PendingOrderSurvivesClockStepBack(t *testing.T) {
	q := newSQLiteQueue(t)
	enqueueTickets(t, q, "before-step", "after-step")

	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	setEnqueuedAt(t, q, "before-step", base.Add(30*time.Second))
	setEnqueuedAt(t, q, "after-step", base) // clock stepped back 30s

	want := []string{"before-step", "after-step"}
	if got := pendingTicketIDs(t, q, 0); !equalStrings(got, want) {
		t.Fatalf("after a backwards clock step Pending returned %v, want insertion order %v", got, want)
	}
}

// TestQueue_PendingOrderMatchesAcrossImplementations holds the two Queue
// implementations to ONE contract. MemoryQueue returns strict insertion order
// (its `order` slice), and a caller that swaps a MemoryQueue for a SQLiteQueue
// must not get a different batch order out of the same enqueues — including
// the re-enqueue case, where an existing key keeps its original position
// rather than jumping to the back.
//
// MUTATION: restore `ORDER BY enqueued_at ASC`. This one fails INTERMITTENTLY,
// because it runs on whatever stamps real time.Now() produced — which is
// precisely the flake profile that hid the bug. It caught the un-tiebroken
// ordering on the first run of this file, reporting [t1 t3 t2 t4] against an
// insertion order of [t1 t2 t3 t4]. It is kept for that: it is the only test
// here that exercises the actual writer. The guards above are the reliable
// ones.
func TestQueue_PendingOrderMatchesAcrossImplementations(t *testing.T) {
	ctx := context.Background()
	ids := []string{"t1", "t2", "t3", "t4"}

	orders := map[string][]string{}
	for name, factory := range queueFactories(t) {
		q := factory()
		enqueueTickets(t, q, ids...)
		// Re-enqueue the first one with an updated note: same key, so it must
		// keep its place at the head rather than move to the tail.
		again := sampleAdmission("t1", "d1", time.Unix(1000, 0), Admitted)
		again.Note = "re-enqueued"
		if err := q.Enqueue(ctx, again); err != nil {
			t.Fatalf("%s: re-Enqueue: %v", name, err)
		}
		orders[name] = pendingTicketIDs(t, q, 0)
	}

	if !equalStrings(orders["memory"], ids) {
		t.Fatalf("memory queue order %v, want %v", orders["memory"], ids)
	}
	if !equalStrings(orders["sqlite"], orders["memory"]) {
		t.Fatalf("sqlite order %v disagrees with memory order %v for the same enqueues",
			orders["sqlite"], orders["memory"])
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
