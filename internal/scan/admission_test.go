package scan

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/tickets"
)

func mustKeyPair(t testing.TB) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

// issueTestTicket builds a ring + signed capability for a given event id,
// ticket id, and validity window, ready to feed into Decide.
func issueTestTicket(t testing.TB, eventID, ticketID string, nbf, exp int64) (tickets.KeyRing, string) {
	t.Helper()
	pub, priv := mustKeyPair(t)
	kid := tickets.KeyID(pub)
	token, err := tickets.Issue(tickets.Payload{
		TID:  ticketID,
		EID:  eventID,
		TT:   "tt_1",
		KID:  kid,
		Sub:  "user_1",
		Name: "Ada Lovelace",
		IAT:  nbf,
		NBF:  nbf,
		EXP:  exp,
		Seat: "A1",
	}, priv)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	ring := tickets.NewKeyRing(eventID)
	ring.Add(kid, pub)
	return ring, token
}

func TestDecide_Admitted(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := Decide(context.Background(), token, ring, "event-1", seen, now)
	if res.Status != Admitted {
		t.Fatalf("expected Admitted, got %s (%s)", res.Status, res.Reason)
	}
	if res.Payload == nil || res.Payload.TID != "ticket-1" {
		t.Fatalf("expected payload with tid ticket-1, got %+v", res.Payload)
	}
}

func TestDecide_Duplicate(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	first := Decide(context.Background(), token, ring, "event-1", seen, now)
	if first.Status != Admitted {
		t.Fatalf("first scan: expected Admitted, got %s", first.Status)
	}
	second := Decide(context.Background(), token, ring, "event-1", seen, now.Add(time.Minute))
	if second.Status != Duplicate {
		t.Fatalf("second scan: expected Duplicate, got %s (%s)", second.Status, second.Reason)
	}
	if second.Payload == nil || second.Payload.TID != "ticket-1" {
		t.Fatalf("duplicate result should still carry the payload, got %+v", second.Payload)
	}
}

func TestDecide_WrongEvent(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := Decide(context.Background(), token, ring, "event-OTHER", seen, now)
	if res.Status != WrongEvent {
		t.Fatalf("expected WrongEvent, got %s (%s)", res.Status, res.Reason)
	}
	if res.Payload == nil {
		t.Fatalf("wrong_event result should still carry the payload")
	}
}

func TestDecide_Invalid_UnknownKey(t *testing.T) {
	_, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	otherRing := tickets.NewKeyRing("event-1") // empty ring, doesn't know the kid
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := Decide(context.Background(), token, otherRing, "event-1", seen, now)
	if res.Status != Invalid {
		t.Fatalf("expected Invalid, got %s", res.Status)
	}
	if res.Payload != nil {
		t.Fatalf("invalid result must not carry a payload, got %+v", res.Payload)
	}
	if res.Reason == "" {
		t.Fatalf("expected a non-empty reason")
	}
}

func TestDecide_Invalid_Expired(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	seen := NewMemorySeenSet()
	now := time.Unix(3000, 0) // after exp

	res := Decide(context.Background(), token, ring, "event-1", seen, now)
	if res.Status != Invalid {
		t.Fatalf("expected Invalid for expired ticket, got %s", res.Status)
	}
}

func TestDecide_Invalid_NotYetValid(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	seen := NewMemorySeenSet()
	now := time.Unix(500, 0) // before nbf

	res := Decide(context.Background(), token, ring, "event-1", seen, now)
	if res.Status != Invalid {
		t.Fatalf("expected Invalid for not-yet-valid ticket, got %s", res.Status)
	}
}

func TestDecide_Invalid_MalformedToken(t *testing.T) {
	ring := tickets.NewKeyRing("event-1")
	seen := NewMemorySeenSet()
	res := Decide(context.Background(), "not-a-real-token", ring, "event-1", seen, time.Now())
	if res.Status != Invalid {
		t.Fatalf("expected Invalid for malformed token, got %s", res.Status)
	}
	if res.Payload != nil {
		t.Fatalf("expected nil payload for malformed token")
	}
}

func TestDecide_Invalid_TamperedToken(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	seen := NewMemorySeenSet()
	tampered := token[:len(token)-2] + "xx"

	res := Decide(context.Background(), tampered, ring, "event-1", seen, time.Unix(1500, 0))
	if res.Status != Invalid {
		t.Fatalf("expected Invalid for tampered token, got %s", res.Status)
	}
}

// TestDecide_WithSQLiteSeenSet exercises the same admitted/duplicate flow
// against the durable backend, not just the in-memory one.
func TestDecide_WithSQLiteSeenSet(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	seen, err := OpenSQLiteSeenSet(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteSeenSet: %v", err)
	}
	defer seen.Close()

	now := time.Unix(1500, 0)
	first := Decide(context.Background(), token, ring, "event-1", seen, now)
	if first.Status != Admitted {
		t.Fatalf("expected Admitted, got %s", first.Status)
	}
	second := Decide(context.Background(), token, ring, "event-1", seen, now.Add(time.Second))
	if second.Status != Duplicate {
		t.Fatalf("expected Duplicate, got %s", second.Status)
	}
}

// TestDecide_ConcurrentScans_ExactlyOneAdmitted hammers Decide with the
// same ticket from many goroutines at once and asserts exactly one call
// observes Admitted — this is the property that makes "first scan wins"
// true even under real concurrent gate hardware (e.g. two lanes at one
// gate sharing a SeenSet).
func TestDecide_ConcurrentScans_ExactlyOneAdmitted(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)

			var seen SeenSet
			switch backend {
			case "memory":
				seen = NewMemorySeenSet()
			case "sqlite":
				s, err := OpenSQLiteSeenSet(":memory:")
				if err != nil {
					t.Fatalf("OpenSQLiteSeenSet: %v", err)
				}
				defer s.Close()
				seen = s
			}

			const n = 50
			results := make(chan Status, n)
			var wg sync.WaitGroup
			wg.Add(n)
			for i := 0; i < n; i++ {
				go func() {
					defer wg.Done()
					res := Decide(context.Background(), token, ring, "event-1", seen, time.Unix(1500, 0))
					results <- res.Status
				}()
			}
			wg.Wait()
			close(results)

			admittedCount := 0
			for status := range results {
				if status == Admitted {
					admittedCount++
				} else if status != Duplicate {
					t.Fatalf("unexpected status in concurrent scan: %s", status)
				}
			}
			if admittedCount != 1 {
				t.Fatalf("expected exactly 1 Admitted out of %d concurrent scans, got %d", n, admittedCount)
			}
		})
	}
}
