package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vul-os/cackle/internal/store"
)

// Opt-in event feeds between Cackle nodes.
//
// # What this is
//
// An organiser running their own box can let ANOTHER OPERATOR THEY CHOSE see
// their published events, and can choose to show that operator's events beside
// their own. Both directions are per-peer and both default to off, so enrolling
// a peer to reconcile door scans (migration 0003) grants neither.
//
// It rides the peer channel that already exists. There is no second transport,
// no second credential and no second trust model: the publish route is
// authenticated by the same signed envelope every /api/sync route uses
// (internal/scan/substrate/peerauth.go), the answer is signed with the same node
// key, and a subscriber verifies that answer against the key its operator
// PINNED — so a listing this node shows is one it can prove came from the
// operator it thinks it did.
//
// # What this is NOT, and will not become
//
// There is no discovery. No directory, no index, no rendezvous, no DHT, no
// "organisers near you", no global feed, nothing that finds a peer for you.
// Cackle contacts an address a human typed and no other. That is not a stage
// this feature is at — it is the design, and the substrate underneath it has no
// discovery mechanism to grow into either.
//
// # A peer's event is never sold here
//
// A borrowed listing carries a title, a time, a place, a publisher and a link.
// It carries no price, no ticket type and no order path, because a ticket for
// somebody else's event is issued by SOMEBODY ELSE'S KEY on SOMEBODY ELSE'S BOX.
// Cackle has no cross-node checkout, and the wire shape here is deliberately
// missing every field one could be built out of. The link goes to the
// publisher's own /h/ page, and it is composed on THIS node out of the origin an
// operator typed — never copied out of the publisher's answer, so a publisher
// cannot point a listing that borrows a neighbour's trust at a site nobody
// enrolled.
//
// # Fail closed
//
// A feed that cannot be verified, that arrives from a key nobody pinned, that is
// stale beyond the peer-auth clock skew, or that comes from a peer whose switch
// is off is REFUSED and reported. Nothing is shown on a "well, probably" — and a
// peer that has not been enrolled is never contacted at all, because the refusal
// happens before the request is built.

const (
	// maxFeedEvents bounds one feed answer. A publisher with more published
	// events than this serves the soonest ones and says so; the alternative is
	// an unbounded response an authenticated peer can ask for repeatedly.
	maxFeedEvents = 200

	// maxFeedBody caps a feed answer before it is read. Smaller than
	// maxSyncBody: a feed is text about events, not a batch of signed envelopes.
	maxFeedBody = 512 << 10

	// maxFeedFieldLen caps every display string copied out of another operator's
	// machine. Untrusted text with no length bound is a way to fill this node's
	// disk through a route that is otherwise perfectly authenticated.
	maxFeedFieldLen = 500
)

// feedCaveat travels on every feed answer and every borrowed listing this node
// hands to its own UI. It is the sentence that keeps a peer's event from
// reading as this host's stock.
const feedCaveat = "These events belong to another organiser and are hosted on their own Cackle box. " +
	"Tickets for them are sold there, by them — this node cannot sell them, does not hold them, and " +
	"reports only what that operator published the last time it was asked. Cackle finds no peers on " +
	"its own: every peer here was enrolled by hand, by key."

// --- wire shapes -------------------------------------------------------------

// feedEventWire is one published event as a publisher describes it.
//
// Slug rather than a URL, deliberately. The subscriber builds the link from the
// origin its own operator typed plus this slug, so the worst a hostile publisher
// can do with this field is name an event of its own that does not exist.
type feedEventWire struct {
	EventID string `json:"event_id"`
	Slug    string `json:"slug"`
	// Publisher is the publishing organisation's display name. It is UNTRUSTED
	// text for display; the pinned node key is the identity, and it is the key a
	// subscriber stores alongside every row.
	Publisher string    `json:"publisher"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	VenueName string    `json:"venue_name,omitempty"`
	Address   string    `json:"address,omitempty"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Timezone  string    `json:"timezone,omitempty"`
	Category  string    `json:"category,omitempty"`
}

type feedResponse struct {
	// Node is the answering node's public key — the value the caller pinned, and
	// the value the caller's VerifyResponse has already checked the signature
	// against by the time this is decoded.
	Node   string          `json:"node"`
	Events []feedEventWire `json:"events"`
	// Complete is false when the publisher had more published events than one
	// answer carries. It is reported rather than paged: a programme is a display
	// surface, not a ledger, and an operator who wants all of a very large one
	// is better served by knowing it was truncated.
	Complete bool   `json:"complete"`
	Caveat   string `json:"caveat"`
}

// --- publish: GET /api/sync/feed --------------------------------------------

// handlePeerFeed serves this node's PUBLISHED events to an enrolled peer whose
// operator has turned publishing on for that key.
//
// Two independent gates, and the second is the new one: the caller must
// authenticate as an enrolled peer (authenticatePeer, exactly as every other
// peer route does), AND at least one of that key's enrolments must have
// feed_publish set. An enrolment on its own is not consent to be listed
// elsewhere.
func (s *server) handlePeerFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, ok := readCappedBody(w, r)
	if !ok {
		return
	}
	peers, ok := s.authenticatePeer(w, r, body)
	if !ok {
		return
	}

	// Only the enrolments that opted in. A key enrolled for two organisations
	// where one publishes and the other does not sees exactly the one.
	publishing := make([]store.SyncPeer, 0, len(peers))
	for _, p := range peers {
		if p.FeedPublish {
			publishing = append(publishing, p)
		}
	}
	if len(publishing) == 0 {
		s.log().Warn("feed: refused a peer whose operator has not turned publishing on",
			"key", peers[0].PublicKey, "remote_ip", clientIP(r))
		forbidden(w, "this node does not publish its event feed to your key")
		return
	}

	signer, id, err := s.syncIdentity(ctx)
	if err != nil {
		internalError(w, s.log(), "feed identity", err)
		return
	}

	resp := feedResponse{Node: id.PublicKey, Events: make([]feedEventWire, 0), Complete: true, Caveat: feedCaveat}
	for _, p := range publishing {
		org, err := s.deps.Store.GetOrgByID(ctx, p.OrgID)
		if err != nil {
			internalError(w, s.log(), "feed org", err)
			return
		}
		// One over the bound, so "the list ends here" and "the page is full" are
		// distinguishable rather than guessed — the same reason handleSyncPull
		// reads limit+1.
		evs, err := s.deps.Store.PublishedEventsForOrg(ctx, p.OrgID, maxFeedEvents+1)
		if err != nil {
			internalError(w, s.log(), "feed events", err)
			return
		}
		if len(evs) > maxFeedEvents {
			evs = evs[:maxFeedEvents]
			resp.Complete = false
		}
		for _, ev := range evs {
			resp.Events = append(resp.Events, feedEventWire{
				EventID:   ev.ID,
				Slug:      ev.Slug,
				Publisher: org.Name,
				Title:     ev.Title,
				Summary:   ev.Summary,
				VenueName: ev.VenueName,
				Address:   ev.Address,
				StartsAt:  ev.StartsAt,
				EndsAt:    ev.EndsAt,
				Timezone:  ev.Timezone,
				Category:  ev.Category,
			})
		}
	}
	s.writeSignedJSON(w, r, signer, http.StatusOK, resp)
}

// --- subscribe: the operator's three routes ---------------------------------

// feedSwitchRequest is an operator's answer to the two questions. Both are
// required and both are written: a body that omits one is refused rather than
// treated as "leave it alone", because a consent switch that can be left at a
// value the caller did not state is one a stale UI can hold open.
type feedSwitchRequest struct {
	Publish   *bool `json:"publish"`
	Subscribe *bool `json:"subscribe"`
}

// handleSetPeerFeed serves PUT /api/sync/peers/{id}/feed.
//
// Turning `subscribe` OFF deletes what was cached from that peer in the same
// call. Withdrawing consent has to remove the listings, not merely stop
// refreshing them — a peer an operator has stopped showing must stop being shown
// now, not at some next pull that may never happen.
func (s *server) handleSetPeerFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.syncPeerForOperator(w, r)
	if !ok {
		return
	}
	var req feedSwitchRequest
	if err := decodeJSONBody(r, 8<<10, &req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Publish == nil || req.Subscribe == nil {
		badRequest(w, "publish and subscribe are both required: say yes or no to each direction")
		return
	}
	if *req.Subscribe && p.URL == "" {
		// Nothing to dial. Accepting this would leave a switch on that can never
		// do anything, which reads to an operator as a working subscription.
		badRequest(w, "this peer has no address, so this node cannot fetch its events; "+
			"enrol it again with its URL to subscribe")
		return
	}
	if err := s.deps.Store.SetSyncPeerFeed(ctx, p.ID, *req.Publish, *req.Subscribe); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "peer not found")
			return
		}
		internalError(w, s.log(), "feed switches", err)
		return
	}
	if !*req.Subscribe {
		if err := s.deps.Store.DeletePeerEventsForPeer(ctx, p.ID); err != nil {
			internalError(w, s.log(), "feed revoke cache", err)
			return
		}
	}
	updated, err := s.deps.Store.GetSyncPeer(ctx, p.ID)
	if err != nil {
		internalError(w, s.log(), "feed reload peer", err)
		return
	}
	writeJSON(w, http.StatusOK, syncPeerToView(updated))
}

// feedPullResult is what a triggered feed pull reports back.
type feedPullResult struct {
	PeerID string `json:"peer_id"`
	Peer   string `json:"peer"`
	// Fetched is how many listings the peer offered; Stored how many survived
	// this node's own validation and were cached. They differ when a publisher
	// sent something malformed, and Refused names why.
	Fetched  int      `json:"fetched"`
	Stored   int      `json:"stored"`
	Refused  []string `json:"refused,omitempty"`
	Complete bool     `json:"complete"`
	Error    string   `json:"error,omitempty"`
	Caveat   string   `json:"caveat"`
}

// handlePullPeerFeed serves POST /api/sync/peers/{id}/feed: fetch this peer's
// published events now.
//
// Triggered, never polled — the same discipline as a replication round. A
// deployment that never calls this never opens a socket, which is what makes
// "Cackle contacts nothing you did not enrol" true rather than aspirational.
//
// Every refusal below happens BEFORE any network call: a peer that is disabled,
// has no address, or whose subscribe switch is off is refused without a request
// being built, so an un-consented host is never contacted even once.
func (s *server) handlePullPeerFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.syncPeerForOperator(w, r)
	if !ok {
		return
	}
	if !p.Enabled {
		badRequest(w, "this peer is disabled")
		return
	}
	if p.URL == "" {
		badRequest(w, "this peer has no address: this node cannot fetch anything from it")
		return
	}
	if !p.FeedSubscribe {
		badRequest(w, "this node is not subscribed to that peer's events; turn the subscription on first")
		return
	}

	out := s.pullFeedFromPeer(ctx, p)
	status := http.StatusOK
	if out.Error != "" {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, out)
}

// pullFeedFromPeer performs one feed fetch and replaces the cache.
//
// The peer's answer is verified against the PINNED key by callPeer before a byte
// of it is decoded, so a node answering at the right address under a different
// key gets nothing cached and nothing re-pinned. Everything after that is this
// node distrusting the CONTENT of an answer it has authenticated: the sender is
// who it claims, which says nothing about whether what it sent is well-formed.
func (s *server) pullFeedFromPeer(ctx context.Context, p store.SyncPeer) feedPullResult {
	out := feedPullResult{PeerID: p.ID, Peer: p.PublicKey, Complete: true, Caveat: feedCaveat}

	signer, _, err := s.syncIdentity(ctx)
	if err != nil {
		out.Error = "this node has no usable replication identity"
		s.log().Error("feed: identity unavailable", "peer_id", p.ID, "err", err)
		return out
	}

	client := &http.Client{
		Timeout: peerRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("peer replied with a redirect; a peer URL must be the node's own origin")
		},
	}

	var resp feedResponse
	if err := s.callPeer(ctx, client, p, signer, http.MethodGet, "/api/sync/feed", nil, nil, &resp); err != nil {
		out.Error = err.Error()
		s.recordFeedOutcome(ctx, p, out)
		return out
	}
	out.Complete = resp.Complete
	out.Fetched = len(resp.Events)

	rows := make([]store.PeerEvent, 0, len(resp.Events))
	seen := make(map[string]struct{}, len(resp.Events))
	now := time.Now()
	for _, e := range resp.Events {
		row, err := peerEventFromWire(p, e, now)
		if err != nil {
			out.Refused = append(out.Refused, err.Error())
			continue
		}
		// The unique index would catch this, but as a 500 rather than as a
		// sentence. A publisher repeating an id is a malformed answer, not a
		// database fault.
		if _, dup := seen[row.RemoteEventID]; dup {
			out.Refused = append(out.Refused, "peer listed event "+row.RemoteEventID+" twice")
			continue
		}
		seen[row.RemoteEventID] = struct{}{}
		rows = append(rows, row)
	}

	if err := s.deps.Store.ReplacePeerEvents(ctx, p.ID, rows); err != nil {
		out.Error = "could not store this peer's events"
		s.log().Error("feed: storing peer events", "peer_id", p.ID, "err", err)
		s.recordFeedOutcome(ctx, p, out)
		return out
	}
	out.Stored = len(rows)
	s.recordFeedOutcome(ctx, p, out)
	return out
}

func (s *server) recordFeedOutcome(ctx context.Context, p store.SyncPeer, out feedPullResult) {
	status := fmt.Sprintf("showing %d of %d listings", out.Stored, out.Fetched)
	if n := len(out.Refused); n > 0 {
		status += fmt.Sprintf(", refused %d", n)
	}
	if !out.Complete {
		status += " (peer had more than this node asks for)"
	}
	if out.Error != "" {
		status = "refused: " + out.Error
	}
	if err := s.deps.Store.SetSyncPeerFeedStatus(ctx, p.ID, time.Now(), status); err != nil {
		s.log().Error("feed: could not record pull outcome", "peer_id", p.ID, "err", err)
	}
}

// peerEventFromWire validates one listing and composes its link locally.
//
// This is where an authenticated answer stops being trusted. The signature
// proved WHO sent it; nothing about it proves the contents are usable, and the
// two things a hostile publisher would most like to control are the link and the
// length of the text. It gets neither: the URL is built from the origin an
// operator typed plus a slug matched against a strict pattern, and every display
// string is truncated.
func peerEventFromWire(p store.SyncPeer, e feedEventWire, now time.Time) (store.PeerEvent, error) {
	id := strings.TrimSpace(e.EventID)
	if id == "" {
		return store.PeerEvent{}, errors.New("a listing arrived with no event id")
	}
	if len(id) > 64 {
		return store.PeerEvent{}, errors.New("a listing arrived with an implausible event id")
	}
	title := strings.TrimSpace(e.Title)
	if title == "" {
		return store.PeerEvent{}, errors.New("listing " + id + " has no title")
	}
	slug, err := feedSlug(e.Slug)
	if err != nil {
		return store.PeerEvent{}, fmt.Errorf("listing %s: %w", id, err)
	}
	if e.StartsAt.IsZero() {
		return store.PeerEvent{}, errors.New("listing " + id + " has no start time")
	}
	ends := e.EndsAt
	if ends.IsZero() {
		ends = e.StartsAt
	}
	return store.PeerEvent{
		PeerID:        p.ID,
		OrgID:         p.OrgID,
		PublisherKey:  p.PublicKey,
		PublisherName: clampField(e.Publisher),
		RemoteEventID: id,
		Slug:          slug,
		// The link, composed here. `p.URL` was validated at enrolment
		// (normalizePeerURL) down to a scheme and a host with no path, no query
		// and no credentials, and `slug` has just been matched against
		// feedSlugPattern — so this string cannot be steered anywhere by the
		// answer that prompted it.
		URL:       p.URL + "/h/" + slug,
		Title:     clampField(title),
		Summary:   clampField(e.Summary),
		VenueName: clampField(e.VenueName),
		Address:   clampField(e.Address),
		StartsAt:  e.StartsAt,
		EndsAt:    ends,
		Timezone:  clampField(e.Timezone),
		Category:  clampField(e.Category),
		FetchedAt: now,
	}, nil
}

// feedSlug accepts only what an event slug can be on the publisher's own node:
// lower-case letters, digits and hyphens.
//
// Written as an allow-list rather than as a search for "../" or "://", because a
// deny-list on a string that becomes part of a URL is a game you lose eventually.
// Anything with a scheme, a host, a path segment, a query, a fragment, an
// encoded byte or a control character fails to match and is refused whole.
func feedSlug(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("no slug, so this node cannot link to it on the publisher's own site")
	}
	if len(s) > 120 {
		return "", errors.New("slug is implausibly long")
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			return "", fmt.Errorf("slug %q is not a plain lower-case slug, so this node will not build a link out of it", clampField(s))
		}
	}
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return "", errors.New("slug starts or ends with a hyphen")
	}
	return s, nil
}

// clampField bounds one untrusted display string.
func clampField(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxFeedFieldLen {
		return s
	}
	return s[:maxFeedFieldLen] + "…"
}

// --- reading what was borrowed ----------------------------------------------

// peerEventView is one borrowed listing as this node's own UI receives it.
//
// The three fields that are not data — External, Publisher and Notice — are the
// contract with every renderer: a listing that reaches a screen carries, in the
// same object, the fact that it is somebody else's and the sentence that says
// so. There is no price and no ticket type anywhere in this shape, so no UI can
// accidentally draw a buy button that would take money for a ticket this node
// cannot issue.
type peerEventView struct {
	// External is always true. It is sent anyway, and it is sent first, because
	// a renderer that forgets to check it is a renderer that will make somebody
	// else's show look like this host's stock.
	External bool `json:"external"`
	// Publisher is the publishing organiser's display name (untrusted text) and
	// PublisherKey the pinned node key it was fetched from (the identity).
	Publisher    string `json:"publisher"`
	PublisherKey string `json:"publisher_key"`
	// URL is the publisher's own page, on the publisher's own box. It is where
	// tickets are bought, and it is the ONLY action a borrowed listing offers.
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	VenueName string    `json:"venue_name,omitempty"`
	Address   string    `json:"address,omitempty"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Timezone  string    `json:"timezone,omitempty"`
	Category  string    `json:"category,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	// Notice is the plain-language sentence a renderer must show beside the
	// listing. It travels with the row rather than living in the frontend so it
	// cannot be dropped by a redesign.
	Notice string `json:"notice"`
}

// peerEventNotice is the per-listing sentence. Short, plain, and it names the
// one thing a buyer needs to know.
const peerEventNotice = "Hosted by another organiser. Tickets are sold on their own site, not here."

func peerEventToView(e store.PeerEvent) peerEventView {
	return peerEventView{
		External:     true,
		Publisher:    e.PublisherName,
		PublisherKey: e.PublisherKey,
		URL:          e.URL,
		Title:        e.Title,
		Summary:      e.Summary,
		VenueName:    e.VenueName,
		Address:      e.Address,
		StartsAt:     e.StartsAt,
		EndsAt:       e.EndsAt,
		Timezone:     e.Timezone,
		Category:     e.Category,
		FetchedAt:    e.FetchedAt,
		Notice:       peerEventNotice,
	}
}

type peerEventsResponse struct {
	Events []peerEventView `json:"events"`
	Caveat string          `json:"caveat"`
}

// handleListPeerEventsForOrg serves GET /api/peer-events?org=<id>: the listings
// one organisation currently borrows.
//
// Public, because every row in it is an event its own publisher already made
// public on their own site — this route shows a copy of something anyone could
// already read, and hiding it behind auth would not make it less public. It
// makes no network call: it reads this node's cache, which is only ever filled
// by an operator triggering a pull against a peer they enrolled.
func (s *server) handleListPeerEventsForOrg(w http.ResponseWriter, r *http.Request) {
	orgID := strings.TrimSpace(r.URL.Query().Get("org"))
	if orgID == "" {
		badRequest(w, "org is required")
		return
	}
	rows, err := s.deps.Store.ListPeerEventsForOrg(r.Context(), orgID)
	if err != nil {
		internalError(w, s.log(), "peer events", err)
		return
	}
	writeJSON(w, http.StatusOK, peerEventsToResponse(rows))
}

// handleListPeerFeedCache serves GET /api/sync/peers/{id}/feed: what one peer's
// last pull actually brought back, for the operator screen that has to show it.
func (s *server) handleListPeerFeedCache(w http.ResponseWriter, r *http.Request) {
	p, ok := s.syncPeerForOperator(w, r)
	if !ok {
		return
	}
	rows, err := s.deps.Store.ListPeerEventsForPeer(r.Context(), p.ID)
	if err != nil {
		internalError(w, s.log(), "peer feed cache", err)
		return
	}
	writeJSON(w, http.StatusOK, peerEventsToResponse(rows))
}

func peerEventsToResponse(rows []store.PeerEvent) peerEventsResponse {
	out := peerEventsResponse{Events: make([]peerEventView, 0, len(rows)), Caveat: feedCaveat}
	for _, e := range rows {
		out.Events = append(out.Events, peerEventToView(e))
	}
	return out
}

// decodeJSONBody reads a bounded JSON body. Bounded because every other body in
// this package is, and an operator route being owner-only is not a reason to let
// it buffer without limit.
func decodeJSONBody(r *http.Request, limit int64, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}
