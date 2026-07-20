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
//     shift changes), not a code fix.
//   - TicketIndexPresent: whether TicketIndex is authoritative. This is the
//     crucial distinction. An empty TicketIndex is ambiguous on its own:
//     it could mean "no tickets have been issued yet" OR "every ticket for
//     this event has been voided/refunded". Treating both as "no data, fall
//     back to signature-only" would silently re-admit every physically-held
//     ticket for a fully-cancelled event. So the server ALWAYS sets
//     TicketIndexPresent=true (it queried the authoritative valid set, even
//     if that set is empty); an empty present index therefore means "admit
//     nothing". TicketIndexPresent=false is reserved for a legacy or
//     hand-built bundle that carries no index data at all, and only then
//     does DecideWithBundle fall back to signature-only checking.
//   - Allocation: optional — present only when this specific device is a
//     delegated sub-issuer allowed to mint tickets of its own while
//     disconnected (see allocation.go). nil for an ordinary scan-only gate.
//   - IssuedAt: when the server generated this bundle, so a gate can warn
//     an operator if it's stale relative to some operationally-chosen
//     staleness threshold (Bundle itself does not enforce a threshold —
//     that policy decision belongs to the caller).
type Bundle struct {
	Event              EventMeta       `json:"event"`
	IssuerKeys         tickets.KeyRing `json:"issuer_keys"`
	TicketIndex        []string        `json:"ticket_index"`
	TicketIndexPresent bool            `json:"ticket_index_present"`
	Allocation         *Allocation     `json:"allocation,omitempty"`
	IssuedAt           time.Time       `json:"issued_at"`
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
// empty index is a legitimate value. Its MEANING depends on
// TicketIndexPresent — an empty present index means "admit nothing" (every
// ticket revoked, or none issued), while an absent index (present=false)
// means "fall back to signature-only checking". See DecideWithBundle.
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
