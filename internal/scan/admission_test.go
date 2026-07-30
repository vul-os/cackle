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

// issueTestBundle wraps issueTestTicket into a full Bundle ready for
// DecideWithBundle, with a caller-supplied ticket_index.
// Convention: a nil ticketIndex models an ABSENT index (legacy bundle,
// TicketIndexPresent=false → signature-only fallback). A non-nil slice —
// EVEN AN EMPTY ONE — models an AUTHORITATIVE index (present=true), so an
// empty non-nil index means "admit nothing".
func issueTestBundle(t testing.TB, eventID, ticketID string, nbf, exp int64, ticketIndex []string) (Bundle, string) {
	t.Helper()
	ring, token := issueTestTicket(t, eventID, ticketID, nbf, exp)
	b := Bundle{
		Event:              EventMeta{EventID: eventID},
		IssuerKeys:         ring,
		TicketIndex:        ticketIndex,
		TicketIndexPresent: ticketIndex != nil,
		IssuedAt:           time.Unix(nbf, 0),
	}
	return b, token
}

func TestDecideWithBundle_Admitted_TicketInIndex(t *testing.T) {
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-1", "ticket-2"})
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), token, b, seen, now)
	if res.Status != Admitted {
		t.Fatalf("expected Admitted, got %s (%s)", res.Status, res.Reason)
	}
	if res.Payload == nil || res.Payload.TID != "ticket-1" {
		t.Fatalf("expected payload with tid ticket-1, got %+v", res.Payload)
	}
}

func TestDecideWithBundle_Invalid_TicketAbsentFromIndex(t *testing.T) {
	// Perfectly signed, current, right event — but not in the index, e.g.
	// because it was refunded after issuance. This is the whole gap this
	// feature closes.
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-2", "ticket-3"})
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), token, b, seen, now)
	if res.Status != Invalid {
		t.Fatalf("expected Invalid for ticket absent from index, got %s (%s)", res.Status, res.Reason)
	}
	if res.Payload != nil {
		t.Fatalf("invalid result must not carry a payload, got %+v", res.Payload)
	}
	if res.Reason == "" {
		t.Fatalf("expected a non-empty, human-readable reason")
	}
}

func TestDecideWithBundle_Invalid_RevokedTicket_SignatureOtherwisePerfect(t *testing.T) {
	// Same as above, spelled out explicitly per the spec's required case:
	// a revoked ticket that is otherwise perfectly signed must be rejected.
	b, token := issueTestBundle(t, "event-1", "ticket-refunded", 1000, 2000, []string{"ticket-other"})
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), token, b, seen, now)
	if res.Status != Invalid {
		t.Fatalf("expected revoked ticket to be Invalid, got %s (%s)", res.Status, res.Reason)
	}

	// And it must not have consumed the dedupe slot — a revoked scan was
	// never admitted, so a legitimate first scan of some OTHER ticket must
	// still be possible afterwards (sanity check that we didn't call
	// seen.MarkSeen for the rejected ticket).
	seenAlready, err := seen.Seen(context.Background(), "ticket-refunded")
	if err != nil {
		t.Fatalf("Seen: %v", err)
	}
	if seenAlready {
		t.Fatalf("a rejected/revoked ticket must not be recorded in the dedupe set")
	}
}

func TestDecideWithBundle_FallsBackToSignatureOnly_WhenIndexAbsent(t *testing.T) {
	// nil index => TicketIndexPresent=false => a legacy/hand-built bundle with
	// no index data at all. This is the ONLY case that falls back to
	// signature-only admission.
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, nil)
	if b.TicketIndexPresent {
		t.Fatalf("nil index must model an absent (not present) index")
	}
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), token, b, seen, now)
	if res.Status != Admitted {
		t.Fatalf("expected Admitted (signature-only fallback) with absent index, got %s (%s)", res.Status, res.Reason)
	}
}

func TestDecideWithBundle_Invalid_WhenIndexPresentButEmpty(t *testing.T) {
	// The bug this whole field fixes: an authoritative but EMPTY index means
	// every ticket was voided/refunded (or none issued) — admit NOTHING. A
	// perfectly-signed, current, right-event ticket must still be rejected.
	// Before TicketIndexPresent existed this was indistinguishable from
	// "no index data" and silently admitted, which would re-admit every
	// physically-held ticket for a fully-cancelled event.
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{})
	if !b.TicketIndexPresent {
		t.Fatalf("a non-nil (even empty) index must model an authoritative/present index")
	}
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), token, b, seen, now)
	if res.Status != Invalid {
		t.Fatalf("expected Invalid (present-but-empty index admits nothing), got %s (%s)", res.Status, res.Reason)
	}
	// And it must not consume the dedupe slot.
	if seenAlready, _ := seen.Seen(context.Background(), "ticket-1"); seenAlready {
		t.Fatalf("a ticket rejected by an empty authoritative index must not be recorded as seen")
	}
}

func TestDecideWithBundle_Invalid_BadSignature_EvenWithIndexPresent(t *testing.T) {
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-1"})
	tampered := token[:len(token)-2] + "xx"
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), tampered, b, seen, now)
	if res.Status != Invalid {
		t.Fatalf("expected Invalid for tampered token even with a matching index, got %s", res.Status)
	}
	if res.Payload != nil {
		t.Fatalf("expected nil payload for invalid signature")
	}
}

func TestDecideWithBundle_Duplicate_IndexedTicketScannedTwice(t *testing.T) {
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-1"})
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	first := DecideWithBundle(context.Background(), token, b, seen, now)
	if first.Status != Admitted {
		t.Fatalf("first scan: expected Admitted, got %s (%s)", first.Status, first.Reason)
	}
	second := DecideWithBundle(context.Background(), token, b, seen, now.Add(time.Minute))
	if second.Status != Duplicate {
		t.Fatalf("second scan: expected Duplicate, got %s (%s)", second.Status, second.Reason)
	}
	if second.Payload == nil || second.Payload.TID != "ticket-1" {
		t.Fatalf("duplicate result should still carry the payload, got %+v", second.Payload)
	}
}

func TestDecideWithBundle_WrongEvent_NotShadowedByIndex(t *testing.T) {
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-1"})
	b.Event.EventID = "event-OTHER" // gate scanning for a different event than the one the bundle/ring belong to
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), token, b, seen, now)
	if res.Status != WrongEvent {
		t.Fatalf("expected WrongEvent, got %s (%s)", res.Status, res.Reason)
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

// --- AdmittedIndex: the downstream convergence channel -------------------

// TestDecideWithBundle_Duplicate_AdmittedAtAnotherGate is the property that
// makes "sync opportunistically and the window shrinks" true rather than
// hopeful. The local SeenSet has never seen this ticket — a fresh device, or
// one that only just came back — and the ONLY evidence that somebody already
// walked through with it is the bundle's admitted_index.
func TestDecideWithBundle_Duplicate_AdmittedAtAnotherGate(t *testing.T) {
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-1", "ticket-2"})
	b.AdmittedIndex = []string{"ticket-1"} // gate A admitted it; this is gate B
	seen := NewMemorySeenSet()
	now := time.Unix(1500, 0)

	res := DecideWithBundle(context.Background(), token, b, seen, now)
	if res.Status != Duplicate {
		t.Fatalf("expected Duplicate for a ticket admitted at another gate, got %s (%s)", res.Status, res.Reason)
	}
	if res.Payload == nil || res.Payload.TID != "ticket-1" {
		t.Fatalf("a duplicate must still identify the ticket, got %+v", res.Payload)
	}
	if res.Reason == "" {
		t.Fatalf("expected a reason an operator can act on")
	}

	// The local set converged: this gate now refuses on its own evidence, so a
	// later bundle that no longer lists the ticket cannot re-open the door.
	if ok, err := seen.Seen(context.Background(), "ticket-1"); err != nil {
		t.Fatalf("Seen: %v", err)
	} else if !ok {
		t.Fatal("the other gate's admission must be folded into the local dedupe log")
	}
	b.AdmittedIndex = nil
	if res := DecideWithBundle(context.Background(), token, b, seen, now); res.Status != Duplicate {
		t.Fatalf("after convergence the local log alone must refuse, got %s", res.Status)
	}
}

// TestDecideWithBundle_AdmittedIndex_EmptyAndAbsentBehaveIdentically pins the
// documented asymmetry with TicketIndex: an empty admitted index carries no
// hidden "admit nothing" meaning, so it needs no present flag.
func TestDecideWithBundle_AdmittedIndex_EmptyAndAbsentBehaveIdentically(t *testing.T) {
	for name, idx := range map[string][]string{"absent": nil, "empty": {}} {
		b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-1"})
		b.AdmittedIndex = idx
		res := DecideWithBundle(context.Background(), token, b, NewMemorySeenSet(), time.Unix(1500, 0))
		if res.Status != Admitted {
			t.Fatalf("%s admitted_index must not block admission, got %s (%s)", name, res.Status, res.Reason)
		}
	}
}

// TestDecideWithBundle_AdmittedIndex_DoesNotShadowRevocation keeps the check
// order honest: a revoked ticket that also happens to appear in the admitted
// index must still be Invalid, not merely Duplicate. Reporting "already used"
// for a ticket that was never legitimately usable would tell an operator the
// wrong story.
func TestDecideWithBundle_AdmittedIndex_DoesNotShadowRevocation(t *testing.T) {
	b, token := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-other"})
	b.AdmittedIndex = []string{"ticket-1"}
	res := DecideWithBundle(context.Background(), token, b, NewMemorySeenSet(), time.Unix(1500, 0))
	if res.Status != Invalid {
		t.Fatalf("a revoked ticket must be Invalid even if it is in admitted_index, got %s", res.Status)
	}
}

// TestDecideWithBundle_AdmittedIndex_DoesNotShadowWrongEvent is the same
// ordering guarantee for the other terminal outcome.
func TestDecideWithBundle_AdmittedIndex_DoesNotShadowWrongEvent(t *testing.T) {
	b, _ := issueTestBundle(t, "event-1", "ticket-1", 1000, 2000, []string{"ticket-1"})
	otherRing, otherToken := issueTestTicket(t, "event-2", "ticket-1", 1000, 2000)
	b.IssuerKeys = otherRing
	b.AdmittedIndex = []string{"ticket-1"}

	res := DecideWithBundle(context.Background(), otherToken, b, NewMemorySeenSet(), time.Unix(1500, 0))
	if res.Status != WrongEvent {
		t.Fatalf("a wrong-event ticket must stay WrongEvent, got %s", res.Status)
	}
}

// TestDecide_IgnoresAdmittedIndex_BecauseItHasNoBundle guards the stable
// signature Decide's JS port and internal/httpapi depend on: Decide takes no
// Bundle, so it cannot and must not gain the cross-gate check.
func TestDecide_IgnoresAdmittedIndex_BecauseItHasNoBundle(t *testing.T) {
	ring, token := issueTestTicket(t, "event-1", "ticket-1", 1000, 2000)
	res := Decide(context.Background(), token, ring, "event-1", NewMemorySeenSet(), time.Unix(1500, 0))
	if res.Status != Admitted {
		t.Fatalf("Decide is signature+local-dedupe only, got %s (%s)", res.Status, res.Reason)
	}
}
