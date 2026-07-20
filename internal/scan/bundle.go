package scan

import (
	"fmt"
	"time"

	"github.com/vul-os/cackle/internal/tickets"
)

// EventMeta is the minimal event context a gate needs to display something
// sensible to an operator ("You are scanning: <Title> at <VenueName>")
// without any further network access. It deliberately does not duplicate
// the full events schema (internal/events owns that) — just enough for the
// offline UI.
type EventMeta struct {
	EventID   string    `json:"event_id"`
	Title     string    `json:"title"`
	VenueName string    `json:"venue_name"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}

// Bundle is everything a gate scanner downloads once, while it still has
// signal, in order to run for the entire duration of an event with the
// network unplugged:
//
//   - Event: which event this is, for display.
//   - IssuerKeys: the pinned KeyRing (kid -> pubkey) tickets.VerifyWithRing
//     checks every scanned capability against.
//   - TicketIndex: the set of ticket IDs currently valid (issued, not void,
//     not refunded) for this event, as of IssuedAt. A signature alone only
//     proves a capability was validly issued at some point — it says
//     nothing about whether the ticket was later voided or refunded. This
//     index is what lets DecideWithBundle (see admission.go) catch that: a
//     ticket whose id is absent from a non-empty index is rejected even
//     though its signature checks out.
//
//     IMPORTANT — this is a point-in-time snapshot, not a live revocation
//     feed: a ticket refunded *after* a gate downloaded this bundle is
//     still admittable at that gate until it re-pulls a fresh bundle. That
//     is an inherent limitation of fully offline operation, not a bug; the
//     mitigation is operational (re-sync the bundle periodically, e.g. at
//     shift changes), not a code fix. When TicketIndex is empty (an older
//     bundle predating this field, or an event with no issued tickets yet)
//     DecideWithBundle falls back to signature-only behaviour rather than
//     refusing to admit anyone — see its doc comment.
//   - Allocation: optional — present only when this specific device is a
//     delegated sub-issuer allowed to mint tickets of its own while
//     disconnected (see allocation.go). nil for an ordinary scan-only gate.
//   - IssuedAt: when the server generated this bundle, so a gate can warn
//     an operator if it's stale relative to some operationally-chosen
//     staleness threshold (Bundle itself does not enforce a threshold —
//     that policy decision belongs to the caller).
type Bundle struct {
	Event       EventMeta       `json:"event"`
	IssuerKeys  tickets.KeyRing `json:"issuer_keys"`
	TicketIndex []string        `json:"ticket_index"`
	Allocation  *Allocation     `json:"allocation,omitempty"`
	IssuedAt    time.Time       `json:"issued_at"`
}

// Validate checks internal consistency of a Bundle before a gate starts
// trusting it: the event id is present, the key ring is non-empty and
// scoped to the same event, the ticket index (if any) contains no garbage
// entries, and (if present) the allocation is scoped to the same event too.
// Validate does NOT check any signature — that is VerifyWithRing's job
// per-token and VerifyAllocation's job for the allocation; Validate is
// purely a structural/consistency sanity check run once when the bundle is
// first loaded.
//
// Validate deliberately does NOT require TicketIndex to be non-empty: an
// empty (or absent, pre-this-field) index is a legitimate value meaning
// "fall back to signature-only checking" — see DecideWithBundle — not a
// malformed bundle.
func (b Bundle) Validate() error {
	if b.Event.EventID == "" {
		return fmt.Errorf("scan: bundle: event_id is empty")
	}
	if len(b.IssuerKeys.Keys) == 0 {
		return fmt.Errorf("scan: bundle: issuer_keys is empty — a gate with no pinned keys cannot verify anything")
	}
	if b.IssuerKeys.EventID != "" && b.IssuerKeys.EventID != b.Event.EventID {
		return fmt.Errorf("scan: bundle: issuer_keys event_id %q does not match event %q", b.IssuerKeys.EventID, b.Event.EventID)
	}
	for _, tid := range b.TicketIndex {
		if tid == "" {
			return fmt.Errorf("scan: bundle: ticket_index contains an empty ticket id")
		}
	}
	if b.Allocation != nil && b.Allocation.EventID != b.Event.EventID {
		return fmt.Errorf("scan: bundle: allocation event_id %q does not match event %q", b.Allocation.EventID, b.Event.EventID)
	}
	if b.IssuedAt.IsZero() {
		return fmt.Errorf("scan: bundle: issued_at is zero")
	}
	return nil
}

// Bundle needs no custom MarshalJSON/UnmarshalJSON of its own: it has no
// unexported fields, and tickets.KeyRing already defines its own JSON
// shape via its own Marshal/UnmarshalJSON methods, which encoding/json
// picks up automatically for the embedded IssuerKeys field.
