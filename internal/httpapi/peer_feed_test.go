package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kotvasync "github.com/vul-os/kotva/bindings/go"

	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/scan/substrate"
	"github.com/vul-os/cackle/internal/store"
)

// Tests for the opt-in event feed between enrolled peers.
//
// The centrepiece, TestTwoNodesShareEventFeedsAndAttributeThem, runs two whole
// Cackle stacks on real sockets and drives them the way two organisers would:
// each enrols the other by hand, each answers the two consent questions
// separately, and only then does one node show the other's programme — attributed,
// linked back to the publisher's own box, and with no way to buy anything here.
//
// Everything else in this file is a refusal. That is the right proportion: the
// feature is one route that answers, and a great many things that must not.

// --- helpers ----------------------------------------------------------------

// enrolPeerOn enrols `key` on node n for n's org and returns the new peer's id.
func enrolPeerOn(t *testing.T, n *syncNode, name, url, key string) string {
	t.Helper()
	var view syncPeerView
	code := n.call(t, http.MethodPost, "/api/sync/peers", n.token, enrolPeerRequest{
		OrgID: n.org, Name: name, URL: url, PublicKey: key,
	}, &view)
	if code != http.StatusOK {
		t.Fatalf("enrol peer %s: status %d", name, code)
	}
	if view.FeedPublish || view.FeedSubscribe {
		t.Fatalf("a freshly enrolled peer must publish nothing and show nothing: %+v", view)
	}
	return view.ID
}

// setFeed answers the two consent questions for one peer.
func setFeed(t *testing.T, n *syncNode, peerID string, publish, subscribe bool) syncPeerView {
	t.Helper()
	var view syncPeerView
	code := n.call(t, http.MethodPut, "/api/sync/peers/"+peerID+"/feed", n.token,
		map[string]bool{"publish": publish, "subscribe": subscribe}, &view)
	if code != http.StatusOK {
		t.Fatalf("set feed switches (publish=%v subscribe=%v): status %d", publish, subscribe, code)
	}
	if view.FeedPublish != publish || view.FeedSubscribe != subscribe {
		t.Fatalf("switches did not take: wanted publish=%v subscribe=%v, got %+v", publish, subscribe, view)
	}
	return view
}

// newDraftEvent creates an event and leaves it a DRAFT. It exists so a test can
// prove a draft never reaches a peer.
func newDraftEvent(t *testing.T, h *testHarness, orgID, ownerToken, slug, title string) string {
	t.Helper()
	starts := time.Now().Add(60 * 24 * time.Hour)
	body := struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: orgID,
		CreateEventInput: events.CreateEventInput{
			Slug: slug, Title: title, VenueName: "Back Room", Address: "9 Draft Lane",
			StartsAt: starts, EndsAt: starts.Add(3 * time.Hour),
			Timezone: "Africa/Johannesburg", Currency: "ZAR",
		},
	}
	rec := h.do(http.MethodPost, "/api/events", ownerToken, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create draft event: %d %s", rec.Code, rec.Body.String())
	}
	created := decodeBody[struct {
		Event events.Event `json:"event"`
	}](t, rec)
	if created.Event.Status != "draft" {
		t.Fatalf("fixture is meant to be a draft, got %q", created.Event.Status)
	}
	return created.Event.ID
}

// --- the main event ----------------------------------------------------------

func TestTwoNodesShareEventFeedsAndAttributeThem(t *testing.T) {
	// Node A — the publisher. One published event and one draft.
	ha := newTestHarness(t)
	fxa := ha.newPublishedEvent(t, "feed-a")
	draftID := newDraftEvent(t, ha, fxa.orgID, fxa.ownerToken, "secret-rehearsal", "Secret Rehearsal")
	nodeA := newSyncNode(t, ha, fxa.orgID, fxa.ownerToken)

	// Node B — the subscriber. Its own org, its own event, its own identity.
	hb := newTestHarness(t)
	fxb := hb.newPublishedEvent(t, "feed-b")
	nodeB := newSyncNode(t, hb, fxb.orgID, fxb.ownerToken)

	if nodeA.key == nodeB.key {
		t.Fatal("two nodes must not share a replication identity")
	}

	// Each operator enrols the other, by hand, by key. Node A does NOT record
	// node B's address: it never dials B, it only answers when B dials in.
	peerOnA := enrolPeerOn(t, nodeA, "the other venue", "", nodeB.key)
	peerOnB := enrolPeerOn(t, nodeB, "the first venue", nodeA.url, nodeA.key)

	// ── 1. Enrolment alone grants nothing in either direction ────────────────

	var refused feedPullResult
	if code := nodeB.call(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil, &refused); code != http.StatusBadRequest {
		t.Fatalf("an un-subscribed peer must not be pulled: status %d", code)
	}
	// And node A refuses to serve its feed to a key it has not opted in for,
	// even though that key is fully enrolled for admission replication.
	beforeAnyConsent := peerRequestTo(t, nodeA, http.MethodGet, "/api/sync/feed", hb, nodeB)
	if beforeAnyConsent.StatusCode != http.StatusForbidden {
		t.Fatalf("node A published its programme to a peer whose operator never turned publishing on: %d",
			beforeAnyConsent.StatusCode)
	}
	beforeAnyConsent.Body.Close()

	// ── 2. Both operators say yes, separately ────────────────────────────────

	setFeed(t, nodeA, peerOnA, true, false) // A publishes to B; A shows nothing of B's
	setFeed(t, nodeB, peerOnB, false, true) // B shows A's; B publishes nothing to A

	var pull feedPullResult
	if code := nodeB.call(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil, &pull); code != http.StatusOK {
		t.Fatalf("feed pull: status %d (%+v)", code, pull)
	}
	if pull.Error != "" {
		t.Fatalf("feed pull reported an error: %s", pull.Error)
	}
	if pull.Fetched != 1 || pull.Stored != 1 {
		t.Fatalf("expected exactly node A's one PUBLISHED event, got fetched=%d stored=%d refused=%v",
			pull.Fetched, pull.Stored, pull.Refused)
	}
	if pull.Caveat == "" {
		t.Fatal("a feed pull must carry the sentence that says whose events these are")
	}

	// ── 3. What node B now shows ─────────────────────────────────────────────

	var shown peerEventsResponse
	if code := nodeB.call(t, http.MethodGet, "/api/peer-events?org="+nodeB.org, "", nil, &shown); code != http.StatusOK {
		t.Fatalf("read borrowed listings: status %d", code)
	}
	if len(shown.Events) != 1 {
		t.Fatalf("expected one borrowed listing, got %d", len(shown.Events))
	}
	got := shown.Events[0]

	if !got.External {
		t.Fatal("a borrowed listing must announce that it is somebody else's")
	}
	if got.PublisherKey != nodeA.key {
		t.Fatalf("listing attributed to %q, but it was fetched from %q", got.PublisherKey, nodeA.key)
	}
	if got.Publisher == "" {
		t.Fatal("a borrowed listing must name the organiser who is selling it")
	}
	if got.Notice == "" || !strings.Contains(strings.ToLower(got.Notice), "not here") {
		t.Fatalf("a borrowed listing must say the tickets are not sold here, got %q", got.Notice)
	}
	if got.Title != "Test Event feed-a" {
		t.Fatalf("borrowed the wrong event: %q", got.Title)
	}
	// The link points at the PUBLISHER's own box, at the publisher's own public
	// page, and it is built from the origin node B's operator typed.
	wantURL := nodeA.url + "/h/test-event-feed-a"
	if got.URL != wantURL {
		t.Fatalf("borrowed listing links to %q, want the publisher's own page %q", got.URL, wantURL)
	}
	if !strings.HasPrefix(got.URL, nodeA.url) {
		t.Fatalf("a borrowed listing must link to the publisher's origin, not this node's: %q", got.URL)
	}

	// The publisher's own page really is there and really is served by node A.
	pageResp, err := http.Get(got.URL) //nolint:noctx,gosec // test-local URL built above
	if err != nil {
		t.Fatalf("following a borrowed listing's link: %v", err)
	}
	defer pageResp.Body.Close()
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("a borrowed listing linked to a page that answers %d", pageResp.StatusCode)
	}

	// ── 4. The draft never left node A ───────────────────────────────────────

	for _, e := range shown.Events {
		if strings.Contains(strings.ToLower(e.Title), "rehearsal") {
			t.Fatalf("a DRAFT event reached a peer: %+v", e)
		}
	}
	var drafts int
	if err := hb.store.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM peer_event WHERE remote_event_id = ?`, draftID).Scan(&drafts); err != nil {
		t.Fatalf("count cached drafts: %v", err)
	}
	if drafts != 0 {
		t.Fatalf("node B cached %d rows for node A's draft", drafts)
	}

	// ── 5. Nothing here is for sale on node B ────────────────────────────────

	raw := nodeB.rawCall(t, http.MethodGet, "/api/peer-events?org="+nodeB.org, "", nil)
	for _, banned := range []string{"price", "price_minor", "ticket_type", "currency", "checkout", "order"} {
		if strings.Contains(strings.ToLower(string(raw)), `"`+banned) {
			t.Fatalf("a borrowed listing carried a %q field — this node cannot sell a peer's ticket: %s", banned, raw)
		}
	}

	// ── 6. Withdrawing consent removes the listings at once ──────────────────

	setFeed(t, nodeB, peerOnB, false, false)
	var afterRevoke peerEventsResponse
	if code := nodeB.call(t, http.MethodGet, "/api/peer-events?org="+nodeB.org, "", nil, &afterRevoke); code != http.StatusOK {
		t.Fatalf("read borrowed listings after revoke: status %d", code)
	}
	if len(afterRevoke.Events) != 0 {
		t.Fatalf("a peer whose subscription was withdrawn is still being shown: %+v", afterRevoke.Events)
	}
	var leftBehind int
	if err := hb.store.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM peer_event WHERE peer_id = ?`, peerOnB).Scan(&leftBehind); err != nil {
		t.Fatalf("count cached rows: %v", err)
	}
	if leftBehind != 0 {
		t.Fatalf("withdrawing a subscription left %d cached rows on disk", leftBehind)
	}

	// ── 7. And the publisher can withdraw too ────────────────────────────────

	setFeed(t, nodeB, peerOnB, false, true)  // B re-subscribes...
	setFeed(t, nodeA, peerOnA, false, false) // ...but A stops publishing to B.

	var afterPublisherStops feedPullResult
	code := nodeB.call(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil, &afterPublisherStops)
	if code != http.StatusBadGateway {
		t.Fatalf("a pull from a publisher that stopped publishing must be refused loudly, got %d", code)
	}
	raw = nodeB.rawCall(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil)
	if !strings.Contains(string(raw), "403") && !strings.Contains(strings.ToLower(string(raw)), "does not publish") {
		t.Fatalf("the refusal must name what happened, got %s", raw)
	}
	// The refusal is durable: an operator who looks later still sees it.
	peerAfter, err := hb.store.GetSyncPeer(t.Context(), peerOnB)
	if err != nil {
		t.Fatalf("reload peer: %v", err)
	}
	if !strings.HasPrefix(peerAfter.FeedStatus, "refused:") {
		t.Fatalf("a refused pull must be recorded as a refusal, got %q", peerAfter.FeedStatus)
	}
}

// peerRequestTo signs a request as `from`'s node and sends it over a real socket
// to `to`. It is the honest version of the in-process peerRequest helper for the
// cases where the two nodes are separate stacks.
func peerRequestTo(t *testing.T, to *syncNode, method, path string, fromHarness *testHarness, from *syncNode) *http.Response {
	t.Helper()
	id, err := fromHarness.store.NodeIdentity(t.Context())
	if err != nil {
		t.Fatalf("read %s's identity: %v", from.key, err)
	}
	signer, err := substrate.NewSigner(id.PrivateKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, to.url+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, err := substrate.SignRequest(req, signer, nil, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// --- the publish route's refusals -------------------------------------------

func TestPeerFeedRouteFailsClosed(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "feed-closed")

	// A key nobody ever enrolled.
	stranger := newTestSigner(t)
	// A key that IS enrolled for admission replication, with publishing off.
	enrolled := enrolledSigner(t, h, fx.orgID)

	checks := 0

	// 1. No signature at all.
	if rec := peerRequest(t, h, http.MethodGet, "/api/sync/feed", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned feed request answered %d, want 401", rec.Code)
	}
	checks++

	// 2. Signed by a key nobody pinned.
	if rec := peerRequest(t, h, http.MethodGet, "/api/sync/feed", stranger, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a stranger's signed feed request answered %d, want 401", rec.Code)
	}
	checks++

	// 3. Enrolled, but publishing was never turned on. THIS is the switch's own
	//    check, and it is separable from the authentication above: the key here
	//    passes authenticatePeer completely.
	if rec := peerRequest(t, h, http.MethodGet, "/api/sync/feed", enrolled, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("an enrolled peer with publishing OFF answered %d, want 403", rec.Code)
	}
	checks++

	// 4. Publishing on: the route answers, and the answer is SIGNED by this
	//    node's own key over the exact bytes it wrote.
	peers, err := h.store.EnabledSyncPeersByKey(t.Context(), hex.EncodeToString(enrolled.Public()))
	if err != nil || len(peers) != 1 {
		t.Fatalf("expected one enrolment for the test peer: %v %d", err, len(peers))
	}
	if err := h.store.SetSyncPeerFeed(t.Context(), peers[0].ID, true, false); err != nil {
		t.Fatalf("turn publishing on: %v", err)
	}
	rec := peerRequestSigned(t, h, enrolled)
	if rec.Code != http.StatusOK {
		t.Fatalf("an enrolled peer with publishing ON answered %d: %s", rec.Code, rec.Body.String())
	}
	checks++

	nodeID, err := h.store.NodeIdentity(t.Context())
	if err != nil {
		t.Fatalf("node identity: %v", err)
	}
	body := rec.Body.Bytes()
	nonce := rec.Header().Get(substrate.HeaderNonce)
	if err := substrate.VerifyResponse(rec.Header(), nodeID.PublicKey, nonce, body, time.Now()); err != nil {
		t.Fatalf("a feed answer must be verifiable against the publisher's pinned key: %v", err)
	}
	checks++

	// 5. One byte changed anywhere in the feed and the signature stops verifying.
	//    This is what "a subscriber can tell it was not altered in transit" means.
	tampered := append([]byte(nil), body...)
	tampered = []byte(strings.Replace(string(tampered), "Test Event feed-closed", "Test Event feed-closeD", 1))
	if string(tampered) == string(body) {
		t.Fatal("the tamper did not change anything; the test proves nothing")
	}
	if err := substrate.VerifyResponse(rec.Header(), nodeID.PublicKey, nonce, tampered, time.Now()); err == nil {
		t.Fatal("a tampered feed verified against the publisher's key")
	}
	checks++

	// 6. Signed by the RIGHT key but verified against the WRONG one.
	other := newTestSigner(t)
	if err := substrate.VerifyResponse(rec.Header(), hex.EncodeToString(other.Public()), nonce, body, time.Now()); err == nil {
		t.Fatal("a feed verified against a key that did not sign it")
	}
	checks++

	// 7. A disabled enrolment is not an enrolment. Publishing stays on in the
	//    row; the peer is refused anyway, at the authentication layer.
	if _, err := h.store.DB().ExecContext(t.Context(),
		`UPDATE sync_peer SET enabled = 0 WHERE id = ?`, peers[0].ID); err != nil {
		t.Fatalf("disable peer: %v", err)
	}
	if rec := peerRequest(t, h, http.MethodGet, "/api/sync/feed", enrolled, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a DISABLED peer with publishing on answered %d, want 401", rec.Code)
	}
	checks++

	// 8. The enrolled key's own header, with a signature that does not verify.
	//    Separate from checks 1 and 2 on purpose: those are refused by "no key"
	//    and "no pin", so neither of them reaches the signature check at all.
	//    Only this one proves the signature is checked rather than the header
	//    believed.
	if _, err := h.store.DB().ExecContext(t.Context(),
		`UPDATE sync_peer SET enabled = 1 WHERE id = ?`, peers[0].ID); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	forged := httptest.NewRequest(http.MethodGet, "/api/sync/feed", nil)
	forged.RemoteAddr = "198.51.100.11:5555"
	if _, err := substrate.SignRequest(forged, enrolled, nil, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	real := forged.Header.Get(substrate.HeaderSig)
	// Flip one hex digit of the signature and nothing else. The key header, the
	// timestamp and the nonce all stay exactly as a legitimate caller sent them.
	flipped := "0" + real[1:]
	if flipped == real {
		flipped = "1" + real[1:]
	}
	forged.Header.Set(substrate.HeaderSig, flipped)
	forgedRec := httptest.NewRecorder()
	h.handler.ServeHTTP(forgedRec, forged)
	if forgedRec.Code != http.StatusUnauthorized {
		t.Fatalf("a forged signature under an enrolled key answered %d, want 401", forgedRec.Code)
	}
	checks++

	// 9. A perfectly valid signature, minted outside the freshness window. A
	//    recorded request must not work forever.
	stale := httptest.NewRequest(http.MethodGet, "/api/sync/feed", nil)
	stale.RemoteAddr = "198.51.100.12:5555"
	if _, err := substrate.SignRequest(stale, enrolled, nil, time.Now().Add(-substrate.MaxClockSkew-time.Minute)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	staleRec := httptest.NewRecorder()
	h.handler.ServeHTTP(staleRec, stale)
	if staleRec.Code != http.StatusUnauthorized {
		t.Fatalf("a signed request from outside the clock-skew window answered %d, want 401", staleRec.Code)
	}
	checks++

	// 10. And a replay of a request that already worked. The nonce cache is what
	//     stops a captured, still-fresh request being used twice.
	replay := httptest.NewRequest(http.MethodGet, "/api/sync/feed", nil)
	replay.RemoteAddr = "198.51.100.13:5555"
	if _, err := substrate.SignRequest(replay, enrolled, nil, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	first := httptest.NewRecorder()
	h.handler.ServeHTTP(first, cloneReq(replay))
	if first.Code != http.StatusOK {
		t.Fatalf("the request being replayed must work the first time, got %d", first.Code)
	}
	second := httptest.NewRecorder()
	h.handler.ServeHTTP(second, cloneReq(replay))
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("a replayed feed request answered %d, want 401", second.Code)
	}
	checks++

	if checks != 11 {
		t.Fatalf("this test is meant to make 11 assertions, made %d", checks)
	}
}

// cloneReq returns a fresh request carrying the same signed envelope, so the
// same bytes can be sent twice.
func cloneReq(r *http.Request) *http.Request {
	out := httptest.NewRequest(r.Method, r.URL.String(), nil)
	out.RemoteAddr = r.RemoteAddr
	for k, v := range r.Header {
		out.Header[k] = v
	}
	return out
}

// peerRequestSigned is peerRequest with the recorder kept, for a case that needs
// to inspect the response headers as well as the code.
func peerRequestSigned(t *testing.T, h *testHarness, signer kotvasync.Signer) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sync/feed", nil)
	req.RemoteAddr = "198.51.100.9:5555"
	if _, err := substrate.SignRequest(req, signer, nil, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

// TestPeerFeedServesOnlyPublishedEvents pins the one WHERE clause that decides
// what another operator may see.
func TestPeerFeedServesOnlyPublishedEvents(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "feed-published-only")
	newDraftEvent(t, h, fx.orgID, fx.ownerToken, "unannounced-show", "Unannounced Show")

	// A cancelled event: published once, then called off. It must not be handed
	// to a peer either — a listing that sends a stranger to a cancelled show is
	// worse than no listing.
	cancelled := newDraftEvent(t, h, fx.orgID, fx.ownerToken, "called-off", "Called Off")
	if rec := h.do(http.MethodPost, "/api/events/"+cancelled+"/publish", fx.ownerToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("publish before cancelling: %d %s", rec.Code, rec.Body.String())
	}
	if err := h.store.SetEventStatus(t.Context(), cancelled, "cancelled", time.Now()); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	signer := enrolledSigner(t, h, fx.orgID)
	peers, err := h.store.EnabledSyncPeersByKey(t.Context(), hex.EncodeToString(signer.Public()))
	if err != nil || len(peers) != 1 {
		t.Fatalf("expected one enrolment: %v %d", err, len(peers))
	}
	if err := h.store.SetSyncPeerFeed(t.Context(), peers[0].ID, true, false); err != nil {
		t.Fatalf("turn publishing on: %v", err)
	}

	rec := peerRequestSigned(t, h, signer)
	if rec.Code != http.StatusOK {
		t.Fatalf("feed: %d %s", rec.Code, rec.Body.String())
	}
	var feed feedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if len(feed.Events) != 1 {
		t.Fatalf("expected exactly the one PUBLISHED event, got %d: %+v", len(feed.Events), feed.Events)
	}
	if feed.Events[0].Slug != "test-event-feed-published-only" {
		t.Fatalf("served the wrong event: %+v", feed.Events[0])
	}
	if strings.Contains(rec.Body.String(), "Unannounced") {
		t.Fatal("a draft leaked into a peer feed")
	}
	if strings.Contains(rec.Body.String(), "Called Off") {
		t.Fatal("a cancelled event leaked into a peer feed")
	}
}

// --- the subscribe side's refusals ------------------------------------------

// TestPullRefusesBeforeOpeningASocket is the "no network call to a host you did
// not consent to" proof. The peer's URL points at a server that COUNTS requests,
// so "the pull was refused" and "nothing was contacted" are two separate
// assertions rather than one hopeful one.
func TestPullRefusesBeforeOpeningASocket(t *testing.T) {
	var hits atomic.Int64
	spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer spy.Close()

	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "feed-nosocket")

	other := newTestSigner(t)
	p := &store.SyncPeer{
		OrgID: fx.orgID, Name: "spy", URL: spy.URL,
		PublicKey: hex.EncodeToString(other.Public()), Enabled: true,
	}
	if err := h.store.CreateSyncPeer(t.Context(), p); err != nil {
		t.Fatalf("enrol: %v", err)
	}

	cases := []struct {
		name  string
		setup func()
	}{
		{"subscription never turned on", func() {}},
		{"peer disabled", func() {
			if err := h.store.SetSyncPeerFeed(t.Context(), p.ID, false, true); err != nil {
				t.Fatalf("set feed: %v", err)
			}
			if _, err := h.store.DB().ExecContext(t.Context(),
				`UPDATE sync_peer SET enabled = 0 WHERE id = ?`, p.ID); err != nil {
				t.Fatalf("disable: %v", err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.setup()
			rec := h.do(http.MethodPost, "/api/sync/peers/"+p.ID+"/feed", fx.ownerToken, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("pull answered %d, want a 400 refusal: %s", rec.Code, rec.Body.String())
			}
			if n := hits.Load(); n != 0 {
				t.Fatalf("this node opened %d connection(s) to a host it had no consent to contact", n)
			}
		})
	}

	// And the positive control: with the subscription on and the peer enabled,
	// a socket IS opened — so the zero above means "refused", not "the spy
	// server was never reachable in the first place".
	if _, err := h.store.DB().ExecContext(t.Context(),
		`UPDATE sync_peer SET enabled = 1 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if err := h.store.SetSyncPeerFeed(t.Context(), p.ID, false, true); err != nil {
		t.Fatalf("set feed: %v", err)
	}
	rec := h.do(http.MethodPost, "/api/sync/peers/"+p.ID+"/feed", fx.ownerToken, nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("the spy is not a Cackle node, so the pull must fail: %d %s", rec.Code, rec.Body.String())
	}
	if hits.Load() == 0 {
		t.Fatal("the positive control never reached the spy, so the zeroes above prove nothing")
	}
}

// TestPullRefusesAFeedSignedByTheWrongKey is the pin discipline, on the feed
// route: the address answers, but under a key nobody enrolled.
func TestPullRefusesAFeedSignedByTheWrongKey(t *testing.T) {
	ha := newTestHarness(t)
	fxa := ha.newPublishedEvent(t, "feed-wrongkey-a")
	nodeA := newSyncNode(t, ha, fxa.orgID, fxa.ownerToken)

	hb := newTestHarness(t)
	fxb := hb.newPublishedEvent(t, "feed-wrongkey-b")
	nodeB := newSyncNode(t, hb, fxb.orgID, fxb.ownerToken)

	// Node A is entirely willing to serve node B: B's key is enrolled there and
	// publishing is on. So node A ANSWERS, with a well-formed feed carrying a
	// real event, signed by node A's own real key. Everything on the publisher's
	// side is correct — which is the only way to prove the refusal below comes
	// from the response-signature check and not from something upstream of it.
	peerOnA := enrolPeerOn(t, nodeA, "the subscriber", "", nodeB.key)
	setFeed(t, nodeA, peerOnA, true, false)

	// Node B, however, pinned the WRONG key for that address — the shape an
	// impostor at a known address has, and the shape a node whose key rotated
	// has too. Node B cannot tell those apart, and must refuse both.
	impostor := newTestSigner(t)
	peerOnB := enrolPeerOn(t, nodeB, "mislabelled", nodeA.url, hex.EncodeToString(impostor.Public()))
	setFeed(t, nodeB, peerOnB, false, true)

	var out feedPullResult
	code := nodeB.call(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil, &out)
	if code != http.StatusBadGateway {
		t.Fatalf("a feed from an address answering under an unpinned key answered %d: %+v", code, out)
	}
	// The refusal must be the PIN's, naming both keys — not a 401 from the far
	// side, which would mean node A never answered and this test proved nothing
	// about verification.
	raw := string(nodeB.rawCall(t, http.MethodPost, "/api/sync/peers/"+peerOnB+"/feed", nodeB.token, nil))
	if !strings.Contains(raw, "is pinned") {
		t.Fatalf("the refusal did not come from the response-signature check: %s", raw)
	}
	var cached int
	if err := hb.store.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM peer_event WHERE peer_id = ?`, peerOnB).Scan(&cached); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cached != 0 {
		t.Fatalf("%d listings were cached from a node that could not prove who it was", cached)
	}
	// The pin is unchanged. A key that stopped matching is a refusal, never a
	// re-pin, and the feed route does not get its own weaker rule.
	after, err := hb.store.GetSyncPeer(t.Context(), peerOnB)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.PublicKey != hex.EncodeToString(impostor.Public()) {
		t.Fatalf("the pin was rewritten to %q", after.PublicKey)
	}
}

// --- the link is built here, never taken from the answer ---------------------

func TestPeerEventLinkIsComposedLocally(t *testing.T) {
	peer := store.SyncPeer{ID: "peer-1", OrgID: "org-1", PublicKey: "aa", URL: "https://venue.example"}
	now := time.Now()
	starts := now.Add(24 * time.Hour)

	good := feedEventWire{
		EventID: "ev-1", Slug: "friday-night", Publisher: "The Other Venue",
		Title: "Friday Night", StartsAt: starts, EndsAt: starts.Add(3 * time.Hour),
	}
	row, err := peerEventFromWire(peer, good, now)
	if err != nil {
		t.Fatalf("a well-formed listing was refused: %v", err)
	}
	if row.URL != "https://venue.example/h/friday-night" {
		t.Fatalf("link is %q, want the publisher's own /h/ page", row.URL)
	}
	if row.PublisherKey != "aa" || row.OrgID != "org-1" || row.PeerID != "peer-1" {
		t.Fatalf("a listing must be bound to the peer it came from: %+v", row)
	}

	// Everything a hostile publisher would try in the one field that becomes
	// part of a URL. Each must be refused WHOLE — no sanitising, no salvaging.
	hostile := []string{
		"https://evil.example/phish",
		"//evil.example",
		"../../../etc/passwd",
		"friday-night?utm=1",
		"friday-night#x",
		"friday night",
		"Friday-Night",
		"friday-night/../../evil",
		"%2e%2e%2fevil",
		"friday-night\n\rSet-Cookie: x=1",
		"-leading",
		"trailing-",
		"",
	}
	for _, slug := range hostile {
		if _, err := peerEventFromWire(peer, feedEventWire{
			EventID: "ev-2", Slug: slug, Title: "x", StartsAt: starts,
		}, now); err == nil {
			t.Fatalf("slug %q was accepted into a link", slug)
		}
	}
	if len(hostile) != 13 {
		t.Fatalf("this test is meant to try 13 hostile slugs, tried %d", len(hostile))
	}

	// A listing with no title, no id or no start time is not a listing.
	for name, w := range map[string]feedEventWire{
		"no id":    {Slug: "a", Title: "x", StartsAt: starts},
		"no title": {EventID: "ev", Slug: "a", StartsAt: starts},
		"no start": {EventID: "ev", Slug: "a", Title: "x"},
	} {
		if _, err := peerEventFromWire(peer, w, now); err == nil {
			t.Fatalf("a listing with %s was accepted", name)
		}
	}

	// Untrusted display text is length-bounded.
	long := strings.Repeat("A", maxFeedFieldLen*3)
	row, err = peerEventFromWire(peer, feedEventWire{
		EventID: "ev-3", Slug: "ok", Title: long, Summary: long, Publisher: long, StartsAt: starts,
	}, now)
	if err != nil {
		t.Fatalf("a long-but-valid listing was refused: %v", err)
	}
	for field, v := range map[string]string{"title": row.Title, "summary": row.Summary, "publisher": row.PublisherName} {
		if len(v) > maxFeedFieldLen+len("…") {
			t.Fatalf("%s is %d bytes: untrusted text from another machine is unbounded", field, len(v))
		}
	}
}

// --- consent is re-checked on every read -------------------------------------

// TestCachedListingsDieWithTheConsentThatFetchedThem proves the read path does
// not trust the cache on its own. Rows are written directly here, so the only
// thing under test is whether a reader will serve them once consent is gone.
func TestCachedListingsDieWithTheConsentThatFetchedThem(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "feed-consent")

	other := newTestSigner(t)
	p := &store.SyncPeer{
		OrgID: fx.orgID, Name: "neighbour", URL: "https://neighbour.example",
		PublicKey: hex.EncodeToString(other.Public()), Enabled: true,
	}
	if err := h.store.CreateSyncPeer(t.Context(), p); err != nil {
		t.Fatalf("enrol: %v", err)
	}
	if err := h.store.SetSyncPeerFeed(t.Context(), p.ID, false, true); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	starts := time.Now().Add(48 * time.Hour)
	if err := h.store.ReplacePeerEvents(t.Context(), p.ID, []store.PeerEvent{{
		OrgID: fx.orgID, PublisherKey: p.PublicKey, PublisherName: "Neighbour Hall",
		RemoteEventID: "remote-1", Slug: "spring-fair", URL: p.URL + "/h/spring-fair",
		Title: "Spring Fair", StartsAt: starts, EndsAt: starts.Add(6 * time.Hour),
	}}); err != nil {
		t.Fatalf("cache: %v", err)
	}

	read := func() int {
		rows, err := h.store.ListPeerEventsForOrg(t.Context(), fx.orgID)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		return len(rows)
	}
	if read() != 1 {
		t.Fatal("a subscribed, enabled peer's listing should be readable")
	}

	// Consent withdrawn in the row only — the cache is deliberately left in
	// place, so what is being tested is the READ, not the delete.
	if _, err := h.store.DB().ExecContext(t.Context(),
		`UPDATE sync_peer SET feed_subscribe = 0 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if n := read(); n != 0 {
		t.Fatalf("%d listings survived the subscription being withdrawn", n)
	}

	if _, err := h.store.DB().ExecContext(t.Context(),
		`UPDATE sync_peer SET feed_subscribe = 1, enabled = 0 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if n := read(); n != 0 {
		t.Fatalf("%d listings survived the peer being disabled", n)
	}

	// Deleting the enrolment takes the cache with it, at the database level.
	if _, err := h.store.DB().ExecContext(t.Context(),
		`UPDATE sync_peer SET enabled = 1 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if err := h.store.DeleteSyncPeer(t.Context(), p.ID); err != nil {
		t.Fatalf("delete peer: %v", err)
	}
	var left int
	if err := h.store.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM peer_event WHERE peer_id = ?`, p.ID).Scan(&left); err != nil {
		t.Fatalf("count: %v", err)
	}
	if left != 0 {
		t.Fatalf("deleting an enrolment left %d borrowed listings behind", left)
	}
}

// TestFeedSwitchesRequireBothAnswers pins the shape of the consent call.
func TestFeedSwitchesRequireBothAnswers(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "feed-switches")
	other := newTestSigner(t)
	p := &store.SyncPeer{
		OrgID: fx.orgID, Name: "n", PublicKey: hex.EncodeToString(other.Public()), Enabled: true,
	}
	if err := h.store.CreateSyncPeer(t.Context(), p); err != nil {
		t.Fatalf("enrol: %v", err)
	}

	for _, body := range []any{
		map[string]bool{"publish": true},
		map[string]bool{"subscribe": true},
		map[string]any{},
	} {
		rec := h.do(http.MethodPut, "/api/sync/peers/"+p.ID+"/feed", fx.ownerToken, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("a half-answered consent call was accepted (%v): %d", body, rec.Code)
		}
	}

	// Subscribing to a peer with no address is refused: a switch that can never
	// do anything reads to an operator as a working subscription.
	rec := h.do(http.MethodPut, "/api/sync/peers/"+p.ID+"/feed", fx.ownerToken,
		map[string]bool{"publish": false, "subscribe": true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("subscribing to an unreachable peer was accepted: %d %s", rec.Code, rec.Body.String())
	}

	// A non-owner cannot touch either switch, and learns nothing by trying.
	strangerToken, _ := h.signupUser("stranger-feed@example.com", "stranger-password-1", "Stranger")
	rec = h.do(http.MethodPut, "/api/sync/peers/"+p.ID+"/feed", strangerToken,
		map[string]bool{"publish": true, "subscribe": false})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a stranger got %d rather than a flat not-found", rec.Code)
	}
	rec = h.do(http.MethodPost, "/api/sync/peers/"+p.ID+"/feed", strangerToken, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a stranger could trigger a pull: %d", rec.Code)
	}
	rec = h.do(http.MethodPut, "/api/sync/peers/"+p.ID+"/feed", "",
		map[string]bool{"publish": true, "subscribe": false})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller got %d rather than 401", rec.Code)
	}

	// Nothing above moved a switch.
	after, err := h.store.GetSyncPeer(t.Context(), p.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.FeedPublish || after.FeedSubscribe {
		t.Fatalf("a refused call still changed the switches: %+v", after)
	}
}
