package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAdmissionConflicts_TwoPartitionedGatesSurfacedAfterSync is the
// end-to-end version of the property internal/scan/substrate tests in
// isolation, driven entirely over HTTP the way two real gates would drive it.
//
// The scenario is the one Cackle's headline claim has to survive. Two gates are
// offline from each other. Each holds its own dedupe log, so each sees a first
// scan of the SAME ticket and each admits — one person, two entrances. Neither
// gate can prevent that and neither can know about it.
//
// Then both sync. The server keeps its single-admitted-row invariant (that part
// already worked), and — the part that did not exist before — the conflict is
// reported afterwards instead of being erased by the downgrade.
func TestAdmissionConflicts_TwoPartitionedGatesSurfacedAfterSync(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "conflicts")

	buyerToken, _ := h.signupUser("buyer-conflicts@example.com", "buyer-password-123", "Nomvula Dlamini")
	tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-conflicts@example.com", "Nomvula Dlamini", 2)
	shared, clean := tickets[0], tickets[1]

	base := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	// Before anything is synced there is nothing to report — and the response
	// still carries its caveat, because an empty list must never read as
	// "no double admissions happened".
	empty := h.conflicts(t, fx, http.StatusOK)
	if len(empty.Conflicts) != 0 || empty.ExtraAdmissions != 0 {
		t.Fatalf("expected no conflicts before any sync, got %+v", empty)
	}
	if empty.Caveat == "" {
		t.Fatal("an empty conflicts response must still state its limits")
	}
	if !empty.Complete {
		t.Fatalf("nothing was refused, so the answer is complete: %+v", empty)
	}
	if empty.Algebra == "" || empty.Engine == "" {
		t.Fatalf("the report must name the algebra and engine it merged under: %+v", empty)
	}

	// --- gate A syncs: it admitted the shared ticket -----------------------
	rec := h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, scanSyncRequest{Admissions: []scanSyncItem{
		{TicketID: shared.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base, Result: "admitted"},
		{TicketID: clean.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base.Add(time.Minute), Result: "admitted"},
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("gate A sync: status %d body %s", rec.Code, rec.Body.String())
	}

	// One gate's admissions are never a conflict, however many there are.
	if got := h.conflicts(t, fx, http.StatusOK); len(got.Conflicts) != 0 {
		t.Fatalf("one gate alone cannot produce a conflict, got %+v", got.Conflicts)
	}

	// --- gate B syncs: it ALSO admitted the shared ticket ------------------
	//
	// Gate B reports 'admitted' because that is genuinely what it did at the
	// door — it had no way to know gate A had already let this ticket through.
	// The server downgrades the stored row to 'duplicate' (correct: one ticket,
	// one admitted row) while keeping what B reported.
	rec = h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, scanSyncRequest{Admissions: []scanSyncItem{
		{TicketID: shared.ID, EventID: fx.eventID, GateID: "South", DeviceID: "device-B",
			ScannedAt: base.Add(90 * time.Second), Result: "admitted"},
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("gate B sync: status %d body %s", rec.Code, rec.Body.String())
	}
	applied := decodeBody[scanSyncResponse](t, rec)
	if len(applied.Applied) != 1 || !applied.Applied[0] {
		t.Fatalf("gate B's claim must be recorded, not dropped: %+v", applied.Applied)
	}

	// --- the conflict is now surfaced -------------------------------------
	got := h.conflicts(t, fx, http.StatusOK)
	if len(got.Conflicts) != 1 {
		t.Fatalf("expected exactly one conflict, got %d: %+v", len(got.Conflicts), got.Conflicts)
	}
	c := got.Conflicts[0]
	if c.TicketID != shared.ID {
		t.Fatalf("conflict names ticket %q, want the shared one %q", c.TicketID, shared.ID)
	}
	if c.Devices != 2 {
		t.Fatalf("expected 2 conflicting devices, got %d", c.Devices)
	}
	if c.ExtraAdmissions != 1 || got.ExtraAdmissions != 1 {
		t.Fatalf("one extra person got in; report says %d / total %d", c.ExtraAdmissions, got.ExtraAdmissions)
	}
	if len(c.Claims) != 2 {
		t.Fatalf("expected both gates' claims, got %d: %+v", len(c.Claims), c.Claims)
	}
	if !got.Complete {
		t.Fatalf("nothing was refused, so the report is complete: %+v", got)
	}

	// Both claims say 'admitted' — that is what happened at the doors.
	// Ordered earliest scan first.
	if c.Claims[0].DeviceID != "device-A" || c.Claims[1].DeviceID != "device-B" {
		t.Fatalf("claims not in scan order: %+v", c.Claims)
	}
	if c.Claims[0].GateID != "North" || c.Claims[1].GateID != "South" {
		t.Fatalf("gate ids did not survive: %+v", c.Claims)
	}
	for i, cl := range c.Claims {
		if cl.Result != "admitted" {
			t.Fatalf("claim %d reports %q; both gates admitted, and folding the server's "+
				"downgraded verdict back in would hide exactly this conflict", i, cl.Result)
		}
	}
	// Gate A won the stored row, so its claim and the server's verdict agree
	// and server_result is omitted. Gate B's was downgraded, so it is shown.
	if c.Claims[0].ServerResult != "" {
		t.Fatalf("gate A's claim was not downgraded; server_result should be omitted, got %q",
			c.Claims[0].ServerResult)
	}
	if c.Claims[1].ServerResult != "duplicate" {
		t.Fatalf("gate B's claim was downgraded to duplicate server-side; report says %q",
			c.Claims[1].ServerResult)
	}

	// The clean ticket, admitted once at one gate, is absent from the report.
	for _, cf := range got.Conflicts {
		if cf.TicketID == clean.ID {
			t.Fatalf("a singly-admitted ticket must not appear as a conflict: %+v", cf)
		}
	}

	// Re-reading is stable: the view is derived by re-folding the table, so an
	// idempotence bug in the mapping would show up as the counts growing.
	again := h.conflicts(t, fx, http.StatusOK)
	if len(again.Conflicts) != 1 || again.Conflicts[0].Devices != 2 || len(again.Conflicts[0].Claims) != 2 {
		t.Fatalf("re-reading the report changed it: %+v", again.Conflicts)
	}
}

// TestAdmissionConflicts_SameDeviceDuplicateIsNotAConflict is the distinction
// migration 0002 exists to make. A gate re-scanning a ticket its OWN log
// already had is an ordinary duplicate — nobody got in twice — and it must not
// be reported as a cross-gate double admission. Before reported_result existed
// this row was indistinguishable from gate B's downgraded claim above.
func TestAdmissionConflicts_SameDeviceDuplicateIsNotAConflict(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "samedevice")

	buyerToken, _ := h.signupUser("buyer-samedev@example.com", "buyer-password-123", "Sipho Mahlangu")
	tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-samedev@example.com", "Sipho Mahlangu", 1)

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	rec := h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, scanSyncRequest{Admissions: []scanSyncItem{
		{TicketID: tickets[0].ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base, Result: "admitted"},
		// The same device, later, correctly refusing it from its own log.
		{TicketID: tickets[0].ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base.Add(time.Minute), Result: "duplicate"},
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("sync: status %d body %s", rec.Code, rec.Body.String())
	}

	got := h.conflicts(t, fx, http.StatusOK)
	if len(got.Conflicts) != 0 {
		t.Fatalf("one device admitting once and then refusing is not a cross-gate conflict, got %+v",
			got.Conflicts)
	}
	if got.ExtraAdmissions != 0 {
		t.Fatalf("nobody got in twice; report claims %d extra", got.ExtraAdmissions)
	}
}

// TestAdmissionConflicts_RBAC keeps the route behind the same role floor as
// the scan routes it reports on, and confirms a non-member gets 403 rather
// than a read of another org's door log.
func TestAdmissionConflicts_RBAC(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "conflictsrbac")

	strangerToken, _ := h.signupUser("stranger-conflicts@example.com", "stranger-password-123", "Stranger")
	rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/admission-conflicts", strangerToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a non-member must not read the door log: status %d body %s", rec.Code, rec.Body.String())
	}

	// Unauthenticated is rejected too, and never with a body that leaks the
	// event's existence.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/admission-conflicts", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthenticated, got %d", rec.Code)
	}
}

// TestAdmissionConflicts_UnknownEventIsNotAnOracle: an event that does not
// exist must not be distinguishable from one the caller has no rights to.
func TestAdmissionConflicts_UnknownEventIsNotAnOracle(t *testing.T) {
	h := newTestHarness(t)
	token, _ := h.signupUser("nobody-conflicts@example.com", "nobody-password-123", "Nobody")

	rec := h.do(http.MethodGet, "/api/events/evt_does_not_exist/admission-conflicts", token, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an unknown event (same as no access), got %d body %s",
			rec.Code, rec.Body.String())
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "not found") {
		t.Fatalf("the refusal must not reveal whether the event exists: %s", rec.Body.String())
	}
}

// conflicts calls the report route and decodes it.
func (h *testHarness) conflicts(t *testing.T, fx eventFixture, wantStatus int) admissionConflictsResponse {
	t.Helper()
	rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/admission-conflicts", fx.ownerToken, nil)
	if rec.Code != wantStatus {
		t.Fatalf("admission-conflicts: status %d (want %d) body %s", rec.Code, wantStatus, rec.Body.String())
	}
	return decodeBody[admissionConflictsResponse](t, rec)
}

// TestScanBundle_CarriesAdmittedIndex covers the OTHER half of the story: the
// convergence channel that runs back DOWN to the gates.
//
// Before admitted_index existed, docs/OFFLINE-GATES.md told operators that "a
// synced ticket comes back as a duplicate everywhere after the next bundle
// refresh". That was false — the bundle carried only the valid-ticket index,
// which is unaffected by admission — so a gate could re-pull all day and still
// admit a ticket another gate had already used. This test is what makes the
// sentence true.
func TestScanBundle_CarriesAdmittedIndex(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "admittedidx")

	buyerToken, _ := h.signupUser("buyer-admittedidx@example.com", "buyer-password-123", "Lerato Molefe")
	tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-admittedidx@example.com", "Lerato Molefe", 2)

	type bundleView struct {
		TicketIndex        []string `json:"ticket_index"`
		TicketIndexPresent bool     `json:"ticket_index_present"`
		AdmittedIndex      []string `json:"admitted_index"`
	}
	getBundle := func() bundleView {
		t.Helper()
		rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/scan-bundle", fx.ownerToken, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("scan-bundle: status %d body %s", rec.Code, rec.Body.String())
		}
		return decodeBody[bundleView](t, rec)
	}

	// Nobody through the door yet: admitted_index is absent/empty, and that
	// unambiguously means "admit normally" — it needs no present flag.
	before := getBundle()
	if len(before.AdmittedIndex) != 0 {
		t.Fatalf("expected an empty admitted_index before any admission, got %+v", before.AdmittedIndex)
	}
	if len(before.TicketIndex) != 2 || !before.TicketIndexPresent {
		t.Fatalf("ticket_index must still be the authoritative valid set: %+v", before)
	}

	// Gate A admits one ticket and syncs.
	rec := h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, scanSyncRequest{Admissions: []scanSyncItem{
		{TicketID: tickets[0].ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: time.Now().UTC().Add(-time.Minute).Truncate(time.Second), Result: "admitted"},
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("sync: status %d body %s", rec.Code, rec.Body.String())
	}

	// Gate B re-pulls and now learns about it.
	after := getBundle()
	if len(after.AdmittedIndex) != 1 || after.AdmittedIndex[0] != tickets[0].ID {
		t.Fatalf("re-pulled bundle must carry the admitted ticket, got %+v", after.AdmittedIndex)
	}
	// The valid-ticket index is unchanged — an admitted ticket is still a valid
	// ticket, which is exactly why ticket_index could never have conveyed this.
	if len(after.TicketIndex) != 2 {
		t.Fatalf("admission must not change ticket_index, got %+v", after.TicketIndex)
	}
	for _, id := range after.TicketIndex {
		if id == tickets[1].ID {
			return // the un-admitted ticket is still admittable
		}
	}
	t.Fatal("the un-admitted ticket vanished from ticket_index")
}
