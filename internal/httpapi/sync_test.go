package httpapi

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	kotvasync "github.com/vul-os/kotva/bindings/go"

	"github.com/vul-os/cackle/internal/scan/substrate"
	"github.com/vul-os/cackle/internal/store"
)

// Tests for server-to-server replication of the admission ledger.
//
// The centrepiece is TestTwoNodesConvergeAndRefuseAChangedKey, which runs two
// complete Cackle stacks on real TCP sockets — on this host's own interface
// address rather than loopback where there is one — and drives them the way two
// deployments are driven: an operator on each side enrols the other by address
// and key, gates sync their scans to whichever node they can reach, and one
// triggered round makes the cross-gate double admission visible on BOTH.
//
// The second half of that test is the property the whole peer model rests on: a
// peer whose key has changed is REFUSED, and the pin is not quietly replaced.

// --- a second node -----------------------------------------------------------

// syncNode is one Cackle instance listening on a real socket.
type syncNode struct {
	*testHarness
	srv   *httptest.Server
	url   string
	key   string // this node's replication public key, lowercase hex
	org   string
	token string // an owner session on this node
}

// hostListener binds a listener on this host's own interface address, falling
// back to loopback only when the machine has no other address.
//
// Non-loopback matters here because loopback can hide real bugs: a signature that
// covers the request path, a body hash over bytes that went through a kernel
// buffer, a peer URL that has to be dialled by address rather than by name — none
// of those are exercised by an in-process handler call, and only some of them are
// exercised by 127.0.0.1. It does NOT skip when there is no such address: a test
// that silently does not run is worse than one that runs over loopback, and the
// two-binary procedure in docs/CLUSTERING.md is the demonstration that is
// explicitly non-loopback.
func hostListener(t *testing.T) (net.Listener, bool) {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
				continue
			}
			l, err := net.Listen("tcp", net.JoinHostPort(ipnet.IP.String(), "0"))
			if err == nil {
				return l, true
			}
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen on loopback either: %v", err)
	}
	return l, false
}

func newSyncNode(t *testing.T, h *testHarness, orgID, token string) *syncNode {
	t.Helper()
	l, nonLoopback := hostListener(t)
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: h.handler}}
	srv.Start()
	t.Cleanup(srv.Close)
	if !nonLoopback {
		t.Logf("no non-loopback interface address available; this node is on %s", srv.URL)
	}

	n := &syncNode{testHarness: h, srv: srv, url: srv.URL, org: orgID, token: token}
	var status syncStatusResponse
	if code := n.call(t, http.MethodGet, "/api/sync/status?org="+orgID, token, nil, &status); code != http.StatusOK {
		t.Fatalf("sync status: %d", code)
	}
	if status.Node == "" {
		t.Fatal("a node must report its own replication key so an operator can hand it over")
	}
	n.key = status.Node
	return n
}

// call makes a real HTTP request against this node.
func (n *syncNode) call(t *testing.T, method, path, token string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, n.url+path, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	if resp.StatusCode >= 500 {
		t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, raw)
	}
	return resp.StatusCode
}

// mirrorTables copies whole tables from one node's database to another's.
//
// This is the deployment story replication is for, reproduced honestly: Cackle
// does NOT replicate events, tickets or orders, so two nodes hold the same event
// because one was brought up from a copy of the other. `sync_node_identity` is
// deliberately NOT copied — copying it would clone the first node's identity and
// give two nodes the same name in the mesh, which is exactly what migration 0003
// warns an operator about.
func mirrorTables(t *testing.T, from, to *store.Store, tables ...string) {
	t.Helper()
	for _, table := range tables {
		rows, err := from.DB().QueryContext(t.Context(), "SELECT * FROM "+table) //nolint:gosec // fixed test-local table names
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatalf("columns of %s: %v", table, err)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
		insert := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(cols, ","), placeholders)
		var batch [][]any
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			batch = append(batch, vals)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows %s: %v", table, err)
		}
		rows.Close()
		for _, vals := range batch {
			if _, err := to.DB().ExecContext(t.Context(), insert, vals...); err != nil {
				t.Fatalf("write %s: %v", table, err)
			}
		}
	}
}

// --- the main event ----------------------------------------------------------

func TestTwoNodesConvergeAndRefuseAChangedKey(t *testing.T) {
	// Node A: a full stack with a published event and two tickets.
	ha := newTestHarness(t)
	fx := ha.newPublishedEvent(t, "cluster-a")
	buyerToken, _ := ha.signupUser("buyer-cluster@example.com", "buyer-password-123", "Thandi Mokoena")
	tickets := ha.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-cluster@example.com", "Thandi Mokoena", 2)
	shared, clean := tickets[0], tickets[1]

	// Node B: brought up from a copy of node A's event data, then given its own
	// owner. Same org, same event, same tickets; its own identity.
	hb := newTestHarness(t)
	mirrorTables(t, ha.store, hb.store,
		"users", "orgs", "org_members", "events", "event_keys", "ticket_types",
		"orders", "order_items", "tickets")
	tokenB, ownerB := hb.signupUser("owner-node-b@example.com", "owner-password-123", "Owner B")
	if err := hb.store.AddOrgMember(t.Context(),
		&store.OrgMember{OrgID: fx.orgID, UserID: ownerB, Role: "owner"}); err != nil {
		t.Fatalf("make owner on node B: %v", err)
	}

	a := newSyncNode(t, ha, fx.orgID, fx.ownerToken)
	b := newSyncNode(t, hb, fx.orgID, tokenB)
	t.Logf("node A %s key %s…", a.url, a.key[:12])
	t.Logf("node B %s key %s…", b.url, b.key[:12])

	if a.key == b.key {
		t.Fatal("two nodes must not share a replication identity; sync_node_identity was not copied")
	}

	// --- enrolment: a human types an address and a key, on each side ---------
	var peerOnA syncPeerView
	if code := a.call(t, http.MethodPost, "/api/sync/peers", a.token,
		enrolPeerRequest{OrgID: fx.orgID, Name: "node-b", URL: b.url, PublicKey: b.key}, &peerOnA); code != http.StatusOK {
		t.Fatalf("enrol B on A: %d", code)
	}
	if !peerOnA.Dialable || peerOnA.Key != b.key {
		t.Fatalf("enrolment did not pin what was typed: %+v", peerOnA)
	}
	// Node B enrols A by KEY ONLY, with no address — the inbound-only shape a
	// node behind NAT has. B will never dial A; A drives both directions.
	var peerOnB syncPeerView
	if code := b.call(t, http.MethodPost, "/api/sync/peers", b.token,
		enrolPeerRequest{OrgID: fx.orgID, Name: "node-a", PublicKey: a.key}, &peerOnB); code != http.StatusOK {
		t.Fatalf("enrol A on B: %d", code)
	}
	if peerOnB.Dialable {
		t.Fatal("a peer enrolled without an address must never be dialled")
	}

	// --- two partitioned gates, two doors, one ticket ------------------------
	//
	// Gate A reaches node A; gate B reaches node B. Neither gate could see the
	// other at the moment of the scan, so both admitted the same ticket. That is
	// the thing replication cannot prevent, and it is why this test exists.
	base := time.Now().UTC().Add(-90 * time.Minute).Truncate(time.Second)
	syncGate := func(n *syncNode, items []scanSyncItem) {
		t.Helper()
		var resp scanSyncResponse
		if code := n.call(t, http.MethodPost, "/api/scan/sync", n.token,
			scanSyncRequest{Admissions: items}, &resp); code != http.StatusOK {
			t.Fatalf("gate sync on %s: %d", n.url, code)
		}
	}
	syncGate(a, []scanSyncItem{
		{TicketID: shared.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base, Result: "admitted"},
		{TicketID: clean.ID, EventID: fx.eventID, GateID: "North", DeviceID: "device-A",
			ScannedAt: base.Add(time.Minute), Result: "admitted"},
	})
	syncGate(b, []scanSyncItem{
		{TicketID: shared.ID, EventID: fx.eventID, GateID: "South", DeviceID: "device-B",
			ScannedAt: base.Add(2 * time.Minute), Result: "admitted"},
	})

	// Before replication each node sees only its own gate, so neither can see the
	// duplicate. This is the "before" half of the honest claim.
	if got := a.conflicts(t, fx.eventID); len(got.Conflicts) != 0 {
		t.Fatalf("node A cannot know about gate B yet: %+v", got.Conflicts)
	}

	// --- one triggered round ------------------------------------------------
	var round syncRoundResult
	if code := a.call(t, http.MethodPost, "/api/sync/peers/"+peerOnA.ID+"/sync", a.token, nil, &round); code != http.StatusOK {
		t.Fatalf("replication round: %d (%+v)", code, round)
	}
	if round.Error != "" {
		t.Fatalf("round reported an error: %s", round.Error)
	}
	if round.Minted != 2 {
		t.Fatalf("node A holds two local claims to mint, minted %d: %+v", round.Minted, round)
	}
	if round.Pushed != 2 {
		t.Fatalf("node B should have accepted both of A's ops as new, accepted %d: %+v", round.Pushed, round)
	}
	// A pulls THREE ops and stores ONE. The other two are the ops it pushed a
	// moment earlier coming back to it, and they are recognised as already held
	// rather than refused or duplicated — which is the content-addressed
	// idempotency doing its job, and the reason a pair of nodes does not amplify
	// one claim into a copy per round.
	if round.Pulled != 3 {
		t.Fatalf("node A should have pulled B's whole log (its own two ops plus gate B's): %+v", round)
	}
	if round.Stored != 1 {
		t.Fatalf("exactly one of the pulled ops is new to node A: %+v", round)
	}
	if round.PullRefused != 0 || round.PushRefused != 0 {
		t.Fatalf("nothing should have been refused between two properly enrolled nodes: %+v", round)
	}
	if round.Caveat == "" {
		t.Fatal("a replication result must carry the limit of what it bought")
	}
	if !strings.Contains(round.Caveat, "cannot prevent") {
		t.Fatalf("the caveat must say replication cannot prevent a double admission: %q", round.Caveat)
	}

	// --- both nodes now see the same double admission ------------------------
	for _, tc := range []struct {
		name string
		n    *syncNode
	}{{"node A", a}, {"node B", b}} {
		got := tc.n.conflicts(t, fx.eventID)
		if len(got.Conflicts) != 1 {
			t.Fatalf("%s: expected one conflict after replication, got %d: %+v",
				tc.name, len(got.Conflicts), got.Conflicts)
		}
		c := got.Conflicts[0]
		if c.TicketID != shared.ID {
			t.Fatalf("%s: conflict names %q, want %q", tc.name, c.TicketID, shared.ID)
		}
		if c.Devices != 2 || c.ExtraAdmissions != 1 {
			t.Fatalf("%s: expected 2 devices / 1 extra admission, got %d / %d",
				tc.name, c.Devices, c.ExtraAdmissions)
		}
		devices := map[string]bool{}
		for _, cl := range c.Claims {
			devices[cl.DeviceID] = true
			if cl.Result != "admitted" {
				t.Fatalf("%s: claim from %s reports %q; both gates admitted, and replicating the "+
					"server's downgraded verdict instead of the device's own would hide this conflict",
					tc.name, cl.DeviceID, cl.Result)
			}
		}
		if !devices["device-A"] || !devices["device-B"] {
			t.Fatalf("%s: both gates' claims must survive the merge, got %v", tc.name, devices)
		}
	}

	// Both op logs converged on the same three claims, whichever node is asked.
	for _, tc := range []struct {
		name string
		n    *syncNode
	}{{"node A", a}, {"node B", b}} {
		var status syncStatusResponse
		if code := tc.n.call(t, http.MethodGet, "/api/sync/status?org="+fx.orgID, tc.n.token, nil, &status); code != http.StatusOK {
			t.Fatalf("%s status: %d", tc.name, code)
		}
		if status.OpLog.Ops != 3 {
			t.Fatalf("%s holds %d ops, want 3 (two gates at node A, one at node B)", tc.name, status.OpLog.Ops)
		}
		if status.OpLog.Unapplied != 0 {
			t.Fatalf("%s has %d verified claims it could not apply; both nodes hold every ticket here",
				tc.name, status.OpLog.Unapplied)
		}
		if status.OpLog.Pending != 0 {
			t.Fatalf("%s still has %d unminted claims after a completed round", tc.name, status.OpLog.Pending)
		}
		if status.Standalone {
			t.Fatalf("%s has a peer enrolled and must not report itself standalone", tc.name)
		}
	}

	// A second round is a no-op: idempotent by content address and by claim key.
	var again syncRoundResult
	if code := a.call(t, http.MethodPost, "/api/sync/peers/"+peerOnA.ID+"/sync", a.token, nil, &again); code != http.StatusOK {
		t.Fatalf("second round: %d", code)
	}
	if again.Error != "" || again.Pushed != 0 || again.Stored != 0 || again.Minted != 0 {
		t.Fatalf("a second round must move nothing: %+v", again)
	}

	// --- a peer whose key changed is REFUSED, and the pin is not replaced ----
	//
	// Simulated the way it actually happens: the address still answers, but with
	// a different key than the operator pinned. Here the operator pinned the
	// wrong key (a typo, or an impostor's key handed over); node B answers as
	// itself; the round must refuse rather than "helpfully" adopt what answered.
	if code := a.call(t, http.MethodDelete, "/api/sync/peers/"+peerOnA.ID, a.token, nil, nil); code != http.StatusOK {
		t.Fatalf("delete peer: %d", code)
	}
	wrongPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate a wrong key: %v", err)
	}
	wrongKey := hex.EncodeToString(wrongPub)
	var mispinned syncPeerView
	if code := a.call(t, http.MethodPost, "/api/sync/peers", a.token,
		enrolPeerRequest{OrgID: fx.orgID, Name: "node-b-mispinned", URL: b.url, PublicKey: wrongKey}, &mispinned); code != http.StatusOK {
		t.Fatalf("enrol mispinned peer: %d", code)
	}

	var refused syncRoundResult
	code := a.call(t, http.MethodPost, "/api/sync/peers/"+mispinned.ID+"/sync", a.token, nil, &refused)
	if code != http.StatusBadGateway {
		t.Fatalf("a round against a peer answering with a different key must fail, got %d", code)
	}
	// The body is still the round result, so read it explicitly.
	var body syncRoundResult
	rec := a.rawCall(t, http.MethodPost, "/api/sync/peers/"+mispinned.ID+"/sync", a.token, nil)
	if err := json.Unmarshal(rec, &body); err != nil {
		t.Fatalf("decode refusal %q: %v", rec, err)
	}
	if body.Error == "" {
		t.Fatalf("a refusal must say why: %+v", body)
	}
	if !strings.Contains(body.Error, "pinned") {
		t.Fatalf("the refusal must name the pin as the reason, got %q", body.Error)
	}

	// The pin is UNCHANGED: nothing re-pinned it to the key that answered.
	var peers []syncPeerView
	var status syncStatusResponse
	if code := a.call(t, http.MethodGet, "/api/sync/status?org="+fx.orgID, a.token, nil, &status); code != http.StatusOK {
		t.Fatalf("status after refusal: %d", code)
	}
	peers = status.Peers
	if len(peers) != 1 {
		t.Fatalf("expected the one mispinned peer, got %d", len(peers))
	}
	if peers[0].Key != wrongKey {
		t.Fatalf("the pin changed to %q — a key change must be a refusal, never a re-pin", peers[0].Key)
	}
	if peers[0].PullCursor != 0 || peers[0].PushCursor != 0 {
		t.Fatalf("a refused peer must not have advanced any cursor: %+v", peers[0])
	}
	if !strings.Contains(peers[0].LastStatus, "refused") {
		t.Fatalf("the refusal must be recorded on the peer for an operator to find, got %q", peers[0].LastStatus)
	}
}

// conflicts reads this node's cross-gate conflict report over its real socket,
// under this node's own owner session. Each node has its own operator, so the
// package-level helper (which reuses one fixture's token) cannot be used here.
func (n *syncNode) conflicts(t *testing.T, eventID string) admissionConflictsResponse {
	t.Helper()
	var out admissionConflictsResponse
	if code := n.call(t, http.MethodGet, "/api/events/"+eventID+"/admission-conflicts", n.token, nil, &out); code != http.StatusOK {
		t.Fatalf("admission-conflicts on %s: %d", n.url, code)
	}
	return out
}

// rawCall returns the raw response body, for the cases where the status code and
// the body both matter.
func (n *syncNode) rawCall(t *testing.T, method, path, token string, body any) []byte {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, n.url+path, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw
}

// --- a node alone ------------------------------------------------------------

// TestNodeWithNoPeersMintsNoIdentityAndOpensNoSocket is requirement (e): running
// alone is the DEFAULT, not a degraded mode.
//
// A stack that scans tickets, syncs gates and reads conflict reports must not
// acquire replication key material, an op log, or a peer along the way. The
// identity is created the first time a human asks to see it, and not before.
func TestNodeWithNoPeersMintsNoIdentityAndOpensNoSocket(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "standalone")
	buyerToken, _ := h.signupUser("buyer-standalone@example.com", "buyer-password-123", "Solo")
	tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-standalone@example.com", "Solo", 1)

	rec := h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, scanSyncRequest{Admissions: []scanSyncItem{
		{TicketID: tickets[0].ID, EventID: fx.eventID, GateID: "Main", DeviceID: "device-1",
			ScannedAt: time.Now().UTC().Truncate(time.Second), Result: "admitted"},
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("scan sync: %d %s", rec.Code, rec.Body.String())
	}
	h.conflicts(t, fx, http.StatusOK)

	if _, err := h.store.NodeIdentity(t.Context()); err == nil {
		t.Fatal("a node with no peers generated replication key material it has no use for")
	}
	var ops int
	if err := h.store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sync_op`).Scan(&ops); err != nil {
		t.Fatalf("count ops: %v", err)
	}
	if ops != 0 {
		t.Fatalf("a node with no peers minted %d ops nobody will ever read", ops)
	}

	// Asking for the status is the operator's first step towards clustering, and
	// it is where the identity appears.
	rec = h.do(http.MethodGet, "/api/sync/status?org="+fx.orgID, fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync status: %d %s", rec.Code, rec.Body.String())
	}
	status := decodeBody[syncStatusResponse](t, rec)
	if status.Node == "" || !status.Standalone {
		t.Fatalf("a node with no peers must report its key and that it stands alone: %+v", status)
	}
	if status.Caveat == "" {
		t.Fatal("the status must carry the limit of what replication buys")
	}
	if _, err := h.store.NodeIdentity(t.Context()); err != nil {
		t.Fatalf("the identity should exist once an operator has asked for it: %v", err)
	}
}

// --- inbound authentication --------------------------------------------------

// peerRequest signs a request the way a peer would and returns the response.
func peerRequest(t *testing.T, h *testHarness, method, path string, signer kotvasync.Signer, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.RemoteAddr = "198.51.100.7:5555"
	if signer != nil {
		if _, err := substrate.SignRequest(req, signer, body, time.Now()); err != nil {
			t.Fatalf("sign peer request: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func newTestSigner(t *testing.T) kotvasync.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s, err := substrate.NewSigner(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return s
}

// enrolledSigner enrols a fresh key on h for orgID and returns a signer for it,
// standing in for the peer at the other end.
func enrolledSigner(t *testing.T, h *testHarness, orgID string) kotvasync.Signer {
	t.Helper()
	s := newTestSigner(t)
	p := &store.SyncPeer{
		OrgID: orgID, Name: "test-peer", PublicKey: hex.EncodeToString(s.Public()), Enabled: true,
	}
	if err := h.store.CreateSyncPeer(t.Context(), p); err != nil {
		t.Fatalf("enrol peer: %v", err)
	}
	return s
}

func TestSyncPeerRoutesFailClosed(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "failclosed")
	enrolled := enrolledSigner(t, h, fx.orgID)

	t.Run("unsigned request is refused", func(t *testing.T) {
		rec := peerRequest(t, h, http.MethodGet, "/api/sync/ops", nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("an unsigned peer request must be refused, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("a session token is not a peer credential", func(t *testing.T) {
		// The operator's own session must not open the peer routes: they are a
		// different credential for a different caller, and conflating them would
		// let any scanner-level user read another org's ledger.
		rec := h.do(http.MethodGet, "/api/sync/ops", fx.ownerToken, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("a session must not authenticate a peer route, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("an unenrolled key is refused", func(t *testing.T) {
		rec := peerRequest(t, h, http.MethodGet, "/api/sync/ops", newTestSigner(t), nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("a key nobody pinned must be refused, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("an enrolled key is accepted and the answer is signed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sync/ops?after=0&limit=10", nil)
		req.RemoteAddr = "198.51.100.7:5555"
		nonce, err := substrate.SignRequest(req, enrolled, nil, time.Now())
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		rec := httptest.NewRecorder()
		h.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("an enrolled peer must be served, got %d %s", rec.Code, rec.Body.String())
		}
		id, err := h.store.NodeIdentity(t.Context())
		if err != nil {
			t.Fatalf("node identity: %v", err)
		}
		if err := substrate.VerifyResponse(rec.Header(), id.PublicKey, nonce, rec.Body.Bytes(), time.Now()); err != nil {
			t.Fatalf("the answer must be signed by this node so a caller can detect an impostor: %v", err)
		}
	})

	t.Run("the signature covers the query string", func(t *testing.T) {
		// The cursor lives in the query. A signature that did not cover it would
		// let anything on the path rewrite `after` and starve a node of history
		// it believes it received.
		req := httptest.NewRequest(http.MethodGet, "/api/sync/ops?after=0&limit=10", nil)
		req.RemoteAddr = "198.51.100.7:5555"
		if _, err := substrate.SignRequest(req, enrolled, nil, time.Now()); err != nil {
			t.Fatalf("sign: %v", err)
		}
		tampered := httptest.NewRequest(http.MethodGet, "/api/sync/ops?after=999999&limit=10", nil)
		tampered.RemoteAddr = req.RemoteAddr
		tampered.Header = req.Header.Clone()
		rec := httptest.NewRecorder()
		h.handler.ServeHTTP(rec, tampered)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("a rewritten cursor must invalidate the signature, got %d", rec.Code)
		}
	})

	t.Run("a replayed request is refused", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sync/ops?after=0", nil)
		req.RemoteAddr = "198.51.100.7:5555"
		if _, err := substrate.SignRequest(req, enrolled, nil, time.Now()); err != nil {
			t.Fatalf("sign: %v", err)
		}
		first := httptest.NewRecorder()
		h.handler.ServeHTTP(first, req)
		if first.Code != http.StatusOK {
			t.Fatalf("first request: %d %s", first.Code, first.Body.String())
		}
		replay := httptest.NewRequest(http.MethodGet, "/api/sync/ops?after=0", nil)
		replay.RemoteAddr = req.RemoteAddr
		replay.Header = req.Header.Clone()
		second := httptest.NewRecorder()
		h.handler.ServeHTTP(second, replay)
		if second.Code != http.StatusUnauthorized {
			t.Fatalf("a byte-identical replay must be refused, got %d", second.Code)
		}
	})

	t.Run("a stale timestamp is refused", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sync/ops?after=0", nil)
		req.RemoteAddr = "198.51.100.7:5555"
		if _, err := substrate.SignRequest(req, enrolled, nil,
			time.Now().Add(-2*substrate.MaxClockSkew)); err != nil {
			t.Fatalf("sign: %v", err)
		}
		rec := httptest.NewRecorder()
		h.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("a request outside the freshness window must be refused, got %d", rec.Code)
		}
	})

	t.Run("an oversized push body is refused before it is trusted", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), maxSyncBody+1)
		rec := peerRequest(t, h, http.MethodPost, "/api/sync/ops", enrolled, body)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("an oversized body must be refused with 413, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("too many ops in one push is refused", func(t *testing.T) {
		ops := make([]syncOpWire, maxSyncOpsPerPush+1)
		for i := range ops {
			ops[i] = syncOpWire{EventID: fx.eventID, Cose: base64.StdEncoding.EncodeToString([]byte("nope"))}
		}
		body, err := json.Marshal(syncPushRequest{Ops: ops})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rec := peerRequest(t, h, http.MethodPost, "/api/sync/ops", enrolled, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("a push over the item cap must be refused, got %d %s", rec.Code, rec.Body.String())
		}
	})
}

// TestPushedOpFromAnUnenrolledAuthorIsRefused is requirement (c)'s sharpest
// edge: an authenticated peer does not get to speak for a key nobody pinned.
//
// The transport is legitimate — an enrolled peer, correctly signed. The OP inside
// it is authored by a stranger. Authorship is not transitively trusted, so it is
// refused, per-op and by name, and nothing is stored.
func TestPushedOpFromAnUnenrolledAuthorIsRefused(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "author")
	buyerToken, _ := h.signupUser("buyer-author@example.com", "buyer-password-123", "Author Test")
	tickets := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID,
		"buyer-author@example.com", "Author Test", 1)
	enrolled := enrolledSigner(t, h, fx.orgID)

	// A stranger mints a perfectly valid, correctly signed op.
	stranger := newTestSigner(t)
	ledger, err := substrate.Open(t.Context(), substrate.Options{EventID: fx.eventID, Signer: stranger})
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer ledger.Close(t.Context()) //nolint:errcheck // test cleanup
	scannedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	rec, err := ledger.Fold(claimFromAdmission(store.AdmissionClaim{
		TicketID: tickets[0].ID, EventID: fx.eventID, GateID: "Ghost", DeviceID: "device-ghost",
		ScannedAt: scannedAt, Result: "admitted", ReportedResult: "admitted",
	}), scannedAt, time.Now())
	if err != nil {
		t.Fatalf("fold: %v", err)
	}

	body, err := json.Marshal(syncPushRequest{Ops: []syncOpWire{{
		EventID: fx.eventID, Cose: base64.StdEncoding.EncodeToString(rec.Cose),
	}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := peerRequest(t, h, http.MethodPost, "/api/sync/ops", enrolled, body)
	if got.Code != http.StatusOK {
		t.Fatalf("the request itself is legitimate: %d %s", got.Code, got.Body.String())
	}
	resp := decodeBody[syncPushResponse](t, got)
	if len(resp.Results) != 1 {
		t.Fatalf("expected one outcome, got %+v", resp.Results)
	}
	if resp.Results[0].Stored {
		t.Fatalf("an op authored by an unpinned key must not be stored: %+v", resp.Results[0])
	}
	if !strings.Contains(resp.Results[0].Reason, "not enrolled") {
		t.Fatalf("the refusal must name authorship as the reason, got %q", resp.Results[0].Reason)
	}

	var ops int
	if err := h.store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sync_op`).Scan(&ops); err != nil {
		t.Fatalf("count: %v", err)
	}
	if ops != 0 {
		t.Fatalf("a refused op left %d rows behind; refusals must change nothing", ops)
	}
}

// TestPushedOpForAnUnknownEventIsRefused pins down what two nodes that do NOT
// share an event look like. Cackle replicates admission claims and nothing else;
// a claim about an event this node has never heard of cannot be authorised
// against an organisation, so it is refused with a sentence an operator can act
// on rather than accepted into a corner of the database nobody reads.
func TestPushedOpForAnUnknownEventIsRefused(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "unknown-event")
	enrolled := enrolledSigner(t, h, fx.orgID)

	body, err := json.Marshal(syncPushRequest{Ops: []syncOpWire{{
		EventID: "01JZZZNOSUCHEVENT0000000000",
		Cose:    base64.StdEncoding.EncodeToString([]byte("irrelevant, never opened")),
	}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := peerRequest(t, h, http.MethodPost, "/api/sync/ops", enrolled, body)
	if got.Code != http.StatusOK {
		t.Fatalf("status: %d %s", got.Code, got.Body.String())
	}
	resp := decodeBody[syncPushResponse](t, got)
	if len(resp.Results) != 1 || resp.Results[0].Stored {
		t.Fatalf("expected one refused op, got %+v", resp.Results)
	}
	if !strings.Contains(resp.Results[0].Reason, "does not hold event") {
		t.Fatalf("the refusal must say the event is unknown here, got %q", resp.Results[0].Reason)
	}
}

// TestSyncPullPagesThroughTheWholeLog is a regression test for a pull that
// reported itself finished one page early.
//
// `complete` was derived from a page read with exactly `limit` rows, which cannot
// tell "the log ends here" from "the page is full". A caller stopped at the first
// full page and reported SUCCESS having never seen anything past it — the worst
// shape a sync bug can take, because nothing anywhere looks wrong. The page is
// now read with one row of slack, and that extra row is the only thing that knows
// the answer.
func TestSyncPullPagesThroughTheWholeLog(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "paging")
	enrolled := enrolledSigner(t, h, fx.orgID)

	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	for i := range 3 {
		if _, err := h.store.AppendSyncOp(t.Context(), store.SyncOp{
			OpID: fmt.Sprintf("op-%d", i), EventID: fx.eventID,
			Author: hex.EncodeToString(enrolled.Public()),
			// Distinct claim keys, so all three are distinct rows rather than
			// one row and two idempotent no-ops.
			ClaimTicket: fmt.Sprintf("ticket-%d", i), ClaimDevice: "device-A",
			ClaimScannedAt: at, Cose: []byte(fmt.Sprintf("envelope-%d", i)),
		}); err != nil {
			t.Fatalf("append op %d: %v", i, err)
		}
	}

	pull := func(after, limit int) syncOpsResponse {
		t.Helper()
		path := fmt.Sprintf("/api/sync/ops?after=%d&limit=%d", after, limit)
		rec := peerRequest(t, h, http.MethodGet, path, enrolled, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("pull %s: %d %s", path, rec.Code, rec.Body.String())
		}
		return decodeBody[syncOpsResponse](t, rec)
	}

	first := pull(0, 2)
	if len(first.Ops) != 2 {
		t.Fatalf("first page should hold the limit, got %d", len(first.Ops))
	}
	if first.Complete {
		t.Fatal("a full page with more behind it must not report itself complete")
	}
	if first.NextAfter != first.Ops[1].Seq {
		t.Fatalf("cursor is %d, want the last served seq %d", first.NextAfter, first.Ops[1].Seq)
	}

	second := pull(int(first.NextAfter), 2)
	if len(second.Ops) != 1 {
		t.Fatalf("second page should hold the remaining op, got %d", len(second.Ops))
	}
	if !second.Complete {
		t.Fatal("a partial page means the caller is caught up")
	}

	third := pull(int(second.NextAfter), 2)
	if len(third.Ops) != 0 || !third.Complete || third.NextAfter != second.NextAfter {
		t.Fatalf("an exhausted log must serve nothing and hold the cursor: %+v", third)
	}

	// A caller asking for more than the maximum gets the maximum, not an error.
	if got := pull(0, maxSyncPullLimit*10); len(got.Ops) != 3 {
		t.Fatalf("an oversized limit should be clamped, not refused: %d ops", len(got.Ops))
	}
}

// --- enrolment ---------------------------------------------------------------

func TestEnrolSyncPeerValidation(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "enrol")
	scannerToken, scannerID := h.signupUser("scanner-enrol@example.com", "scanner-password-123", "Scanner")
	if err := h.store.AddOrgMember(t.Context(),
		&store.OrgMember{OrgID: fx.orgID, UserID: scannerID, Role: "scanner"}); err != nil {
		t.Fatalf("add scanner: %v", err)
	}
	goodKey := hex.EncodeToString(bytes.Repeat([]byte{7}, ed25519.PublicKeySize))

	cases := []struct {
		name  string
		token string
		body  enrolPeerRequest
		want  int
	}{
		{"anonymous", "", enrolPeerRequest{OrgID: fx.orgID, PublicKey: goodKey}, http.StatusUnauthorized},
		{"scanner is not an owner", scannerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: goodKey}, http.StatusForbidden},
		{"short key", fx.ownerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: "abcd"}, http.StatusBadRequest},
		{"key is not hex", fx.ownerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: strings.Repeat("z", 64)}, http.StatusBadRequest},
		{"url with a path", fx.ownerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: goodKey, URL: "https://example.org/cackle"}, http.StatusBadRequest},
		{"url with credentials", fx.ownerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: goodKey, URL: "https://user:pw@example.org"}, http.StatusBadRequest},
		{"non-http scheme", fx.ownerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: goodKey, URL: "ftp://example.org"}, http.StatusBadRequest},
		{"owner enrols by key alone", fx.ownerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: goodKey}, http.StatusOK},
		{"the same pin twice is a conflict, not a re-pin", fx.ownerToken,
			enrolPeerRequest{OrgID: fx.orgID, PublicKey: goodKey, URL: "https://elsewhere.example"}, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(http.MethodPost, "/api/sync/peers", tc.token, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("want %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}

	// A node must not be able to enrol itself: it would replicate its own ops
	// back into its own log and report a mirror as a peer.
	rec := h.do(http.MethodGet, "/api/sync/status?org="+fx.orgID, fx.ownerToken, nil)
	self := decodeBody[syncStatusResponse](t, rec).Node
	rec = h.do(http.MethodPost, "/api/sync/peers", fx.ownerToken,
		enrolPeerRequest{OrgID: fx.orgID, PublicKey: self})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a node enrolling itself must be refused, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestNormalizePeerURL(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false}, // inbound-only peer: this node never dials it
		{"https://cackle.example.org", "https://cackle.example.org", false},
		{"https://cackle.example.org/", "https://cackle.example.org", false},
		{"http://10.0.0.4:8080", "http://10.0.0.4:8080", false},
		{"  https://cackle.example.org  ", "https://cackle.example.org", false},
		{"https://cackle.example.org/api", "", true},
		{"https://cackle.example.org?x=1", "", true},
		{"wss://cackle.example.org", "", true},
		{"https://", "", true},
		{"not a url", "", true},
	}
	for _, tc := range cases {
		got, err := normalizePeerURL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizePeerURL(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizePeerURL(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizePeerURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
