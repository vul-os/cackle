package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	kotvasync "github.com/vul-os/kotva/bindings/go"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/scan"
	"github.com/vul-os/cackle/internal/scan/substrate"
	"github.com/vul-os/cackle/internal/store"
)

// Server-to-server replication of the admission ledger: the HTTP surface.
//
// Two Cackle instances used to exchange nothing at all. The ledger converged
// between GATES and a server and stopped there, so an organiser running a venue
// node and a cloud node had two databases that never learned about each other.
// These routes are that missing path, and docs/CLUSTERING.md is the operator's
// version of this comment.
//
// # What replication does and does not change
//
// It makes a cross-gate double admission VISIBLE SOONER and on MORE NODES. It
// cannot prevent one. The gates that produced the duplicate could not see each
// other at the moment of the scan; nothing that happens afterwards reaches back
// to that moment. Every response on these routes carries that sentence in
// `caveat`, including the ones that report nothing wrong.
//
// # Trust
//
// Nothing here is discovered, defaulted or assumed. An operator types the other
// node's URL and public key (POST /api/sync/peers) and that pin is the whole
// trust model:
//
//   - Inbound, a request must carry a signed envelope (internal/scan/substrate's
//     peerauth.go) under a key with an enabled `sync_peer` row. An unpinned key is
//     refused. There is no pairing secret, no trust-on-first-use, and no unsigned
//     fallback to be misconfigured into.
//
//   - Outbound, the peer's RESPONSE must be signed by the pinned key too. A node
//     answering at the right address with a different key is refused and the pin
//     is left exactly as it was.
//
//   - Every op is verified ON ITS OWN — signature, structure, namespace, stamp
//     agreement, future-claim bound — before it is stored, and its AUTHOR must
//     itself be a pinned key. Arriving over an authenticated connection buys an
//     op nothing.
//
// # Bounds
//
// `POST /api/scan/sync` was an unbounded authenticated write until it was
// rate-limited; these routes are bounded from the start. Bodies are size-capped
// before they are read, pages are count-capped, a round is page-capped, and both
// peer routes share a rate limiter. An authenticated peer is a bounded peer.
//
// # A node alone
//
// The default is a node with no peers: no identity key is generated, no row is
// written, and no socket is opened. Replication happens only when an operator has
// enrolled a peer AND a round is triggered (POST /api/sync/peers/{id}/sync).
// Nothing in Cackle polls the network on its own.

const (
	// maxSyncBody caps a push body before it is read. It must be read whole to
	// hash it for signature verification, so the cap is what stops an
	// authenticated peer from making this node buffer arbitrary bytes. 1 MiB is
	// roughly 2,000 admission ops — far more than maxSyncOpsPerPush allows
	// anyway, so the count cap binds first and this one is the backstop.
	maxSyncBody = 1 << 20

	// maxSyncOpsPerPush caps how many ops one push may carry. Each one costs a
	// signature verification inside the WebAssembly engine plus a write.
	maxSyncOpsPerPush = 256

	// defaultSyncPullLimit and maxSyncPullLimit bound a pull page. A caller
	// asking for more gets the maximum rather than an error — the point is that
	// the page is bounded, not that the caller is scolded for asking.
	defaultSyncPullLimit = 128
	maxSyncPullLimit     = 256

	// maxSyncMintPerRequest bounds the mint pass that turns local `admissions`
	// rows into signed ops. A first sync after a large event pages through
	// rather than folding the whole table in one request.
	maxSyncMintPerRequest = 512
)

// syncCaveat travels on every response from these routes, including the empty
// and the successful ones. An operator reading a clean replication report must
// not come away believing they bought a guarantee that cannot exist.
const syncCaveat = "Replication makes a cross-gate double admission visible sooner and on more nodes. " +
	"It cannot prevent one: two gates that could not see each other at the moment of the scan both " +
	"opened the door, and no merge rule, transport, or number of nodes reaches back to that moment. " +
	"It is also only as complete as the sync — claims on a node that has not been reached yet, or on " +
	"a gate whose log never arrived anywhere, are invisible here."

// --- wire shapes -----------------------------------------------------------

// syncOpWire is one op in transit.
//
// EventID is UNTRUSTED routing metadata: it says which namespace's replica should
// open the envelope, and substrate.VerifyOp refuses the op if the signed bytes
// name a different one. It is carried outside the envelope because a receiver has
// to know which per-event replica to hand it to before it can decode it.
type syncOpWire struct {
	Seq     int64  `json:"seq,omitempty"`
	EventID string `json:"event_id"`
	Cose    string `json:"cose"` // COSE_Sign1, standard base64
}

type syncOpsResponse struct {
	// Node is the answering node's public key, lowercase hex — the value the
	// caller pinned.
	Node string `json:"node"`
	// Algebra is the merge algebra these ops were minted under. A caller that
	// does not speak it must refuse them rather than merge on the assumption
	// that the rules match.
	Algebra string       `json:"algebra"`
	Ops     []syncOpWire `json:"ops"`
	// NextAfter is the cursor to pass back to continue. It never moves
	// backwards.
	NextAfter int64 `json:"next_after"`
	// Complete reports whether this page exhausted what the caller may see.
	// False means "call again"; it does NOT mean anything is wrong.
	Complete bool   `json:"complete"`
	Caveat   string `json:"caveat"`
}

type syncPushRequest struct {
	Ops []syncOpWire `json:"ops"`
}

// syncPushResult is one op's outcome, positionally matching the request.
type syncPushResult struct {
	// OpID is the §4.1 content address the receiver computed from the envelope
	// itself, present only when the op verified.
	OpID string `json:"op_id,omitempty"`
	// Stored is true when the op was new to this node's log. False with an empty
	// Reason means it was already held — which is a success, not a rejection.
	Stored bool `json:"stored"`
	// Applied reports whether the claim is also represented in the `admissions`
	// table. False means this node holds and reports the claim but cannot count
	// it — most often a claim about a ticket this node does not have.
	Applied bool `json:"applied"`
	// Reason is a refusal, in plain language. It is per-op: one bad op in a batch
	// does not reject the batch, because the other claims in it are still
	// evidence about a door.
	Reason string `json:"reason,omitempty"`
}

type syncPushResponse struct {
	Node    string           `json:"node"`
	Algebra string           `json:"algebra"`
	Results []syncPushResult `json:"results"`
	Caveat  string           `json:"caveat"`
}

// --- shared plumbing -------------------------------------------------------

// syncIdentity loads this node's replication identity, creating it on first use,
// and returns a Signer over it.
//
// Read from the database on every call rather than cached. It is one indexed
// single-row read, and caching it would mean a rotation (the operator deleting
// the row) kept signing under the old key until the process restarted — the one
// moment where being stale is a security property rather than a performance one.
func (s *server) syncIdentity(ctx context.Context) (kotvasync.Signer, store.NodeIdentity, error) {
	id, err := s.deps.Store.EnsureNodeIdentity(ctx)
	if err != nil {
		return nil, store.NodeIdentity{}, err
	}
	signer, err := substrate.NewSigner(id.PrivateKey)
	if err != nil {
		return nil, store.NodeIdentity{}, err
	}
	return signer, id, nil
}

// syncLedger opens a per-event replica on the process-wide compiled engine.
//
// One per event because the §7 namespace IS the event id. The caller closes it.
func (s *server) syncLedger(ctx context.Context, eventID string, signer kotvasync.Signer) (*substrate.Ledger, error) {
	compiler, err := s.ledgers.get(ctx, s.deps.Config.ScanCacheDir())
	if err != nil {
		return nil, err
	}
	return compiler.Ledger(ctx, substrate.Options{EventID: eventID, Signer: signer})
}

// readCappedBody reads at most maxSyncBody bytes and refuses a longer body
// rather than silently truncating it.
//
// Truncating would be the worst of the options available: the body hash would
// not match, the request would fail as a signature error, and an operator would
// spend the afternoon looking at their keys.
func readCappedBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSyncBody+1))
	if err != nil {
		badRequest(w, "could not read request body")
		return nil, false
	}
	if len(body) > maxSyncBody {
		writeError(w, http.StatusRequestEntityTooLarge, codeInvalidRequest,
			fmt.Sprintf("sync request body exceeds %d bytes", maxSyncBody))
		return nil, false
	}
	return body, true
}

// authenticatePeer authenticates an inbound peer request and returns the enabled
// enrolments of the key that signed it.
//
// The pinned key passed to VerifyRequest is read out of the peer ROW, not out of
// the request header, so the signature is checked against what an operator
// pinned. A key with no enabled row anywhere is refused here and never reaches a
// handler.
func (s *server) authenticatePeer(w http.ResponseWriter, r *http.Request, body []byte) ([]store.SyncPeer, bool) {
	presented := substrate.PresentedKey(r.Header)
	if presented == "" {
		unauthorized(w, "peer authentication required: sign the request with this node's replication key")
		return nil, false
	}
	peers, err := s.deps.Store.EnabledSyncPeersByKey(r.Context(), presented)
	if err != nil {
		internalError(w, s.log(), "sync peer lookup", err)
		return nil, false
	}
	if len(peers) == 0 {
		// Not enrolled. Deliberately the same answer as a bad signature: a
		// stranger learns whether a key is enrolled here only by holding it.
		s.log().Warn("sync: refused an unenrolled node key",
			"key", presented, "path", r.URL.Path, "remote_ip", clientIP(r))
		unauthorized(w, "this node key is not enrolled here")
		return nil, false
	}
	if err := substrate.VerifyRequest(r, body, peers[0].PublicKey, s.syncNonces, time.Now()); err != nil {
		s.log().Warn("sync: refused a peer request",
			"key", presented, "path", r.URL.Path, "remote_ip", clientIP(r), "err", err)
		unauthorized(w, "peer request authentication failed")
		return nil, false
	}
	return peers, true
}

// writeSignedJSON marshals v, signs the exact bytes with this node's key, and
// writes them.
//
// Marshal-then-sign-then-write rather than writeJSON's stream-encode, because the
// signature has to cover the bytes the peer will hash — which means they must
// exist before the headers are set.
func (s *server) writeSignedJSON(w http.ResponseWriter, r *http.Request, signer kotvasync.Signer, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		internalError(w, s.log(), "sync marshal response", err)
		return
	}
	if err := substrate.SignResponse(w.Header(), signer, r.Header.Get(substrate.HeaderNonce), body, time.Now()); err != nil {
		internalError(w, s.log(), "sync sign response", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// --- peer-facing routes ----------------------------------------------------

// handleSyncPull serves GET /api/sync/ops: one bounded page of the admission
// ledger, for a peer enrolled on this node.
func (s *server) handleSyncPull(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, ok := readCappedBody(w, r)
	if !ok {
		return
	}
	peers, ok := s.authenticatePeer(w, r, body)
	if !ok {
		return
	}

	after, err := strconv.ParseInt(firstNonEmptyStr(r.URL.Query().Get("after"), "0"), 10, 64)
	if err != nil || after < 0 {
		badRequest(w, "after must be a non-negative integer cursor")
		return
	}
	limit := defaultSyncPullLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			badRequest(w, "limit must be a positive integer")
			return
		}
		limit = min(n, maxSyncPullLimit)
	}

	signer, id, err := s.syncIdentity(ctx)
	if err != nil {
		internalError(w, s.log(), "sync identity", err)
		return
	}

	// Mint this node's own un-minted admissions first, or a pull would serve a
	// log that lags the table it is derived from and the caller would page to
	// "complete" while claims sat unsent.
	for _, p := range peers {
		if _, err := s.mintPendingOps(ctx, p.OrgID, signer, maxSyncMintPerRequest); err != nil {
			internalError(w, s.log(), "sync mint ops", err)
			return
		}
	}

	// The union across every organisation this key is enrolled for, then one
	// page of it. Taking `limit` per org and truncating the merge cannot skip a
	// row: anything dropped by the truncation has a seq above the cursor this
	// answer reports, so the next page starts at it.
	//
	// limit+1 rows are read, and the extra one is the ONLY way to know whether
	// more remain. Reading exactly `limit` and reporting `complete` from that
	// count cannot distinguish "the log ends here" from "the page is full", and
	// guessing "complete" makes a caller stop one page short — which is not a
	// slow sync but a SILENT one: it reports success having never reached the
	// claims past the first page.
	var ops []store.SyncOp
	for _, p := range peers {
		page, err := s.deps.Store.SyncOpsForOrgAfter(ctx, p.OrgID, after, limit+1)
		if err != nil {
			internalError(w, s.log(), "sync read ops", err)
			return
		}
		ops = append(ops, page...)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Seq < ops[j].Seq })
	ops = dedupeOpsBySeq(ops)
	complete := len(ops) <= limit
	if !complete {
		ops = ops[:limit]
	}

	resp := syncOpsResponse{
		Node:      id.PublicKey,
		Algebra:   substrate.AlgebraName,
		Ops:       make([]syncOpWire, 0, len(ops)),
		NextAfter: after,
		Complete:  complete,
		Caveat:    syncCaveat,
	}
	for _, op := range ops {
		resp.Ops = append(resp.Ops, syncOpWire{
			Seq:     op.Seq,
			EventID: op.EventID,
			Cose:    base64.StdEncoding.EncodeToString(op.Cose),
		})
		resp.NextAfter = op.Seq
	}
	s.writeSignedJSON(w, r, signer, http.StatusOK, resp)
}

// handleSyncPush serves POST /api/sync/ops: a peer hands this node ops it
// holds.
//
// Every op is verified on its own and its author must be a key this node's
// operator pinned. A refusal is per-op and named, because one malformed op in a
// batch is no reason to discard the rest of the evidence about a door.
func (s *server) handleSyncPush(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, ok := readCappedBody(w, r)
	if !ok {
		return
	}
	peers, ok := s.authenticatePeer(w, r, body)
	if !ok {
		return
	}

	var req syncPushRequest
	if err := json.Unmarshal(body, &req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if len(req.Ops) > maxSyncOpsPerPush {
		badRequest(w, fmt.Sprintf("a push carries at most %d ops, got %d", maxSyncOpsPerPush, len(req.Ops)))
		return
	}

	signer, id, err := s.syncIdentity(ctx)
	if err != nil {
		internalError(w, s.log(), "sync identity", err)
		return
	}

	orgs := make(map[string]struct{}, len(peers))
	for _, p := range peers {
		orgs[p.OrgID] = struct{}{}
	}
	results, err := s.ingestPeerOps(ctx, req.Ops, orgs, substrate.PresentedKey(r.Header), signer)
	if err != nil {
		internalError(w, s.log(), "sync ingest ops", err)
		return
	}

	s.writeSignedJSON(w, r, signer, http.StatusOK, syncPushResponse{
		Node:    id.PublicKey,
		Algebra: substrate.AlgebraName,
		Results: results,
		Caveat:  syncCaveat,
	})
}

// --- operator-facing routes ------------------------------------------------

type syncPeerView struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Key   string `json:"public_key"`
	Ready bool   `json:"enabled"`
	// Dialable reports whether this node will ever contact the peer. False is a
	// valid, common configuration — an inbound-only peer this node cannot reach
	// (behind NAT) which dials in instead.
	Dialable   bool       `json:"dialable"`
	PullCursor int64      `json:"pull_cursor"`
	PushCursor int64      `json:"push_cursor"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	LastStatus string     `json:"last_status,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`

	// The two event-feed switches (migration 0007, internal/httpapi/peer_feed.go).
	// Separate from Ready/`enabled`: enrolling a peer to reconcile door scans is
	// not consent to hand them your programme, nor to show theirs. Both are
	// false on a peer that was just enrolled.
	FeedPublish   bool       `json:"feed_publish"`
	FeedSubscribe bool       `json:"feed_subscribe"`
	FeedPulledAt  *time.Time `json:"feed_pulled_at,omitempty"`
	FeedStatus    string     `json:"feed_status,omitempty"`
}

func syncPeerToView(p store.SyncPeer) syncPeerView {
	return syncPeerView{
		ID: p.ID, Name: p.Name, URL: p.URL, Key: p.PublicKey, Ready: p.Enabled,
		Dialable: p.URL != "", PullCursor: p.PullCursor, PushCursor: p.PushCursor,
		LastSyncAt: p.LastSyncAt, LastStatus: p.LastStatus, CreatedAt: p.CreatedAt,
		FeedPublish: p.FeedPublish, FeedSubscribe: p.FeedSubscribe,
		FeedPulledAt: p.FeedPulledAt, FeedStatus: p.FeedStatus,
	}
}

type syncStatusResponse struct {
	// Node is THIS node's public key: the value to read off here and type into
	// the other node's enrolment form.
	Node       string            `json:"node"`
	Algebra    string            `json:"algebra"`
	Engine     string            `json:"engine,omitempty"`
	Peers      []syncPeerView    `json:"peers"`
	OpLog      store.SyncOpStats `json:"op_log"`
	Caveat     string            `json:"caveat"`
	Standalone bool              `json:"standalone"`
}

// handleSyncStatus serves GET /api/sync/status?org=<id>: this node's replication
// identity, its enrolled peers, and the state of the op log.
//
// This is where an operator reads the key they hand to the other node's operator,
// which is why it is the route that creates the identity on first use — a node
// with no peers has never needed one and never generates one at boot.
func (s *server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, ok := s.syncOrgFromQuery(w, r)
	if !ok {
		return
	}

	_, id, err := s.syncIdentity(ctx)
	if err != nil {
		internalError(w, s.log(), "sync identity", err)
		return
	}
	peers, err := s.deps.Store.ListSyncPeers(ctx, orgID)
	if err != nil {
		internalError(w, s.log(), "sync list peers", err)
		return
	}
	stats, err := s.deps.Store.SyncOpStatsForOrg(ctx, orgID)
	if err != nil {
		internalError(w, s.log(), "sync op stats", err)
		return
	}

	resp := syncStatusResponse{
		Node:       id.PublicKey,
		Algebra:    substrate.AlgebraName,
		Peers:      make([]syncPeerView, 0, len(peers)),
		OpLog:      stats,
		Caveat:     syncCaveat,
		Standalone: len(peers) == 0,
	}
	for _, p := range peers {
		resp.Peers = append(resp.Peers, syncPeerToView(p))
	}
	// The engine revision is read from the engine rather than written down, so
	// two operators comparing deployments can see whether they merge under the
	// same rules. It costs a module instantiation, so it is best-effort: an
	// operator asking about their peers should not get a 500 because the
	// WebAssembly cache directory is unwritable.
	if compiler, err := s.ledgers.get(ctx, s.deps.Config.ScanCacheDir()); err == nil {
		if signer, err := substrate.NewEphemeralSigner(); err == nil {
			if l, err := compiler.Ledger(ctx, substrate.Options{EventID: "version-probe", Signer: signer}); err == nil {
				if v, err := l.AlgebraVersion(); err == nil {
					resp.Engine = v.Substrate
				}
				_ = l.Close(ctx)
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type enrolPeerRequest struct {
	OrgID string `json:"org_id"`
	Name  string `json:"name"`
	// URL is the peer's origin, e.g. "https://cackle.example.org". Omit it for a
	// peer this node cannot reach and must never dial.
	URL string `json:"url"`
	// PublicKey is the peer's node key, read off its own GET /api/sync/status.
	PublicKey string `json:"public_key"`
}

// handleEnrolSyncPeer serves POST /api/sync/peers: a human enrolling another
// node by address and key.
//
// Owner-only, and scoped to one organisation — a peer sees the admission ledger
// of that organisation's events and nothing else, so enrolling a cloud node for
// one organisation does not hand it a multi-tenant node's others.
func (s *server) handleEnrolSyncPeer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := userFromContext(ctx)

	var req enrolPeerRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.OrgID == "" || req.PublicKey == "" {
		badRequest(w, "org_id and public_key are required")
		return
	}
	ok, err := s.deps.Auth.CanManageOrg(ctx, user.ID, req.OrgID, auth.RoleOwner)
	if err != nil {
		internalError(w, s.log(), "sync enrol rbac", err)
		return
	}
	if !ok {
		forbidden(w, "enrolling a replication peer requires the owner role on this org")
		return
	}

	key, err := store.NormalizeNodeKey(req.PublicKey)
	if err != nil {
		badRequest(w, "public_key must be a 32-byte hex Ed25519 key")
		return
	}
	// A node enrolling ITSELF would replicate its own ops back into its own log
	// and report a peer that is a mirror. It is a typo, not a topology.
	if _, id, err := s.syncIdentity(ctx); err == nil && id.PublicKey == key {
		badRequest(w, "that is this node's own key")
		return
	}
	base, err := normalizePeerURL(req.URL)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	p := &store.SyncPeer{
		OrgID: req.OrgID, Name: strings.TrimSpace(req.Name), URL: base,
		PublicKey: key, Enabled: true, LastStatus: "enrolled, never synced",
	}
	if err := s.deps.Store.CreateSyncPeer(ctx, p); err != nil {
		if isUniqueConstraintErr(err) {
			// A pin already exists for this key in this org. Overwriting it here
			// is exactly the silent re-pin this design refuses: if the address
			// changed, a human deletes the enrolment and makes a new one.
			conflict(w, "this node key is already enrolled for this org; delete that peer first")
			return
		}
		internalError(w, s.log(), "sync create peer", err)
		return
	}
	writeJSON(w, http.StatusOK, syncPeerToView(*p))
}

// handleDeleteSyncPeer serves DELETE /api/sync/peers/{id}. It is also how a
// peer's key is rotated: delete, then enrol the new key. Deleting resets the
// cursors, so a fresh enrolment replays that peer's ledger from the beginning —
// which is the recovery path for a peer that refused ops earlier.
func (s *server) handleDeleteSyncPeer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.syncPeerForOperator(w, r)
	if !ok {
		return
	}
	if err := s.deps.Store.DeleteSyncPeer(ctx, p.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "peer not found")
			return
		}
		internalError(w, s.log(), "sync delete peer", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": p.ID, "caveat": syncCaveat})
}

// handleSyncPeerRound serves POST /api/sync/peers/{id}/sync: run one bounded
// replication round against this peer, now.
//
// Rounds are triggered rather than polled. Nothing in Cackle opens a socket on a
// schedule: a deployment that never calls this never talks to anything, which is
// the property that makes "no default network calls" true rather than aspirational.
// An operator who wants continuous replication calls this on a timer (cron,
// systemd, a scheduled job) — see docs/CLUSTERING.md.
func (s *server) handleSyncPeerRound(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.syncPeerForOperator(w, r)
	if !ok {
		return
	}
	if p.URL == "" {
		badRequest(w, "this peer has no address: it is inbound-only and must dial this node itself")
		return
	}
	if !p.Enabled {
		badRequest(w, "this peer is disabled")
		return
	}
	round := s.replicateWithPeer(ctx, p)
	status := http.StatusOK
	if round.Error != "" {
		// A refused peer is not a server fault, and it is not a success either.
		// 502 says "the far side did not answer acceptably" without pretending
		// this node broke.
		status = http.StatusBadGateway
	}
	writeJSON(w, status, round)
}

// --- helpers ---------------------------------------------------------------

// syncOrgFromQuery resolves ?org=<id> and checks the caller owns it.
func (s *server) syncOrgFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	ctx := r.Context()
	user, _ := userFromContext(ctx)
	orgID := r.URL.Query().Get("org")
	if orgID == "" {
		badRequest(w, "org is required")
		return "", false
	}
	ok, err := s.deps.Auth.CanManageOrg(ctx, user.ID, orgID, auth.RoleOwner)
	if err != nil {
		internalError(w, s.log(), "sync org rbac", err)
		return "", false
	}
	if !ok {
		forbidden(w, "replication settings require the owner role on this org")
		return "", false
	}
	return orgID, true
}

// syncPeerForOperator loads the peer named in the URL and checks the caller owns
// the organisation it is enrolled for.
func (s *server) syncPeerForOperator(w http.ResponseWriter, r *http.Request) (store.SyncPeer, bool) {
	ctx := r.Context()
	user, _ := userFromContext(ctx)
	p, err := s.deps.Store.GetSyncPeer(ctx, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		notFound(w, "peer not found")
		return store.SyncPeer{}, false
	}
	if err != nil {
		internalError(w, s.log(), "sync get peer", err)
		return store.SyncPeer{}, false
	}
	ok, err := s.deps.Auth.CanManageOrg(ctx, user.ID, p.OrgID, auth.RoleOwner)
	if err != nil {
		internalError(w, s.log(), "sync peer rbac", err)
		return store.SyncPeer{}, false
	}
	if !ok {
		// Same answer as a missing peer: an owner of one org learns nothing
		// about another org's topology by probing ids.
		notFound(w, "peer not found")
		return store.SyncPeer{}, false
	}
	return p, true
}

// normalizePeerURL validates an operator-typed peer address and returns its
// canonical origin form.
//
// A path is REFUSED rather than accommodated. Cackle mounts its routes at the
// root, and the signature a peer verifies covers the request path — so a node
// behind a prefix-stripping reverse proxy would see a path that does not match
// what was signed, and every request would fail as a signature error. Refusing
// the URL at enrolment turns that into one clear sentence at the moment a human
// can act on it.
func normalizePeerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil // inbound-only peer; this node will never dial it
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("url is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("url must be http or https")
	}
	if u.Host == "" {
		return "", errors.New("url must include a host")
	}
	if u.User != nil {
		return "", errors.New("url must not carry credentials; peers authenticate by key")
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		return "", errors.New("url must be an origin with no path (Cackle serves /api at the root)")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("url must not carry a query or fragment")
	}
	return u.Scheme + "://" + u.Host, nil
}

// dedupeOpsBySeq removes the duplicates a union across organisations can produce
// when one event is somehow reachable through two enrolments. Serving one op
// twice in a page is harmless to convergence and confusing to read, and dropping
// the copy costs one pass.
func dedupeOpsBySeq(ops []store.SyncOp) []store.SyncOp {
	if len(ops) < 2 {
		return ops
	}
	out := ops[:1]
	for _, op := range ops[1:] {
		if op.Seq != out[len(out)-1].Seq {
			out = append(out, op)
		}
	}
	return out
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// claimFromAdmission reads the DEVICE's own verdict off an admission row as the
// claim to replicate.
//
// The device's claim and not the server's `result`: folding the reconciled
// conclusion would fold it back into its own premise, so every downgraded row
// would replicate as 'duplicate' and the double admission would be invisible on
// the peer — which is exactly the bug migration 0002 exists to fix, reintroduced
// one layer further out. claimedResult (reconcile_handlers.go) is the same read
// the conflicts report uses.
func claimFromAdmission(row store.AdmissionClaim) scan.QueuedAdmission {
	return scan.QueuedAdmission{
		TicketID:  row.TicketID,
		EventID:   row.EventID,
		GateID:    row.GateID,
		DeviceID:  row.DeviceID,
		ScannedAt: row.ScannedAt,
		Result:    scan.Status(claimedResult(row)),
		Note:      row.Note,
	}
}
