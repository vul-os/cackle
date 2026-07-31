package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kotvasync "github.com/vul-os/kotva/bindings/go"

	"github.com/vul-os/cackle/internal/scan/substrate"
	"github.com/vul-os/cackle/internal/store"
)

// Tests for paging a peer's event feed.
//
// The feed used to be one answer capped at 200 listings. The cap was reported
// honestly — `complete:false` — but there was no way to ask for the rest, so a
// host with 201 published events could not be mirrored by a peer at all. These
// tests are the proof that page 2 now exists, that a subscriber follows it, and
// that following it stays BOUNDED: against a peer that lies about where the next
// page starts, against one that never says it is finished, and against an
// organiser who edits their programme while the walk is in progress.
//
// Each guard below carries a MUTATION note naming the single edit that must turn
// it red. Every one of them was made and reverted; a guard whose mutation leaves
// the suite green is not a guard, it is a comment.

// seedPublishedEvents writes n published events straight into the events table
// and returns their ids in ascending id order.
//
// Straight into the table on purpose: this fixture needs hundreds of rows and the
// only thing under test is what the feed route does with them. Ids come from
// store.NewID(), so they are real ULIDs minted in creation order — which is
// exactly the key the cursor rides.
func seedPublishedEvents(t *testing.T, h *testHarness, orgID, prefix string, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	base := time.Now().Add(90 * 24 * time.Hour)
	for i := range n {
		// starts_at deliberately DESCENDS as the ids ascend, so a test that
		// passes only because the two orders happen to agree cannot pass here.
		ids = append(ids, insertPublishedEvent(t, h, orgID,
			store.NewID(),
			fmt.Sprintf("%s-%04d", prefix, i),
			fmt.Sprintf("Seeded Show %04d", i),
			base.Add(-time.Duration(i)*time.Hour)))
	}
	return ids
}

func insertPublishedEvent(t *testing.T, h *testHarness, orgID, id, slug, title string, starts time.Time) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := h.store.DB().ExecContext(t.Context(), `
		INSERT INTO events (id, org_id, slug, title, summary, venue_name, address,
		                    starts_at, ends_at, timezone, status, currency, category,
		                    created_at, updated_at)
		VALUES (?, ?, ?, ?, '', 'Main Room', '1 Test Street', ?, ?, 'Africa/Johannesburg',
		        'published', 'ZAR', '', ?, ?)`,
		id, orgID, slug, title,
		starts.UTC().Format(time.RFC3339Nano),
		starts.Add(3*time.Hour).UTC().Format(time.RFC3339Nano),
		now, now); err != nil {
		t.Fatalf("insert event %s: %v", slug, err)
	}
	return id
}

// countCachedFor returns how many rows node h holds for one peer, and how many
// DISTINCT remote event ids those rows carry. The two differing would mean a page
// was stored twice.
func countCachedFor(t *testing.T, h *testHarness, peerID string) (rows, distinct int) {
	t.Helper()
	if err := h.store.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*), COUNT(DISTINCT remote_event_id) FROM peer_event WHERE peer_id = ?`,
		peerID).Scan(&rows, &distinct); err != nil {
		t.Fatalf("count cached rows: %v", err)
	}
	return rows, distinct
}

// --- the two-node proof ------------------------------------------------------

// TestTwoNodesMirrorAFeedLargerThanOnePage is the end-to-end claim: node A
// publishes more than one page of events, node B enrols and fetches ONCE, and
// node B ends up holding EVERY one of them EXACTLY once.
//
// Two whole stacks on real sockets, driven the way two organisers would drive
// them — the same shape as TestTwoNodesShareEventFeedsAndAttributeThem, because a
// paging bug that only appears over a real signed request on a real socket is
// precisely the bug an in-process handler call would miss.
//
// MUTATION: set maxFeedPages = 1 (the old single-answer feed) and this fails at
// fetched=200. MUTATION: stop emitting resp.NextCursor in handlePeerFeed and the
// walk cannot continue past page one. MUTATION: change the page query's
// `ORDER BY id` to `ORDER BY starts_at, id` while the cursor still keys on id,
// and the pages skip and repeat.
func TestTwoNodesMirrorAFeedLargerThanOnePage(t *testing.T) {
	ha := newTestHarness(t)
	fxa := ha.newPublishedEvent(t, "page-a")
	// 470 seeded plus the fixture's own = 471 published events: two full pages of
	// 200 and a part-full third, so "the page is full" and "the list ends here"
	// are both exercised in one walk.
	seeded := seedPublishedEvents(t, ha, fxa.orgID, "paged", 470)
	const wantTotal = 471
	nodeA := newSyncNode(t, ha, fxa.orgID, fxa.ownerToken)

	hb := newTestHarness(t)
	fxb := hb.newPublishedEvent(t, "page-b")
	nodeB := newSyncNode(t, hb, fxb.orgID, fxb.ownerToken)

	peerOnA := enrolPeerOn(t, nodeA, "the publisher's view of B", "", nodeB.key)
	peerOnB := enrolPeerOn(t, nodeB, "the big programme", nodeA.url, nodeA.key)
	setFeed(t, nodeA, peerOnA, true, false)
	setFeed(t, nodeB, peerOnB, false, true)

	var pull feedPullResult
	if code := nodeB.call(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil, &pull); code != http.StatusOK {
		t.Fatalf("feed pull: status %d (%+v)", code, pull)
	}
	if pull.Error != "" {
		t.Fatalf("feed pull reported an error: %s", pull.Error)
	}
	if pull.Fetched != wantTotal || pull.Stored != wantTotal {
		t.Fatalf("a %d-event programme came back as fetched=%d stored=%d refused=%v: the feed is still truncating",
			wantTotal, pull.Fetched, pull.Stored, pull.Refused)
	}
	if !pull.Complete {
		t.Fatalf("every page was fetched, so the pull must report itself complete: %+v", pull)
	}
	if pull.Pages != 3 {
		t.Fatalf("%d events over pages of %d is 3 requests, the walk made %d",
			wantTotal, maxFeedEvents, pull.Pages)
	}

	// Exactly once each, on disk.
	rows, distinct := countCachedFor(t, hb, peerOnB)
	if rows != wantTotal || distinct != wantTotal {
		t.Fatalf("node B holds %d rows carrying %d distinct events, want %d of each",
			rows, distinct, wantTotal)
	}

	// And every seeded id SPECIFICALLY. A count can be right while the set is
	// wrong: two pages that overlap and skip by the same amount total correctly.
	for _, id := range seeded {
		var n int
		if err := hb.store.DB().QueryRowContext(t.Context(),
			`SELECT COUNT(*) FROM peer_event WHERE peer_id = ? AND remote_event_id = ?`,
			peerOnB, id).Scan(&n); err != nil {
			t.Fatalf("look up %s: %v", id, err)
		}
		if n != 1 {
			t.Fatalf("event %s is cached %d times, want exactly 1", id, n)
		}
	}

	// A second fetch REPLACES the cache rather than adding to it. That is the
	// property migration 0007's unique index exists for, and a multi-page walk is
	// the most likely thing to have broken it.
	var again feedPullResult
	if code := nodeB.call(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil, &again); code != http.StatusOK {
		t.Fatalf("second feed pull: status %d (%+v)", code, again)
	}
	if again.Stored != wantTotal {
		t.Fatalf("second pull stored %d, want %d", again.Stored, wantTotal)
	}
	rows, distinct = countCachedFor(t, hb, peerOnB)
	if rows != wantTotal || distinct != wantTotal {
		t.Fatalf("a repeat fetch grew the cache to %d rows (%d distinct), want %d",
			rows, distinct, wantTotal)
	}
}

// TestFeedPageBoundIsStatedNotSilent is the other half of the honesty rule: a
// programme too big even for the bounded walk is still truncated — and SAYS so,
// with the number, where an operator will read it.
//
// MUTATION: set out.Complete = true unconditionally before the cache is replaced
// and this fails on the first assertion. MUTATION: drop the page bound from the
// walk's loop condition and it fails on the page count.
func TestFeedPageBoundIsStatedNotSilent(t *testing.T) {
	ha := newTestHarness(t)
	fxa := ha.newPublishedEvent(t, "bound-a")
	total := maxFeedEvents*maxFeedPages + 1 // one more than the whole walk carries
	seedPublishedEvents(t, ha, fxa.orgID, "bounded", total-1)
	nodeA := newSyncNode(t, ha, fxa.orgID, fxa.ownerToken)

	hb := newTestHarness(t)
	fxb := hb.newPublishedEvent(t, "bound-b")
	nodeB := newSyncNode(t, hb, fxb.orgID, fxb.ownerToken)

	peerOnA := enrolPeerOn(t, nodeA, "b", "", nodeB.key)
	peerOnB := enrolPeerOn(t, nodeB, "a", nodeA.url, nodeA.key)
	setFeed(t, nodeA, peerOnA, true, false)
	setFeed(t, nodeB, peerOnB, false, true)

	var pull feedPullResult
	if code := nodeB.call(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil, &pull); code != http.StatusOK {
		t.Fatalf("feed pull: status %d (%+v)", code, pull)
	}
	if pull.Complete {
		t.Fatal("the walk stopped at its page bound with events left over, and reported itself COMPLETE")
	}
	if pull.Stored != maxFeedEvents*maxFeedPages {
		t.Fatalf("stored %d, want the full bounded walk of %d", pull.Stored, maxFeedEvents*maxFeedPages)
	}
	if pull.Pages != maxFeedPages {
		t.Fatalf("walked %d pages, want the stated bound of %d", pull.Pages, maxFeedPages)
	}

	// The bound is a sentence an operator can read, not a shrug.
	peer, err := hb.store.GetSyncPeer(t.Context(), peerOnB)
	if err != nil {
		t.Fatalf("reload peer: %v", err)
	}
	status := strings.ToLower(peer.FeedStatus)
	for _, want := range []string{"page", "more"} {
		if !strings.Contains(status, want) {
			t.Fatalf("the recorded status must say a page bound was hit and more remain, got %q", peer.FeedStatus)
		}
	}
	if !strings.Contains(status, fmt.Sprintf("%d", maxFeedPages)) {
		t.Fatalf("the stated bound must carry its NUMBER, got %q", peer.FeedStatus)
	}
}

// --- the cursor's own ordering ----------------------------------------------

// TestFeedCursorIsATotalOrderOverAnImmutableKey states the choice and proves the
// two properties it was chosen for.
//
// The cursor is (org id, event id). Both are ULID primary keys: assigned once,
// never rewritten, unique, and never reused. That makes the order TOTAL and
// IMMUTABLE, which an offset is not and an ORDER BY starts_at is not — starts_at
// is a column an organiser can edit from the events screen mid-walk.
func TestFeedCursorIsATotalOrderOverAnImmutableKey(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "cursor-order")
	ids := seedPublishedEvents(t, h, fx.orgID, "cursored", 5)
	checks := 0

	// 1. The page query is a SEEK on the primary key, not an offset.
	page, err := h.store.PublishedEventsAfterForOrg(t.Context(), fx.orgID, ids[1], 10)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("after the 2nd of 5 seeded ids, got %d rows, want the last 3", len(page))
	}
	for i := range page {
		if page[i].ID != ids[i+2] {
			t.Fatalf("page[%d] is %s, want %s: the page is not ordered by id", i, page[i].ID, ids[i+2])
		}
	}
	checks++

	// 2. Unpublishing a row BEHIND the cursor cannot shift the rows ahead of it.
	//    Under an offset it would: every later row slides one place forward and
	//    exactly one is skipped, silently.
	if err := h.store.SetEventStatus(t.Context(), ids[0], "draft", time.Now()); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	after, err := h.store.PublishedEventsAfterForOrg(t.Context(), fx.orgID, ids[1], 10)
	if err != nil {
		t.Fatalf("page after unpublish: %v", err)
	}
	if len(after) != 3 || after[0].ID != ids[2] {
		t.Fatalf("unpublishing a row behind the cursor moved the page: %d rows starting at %s",
			len(after), after[0].ID)
	}
	checks++

	// 3. Rescheduling a row ahead of the cursor cannot move it ACROSS the cursor.
	//    This is the case an ORDER BY starts_at cursor gets wrong — the row either
	//    arrives twice or never arrives at all — and it is why the cursor keys on
	//    the primary key instead.
	if _, err := h.store.DB().ExecContext(t.Context(),
		`UPDATE events SET starts_at = ? WHERE id = ?`,
		time.Now().Add(-500*24*time.Hour).UTC().Format(time.RFC3339Nano), ids[4]); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	rescheduled, err := h.store.PublishedEventsAfterForOrg(t.Context(), fx.orgID, ids[1], 10)
	if err != nil {
		t.Fatalf("page after reschedule: %v", err)
	}
	if len(rescheduled) != 3 || rescheduled[2].ID != ids[4] {
		t.Fatalf("rescheduling an event moved it across the cursor: %+v", rescheduled)
	}
	checks++

	if checks != 3 {
		t.Fatalf("this test is meant to make 3 assertions, made %d", checks)
	}
}

// TestFeedCursorRoundTripsAndRefusesJunk pins the wire format. A cursor arrives
// from another machine and is used to build a SQL predicate, so it is parsed
// against an allow-list rather than trusted.
//
// MUTATION: return the two halves from parseFeedCursor without the per-rune
// allow-list and eight of the eleven malformed cursors are accepted.
func TestFeedCursorRoundTripsAndRefusesJunk(t *testing.T) {
	c := feedCursor{OrgID: "01J000000000000000000000AA", EventID: "01J000000000000000000000BB"}
	parsed, err := parseFeedCursor(c.String())
	if err != nil {
		t.Fatalf("a cursor this node minted did not parse: %v", err)
	}
	if parsed != c {
		t.Fatalf("round trip lost information: %+v -> %+v", c, parsed)
	}
	if empty, err := parseFeedCursor(""); err != nil || empty != (feedCursor{}) {
		t.Fatalf("an absent cursor must mean the start of the feed, got %+v %v", empty, err)
	}

	junk := []string{
		"no-separator",
		".",
		"a.",
		".b",
		"a.b.c",
		"a b.c",
		"../../etc/passwd.x",
		"a.b\x00c",
		"'; DROP TABLE events;--.x",
		strings.Repeat("a", 65) + ".b",
		"a." + strings.Repeat("b", 65),
	}
	for _, raw := range junk {
		if _, err := parseFeedCursor(raw); err == nil {
			t.Fatalf("cursor %q was accepted", raw)
		}
	}
	if len(junk) != 11 {
		t.Fatalf("this test is meant to try 11 malformed cursors, tried %d", len(junk))
	}

	// And the route refuses one too, rather than treating junk as "start again".
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "cursor-junk")
	signer := enrolledSigner(t, h, fx.orgID)
	peers, err := h.store.EnabledSyncPeersByKey(t.Context(), hex.EncodeToString(signer.Public()))
	if err != nil || len(peers) != 1 {
		t.Fatalf("expected one enrolment: %v %d", err, len(peers))
	}
	if err := h.store.SetSyncPeerFeed(t.Context(), peers[0].ID, true, false); err != nil {
		t.Fatalf("turn publishing on: %v", err)
	}
	// The positive control first: the SAME signer, the SAME route, no cursor.
	// Without it a 400 below could just as well be the peer-auth layer refusing.
	if rec := peerRequest(t, h, http.MethodGet, "/api/sync/feed", signer, nil); rec.Code != http.StatusOK {
		t.Fatalf("the control request answered %d, so the refusal below proves nothing", rec.Code)
	}
	rec := peerRequest(t, h, http.MethodGet, "/api/sync/feed?cursor=not+a+cursor", signer, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a malformed cursor answered %d, want 400", rec.Code)
	}
}

// --- a programme edited while the walk is in progress ------------------------

// fetchFeedPage asks `to` for one page of its feed, signed as `from` would sign
// it, over a real socket — the subscriber's own request, one page at a time, so
// a test can change the publisher's data BETWEEN pages.
func fetchFeedPage(t *testing.T, to *syncNode, fromHarness *testHarness, cursor string) feedResponse {
	t.Helper()
	id, err := fromHarness.store.NodeIdentity(t.Context())
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}
	signer, err := substrate.NewSigner(id.PrivateKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	path := "/api/sync/feed"
	if cursor != "" {
		path += "?cursor=" + cursor
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, to.url+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	nonce, err := substrate.SignRequest(req, signer, nil, time.Now())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("feed page answered %d: %s", resp.StatusCode, raw)
	}
	// The page is verified against the publisher's key exactly as the subscriber
	// verifies it, so this test cannot pass on an answer a real pull would refuse.
	if err := substrate.VerifyResponse(resp.Header, to.key, nonce, raw, time.Now()); err != nil {
		t.Fatalf("feed page did not verify against the publisher's key: %v", err)
	}
	var out feedResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode feed page: %v", err)
	}
	return out
}

// TestFeedPagesSurviveThePublisherEditingMidWalk is the mutation case the cursor
// was chosen for: a real publisher, walked page by page, with its programme
// changing between the pages.
//
// Neither an event appearing nor an event disappearing may cause any OTHER event
// to be skipped or delivered twice. That is the whole promise of a keyset cursor
// over an immutable key, and it is the promise an offset cannot make.
//
// MUTATION: order the page query by `starts_at, id` while the cursor still keys
// on id — the ordering an offset-era feed would keep — and this fails with an
// event arriving twice, because the fixture's start times descend as its ids
// ascend and the two orders stop agreeing at the page boundary.
func TestFeedPagesSurviveThePublisherEditingMidWalk(t *testing.T) {
	ha := newTestHarness(t)
	fxa := ha.newPublishedEvent(t, "midwalk-a")
	seeded := seedPublishedEvents(t, ha, fxa.orgID, "midwalk", 320)
	nodeA := newSyncNode(t, ha, fxa.orgID, fxa.ownerToken)

	hb := newTestHarness(t)
	fxb := hb.newPublishedEvent(t, "midwalk-b")
	nodeB := newSyncNode(t, hb, fxb.orgID, fxb.ownerToken)

	peerOnA := enrolPeerOn(t, nodeA, "b", "", nodeB.key)
	setFeed(t, nodeA, peerOnA, true, false)

	// Page one.
	first := fetchFeedPage(t, nodeA, hb, "")
	if len(first.Events) != maxFeedEvents || first.Complete {
		t.Fatalf("page one carried %d events, complete=%v; want a full page and more to come",
			len(first.Events), first.Complete)
	}
	if first.NextCursor == "" {
		t.Fatal("an incomplete page must say where the next one starts")
	}

	// Now the organiser edits the programme, mid-walk:
	//  - publishes a NEW event (a fresh ULID, so it sorts after the cursor),
	//  - unpublishes one that page two was going to carry,
	//  - and publishes one whose id sorts BEHIND the cursor, which this walk will
	//    legitimately miss and the next walk will pick up.
	newAhead := insertPublishedEvent(t, ha, fxa.orgID, store.NewID(),
		"midwalk-added-ahead", "Added Mid-Walk", time.Now().Add(200*24*time.Hour))
	pulled := seeded[len(seeded)-1]
	if err := ha.store.SetEventStatus(t.Context(), pulled, "draft", time.Now()); err != nil {
		t.Fatalf("unpublish mid-walk: %v", err)
	}
	behind := insertPublishedEvent(t, ha, fxa.orgID, "00000000000000000000000001",
		"midwalk-added-behind", "Added Behind The Cursor", time.Now().Add(210*24*time.Hour))

	// The rest of the walk.
	seen := map[string]int{}
	for _, e := range first.Events {
		seen[e.EventID]++
	}
	cursor := first.NextCursor
	for pages := 0; pages < maxFeedPages; pages++ {
		page := fetchFeedPage(t, nodeA, hb, cursor)
		for _, e := range page.Events {
			seen[e.EventID]++
		}
		if page.Complete {
			break
		}
		cursor = page.NextCursor
	}

	for id, n := range seen {
		if n != 1 {
			t.Fatalf("event %s arrived %d times across the walk", id, n)
		}
	}
	// Every event that was published for the whole walk arrived exactly once.
	for _, id := range seeded[:len(seeded)-1] {
		if seen[id] != 1 {
			t.Fatalf("event %s was published throughout and arrived %d times", id, seen[id])
		}
	}
	// The one unpublished mid-walk did not arrive.
	if seen[pulled] != 0 {
		t.Fatalf("an event unpublished mid-walk was still served (%d times)", seen[pulled])
	}
	// The one published mid-walk AHEAD of the cursor did arrive.
	if seen[newAhead] != 1 {
		t.Fatalf("an event published mid-walk ahead of the cursor arrived %d times, want 1", seen[newAhead])
	}
	// The one published mid-walk BEHIND the cursor did not — and that is correct,
	// not a bug: it is behind a cursor already passed. It is picked up by the next
	// walk, which is why a pull is repeatable rather than one-shot.
	if seen[behind] != 0 {
		t.Fatalf("an event inserted behind the cursor appeared in this walk (%d times)", seen[behind])
	}
	nextWalk := fetchFeedPage(t, nodeA, hb, "")
	found := false
	for _, e := range nextWalk.Events {
		if e.EventID == behind {
			found = true
		}
	}
	if !found {
		t.Fatal("the event behind the cursor never arrives, even on a fresh walk")
	}
}

// --- a publisher that misbehaves ---------------------------------------------

// feedStub is a fake publisher. It answers /api/sync/feed with a REAL signed
// Cackle peer response — its own key, over the exact bytes it wrote, bound to the
// caller's nonce — and with whatever pagination behaviour a test wants to inflict
// on the subscriber. Everything upstream of the paging guard is therefore
// correct, which is the only way the guard itself can be what a refusal proves.
type feedStub struct {
	srv    *httptest.Server
	key    string
	signer kotvasync.Signer
	hits   atomic.Int64
}

func newFeedStub(t *testing.T, answer func(hit int, cursor string) feedResponse) *feedStub {
	t.Helper()
	signer := newTestSigner(t)
	st := &feedStub{signer: signer, key: hex.EncodeToString(signer.Public())}
	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit := int(st.hits.Add(1))
		if r.URL.Path != "/api/sync/feed" {
			http.NotFound(w, r)
			return
		}
		resp := answer(hit, r.URL.Query().Get("cursor"))
		resp.Node = st.key
		resp.Caveat = feedCaveat
		body, err := json.Marshal(resp)
		if err != nil {
			t.Errorf("stub marshal: %v", err)
			return
		}
		if err := substrate.SignResponse(w.Header(), signer, r.Header.Get(substrate.HeaderNonce), body, time.Now()); err != nil {
			t.Errorf("stub sign: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(st.srv.Close)
	return st
}

// stubEvents makes n well-formed listings whose ids carry the given prefix.
func stubEvents(prefix string, n int) []feedEventWire {
	out := make([]feedEventWire, 0, n)
	starts := time.Now().Add(30 * 24 * time.Hour)
	for i := range n {
		out = append(out, feedEventWire{
			EventID:  fmt.Sprintf("%s-%04d", prefix, i),
			Slug:     fmt.Sprintf("%s-%04d", strings.ToLower(prefix), i),
			Title:    fmt.Sprintf("Stub %s %d", prefix, i),
			StartsAt: starts,
			EndsAt:   starts.Add(2 * time.Hour),
		})
	}
	return out
}

// subscribeTo enrols the stub on h and turns the subscription on, returning the
// peer id.
func subscribeTo(t *testing.T, h *testHarness, orgID string, st *feedStub) string {
	t.Helper()
	p := &store.SyncPeer{OrgID: orgID, Name: "stub", URL: st.srv.URL, PublicKey: st.key, Enabled: true}
	if err := h.store.CreateSyncPeer(t.Context(), p); err != nil {
		t.Fatalf("enrol stub: %v", err)
	}
	if err := h.store.SetSyncPeerFeed(t.Context(), p.ID, false, true); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return p.ID
}

// TestSubscriberBoundsAMisbehavingPublisher covers every way a peer can try to
// keep this node walking, or walking backwards, forever.
//
// The FIRST case is a positive control: a well-behaved stub IS walked across two
// pages and stored. Without it, a refusal in any later case could just as well be
// peer-auth or the enrolment check refusing before the paging guard ever runs.
//
// MUTATION: delete the `!next.after(cursor)` check and the two cursor cases walk
// to the page bound instead of stopping at two requests. MUTATION: delete the
// empty-NextCursor check and "nowhere to continue from" silently stores a partial
// walk. MUTATION: delete the len(resp.Events) > maxFeedEvents check and an
// oversized page is swallowed. MUTATION: drop the page bound from the loop
// condition and "a peer that never terminates" does exactly that.
func TestSubscriberBoundsAMisbehavingPublisher(t *testing.T) {
	cases := []struct {
		name string
		// answer is the stub's behaviour: hit number (1-based) and the cursor
		// the subscriber sent.
		answer func(hit int, cursor string) feedResponse
		// wantHits is how many requests the subscriber is allowed to make.
		wantHits int
		// wantStored is how many listings survive into the cache.
		wantStored int
		wantErr    string
		// wantComplete is what the pull must report.
		wantComplete bool
	}{
		{
			name: "control: two honest pages are walked and stored",
			answer: func(hit int, cursor string) feedResponse {
				if hit == 1 {
					return feedResponse{Events: stubEvents("A", 2), Complete: false, NextCursor: "org1.ev2"}
				}
				return feedResponse{Events: stubEvents("B", 3), Complete: true}
			},
			wantHits: 2, wantStored: 5, wantComplete: true,
		},
		{
			name: "a cursor that points backwards",
			answer: func(hit int, cursor string) feedResponse {
				if hit == 1 {
					return feedResponse{Events: stubEvents("A", 2), Complete: false, NextCursor: "org5.ev9"}
				}
				// Strictly BEHIND where the walk already is. Following it would
				// re-fetch the same rows forever.
				return feedResponse{Events: stubEvents("B", 2), Complete: false, NextCursor: "org1.ev1"}
			},
			wantHits: 2, wantStored: 0, wantComplete: false,
			wantErr: "does not move forward",
		},
		{
			name: "a cursor that stands still",
			answer: func(hit int, cursor string) feedResponse {
				return feedResponse{Events: stubEvents("A", 2), Complete: false, NextCursor: "org1.ev1"}
			},
			wantHits: 2, wantStored: 0, wantComplete: false,
			wantErr: "does not move forward",
		},
		{
			name: "incomplete, with nowhere to continue from",
			answer: func(hit int, cursor string) feedResponse {
				return feedResponse{Events: stubEvents("A", 2), Complete: false}
			},
			wantHits: 1, wantStored: 0, wantComplete: false,
			wantErr: "did not say where",
		},
		{
			name: "a cursor this node will not parse",
			answer: func(hit int, cursor string) feedResponse {
				return feedResponse{Events: stubEvents("A", 2), Complete: false, NextCursor: "../../etc/passwd"}
			},
			wantHits: 1, wantStored: 0, wantComplete: false,
			wantErr: "cursor",
		},
		{
			name: "a page bigger than this node asks for",
			answer: func(hit int, cursor string) feedResponse {
				return feedResponse{Events: stubEvents("A", maxFeedEvents+1), Complete: true}
			},
			wantHits: 1, wantStored: 0, wantComplete: false,
			wantErr: "listings in one page",
		},
		{
			name: "a peer that never terminates",
			answer: func(hit int, cursor string) feedResponse {
				// Always one more page, always a strictly larger cursor: perfectly
				// well-formed, and endless. Only the page bound stops this.
				return feedResponse{
					Events:     stubEvents(fmt.Sprintf("P%02d", hit), 2),
					Complete:   false,
					NextCursor: fmt.Sprintf("org1.ev%09d", hit),
				}
			},
			wantHits: maxFeedPages, wantStored: 2 * maxFeedPages, wantComplete: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newTestHarness(t)
			fx := h.newPublishedEvent(t, "stub-"+strings.ReplaceAll(c.name, " ", "-"))
			st := newFeedStub(t, c.answer)
			peerID := subscribeTo(t, h, fx.orgID, st)

			rec := h.do(http.MethodPost, "/api/sync/peers/"+peerID+"/feed", fx.ownerToken, nil)
			var out feedPullResult
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode pull result %q: %v", rec.Body.String(), err)
			}
			if got := int(st.hits.Load()); got != c.wantHits {
				t.Fatalf("the subscriber made %d request(s), want exactly %d", got, c.wantHits)
			}
			if out.Complete != c.wantComplete {
				t.Fatalf("pull reported complete=%v, want %v (%+v)", out.Complete, c.wantComplete, out)
			}
			if c.wantErr == "" && out.Error != "" {
				t.Fatalf("pull reported an error it should not have: %s", out.Error)
			}
			if c.wantErr != "" && !strings.Contains(out.Error, c.wantErr) {
				t.Fatalf("pull error is %q, want it to name %q", out.Error, c.wantErr)
			}
			rows, distinct := countCachedFor(t, h, peerID)
			if rows != c.wantStored || distinct != c.wantStored {
				t.Fatalf("cached %d rows (%d distinct), want %d", rows, distinct, c.wantStored)
			}
			// A refusal is durable, and it is a refusal rather than a silent
			// "showing 0 of 0".
			peer, err := h.store.GetSyncPeer(t.Context(), peerID)
			if err != nil {
				t.Fatalf("reload peer: %v", err)
			}
			if c.wantErr != "" && !strings.HasPrefix(peer.FeedStatus, "refused:") {
				t.Fatalf("a refused walk was recorded as %q", peer.FeedStatus)
			}
		})
	}
	if len(cases) != 7 {
		t.Fatalf("this test is meant to cover 7 publisher behaviours, covered %d", len(cases))
	}
}

// TestAMisbehavingPublisherCannotEmptyAGoodCache pins what happens to the
// listings an operator already had when a later walk goes wrong: they are KEPT.
//
// The cache is replaced wholesale, so writing a half-finished walk would silently
// delete listings the peer still publishes. A refusal must cost an operator the
// refresh, never the programme they were already showing.
func TestAMisbehavingPublisherCannotEmptyAGoodCache(t *testing.T) {
	var mode atomic.Int64 // 0 = honest, 1 = hostile
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "cache-keep")
	st := newFeedStub(t, func(hit int, cursor string) feedResponse {
		if mode.Load() == 0 {
			return feedResponse{Events: stubEvents("GOOD", 4), Complete: true}
		}
		if cursor == "" {
			return feedResponse{Events: stubEvents("BAD", 1), Complete: false, NextCursor: "org9.ev9"}
		}
		return feedResponse{Events: stubEvents("BAD", 1), Complete: false, NextCursor: "org0.ev0"}
	})
	peerID := subscribeTo(t, h, fx.orgID, st)

	if rec := h.do(http.MethodPost, "/api/sync/peers/"+peerID+"/feed", fx.ownerToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("the honest walk failed: %d %s", rec.Code, rec.Body.String())
	}
	if rows, _ := countCachedFor(t, h, peerID); rows != 4 {
		t.Fatalf("the honest walk cached %d rows, want 4", rows)
	}

	mode.Store(1)
	rec := h.do(http.MethodPost, "/api/sync/peers/"+peerID+"/feed", fx.ownerToken, nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("a walk that could not be finished answered %d, want a loud 502", rec.Code)
	}
	rows, distinct := countCachedFor(t, h, peerID)
	if rows != 4 || distinct != 4 {
		t.Fatalf("a refused walk left %d rows (%d distinct); the operator's good listings must survive", rows, distinct)
	}
	var titles int
	if err := h.store.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM peer_event WHERE peer_id = ? AND title LIKE 'Stub GOOD%'`,
		peerID).Scan(&titles); err != nil {
		t.Fatalf("count: %v", err)
	}
	if titles != 4 {
		t.Fatalf("%d of the 4 good listings survived the refused walk", titles)
	}
}
