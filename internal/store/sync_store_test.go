package store

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

// Tests for the durable side of replication: the node identity, the pin store,
// and the op log's cursor and idempotency.

func testKey(t *testing.T, seed byte) string {
	t.Helper()
	b := make([]byte, ed25519.PublicKeySize)
	for i := range b {
		b[i] = seed
	}
	return hex.EncodeToString(b)
}

func TestNodeIdentityIsLazyAndStable(t *testing.T) {
	st := openTestStore(t)

	// A node that has never clustered holds no key material. This is the default
	// and it is asserted rather than assumed.
	if _, err := st.NodeIdentity(t.Context()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a fresh node must have no replication identity, got %v", err)
	}

	first, err := st.EnsureNodeIdentity(t.Context())
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}
	if len(first.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatalf("identity private key is %d bytes", len(first.PrivateKey))
	}
	if want := hex.EncodeToString(first.PrivateKey.Public().(ed25519.PublicKey)); want != first.PublicKey {
		t.Fatalf("the two halves of the identity disagree: %s vs %s", want, first.PublicKey)
	}

	// Ensuring twice must not rotate it. A node whose key changed on restart
	// would be refused by every peer that pinned the old one.
	second, err := st.EnsureNodeIdentity(t.Context())
	if err != nil {
		t.Fatalf("ensure identity again: %v", err)
	}
	if second.PublicKey != first.PublicKey {
		t.Fatalf("identity rotated on a second call: %s -> %s", first.PublicKey, second.PublicKey)
	}
}

func TestNodeIdentityRefusesACorruptRow(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.EnsureNodeIdentity(t.Context()); err != nil {
		t.Fatalf("ensure identity: %v", err)
	}
	// Make the public half disagree with the private half, the way a partial
	// restore or a hand-edited row would.
	if _, err := st.DB().ExecContext(t.Context(),
		`UPDATE sync_node_identity SET public_key = ? WHERE id = 1`, testKey(t, 9)); err != nil {
		t.Fatalf("corrupt the row: %v", err)
	}
	_, err := st.NodeIdentity(t.Context())
	if err == nil {
		t.Fatal("an identity whose halves disagree must be refused, not used")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("the refusal should name the mismatch, got %v", err)
	}
	// And EnsureNodeIdentity must NOT paper over it by minting a new key: that
	// would rotate this node's identity silently and every peer would refuse it
	// with no explanation anywhere.
	if _, err := st.EnsureNodeIdentity(t.Context()); err == nil {
		t.Fatal("Ensure must surface a corrupt identity rather than replacing it")
	}
}

func TestNormalizeNodeKey(t *testing.T) {
	good := testKey(t, 3)
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{good, good, false},
		{strings.ToUpper(good), good, false},
		{"  " + good + "\n", good, false},
		{"", "", true},
		{"abcd", "", true},
		{strings.Repeat("z", 64), "", true},
		{good + "00", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeNodeKey(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NormalizeNodeKey(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("NormalizeNodeKey(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestSyncPeerPinStore(t *testing.T) {
	st := openTestStore(t)
	seedAdmittableTicket(t, st, "ev-a", "tk-a")
	seedAdmittableTicket(t, st, "ev-b", "tk-b")
	orgA, orgB := "org_ev-a", "org_ev-b"
	key := testKey(t, 1)

	p := &SyncPeer{OrgID: orgA, Name: "cloud", URL: "https://cloud.example", PublicKey: strings.ToUpper(key), Enabled: true}
	if err := st.CreateSyncPeer(t.Context(), p); err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if p.PublicKey != key {
		t.Fatalf("the pin was stored as %q, not canonicalised to %q", p.PublicKey, key)
	}

	// The same key in the same org is a conflict, so nothing can re-pin an
	// address by enrolling over the top of an existing pin.
	if err := st.CreateSyncPeer(t.Context(), &SyncPeer{OrgID: orgA, PublicKey: key, Enabled: true}); err == nil {
		t.Fatal("enrolling the same key twice for one org must fail")
	}
	// The same key in a DIFFERENT org is legitimate: one peer, two organisations
	// it was separately trusted with.
	if err := st.CreateSyncPeer(t.Context(), &SyncPeer{OrgID: orgB, PublicKey: key, Enabled: true}); err != nil {
		t.Fatalf("enrol the same key for another org: %v", err)
	}

	found, err := st.EnabledSyncPeersByKey(t.Context(), key)
	if err != nil {
		t.Fatalf("by key: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("expected both enrolments of this key, got %d", len(found))
	}

	// A key nobody enrolled resolves to nothing — the refusal the transport
	// depends on.
	none, err := st.EnabledSyncPeersByKey(t.Context(), testKey(t, 2))
	if err != nil {
		t.Fatalf("by key: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("an unpinned key must resolve to no peer, got %d", len(none))
	}
	// A malformed key is not enrolled either, and asking is not an error: the
	// caller has one fail-closed branch, not two.
	if got, err := st.EnabledSyncPeersByKey(t.Context(), "not-a-key"); err != nil || len(got) != 0 {
		t.Fatalf("malformed key lookup = %d, %v; want 0, nil", len(got), err)
	}

	ok, err := st.IsEnrolledNodeKey(t.Context(), orgA, key)
	if err != nil || !ok {
		t.Fatalf("IsEnrolledNodeKey(orgA) = %v, %v; want true", ok, err)
	}
	if ok, _ := st.IsEnrolledNodeKey(t.Context(), orgA, testKey(t, 2)); ok {
		t.Fatal("an unpinned author key must not be reported as enrolled")
	}

	// Cursors only move forward, so a late or repeated round cannot rewind one.
	now := time.Now()
	if err := st.AdvanceSyncPeerCursors(t.Context(), p.ID, 10, 20, now, "pushed 20"); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if err := st.AdvanceSyncPeerCursors(t.Context(), p.ID, 4, 7, now, "stale round"); err != nil {
		t.Fatalf("advance: %v", err)
	}
	got, err := st.GetSyncPeer(t.Context(), p.ID)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if got.PullCursor != 10 || got.PushCursor != 20 {
		t.Fatalf("cursors went backwards: %d/%d", got.PullCursor, got.PushCursor)
	}
	if got.LastSyncAt == nil || got.LastStatus == "" {
		t.Fatalf("the round outcome must be recorded for an operator: %+v", got)
	}

	if err := st.DeleteSyncPeer(t.Context(), p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := st.DeleteSyncPeer(t.Context(), p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting a gone peer = %v, want ErrNotFound", err)
	}
}

func TestSyncOpLogIdempotencyAndCursor(t *testing.T) {
	st := openTestStore(t)
	seedAdmittableTicket(t, st, "ev-oplog", "tk-oplog")
	orgID, eventID := "org_ev-oplog", "ev-oplog"
	author := testKey(t, 4)
	other := testKey(t, 5)
	at := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	op := SyncOp{
		OpID: "aa11", EventID: eventID, Author: author,
		ClaimTicket: "ticket-1", ClaimDevice: "device-A", ClaimScannedAt: at,
		Cose: []byte("envelope-1"), Applied: true,
	}
	fresh, err := st.AppendSyncOp(t.Context(), op)
	if err != nil || !fresh {
		t.Fatalf("first append = %v, %v; want true, nil", fresh, err)
	}
	// The identical envelope again is one row: a re-push after a dropped
	// connection must be a no-op.
	if fresh, err := st.AppendSyncOp(t.Context(), op); err != nil || fresh {
		t.Fatalf("re-appending the same op = %v, %v; want false, nil", fresh, err)
	}
	// The same CLAIM under a different author and a different content address is
	// ALSO one row. Two nodes each minting their own op for one scan describe the
	// same §4.3 element, and keeping one is what stops the log growing a copy per
	// node.
	twin := op
	twin.OpID = "bb22"
	twin.Author = other
	twin.Cose = []byte("envelope-2")
	if fresh, err := st.AppendSyncOp(t.Context(), twin); err != nil || fresh {
		t.Fatalf("the same claim from another author = %v, %v; want false, nil", fresh, err)
	}

	// A different claim is a different row.
	second := op
	second.OpID = "cc33"
	second.ClaimDevice = "device-B"
	second.Cose = []byte("envelope-3")
	second.Applied = false
	if fresh, err := st.AppendSyncOp(t.Context(), second); err != nil || !fresh {
		t.Fatalf("a distinct claim = %v, %v; want true, nil", fresh, err)
	}

	// An incomplete op is refused rather than stored as a row nobody can verify.
	if _, err := st.AppendSyncOp(t.Context(), SyncOp{OpID: "dd44"}); err == nil {
		t.Fatal("an op with no envelope must be refused")
	}

	page, err := st.SyncOpsForOrgAfter(t.Context(), orgID, 0, 10)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(page))
	}
	if page[0].Seq >= page[1].Seq {
		t.Fatalf("ops must come back in cursor order, got %d then %d", page[0].Seq, page[1].Seq)
	}
	if string(page[0].Cose) != "envelope-1" {
		t.Fatalf("the stored envelope must come back byte for byte, got %q", page[0].Cose)
	}
	if !page[0].ClaimScannedAt.Equal(at) {
		t.Fatalf("claim time round-tripped as %s, want %s", page[0].ClaimScannedAt, at)
	}

	// A cursor past the first op returns only what follows it.
	rest, err := st.SyncOpsForOrgAfter(t.Context(), orgID, page[0].Seq, 10)
	if err != nil {
		t.Fatalf("read ops after cursor: %v", err)
	}
	if len(rest) != 1 || rest[0].Seq != page[1].Seq {
		t.Fatalf("cursor did not resume correctly: %+v", rest)
	}
	// A limit bounds the page.
	if one, err := st.SyncOpsForOrgAfter(t.Context(), orgID, 0, 1); err != nil || len(one) != 1 {
		t.Fatalf("limit 1 returned %d ops (%v)", len(one), err)
	}

	// Another organisation sees none of it. This join is the authorisation
	// boundary for a pull.
	seedAdmittableTicket(t, st, "ev-outsider", "tk-outsider")
	otherOrg := "org_ev-outsider"
	if got, err := st.SyncOpsForOrgAfter(t.Context(), otherOrg, 0, 10); err != nil || len(got) != 0 {
		t.Fatalf("another org saw %d ops (%v)", len(got), err)
	}

	stats, err := st.SyncOpStatsForOrg(t.Context(), orgID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Ops != 2 || stats.Unapplied != 1 {
		t.Fatalf("stats = %+v; want 2 ops, 1 unapplied", stats)
	}
	if err := st.MarkSyncOpApplied(t.Context(), "cc33"); err != nil {
		t.Fatalf("mark applied: %v", err)
	}
	if stats, _ = st.SyncOpStatsForOrg(t.Context(), orgID); stats.Unapplied != 0 {
		t.Fatalf("after applying, unapplied = %d", stats.Unapplied)
	}
}

// TestUnmintedAdmissionsSkipsClaimsAlreadyInTheLog is the anti-join that stops a
// claim learned from a peer being re-minted locally under this node's own key.
//
// Without it every hop would add a copy of the same claim to every log, forever,
// and the op count would grow with the number of nodes rather than with the
// number of scans.
func TestUnmintedAdmissionsSkipsClaimsAlreadyInTheLog(t *testing.T) {
	st := openTestStore(t)
	seedAdmittableTicket(t, st, "ev-unminted", "tk-unminted")
	orgID, eventID, ticketID := "org_ev-unminted", "ev-unminted", "tk-unminted"
	at := time.Date(2026, 7, 30, 9, 30, 0, 0, time.UTC)

	if _, err := st.DB().ExecContext(t.Context(), `
		INSERT INTO admissions (id, ticket_id, event_id, gate_id, device_id, scanned_at, result, reported_result, note)
		VALUES (?, ?, ?, 'North', 'device-A', ?, 'admitted', 'admitted', '')`,
		NewID(), ticketID, eventID, timeToText(at)); err != nil {
		t.Fatalf("insert admission: %v", err)
	}

	pending, err := st.UnmintedAdmissions(t.Context(), orgID, 10)
	if err != nil {
		t.Fatalf("unminted: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected the one un-minted claim, got %d", len(pending))
	}
	if pending[0].ReportedResult != "admitted" {
		t.Fatalf("the device's own verdict must come back for replication, got %q", pending[0].ReportedResult)
	}

	if _, err := st.AppendSyncOp(t.Context(), SyncOp{
		OpID: "ee55", EventID: eventID, Author: testKey(t, 6),
		ClaimTicket: ticketID, ClaimDevice: "device-A", ClaimScannedAt: at,
		Cose: []byte("envelope"),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	pending, err = st.UnmintedAdmissions(t.Context(), orgID, 10)
	if err != nil {
		t.Fatalf("unminted: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("a claim already in the log must not be minted again, got %d", len(pending))
	}
}

func TestEventOrgIDsSkipsUnknownEvents(t *testing.T) {
	st := openTestStore(t)
	seedAdmittableTicket(t, st, "ev-org", "tk-org")
	orgID, eventID := "org_ev-org", "ev-org"

	got, err := st.EventOrgIDs(t.Context(), []string{eventID, "no-such-event", eventID, ""})
	if err != nil {
		t.Fatalf("event orgs: %v", err)
	}
	if got[eventID] != orgID {
		t.Fatalf("known event resolved to %q, want %q", got[eventID], orgID)
	}
	if _, ok := got["no-such-event"]; ok {
		t.Fatal("an event this node does not hold must be absent, not guessed at")
	}
}
