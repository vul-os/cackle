package httpapi

import (
	"net/http"
	"testing"
	"time"
)

// Batch order IS the winner rule for a contested ticket, and this file is
// what says so out loud.
//
// dbSyncSink.Apply walks an uploaded batch in array order. The first
// 'admitted' claim it sees for a ticket keeps 'admitted'; every later claim
// for that same ticket is rewritten to 'duplicate', with the device's own
// verdict preserved in reported_result so
// GET /api/events/{id}/admission-conflicts can still see that a second door
// let somebody through. Nothing in the code or the tests previously stated
// that array position decides it — it was simply what the loop did.
//
// It matters because of what sits upstream. web/src/lib/scan-store.js is the
// queue that actually runs on the phones at the doors, and until it was fixed
// it read its queue back through an IndexedDB index, which returns rows
// ordered by index key and then by PRIMARY key — a random UUID v4. The batch a
// gate uploaded was ordered by a random number, so this rule was resolving
// contested tickets by coin toss. The client now uploads in enqueue order.
// These tests are the server half of that contract: they pin the rule the
// client's ordering is worth something against, and they would have caught
// anyone quietly changing it.
//
// MUTATION: sort `batch` inside dbSyncSink.Apply by anything — ScannedAt,
// DeviceID — before the loop, and TestScanSync_BatchOrderDecidesTheWinner or
// TestScanSync_ABackdatedScannedAtDoesNotWinTheTicket fails.
//
// What this does NOT change: a cross-gate double-scan is DETECTED, NEVER
// PREVENTED. Both gates below admitted a real human being through a real door
// while offline, and no ordering rule anywhere unwinds that. All the server
// decides is which of the two claims gets to be the one 'admitted' row
// afterwards, and what the audit trail says. See docs/OFFLINE-GATES.md.

// syncOneBatch uploads a batch and returns the per-item applied flags.
func (h *testHarness) syncOneBatch(t *testing.T, fx eventFixture, items []scanSyncItem) []bool {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, scanSyncRequest{Admissions: items})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/scan/sync: status %d body %s", rec.Code, rec.Body.String())
	}
	return decodeBody[scanSyncResponse](t, rec).Applied
}

// storedClaims reads back what the server actually wrote for one ticket:
// (device_id, result, reported_result), in insertion order.
type storedClaim struct {
	DeviceID       string
	Result         string
	ReportedResult string
}

func (h *testHarness) storedClaims(t *testing.T, ticketID string) []storedClaim {
	t.Helper()
	rows, err := h.store.DB().Query(
		`SELECT device_id, result, COALESCE(reported_result, '') FROM admissions WHERE ticket_id = ? ORDER BY rowid`,
		ticketID)
	if err != nil {
		t.Fatalf("read admissions for %s: %v", ticketID, err)
	}
	defer rows.Close()
	var out []storedClaim
	for rows.Next() {
		var c storedClaim
		if err := rows.Scan(&c.DeviceID, &c.Result, &c.ReportedResult); err != nil {
			t.Fatalf("scan admission row: %v", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate admissions: %v", err)
	}
	return out
}

// admittedDevice returns the single device holding the 'admitted' row, and
// fails if there is not exactly one — the invariant the partial unique index
// on admissions(ticket_id) WHERE result='admitted' exists to keep.
func admittedDevice(t *testing.T, claims []storedClaim) string {
	t.Helper()
	winner := ""
	for _, c := range claims {
		if c.Result == "admitted" {
			if winner != "" {
				t.Fatalf("two rows are 'admitted' for one ticket: %+v", claims)
			}
			winner = c.DeviceID
		}
	}
	if winner == "" {
		t.Fatalf("no row is 'admitted' for a ticket two gates admitted: %+v", claims)
	}
	return winner
}

// TestScanSync_BatchOrderDecidesTheWinner is the contested case, constructed
// explicitly so it cannot be satisfied by a server that would have reached the
// same answer whatever order the batch arrived in.
//
// Two gates, both offline from each other, both scan the SAME ticket and both
// admit — one person, two entrances. Neither could know about the other. The
// two claims are then uploaded together, and the SAME two claims are uploaded
// in the opposite order to a second, identical event.
//
// Different order, different winner. That is the whole point: the array
// position is the rule, so the order the device-side queue produces is not a
// cosmetic detail — it is the input to this decision.
func TestScanSync_BatchOrderDecidesTheWinner(t *testing.T) {
	// Relative to now, and in the past: the conflicts report replays these
	// claims through the sync engine, which refuses a claim stamped in the
	// future. A fixed calendar date would work today and rot.
	at := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	// contest sets up an event with one ticket, uploads the two gates' claims
	// in the given order, and returns the device that ended up 'admitted'.
	contest := func(t *testing.T, name string, first, second string) (string, []storedClaim, admissionConflictsResponse) {
		t.Helper()
		h := newTestHarness(t)
		fx := h.newPublishedEvent(t, name)
		buyerToken, _ := h.signupUser("buyer-"+name+"@example.com", "buyer-password-123", "Nomvula Dlamini")
		tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
			"buyer-"+name+"@example.com", "Nomvula Dlamini", 1)
		shared := tickets[0]

		// Both gates report 'admitted', because that is genuinely what each
		// did at its door. Sending anything else would erase the conflict.
		applied := h.syncOneBatch(t, fx, []scanSyncItem{
			{TicketID: shared.ID, EventID: fx.eventID, GateID: "gate-" + first, DeviceID: first,
				ScannedAt: at, Result: "admitted"},
			{TicketID: shared.ID, EventID: fx.eventID, GateID: "gate-" + second, DeviceID: second,
				ScannedAt: at.Add(30 * time.Second), Result: "admitted"},
		})
		if len(applied) != 2 || !applied[0] || !applied[1] {
			t.Fatalf("both claims must be recorded, not dropped: %+v", applied)
		}
		claims := h.storedClaims(t, shared.ID)
		if len(claims) != 2 {
			t.Fatalf("expected both claims stored, got %d: %+v", len(claims), claims)
		}
		return admittedDevice(t, claims), claims, h.conflicts(t, fx, http.StatusOK)
	}

	winnerAB, claimsAB, conflictsAB := contest(t, "orderab", "device-A", "device-B")
	if winnerAB != "device-A" {
		t.Fatalf("uploaded [A, B]: the FIRST admitted claim in the batch must keep 'admitted', got %q (%+v)",
			winnerAB, claimsAB)
	}

	winnerBA, claimsBA, _ := contest(t, "orderba", "device-B", "device-A")
	if winnerBA != "device-B" {
		t.Fatalf("uploaded [B, A]: the FIRST admitted claim in the batch must keep 'admitted', got %q (%+v)",
			winnerBA, claimsBA)
	}

	if winnerAB == winnerBA {
		t.Fatal("the same two claims produced the same winner in both orders — batch order " +
			"is not actually deciding anything here, so this fixture proves nothing about " +
			"the client's ordering and needs rebuilding")
	}

	// The loser's own verdict survives. Without it, the downgrade would erase
	// the fact that a second door let somebody through, and the conflicts
	// report could not tell a real double admission from an ordinary
	// same-device duplicate. See migration 0002 and reconcile_handlers.go.
	for _, c := range claimsAB {
		if c.ReportedResult != "admitted" {
			t.Fatalf("device %s reported 'admitted' at its door; stored reported_result is %q: %+v",
				c.DeviceID, c.ReportedResult, claimsAB)
		}
	}

	// And the limit is unchanged. Deciding the winner deterministically does
	// not stop the second person walking in — it is still detected, after the
	// fact, and never prevented.
	if len(conflictsAB.Conflicts) != 1 || conflictsAB.ExtraAdmissions != 1 {
		t.Fatalf("one extra person got in and the report must still say so: %+v", conflictsAB)
	}
}

// TestScanSync_ABackdatedScannedAtDoesNotWinTheTicket pins the choice NOT to
// make the server re-order a batch by scanned_at.
//
// scanned_at is the wall clock of the device at the door, supplied by that
// device. If the server sorted a batch by it before applying, a phone with a
// skewed — or deliberately backdated — clock could move its own claims ahead
// of everyone else's and take a contested ticket. It cannot: the claim listed
// SECOND here carries a stamp a full day earlier than the first, and still
// loses.
//
// This is also why the device-side queue orders on a local sequence number
// rather than scanned_at (web/src/lib/scan-store.js) — the same reasoning, one
// layer down, and internal/scan's SQLiteQueue.Pending orders by rowid for the
// same reason again.
func TestScanSync_ABackdatedScannedAtDoesNotWinTheTicket(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "backdated")
	buyerToken, _ := h.signupUser("buyer-backdated@example.com", "buyer-password-123", "Lerato Molefe")
	tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-backdated@example.com", "Lerato Molefe", 1)
	shared := tickets[0]

	honest := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	backdated := honest.Add(-24 * time.Hour)

	applied := h.syncOneBatch(t, fx, []scanSyncItem{
		{TicketID: shared.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-honest",
			ScannedAt: honest, Result: "admitted"},
		{TicketID: shared.ID, EventID: fx.eventID, GateID: "South", DeviceID: "device-backdated",
			ScannedAt: backdated, Result: "admitted"},
	})
	if len(applied) != 2 || !applied[0] || !applied[1] {
		t.Fatalf("both claims must be recorded: %+v", applied)
	}

	claims := h.storedClaims(t, shared.ID)
	if got := admittedDevice(t, claims); got != "device-honest" {
		t.Fatalf("the earlier CLAIMED timestamp must not win the ticket; 'admitted' went to %q: %+v",
			got, claims)
	}
	for _, c := range claims {
		if c.ReportedResult != "admitted" {
			t.Fatalf("both devices admitted at their doors and both must say so: %+v", claims)
		}
	}
}

// TestScanSync_OneDevicesBatchAppliesInTheOrderItWasSent is the ordinary case
// the fixed device-side queue produces: ONE gate, several scans, uploaded
// oldest-first. The ticket it scanned twice must keep the FIRST scan as the
// admitted one, not whichever row happened to be applied first.
func TestScanSync_OneDevicesBatchAppliesInTheOrderItWasSent(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "onedevice")
	buyerToken, _ := h.signupUser("buyer-onedevice@example.com", "buyer-password-123", "Thandi Nkosi")
	tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-onedevice@example.com", "Thandi Nkosi", 2)
	reused, other := tickets[0], tickets[1]

	base := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	applied := h.syncOneBatch(t, fx, []scanSyncItem{
		{TicketID: reused.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base, Result: "admitted"},
		{TicketID: other.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base.Add(time.Minute), Result: "admitted"},
		// The same ticket presented again at the same door. The gate refused
		// it locally, so it uploads its own verdict of 'duplicate'.
		{TicketID: reused.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base.Add(2 * time.Minute), Result: "duplicate"},
	})
	if len(applied) != 3 {
		t.Fatalf("expected 3 applied flags, got %d: %+v", len(applied), applied)
	}
	for i, ok := range applied {
		if !ok {
			t.Fatalf("item %d was not applied: %+v", i, applied)
		}
	}

	claims := h.storedClaims(t, reused.ID)
	if len(claims) != 2 {
		t.Fatalf("expected 2 rows for the twice-scanned ticket, got %d: %+v", len(claims), claims)
	}
	if claims[0].Result != "admitted" || claims[0].ReportedResult != "admitted" {
		t.Fatalf("the first scan of the batch is the admitted one: %+v", claims)
	}
	if claims[1].Result != "duplicate" || claims[1].ReportedResult != "duplicate" {
		t.Fatalf("the second presentation stays a duplicate, reported as one: %+v", claims)
	}
	if got := admittedDevice(t, claims); got != "device-A" {
		t.Fatalf("unexpected admitted device %q: %+v", got, claims)
	}
	if others := h.storedClaims(t, other.ID); len(others) != 1 || others[0].Result != "admitted" {
		t.Fatalf("the other ticket is admitted once: %+v", others)
	}
}
