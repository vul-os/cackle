package scan

import (
	"encoding/json"
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

	var decoded Bundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
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
	if decoded.Allocation == nil || decoded.Allocation.ID != b.Allocation.ID {
		t.Fatalf("allocation mismatch after round trip")
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("round-tripped bundle should still validate: %v", err)
	}
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
