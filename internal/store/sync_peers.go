package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SyncPeer is one node an operator has enrolled by hand: an address they typed
// and a public key they pinned.
//
// Every field here comes from a human or from this node's own bookkeeping.
// Nothing in Cackle discovers a peer, broadcasts for one, or accepts one it was
// not told about — an inbound request presenting an unpinned key is refused, and
// a pinned key that stops matching is refused rather than re-pinned.
type SyncPeer struct {
	ID    string
	OrgID string
	// Name is an operator's label ("cloud", "box-office laptop"). It carries no
	// authority whatsoever; PublicKey is the identity.
	Name string
	// URL is the peer's base address, or "" for an inbound-only peer this node
	// must never dial. Emptiness is a deliberate configuration, not missing
	// data: it is how a node behind NAT is enrolled — the reachable side gets a
	// URL and drives both directions.
	URL string
	// PublicKey is the pinned Ed25519 public key, lowercase hex. It is the
	// peer's identity, and it is the only thing an inbound request is
	// authenticated against.
	PublicKey string
	Enabled   bool
	// PullCursor is the peer's own sync_op.seq this node has consumed up to.
	PullCursor int64
	// PushCursor is THIS node's sync_op.seq that the peer has accepted up to.
	PushCursor int64
	// LastSyncAt and LastStatus are diagnostics for an operator, written on
	// every round including the failures. A refusal is exactly what an operator
	// needs to see, so it is recorded rather than swallowed.
	LastSyncAt *time.Time
	LastStatus string
	CreatedAt  time.Time

	// FeedPublish and FeedSubscribe are the two event-feed switches (migration
	// 0007), and they are SEPARATE from Enabled on purpose. Enabling a peer is
	// consent to reconcile door scans with it; it is not consent to hand that
	// operator a copy of your programme, and it is not consent to show theirs.
	// Both default to false on every row, including rows that existed before the
	// feature did.
	//
	// FeedPublish: this node answers the feed route for that peer's key.
	// FeedSubscribe: this node may DIAL that peer for its feed when an operator
	// asks. False means no socket is opened at all — the refusal happens before
	// the request is built.
	FeedPublish   bool
	FeedSubscribe bool
	// FeedPulledAt and FeedStatus are the feed direction's own diagnostics, kept
	// apart from LastSyncAt/LastStatus so a healthy admission round cannot make
	// a broken feed pull look fine.
	FeedPulledAt *time.Time
	FeedStatus   string
}

// CreateSyncPeer enrols a peer. PublicKey must already be normalized
// (NormalizeNodeKey); it is re-checked here because a malformed pin is a
// security hole rather than a formatting nit, and this is the last boundary
// before it becomes durable.
func (s *Store) CreateSyncPeer(ctx context.Context, p *SyncPeer) error {
	key, err := NormalizeNodeKey(p.PublicKey)
	if err != nil {
		return err
	}
	p.PublicKey = key
	if p.OrgID == "" {
		return errors.New("store: a sync peer requires an org id")
	}
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	// The feed columns are left to their DEFAULT 0 rather than named here. That
	// is the fail-closed default written down in one place: a newly enrolled
	// peer publishes nothing to and shows nothing from this node until an
	// operator turns each direction on by hand, and no caller of this function
	// can pass a value that skips that step.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sync_peer (id, org_id, name, url, public_key, enabled,
		                       pull_cursor, push_cursor, last_sync_at, last_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0, '', ?, ?)`,
		p.ID, p.OrgID, p.Name, p.URL, p.PublicKey, enabled, p.LastStatus, timeToText(p.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create sync peer: %w", err)
	}
	return nil
}

const syncPeerColumns = `id, org_id, name, url, public_key, enabled,
	pull_cursor, push_cursor, last_sync_at, last_status, created_at,
	feed_publish, feed_subscribe, feed_pulled_at, feed_status`

func scanSyncPeer(sc interface{ Scan(...any) error }) (SyncPeer, error) {
	var (
		p             SyncPeer
		enabled       int
		lastSyncAt    string
		createdAt     string
		feedPublish   int
		feedSubscribe int
		feedPulledAt  string
	)
	if err := sc.Scan(&p.ID, &p.OrgID, &p.Name, &p.URL, &p.PublicKey, &enabled,
		&p.PullCursor, &p.PushCursor, &lastSyncAt, &p.LastStatus, &createdAt,
		&feedPublish, &feedSubscribe, &feedPulledAt, &p.FeedStatus); err != nil {
		return SyncPeer{}, err
	}
	p.Enabled = enabled == 1
	// Only the literal 1 is on. A column that somehow holds anything else reads
	// as OFF, which is the direction a consent switch must fail in.
	p.FeedPublish = feedPublish == 1
	p.FeedSubscribe = feedSubscribe == 1
	if lastSyncAt != "" {
		t, err := textToTime(lastSyncAt)
		if err != nil {
			return SyncPeer{}, fmt.Errorf("store: sync peer %s last_sync_at %q: %w", p.ID, lastSyncAt, err)
		}
		p.LastSyncAt = &t
	}
	if feedPulledAt != "" {
		t, err := textToTime(feedPulledAt)
		if err != nil {
			return SyncPeer{}, fmt.Errorf("store: sync peer %s feed_pulled_at %q: %w", p.ID, feedPulledAt, err)
		}
		p.FeedPulledAt = &t
	}
	t, err := textToTime(createdAt)
	if err != nil {
		return SyncPeer{}, fmt.Errorf("store: sync peer %s created_at %q: %w", p.ID, createdAt, err)
	}
	p.CreatedAt = t
	return p, nil
}

// GetSyncPeer reads one enrolled peer by id.
func (s *Store) GetSyncPeer(ctx context.Context, id string) (SyncPeer, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+syncPeerColumns+` FROM sync_peer WHERE id = ?`, id)
	p, err := scanSyncPeer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncPeer{}, ErrNotFound
	}
	if err != nil {
		return SyncPeer{}, fmt.Errorf("store: get sync peer: %w", err)
	}
	return p, nil
}

// ListSyncPeers returns every peer enrolled for one organisation, enabled or
// not, oldest first.
func (s *Store) ListSyncPeers(ctx context.Context, orgID string) ([]SyncPeer, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+syncPeerColumns+` FROM sync_peer WHERE org_id = ? ORDER BY created_at, id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: list sync peers: %w", err)
	}
	defer rows.Close()

	out := make([]SyncPeer, 0)
	for rows.Next() {
		p, err := scanSyncPeer(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan sync peer: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EnabledSyncPeersByKey returns every enabled enrolment of one public key,
// across organisations.
//
// This is the inbound authentication lookup, and the plural is the point. One
// key may be enrolled for several organisations on a multi-tenant node; the
// authenticated caller's reach is the union of exactly those enrolments and
// nothing wider. An empty result is a refusal: the key is not pinned here.
func (s *Store) EnabledSyncPeersByKey(ctx context.Context, publicKey string) ([]SyncPeer, error) {
	key, err := NormalizeNodeKey(publicKey)
	if err != nil {
		// A key that is not even well-formed is not enrolled, by definition.
		// Returning no peers rather than an error keeps the caller's fail-closed
		// path to one branch.
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+syncPeerColumns+` FROM sync_peer WHERE public_key = ? AND enabled = 1
		 ORDER BY created_at, id`, key)
	if err != nil {
		return nil, fmt.Errorf("store: sync peers by key: %w", err)
	}
	defer rows.Close()

	out := make([]SyncPeer, 0)
	for rows.Next() {
		p, err := scanSyncPeer(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan sync peer: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// IsEnrolledNodeKey reports whether publicKey is pinned for orgID.
//
// It is the check that decides whether an op authored by some key may enter this
// node's log: authorship is not transitively trusted, so a claim relayed by an
// enrolled peer but SIGNED by a key this operator never pinned is refused. A
// mesh converges when every node has enrolled every other node — which is a
// human decision, made once, per pair.
func (s *Store) IsEnrolledNodeKey(ctx context.Context, orgID, publicKey string) (bool, error) {
	key, err := NormalizeNodeKey(publicKey)
	if err != nil {
		return false, nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sync_peer WHERE org_id = ? AND public_key = ? AND enabled = 1`,
		orgID, key).Scan(&n); err != nil {
		return false, fmt.Errorf("store: check enrolled node key: %w", err)
	}
	return n > 0, nil
}

// DeleteSyncPeer removes an enrolment. It is also how a key is rotated: delete,
// then enrol the new key. There is deliberately no "update the key" — an
// in-place re-pin is the silent trust change this whole design exists to
// prevent, so replacing a pin takes two explicit operator actions.
func (s *Store) DeleteSyncPeer(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sync_peer WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete sync peer: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete sync peer: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdvanceSyncPeerCursors moves a peer's cursors forward and records the outcome
// of the round.
//
// Cursors only ever move FORWARD (`MAX(existing, new)` in SQL, not assignment):
// a round that fails halfway, or two rounds racing, must never rewind a cursor
// and re-serve history, and must never be able to jump one past ops it did not
// actually transfer.
func (s *Store) AdvanceSyncPeerCursors(ctx context.Context, id string, pull, push int64, at time.Time, status string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sync_peer
		   SET pull_cursor = MAX(pull_cursor, ?),
		       push_cursor = MAX(push_cursor, ?),
		       last_sync_at = ?,
		       last_status = ?
		 WHERE id = ?`,
		pull, push, timeToText(at), status, id)
	if err != nil {
		return fmt.Errorf("store: advance sync peer cursors: %w", err)
	}
	return nil
}

// SetSyncPeerStatus records the outcome of a round without touching cursors —
// what a refused or unreachable peer gets.
func (s *Store) SetSyncPeerStatus(ctx context.Context, id string, at time.Time, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sync_peer SET last_sync_at = ?, last_status = ? WHERE id = ?`,
		timeToText(at), status, id)
	if err != nil {
		return fmt.Errorf("store: set sync peer status: %w", err)
	}
	return nil
}
