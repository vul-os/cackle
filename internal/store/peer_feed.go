package store

import (
	"context"
	"fmt"
	"time"
)

// Storage for the opt-in event feed between enrolled peers (migration 0007):
// the two per-peer consent switches, the published events this node is willing
// to hand over, and the cache of what a peer last handed back.
//
// Nothing in this file discovers anything. Every read and every write is scoped
// to a `sync_peer` row that exists because a human typed an address and pinned
// a key.

// --- the two consent switches ----------------------------------------------

// SetSyncPeerFeed sets a peer's two feed switches.
//
// Both are written on every call, so a caller cannot flip one and leave the
// other at whatever it happened to be — an operator's answer to "may they see
// mine / may I see theirs" is one answer to two questions, and a partial write
// would let a UI that only sends one of them silently keep the other on.
//
// It does NOT touch `enabled`. The two are different consents: a peer may
// reconcile door scans without publishing a programme, and turning a feed off
// must not quietly stop admission replication an operator is relying on.
func (s *Store) SetSyncPeerFeed(ctx context.Context, id string, publish, subscribe bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sync_peer SET feed_publish = ?, feed_subscribe = ? WHERE id = ?`,
		boolToInt(publish), boolToInt(subscribe), id)
	if err != nil {
		return fmt.Errorf("store: set sync peer feed switches: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set sync peer feed switches: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSyncPeerFeedStatus records the outcome of a feed pull — refusals included.
// It is kept apart from SetSyncPeerStatus so a healthy admission round cannot
// overwrite, and thereby hide, a feed pull that was refused.
func (s *Store) SetSyncPeerFeedStatus(ctx context.Context, id string, at time.Time, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sync_peer SET feed_pulled_at = ?, feed_status = ? WHERE id = ?`,
		timeToText(at), status, id)
	if err != nil {
		return fmt.Errorf("store: set sync peer feed status: %w", err)
	}
	return nil
}

// --- the publish side -------------------------------------------------------

// PublishedEventsAfterForOrg returns one page of an organisation's PUBLISHED
// events, ordered by primary key, starting strictly AFTER afterID. An empty
// afterID starts at the beginning.
//
// `status = 'published'` is the whole authorisation for handing an event to
// another operator, and it is written as a WHERE clause rather than filtered in
// Go so there is exactly one place it can be got wrong. A draft is an event its
// own organiser has not shown to the public yet; leaking one to a peer would
// publish it on somebody else's site before it was published on this one. A
// cancelled event is excluded too — a listing that sends a stranger to a
// cancelled show is worse than no listing.
//
// # Why the page is keyed on `id` and not on `starts_at`
//
// This is a keyset seek, not an offset, and the key is the PRIMARY KEY. An offset
// is unusable here for the obvious reason — unpublish one row behind the cursor
// and every later row slides forward, so exactly one is skipped and nobody is
// told. But `ORDER BY starts_at` is unusable too, for a subtler one: starts_at is
// a column an organiser can edit from the events screen. Reschedule a show while
// a peer is mid-walk and the row moves ACROSS the cursor — later, and it is never
// sent at all; earlier, and it is sent twice. `id` is a ULID assigned once at
// creation and never rewritten, so the order it defines is total, immutable and
// unique, and a page boundary computed under it stays valid however the rows
// around it change. Display order is not lost by this: a subscriber re-sorts by
// starts_at when it reads its cache.
func (s *Store) PublishedEventsAfterForOrg(ctx context.Context, orgID, afterID string, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, nil
	}
	return s.queryEvents(ctx, eventSelectColumns+`
		FROM events
		WHERE org_id = ? AND status = 'published' AND id > ?
		ORDER BY id
		LIMIT ?`, orgID, afterID, limit)
}

// --- the subscribe side: a peer's events, as this node last saw them ---------

// PeerEvent is one listing borrowed from an enrolled peer.
//
// It carries no price, no ticket type and no order path, because this node
// cannot sell it. Everything here exists to say WHOSE event it is and WHERE the
// buyer has to go — the publisher's own box — and nothing here exists to make it
// look purchasable at this address.
type PeerEvent struct {
	ID     string
	PeerID string
	// OrgID is the SUBSCRIBING organisation, i.e. the scope this listing is
	// shown in on THIS node. It is never the publisher's org id: this node has
	// no rows for that org and must not pretend to.
	OrgID string
	// PublisherKey is the pinned node key whose signed answer carried this row,
	// and PublisherName the label its operator gave their organisation.
	// PublisherName is UNTRUSTED display text from another machine; PublisherKey
	// is the identity.
	PublisherKey  string
	PublisherName string
	// RemoteEventID and Slug identify the event on the publisher's own node.
	RemoteEventID string
	Slug          string
	// URL is composed HERE from the enrolled peer's typed origin plus Slug. It
	// is never copied from the publisher's answer, so a publisher cannot point a
	// listing that carries its neighbour's trust at a site nobody enrolled.
	URL       string
	Title     string
	Summary   string
	VenueName string
	Address   string
	StartsAt  time.Time
	EndsAt    time.Time
	Timezone  string
	Category  string
	FetchedAt time.Time
}

// ReplacePeerEvents swaps a peer's cached listings for a freshly fetched set, in
// one transaction.
//
// A wholesale replace rather than an upsert, and that is the point: an event the
// publisher has unpublished, deleted or renamed vanishes from this node on the
// next pull instead of surviving as a listing that sends people to a page that
// no longer exists. An empty `evs` is a legitimate and meaningful result — "that
// peer has nothing published right now" — and clears the cache.
//
// The delete and the insert share one transaction so a failure halfway cannot
// leave an operator's site showing half of one pull and half of another.
func (s *Store) ReplacePeerEvents(ctx context.Context, peerID string, evs []PeerEvent) error {
	if peerID == "" {
		return fmt.Errorf("store: replacing peer events needs a peer id")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace peer events: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM peer_event WHERE peer_id = ?`, peerID); err != nil {
		return fmt.Errorf("store: clearing a peer's cached events: %w", err)
	}
	for i := range evs {
		e := &evs[i]
		if e.ID == "" {
			e.ID = NewID()
		}
		e.PeerID = peerID
		if e.FetchedAt.IsZero() {
			e.FetchedAt = time.Now()
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO peer_event (id, peer_id, org_id, publisher_key, publisher_name,
			                        remote_event_id, slug, url, title, summary,
			                        venue_name, address, starts_at, ends_at, timezone,
			                        category, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.PeerID, e.OrgID, e.PublisherKey, e.PublisherName,
			e.RemoteEventID, e.Slug, e.URL, e.Title, e.Summary,
			e.VenueName, e.Address, timeToText(e.StartsAt), timeToText(e.EndsAt), e.Timezone,
			e.Category, timeToText(e.FetchedAt))
		if err != nil {
			return fmt.Errorf("store: caching a peer event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: replace peer events: %w", err)
	}
	return nil
}

// DeletePeerEventsForPeer drops everything cached from one peer.
//
// This is what turning `feed_subscribe` OFF must do, and why it is a separate
// method rather than a side effect of ReplacePeerEvents: withdrawing consent has
// to remove the listings, not merely stop refreshing them. A peer an operator
// has stopped showing must stop being shown at once, not at the next pull that
// may never be triggered.
func (s *Store) DeletePeerEventsForPeer(ctx context.Context, peerID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM peer_event WHERE peer_id = ?`, peerID); err != nil {
		return fmt.Errorf("store: delete cached peer events: %w", err)
	}
	return nil
}

const peerEventColumns = `SELECT pe.id, pe.peer_id, pe.org_id, pe.publisher_key, pe.publisher_name,
	pe.remote_event_id, pe.slug, pe.url, pe.title, pe.summary,
	pe.venue_name, pe.address, pe.starts_at, pe.ends_at, pe.timezone,
	pe.category, pe.fetched_at`

// ListPeerEventsForOrg returns the listings one organisation currently borrows
// from its peers, soonest first.
//
// The join onto `sync_peer` is a live re-check of consent rather than a
// convenience: a row is returned only while its peer is still enrolled, still
// enabled, and still has feed_subscribe on. Cached rows outliving the consent
// that fetched them is precisely the failure this join exists to make
// impossible, and it means a revoked peer disappears from every reader the
// moment the switch moves — without depending on a delete having run first.
func (s *Store) ListPeerEventsForOrg(ctx context.Context, orgID string) ([]PeerEvent, error) {
	return s.queryPeerEvents(ctx, peerEventColumns+`
		FROM peer_event pe
		JOIN sync_peer sp ON sp.id = pe.peer_id
		WHERE pe.org_id = ?
		  AND sp.enabled = 1
		  AND sp.feed_subscribe = 1
		ORDER BY pe.starts_at, pe.id`, orgID)
}

// ListPeerEventsForPeer returns one peer's cached listings, for the operator
// screen that shows what a pull actually brought back. It applies the same
// consent join for the same reason.
func (s *Store) ListPeerEventsForPeer(ctx context.Context, peerID string) ([]PeerEvent, error) {
	return s.queryPeerEvents(ctx, peerEventColumns+`
		FROM peer_event pe
		JOIN sync_peer sp ON sp.id = pe.peer_id
		WHERE pe.peer_id = ?
		  AND sp.enabled = 1
		  AND sp.feed_subscribe = 1
		ORDER BY pe.starts_at, pe.id`, peerID)
}

func (s *Store) queryPeerEvents(ctx context.Context, query string, args ...any) ([]PeerEvent, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list peer events: %w", err)
	}
	defer rows.Close()

	out := make([]PeerEvent, 0)
	for rows.Next() {
		var (
			e                           PeerEvent
			startsAt, endsAt, fetchedAt string
		)
		if err := rows.Scan(&e.ID, &e.PeerID, &e.OrgID, &e.PublisherKey, &e.PublisherName,
			&e.RemoteEventID, &e.Slug, &e.URL, &e.Title, &e.Summary,
			&e.VenueName, &e.Address, &startsAt, &endsAt, &e.Timezone,
			&e.Category, &fetchedAt); err != nil {
			return nil, fmt.Errorf("store: scan peer event: %w", err)
		}
		if e.StartsAt, err = textToTime(startsAt); err != nil {
			return nil, fmt.Errorf("store: peer event %s starts_at %q: %w", e.ID, startsAt, err)
		}
		if e.EndsAt, err = textToTime(endsAt); err != nil {
			return nil, fmt.Errorf("store: peer event %s ends_at %q: %w", e.ID, endsAt, err)
		}
		if e.FetchedAt, err = textToTime(fetchedAt); err != nil {
			return nil, fmt.Errorf("store: peer event %s fetched_at %q: %w", e.ID, fetchedAt, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
