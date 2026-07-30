package scan

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/tickets"
)

func validBundle(t testing.TB) Bundle {
	t.Helper()
	ring := tickets.NewKeyRing("event-1")
	k, err := tickets.GenerateIssuerKey("event-1")
	if err != nil {
		t.Fatalf("GenerateIssuerKey: %v", err)
	}
	ring.AddKey(k)
	return Bundle{
		Event: EventMeta{
			EventID:   "event-1",
			Title:     "Test Fest",
			VenueName: "The Venue",
			StartsAt:  time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC),
			EndsAt:    time.Date(2026, 8, 2, 2, 0, 0, 0, time.UTC),
		},
		IssuerKeys: ring,
		IssuedAt:   time.Now().UTC(),
	}
}

func TestBundle_Validate_Valid(t *testing.T) {
	b := validBundle(t)
	if err := b.Validate(); err != nil {
		t.Fatalf("expected valid bundle, got error: %v", err)
	}
}

func TestBundle_Validate_MissingEventID(t *testing.T) {
	b := validBundle(t)
	b.Event.EventID = ""
	if err := b.Validate(); err == nil {
		t.Fatalf("expected error for missing event id")
	}
}

func TestBundle_Validate_EmptyKeyRing(t *testing.T) {
	b := validBundle(t)
	b.IssuerKeys = tickets.NewKeyRing("event-1")
	if err := b.Validate(); err == nil {
		t.Fatalf("expected error for empty key ring")
	}
}

func TestBundle_Validate_KeyRingEventMismatch(t *testing.T) {
	b := validBundle(t)
	b.IssuerKeys.EventID = "some-other-event"
	if err := b.Validate(); err == nil {
		t.Fatalf("expected error for key ring / event mismatch")
	}
}

func TestBundle_Validate_EmptyTicketIndexOK(t *testing.T) {
	b := validBundle(t)
	b.TicketIndex = nil
	if err := b.Validate(); err != nil {
		t.Fatalf("expected nil/absent ticket_index to be valid (fallback case), got: %v", err)
	}
	b.TicketIndex = []string{}
	if err := b.Validate(); err != nil {
		t.Fatalf("expected empty ticket_index to be valid (fallback case), got: %v", err)
	}
}

func TestBundle_Validate_TicketIndexWithEmptyIDRejected(t *testing.T) {
	b := validBundle(t)
	b.TicketIndex = []string{"ticket-1", ""}
	if err := b.Validate(); err == nil {
		t.Fatalf("expected error for ticket_index containing an empty ticket id")
	}
}

func TestBundle_Validate_AllocationEventMismatch(t *testing.T) {
	b := validBundle(t)
	b.Allocation = &Allocation{EventID: "some-other-event"}
	if err := b.Validate(); err == nil {
		t.Fatalf("expected error for allocation / event mismatch")
	}
}

func TestBundle_Validate_AllocationMatchingEventOK(t *testing.T) {
	b := validBundle(t)
	b.Allocation = &Allocation{EventID: "event-1", DeviceID: "gate-7", Count: 10}
	if err := b.Validate(); err != nil {
		t.Fatalf("expected valid bundle with matching allocation, got: %v", err)
	}
}

func TestBundle_Validate_ZeroIssuedAt(t *testing.T) {
	b := validBundle(t)
	b.IssuedAt = time.Time{}
	if err := b.Validate(); err == nil {
		t.Fatalf("expected error for zero issued_at")
	}
}

func TestBundle_JSONRoundTrip(t *testing.T) {
	b := validBundle(t)
	b.TicketIndex = []string{"ticket-1", "ticket-2", "ticket-3"}
	b.TicketIndexPresent = true
	b.Allocation = &Allocation{
		ID: "alloc-1", EventID: "event-1", DeviceID: "gate-7",
		TicketTypeID: "tt-1", Count: 5,
		IssuedAt: time.Now().UTC().Truncate(time.Second), ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		KID: "k_test",
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytesContain(data, `"ticket_index"`) {
		t.Fatalf("expected ticket_index key in marshaled bundle, got: %s", data)
	}
	if !bytesContain(data, `"ticket_index_present"`) {
		t.Fatalf("expected ticket_index_present key in marshaled bundle, got: %s", data)
	}

	var decoded Bundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.TicketIndexPresent != b.TicketIndexPresent {
		t.Fatalf("ticket_index_present mismatch after round trip: got %v want %v", decoded.TicketIndexPresent, b.TicketIndexPresent)
	}

	if decoded.Event.EventID != b.Event.EventID {
		t.Fatalf("event id mismatch after round trip")
	}
	if !decoded.Event.StartsAt.Equal(b.Event.StartsAt) {
		t.Fatalf("starts_at mismatch after round trip: got %v want %v", decoded.Event.StartsAt, b.Event.StartsAt)
	}
	if len(decoded.IssuerKeys.Keys) != len(b.IssuerKeys.Keys) {
		t.Fatalf("issuer keys count mismatch after round trip")
	}
	if len(decoded.TicketIndex) != len(b.TicketIndex) {
		t.Fatalf("ticket_index length mismatch after round trip: got %v want %v", decoded.TicketIndex, b.TicketIndex)
	}
	for i, tid := range b.TicketIndex {
		if decoded.TicketIndex[i] != tid {
			t.Fatalf("ticket_index[%d] mismatch after round trip: got %q want %q", i, decoded.TicketIndex[i], tid)
		}
	}
	if decoded.Allocation == nil || decoded.Allocation.ID != b.Allocation.ID {
		t.Fatalf("allocation mismatch after round trip")
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("round-tripped bundle should still validate: %v", err)
	}
}

func bytesContain(data []byte, sub string) bool {
	return strings.Contains(string(data), sub)
}

func TestBundle_JSONRoundTrip_NoAllocation(t *testing.T) {
	b := validBundle(t)
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if containsAllocationKey(data) {
		t.Fatalf("expected 'allocation' key to be omitted entirely when nil, got: %s", data)
	}
	var decoded Bundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Allocation != nil {
		t.Fatalf("expected nil allocation after round trip, got %+v", decoded.Allocation)
	}
}

func containsAllocationKey(data []byte) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	_, ok := m["allocation"]
	return ok
}

// TestBundle_Validate_RejectsEmptyAdmittedIndexEntry: a blank entry in the
// admitted index would silently match nothing and quietly weaken the one
// convergence channel an offline gate has, so it is a structural error.
func TestBundle_Validate_RejectsEmptyAdmittedIndexEntry(t *testing.T) {
	b := validBundle(t)
	b.AdmittedIndex = []string{"ticket-1", ""}
	if err := b.Validate(); err == nil {
		t.Fatal("expected Validate to reject an empty admitted_index entry")
	}
}

// TestBundle_Validate_AcceptsEmptyAndAbsentAdmittedIndex pins the documented
// asymmetry with ticket_index: an empty admitted index is a legitimate,
// unambiguous value meaning "nobody is inside yet", so it needs no flag and
// must not be an error.
func TestBundle_Validate_AcceptsEmptyAndAbsentAdmittedIndex(t *testing.T) {
	for name, idx := range map[string][]string{"absent": nil, "empty": {}} {
		b := validBundle(t)
		b.AdmittedIndex = idx
		if err := b.Validate(); err != nil {
			t.Fatalf("%s admitted_index must be valid, got %v", name, err)
		}
	}
}

// TestBundle_JSONRoundTrip_AdmittedIndex keeps the wire field name stable —
// web/src/pages/organizers/scanner/index.jsx reads `admitted_index` off this
// exact JSON, so a rename here silently disables cross-gate convergence in the
// browser gate with nothing failing.
func TestBundle_JSONRoundTrip_AdmittedIndex(t *testing.T) {
	b := validBundle(t)
	b.AdmittedIndex = []string{"ticket-1", "ticket-2"}

	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"admitted_index"`)) {
		t.Fatalf("expected the wire field to be spelled admitted_index: %s", raw)
	}

	var decoded Bundle
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(decoded.AdmittedIndex) != 2 || decoded.AdmittedIndex[0] != "ticket-1" || decoded.AdmittedIndex[1] != "ticket-2" {
		t.Fatalf("admitted_index did not round trip: %+v", decoded.AdmittedIndex)
	}

	// Omitted when empty, so an event with nobody through the door yet does
	// not pay for the field on every bundle download.
	b.AdmittedIndex = nil
	raw, err = json.Marshal(b)
	if err != nil {
		t.Fatalf("Marshal (empty): %v", err)
	}
	if bytes.Contains(raw, []byte(`"admitted_index"`)) {
		t.Fatalf("an empty admitted_index should be omitted: %s", raw)
	}
}
