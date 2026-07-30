package store

import (
	"context"
	"testing"
	"time"
)

// seedAdmittableTicket creates the minimum chain of rows an admissions row's
// foreign keys require: org -> event -> ticket type -> order -> ticket.
//
// Every statement is a plain INSERT and every error is fatal. "INSERT OR
// IGNORE" would be shorter and would be a fail-open: it swallows CHECK and
// NOT NULL violations too, so a mistyped column here would silently seed
// nothing and the test would then fail somewhere unrelated, or — worse — pass
// against an empty table. Idempotence across repeated calls for one event is
// achieved by checking first instead.
func seedAdmittableTicket(t *testing.T, st *Store, eventID, ticketID string) {
	t.Helper()
	ctx := context.Background()
	db := st.DB()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	exists := func(table, id string) bool {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&n); err != nil {
			t.Fatalf("check %s %s: %v", table, id, err)
		}
		return n > 0
	}

	orgID := "org_" + eventID
	if !exists("orgs", orgID) {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO orgs (id, name, slug, created_at) VALUES (?, ?, ?, ?)`,
			orgID, "Org "+eventID, "org-"+eventID, now); err != nil {
			t.Fatalf("seed org: %v", err)
		}
	}
	if !exists("events", eventID) {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO events (id, org_id, slug, title, starts_at, ends_at, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'published', ?, ?)`,
			eventID, orgID, "ev-"+eventID, "Event "+eventID, now, now, now, now); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	ttID := "tt_" + eventID
	if !exists("ticket_types", ttID) {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO ticket_types (id, event_id, name, price_minor, quantity_total)
			 VALUES (?, ?, 'General', 1000, 100)`,
			ttID, eventID); err != nil {
			t.Fatalf("seed ticket type: %v", err)
		}
	}
	orderID := "ord_" + eventID
	if !exists("orders", orderID) {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO orders (id, event_id, buyer_email, status, total_minor, created_at)
			 VALUES (?, ?, 'b@example.com', 'paid', 1000, ?)`,
			orderID, eventID, now); err != nil {
			t.Fatalf("seed order: %v", err)
		}
	}
	if !exists("tickets", ticketID) {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO tickets (id, order_id, event_id, ticket_type_id, holder_name, serial,
			 capability, status, issued_at)
			 VALUES (?, ?, ?, ?, 'Holder', ?, 'cackle.x.y', 'valid', ?)`,
			ticketID, orderID, eventID, ttID, "SER-"+ticketID, now); err != nil {
			t.Fatalf("seed ticket: %v", err)
		}
	}
}

func insertAdmission(t *testing.T, st *Store, eventID, ticketID, deviceID, gateID, result, reported string, at time.Time) {
	t.Helper()
	if _, err := st.DB().ExecContext(context.Background(),
		`INSERT INTO admissions (id, ticket_id, event_id, gate_id, scanned_by, device_id, scanned_at,
		 result, reported_result, note) VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, '')`,
		NewID(), ticketID, eventID, gateID, deviceID, at.UTC().Format(time.RFC3339Nano), result, reported); err != nil {
		t.Fatalf("insert admission: %v", err)
	}
}

// TestListAdmissionClaimsForEvent_SurfacesCrossDeviceDoubleAdmission is the
// query's whole reason for existing: two devices that each believed they
// admitted one ticket. The second device's stored `result` is 'duplicate'
// (the server's correct downgrade) while its `reported_result` is 'admitted'
// (what actually happened at its door), and the query must key on the latter.
func TestListAdmissionClaimsForEvent_SurfacesCrossDeviceDoubleAdmission(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)

	seedAdmittableTicket(t, st, "ev1", "tkt_shared")
	insertAdmission(t, st, "ev1", "tkt_shared", "device-A", "North", "admitted", "admitted", base)
	insertAdmission(t, st, "ev1", "tkt_shared", "device-B", "South", "duplicate", "admitted", base.Add(time.Minute))

	claims, err := st.ListAdmissionClaimsForEvent(ctx, "ev1")
	if err != nil {
		t.Fatalf("ListAdmissionClaimsForEvent: %v", err)
	}
	if len(claims) != 2 {
		t.Fatalf("expected both claims, got %d: %+v", len(claims), claims)
	}
	// Ordered by scanned_at within a ticket.
	if claims[0].DeviceID != "device-A" || claims[1].DeviceID != "device-B" {
		t.Fatalf("claims not in scan order: %+v", claims)
	}
	if claims[1].Result != "duplicate" || claims[1].ReportedResult != "admitted" {
		t.Fatalf("the downgrade and the device's own claim must both survive: %+v", claims[1])
	}
	if !claims[0].ScannedAt.Equal(base) {
		t.Fatalf("scanned_at did not round trip: got %s want %s", claims[0].ScannedAt, base)
	}
}

// TestListAdmissionClaimsForEvent_IgnoresSingleDeviceAndOtherEvents keeps the
// query from crying wolf: a lone admission, a same-device duplicate, and
// another event's conflict must all be absent.
func TestListAdmissionClaimsForEvent_IgnoresSingleDeviceAndOtherEvents(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	seedAdmittableTicket(t, st, "ev1", "tkt_lone")
	seedAdmittableTicket(t, st, "ev1", "tkt_samedev")
	seedAdmittableTicket(t, st, "ev2", "tkt_other")

	// One device, one admission.
	insertAdmission(t, st, "ev1", "tkt_lone", "device-A", "North", "admitted", "admitted", base)
	// One device admitting then correctly refusing from its own log.
	insertAdmission(t, st, "ev1", "tkt_samedev", "device-A", "North", "admitted", "admitted", base)
	insertAdmission(t, st, "ev1", "tkt_samedev", "device-A", "North", "duplicate", "duplicate", base.Add(time.Minute))
	// A genuine conflict, but in a different event.
	insertAdmission(t, st, "ev2", "tkt_other", "device-A", "North", "admitted", "admitted", base)
	insertAdmission(t, st, "ev2", "tkt_other", "device-B", "South", "duplicate", "admitted", base.Add(time.Minute))

	claims, err := st.ListAdmissionClaimsForEvent(ctx, "ev1")
	if err != nil {
		t.Fatalf("ListAdmissionClaimsForEvent: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("expected no contested tickets in ev1, got %+v", claims)
	}

	// The other event's conflict is found when asked for by its own id, which
	// proves the emptiness above is scoping and not a broken query.
	other, err := st.ListAdmissionClaimsForEvent(ctx, "ev2")
	if err != nil {
		t.Fatalf("ListAdmissionClaimsForEvent(ev2): %v", err)
	}
	if len(other) != 2 {
		t.Fatalf("expected ev2's conflict to be found, got %+v", other)
	}
}

// TestListAdmissionClaimsForEvent_LegacyRowsWithNoReportedResult covers the
// pre-migration-0002 shape: reported_result defaults to ” and the query falls
// back to `result`, so an old two-device conflict recorded as
// admitted+duplicate is NOT reported (there is no evidence the second device
// claimed an admission) while two admitted rows would be.
func TestListAdmissionClaimsForEvent_LegacyRowsWithNoReportedResult(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	seedAdmittableTicket(t, st, "ev1", "tkt_legacy")
	insertAdmission(t, st, "ev1", "tkt_legacy", "device-A", "North", "admitted", "", base)
	insertAdmission(t, st, "ev1", "tkt_legacy", "device-B", "South", "duplicate", "", base.Add(time.Minute))

	claims, err := st.ListAdmissionClaimsForEvent(ctx, "ev1")
	if err != nil {
		t.Fatalf("ListAdmissionClaimsForEvent: %v", err)
	}
	// Honest under-reporting rather than invention: a legacy 'duplicate' row
	// carries no evidence that its device believed it was admitting, and
	// asserting otherwise would fabricate double admissions that may never
	// have happened.
	if len(claims) != 0 {
		t.Fatalf("a legacy admitted+duplicate pair carries no evidence of a double admission, got %+v", claims)
	}
}

// TestListAdmittedTicketIDsForEvent covers the bundle's convergence channel:
// the reconciled admitted set, scoped per event.
func TestListAdmittedTicketIDsForEvent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	seedAdmittableTicket(t, st, "ev1", "tkt_in")
	seedAdmittableTicket(t, st, "ev1", "tkt_refused")
	seedAdmittableTicket(t, st, "ev2", "tkt_elsewhere")

	insertAdmission(t, st, "ev1", "tkt_in", "device-A", "North", "admitted", "admitted", base)
	insertAdmission(t, st, "ev1", "tkt_refused", "device-A", "North", "invalid", "invalid", base)
	insertAdmission(t, st, "ev2", "tkt_elsewhere", "device-A", "North", "admitted", "admitted", base)

	ids, err := st.ListAdmittedTicketIDsForEvent(ctx, "ev1")
	if err != nil {
		t.Fatalf("ListAdmittedTicketIDsForEvent: %v", err)
	}
	if len(ids) != 1 || ids[0] != "tkt_in" {
		t.Fatalf("expected only the admitted ticket for ev1, got %+v", ids)
	}

	// Never nil: the bundle marshals this straight to JSON, and a nil slice
	// would serialise as null where the browser gate expects an array.
	none, err := st.ListAdmittedTicketIDsForEvent(ctx, "ev_nothing")
	if err != nil {
		t.Fatalf("ListAdmittedTicketIDsForEvent(empty): %v", err)
	}
	if none == nil {
		t.Fatal("expected an empty slice, not nil")
	}
}
