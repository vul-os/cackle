package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	kotvasync "github.com/vul-os/kotva/bindings/go"

	"github.com/vul-os/cackle/internal/scan"
	"github.com/vul-os/cackle/internal/scan/substrate"
	"github.com/vul-os/cackle/internal/store"
)

// The work behind the /api/sync routes: minting local claims into signed ops,
// verifying and storing a peer's, and running one bounded round against a peer.
//
// Everything here is bounded on purpose. A round pushes at most
// maxRoundPages × maxSyncOpsPerPush ops and pulls at most
// maxRoundPages × defaultSyncPullLimit, then stops and records where it got to.
// An unbounded round would be an unbounded write on the far side, which is the
// lesson /api/scan/sync already taught this codebase; and a round that never
// returns is one an operator cannot reason about.

const (
	// maxRoundPages bounds how many pages one round transfers in each direction.
	// Cursors are durable, so stopping early costs nothing but a second trigger.
	maxRoundPages = 8

	// peerRequestTimeout bounds one HTTP exchange with a peer. A cloud node that
	// has gone away must not hold an operator's request open.
	peerRequestTimeout = 30 * time.Second
)

// --- minting: local admissions -> signed ops -------------------------------

// mintPendingOps signs this node's not-yet-replicated admission claims into ops
// and appends them to the log. It returns how many it minted.
//
// It is idempotent and re-entrant by construction rather than by locking. An op's
// bytes are a pure function of the claim and this node's key, so minting the same
// claim twice produces the same content address and the same claim-key conflict,
// and AppendSyncOp keeps one row. Two concurrent rounds therefore duplicate work,
// never state.
//
// A claim the engine refuses is logged and SKIPPED rather than allowed to wedge
// the pass: one unfoldable row (a corrupt timestamp, a claim stamped in the
// future) must not stop every other gate's claims from reaching a peer. It stays
// un-minted and is retried on the next round, so nothing is quietly forgotten.
func (s *server) mintPendingOps(ctx context.Context, orgID string, signer kotvasync.Signer, limit int) (int, error) {
	rows, err := s.deps.Store.UnmintedAdmissions(ctx, orgID, limit)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	byEvent := make(map[string][]store.AdmissionClaim, 2)
	for _, row := range rows {
		byEvent[row.EventID] = append(byEvent[row.EventID], row)
	}

	now := time.Now()
	minted := 0
	for eventID, claims := range byEvent {
		ledger, err := s.syncLedger(ctx, eventID, signer)
		if err != nil {
			return minted, err
		}
		author := ledger.Author()
		for _, row := range claims {
			claim := claimFromAdmission(row)
			// receiverNow is the claim's own era: this is a replay of a stored
			// log. See substrate's package doc for why that makes the engine's
			// §3 check vacuous here and what replaces it.
			rec, err := ledger.Fold(claim, row.ScannedAt, now)
			if err != nil {
				s.log().Warn("sync: skipping a claim the merge engine refused",
					"event_id", eventID, "ticket_id", row.TicketID,
					"device_id", row.DeviceID, "err", err)
				continue
			}
			ok, err := s.deps.Store.AppendSyncOp(ctx, store.SyncOp{
				OpID:    rec.ID,
				EventID: eventID,
				Author:  author,
				// DeliveredBy is empty: this node minted it from its own
				// `admissions` table rather than receiving it from anyone.
				DeliveredBy:    "",
				ClaimTicket:    claim.TicketID,
				ClaimDevice:    claim.DeviceID,
				ClaimScannedAt: claim.ScannedAt,
				// Applied: this claim came OUT of the admissions table, so it is
				// by definition already represented there.
				Applied:   true,
				Cose:      rec.Cose,
				CreatedAt: now,
			})
			if err != nil {
				_ = ledger.Close(ctx)
				return minted, err
			}
			if ok {
				minted++
			}
		}
		if err := ledger.Close(ctx); err != nil {
			return minted, err
		}
	}
	return minted, nil
}

// --- ingest: a peer's ops -> this node's log and admissions table -----------

// ingestPeerOps verifies each op on its own, stores the ones it accepts, and
// tries to write their claims into the `admissions` table.
//
// The order of checks is the whole security argument, so it is spelled out:
//
//  1. The envelope must verify — signature, structure, namespace, op kind,
//     element/op stamp agreement, future-claim bound (substrate.VerifyOp).
//     Arriving over an authenticated connection buys an op nothing.
//  2. Its declared event must be one THIS node holds, and that event's
//     organisation must be one the calling peer is enrolled for. A peer cannot
//     write into an organisation it was not enrolled for by naming its event id.
//  3. Its AUTHOR must be a node key enrolled for that organisation (or this node
//     itself). Authorship is not transitively trusted: a claim relayed by an
//     enrolled peer but signed by a key nobody here pinned is refused. A mesh
//     converges when every node has enrolled every other node — a human
//     decision, made once per pair.
//  4. Only then is it merged, stored, and applied.
//
// Every refusal is per-op and named. One malformed op is no reason to discard the
// rest of the batch, which is evidence about a door.
func (s *server) ingestPeerOps(
	ctx context.Context,
	ops []syncOpWire,
	allowedOrgs map[string]struct{},
	deliveredBy string,
	signer kotvasync.Signer,
) ([]syncPushResult, error) {
	results := make([]syncPushResult, len(ops))
	if len(ops) == 0 {
		return results, nil
	}

	eventIDs := make([]string, 0, len(ops))
	for _, op := range ops {
		eventIDs = append(eventIDs, op.EventID)
	}
	eventOrgs, err := s.deps.Store.EventOrgIDs(ctx, eventIDs)
	if err != nil {
		return nil, err
	}

	ledgers := make(map[string]*substrate.Ledger, 2)
	defer func() {
		for _, l := range ledgers {
			_ = l.Close(ctx)
		}
	}()

	// This node's own key, so an op of ours that comes back to us (a peer
	// relaying it, or a round re-pushing) is recognised rather than refused as
	// "authored by a key not enrolled here" — a node is not enrolled with itself.
	selfKey := ""
	if signer != nil {
		selfKey = hex.EncodeToString(signer.Public())
	}

	type accepted struct {
		idx   int
		opID  string
		claim scan.QueuedAdmission
	}
	var toApply []accepted

	now := time.Now()
	for i, wire := range ops {
		cose, err := base64.StdEncoding.DecodeString(wire.Cose)
		if err != nil || len(cose) == 0 {
			results[i].Reason = "op envelope is not valid base64"
			continue
		}
		if wire.EventID == "" {
			results[i].Reason = "op is missing its event id"
			continue
		}
		orgID, known := eventOrgs[wire.EventID]
		if !known {
			// Honest and specific: this is what two nodes that do not share the
			// event look like, and it is a configuration fact an operator can
			// act on. Cackle does not replicate events or tickets.
			results[i].Reason = "this node does not hold event " + wire.EventID
			continue
		}
		if _, allowed := allowedOrgs[orgID]; !allowed {
			results[i].Reason = "not enrolled for the organisation that owns event " + wire.EventID
			continue
		}

		ledger, ok := ledgers[wire.EventID]
		if !ok {
			ledger, err = s.syncLedger(ctx, wire.EventID, signer)
			if err != nil {
				return nil, err
			}
			ledgers[wire.EventID] = ledger
		}

		v, err := ledger.VerifyOp(cose, now)
		if err != nil {
			// substrate's messages describe the protocol, not this node's
			// internals, so they are safe and useful to return.
			results[i].Reason = err.Error()
			continue
		}
		results[i].OpID = v.ID

		if v.Author != selfKey {
			enrolled, err := s.deps.Store.IsEnrolledNodeKey(ctx, orgID, v.Author)
			if err != nil {
				return nil, err
			}
			if !enrolled {
				results[i].Reason = "op is authored by a node key that is not enrolled here"
				continue
			}
		}

		// Merge it through the engine before storing it, so an op this node keeps
		// is an op this node's own replica accepted — the same discipline Fold
		// uses when it admits its own op through the ingest path.
		if _, err := ledger.IngestVerified(v, v.Claim.ScannedAt); err != nil {
			results[i].Reason = err.Error()
			continue
		}

		stored, err := s.deps.Store.AppendSyncOp(ctx, store.SyncOp{
			OpID:           v.ID,
			EventID:        v.EventID,
			Author:         v.Author,
			DeliveredBy:    deliveredBy,
			ClaimTicket:    v.Claim.TicketID,
			ClaimDevice:    v.Claim.DeviceID,
			ClaimScannedAt: v.Claim.ScannedAt,
			Applied:        false,
			Cose:           v.Cose,
			CreatedAt:      now,
		})
		if err != nil {
			return nil, err
		}
		results[i].Stored = stored
		toApply = append(toApply, accepted{idx: i, opID: v.ID, claim: v.Claim})
	}

	if len(toApply) == 0 {
		return results, nil
	}

	// One transaction for the whole batch, through the SAME sink POST
	// /api/scan/sync uses. That matters more than it looks: dbSyncSink is where
	// the single-admitted-row downgrade happens and where `reported_result`
	// preserves the device's own verdict, and a second insert path here would be
	// a second reconciliation policy that could disagree with it.
	sink := &dbSyncSink{db: s.deps.Store.DB()}
	batch := make([]scan.QueuedAdmission, 0, len(toApply))
	for _, a := range toApply {
		batch = append(batch, a.claim)
	}
	if _, err := sink.Apply(ctx, batch); err != nil {
		return nil, err
	}

	// Read presence back rather than trusting Apply's per-item bools: those
	// report "this call inserted a row", and a claim that was ALREADY in the
	// table is just as applied as one inserted a microsecond ago.
	for _, a := range toApply {
		present, err := s.admissionPresent(ctx, a.claim)
		if err != nil {
			return nil, err
		}
		results[a.idx].Applied = present
		if present {
			if err := s.deps.Store.MarkSyncOpApplied(ctx, a.opID); err != nil {
				return nil, err
			}
		}
	}
	return results, nil
}

// admissionPresent reports whether a claim is represented in the `admissions`
// table, keyed on the same (ticket_id, device_id, scanned_at) idempotency key
// every other sync path uses.
func (s *server) admissionPresent(ctx context.Context, a scan.QueuedAdmission) (bool, error) {
	var n int
	err := s.deps.Store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admissions WHERE ticket_id = ? AND device_id = ? AND scanned_at = ?`,
		a.TicketID, a.DeviceID, nowText(a.ScannedAt)).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// --- one round against one peer --------------------------------------------

// syncRoundResult is what a triggered round reports back to the operator who
// triggered it.
type syncRoundResult struct {
	PeerID string `json:"peer_id"`
	// Peer is the peer's pinned key. Reported so an operator can see WHICH key
	// answered — or, on a refusal, that the pin is unchanged.
	Peer string `json:"peer"`
	// Minted is how many local admission claims were signed into ops this round.
	Minted int `json:"minted"`
	// Pushed is how many ops the peer accepted as new; PushHeld how many it
	// already had; PushRefused how many it refused, per-op and by its own rules.
	Pushed      int `json:"pushed"`
	PushHeld    int `json:"push_held"`
	PushRefused int `json:"push_refused"`
	// Pulled is how many ops arrived; Stored how many were new here; PullRefused
	// how many THIS node refused after verifying them on their own.
	Pulled      int `json:"pulled"`
	Stored      int `json:"stored"`
	PullRefused int `json:"pull_refused"`
	// Complete is false when a round hit its page bound with work left. It is not
	// an error: trigger another round.
	Complete   bool   `json:"complete"`
	PullCursor int64  `json:"pull_cursor"`
	PushCursor int64  `json:"push_cursor"`
	Error      string `json:"error,omitempty"`
	Caveat     string `json:"caveat"`
}

// replicateWithPeer runs one bounded round: mint, push, pull, record.
//
// It never returns an error to its caller — a peer that is unreachable, that
// answers with the wrong key, or that speaks a different algebra is an OUTCOME an
// operator needs reported, not a 500. Whatever succeeded before the failure is
// kept (cursors only move forward), and the failure is written to the peer row so
// it is still visible after the response is gone.
func (s *server) replicateWithPeer(ctx context.Context, p store.SyncPeer) syncRoundResult {
	out := syncRoundResult{
		PeerID: p.ID, Peer: p.PublicKey,
		PullCursor: p.PullCursor, PushCursor: p.PushCursor,
		Complete: true, Caveat: syncCaveat,
	}

	signer, _, err := s.syncIdentity(ctx)
	if err != nil {
		out.Error = "this node has no usable replication identity"
		s.log().Error("sync: identity unavailable", "peer_id", p.ID, "err", err)
		return out
	}

	minted, err := s.mintPendingOps(ctx, p.OrgID, signer, maxSyncMintPerRequest)
	out.Minted = minted
	if err != nil {
		out.Error = "could not mint local claims into ops"
		s.log().Error("sync: mint failed", "peer_id", p.ID, "err", err)
		s.recordRoundOutcome(ctx, p, out)
		return out
	}

	client := &http.Client{
		Timeout: peerRequestTimeout,
		// A redirect would move the request to a path or host the signature does
		// not cover, so the far side would refuse it anyway — and following one to
		// another host would hand this node's signed envelope to a stranger.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("peer replied with a redirect; a peer URL must be the node's own origin")
		},
	}

	if err := s.pushToPeer(ctx, client, p, signer, &out); err != nil {
		out.Error = err.Error()
		s.recordRoundOutcome(ctx, p, out)
		return out
	}
	if err := s.pullFromPeer(ctx, client, p, signer, &out); err != nil {
		out.Error = err.Error()
		s.recordRoundOutcome(ctx, p, out)
		return out
	}
	s.recordRoundOutcome(ctx, p, out)
	return out
}

func (s *server) recordRoundOutcome(ctx context.Context, p store.SyncPeer, out syncRoundResult) {
	status := fmt.Sprintf("pushed %d, pulled %d, stored %d", out.Pushed, out.Pulled, out.Stored)
	if out.PushRefused > 0 || out.PullRefused > 0 {
		status += fmt.Sprintf(", refused %d out / %d in", out.PushRefused, out.PullRefused)
	}
	if out.Error != "" {
		status = "refused: " + out.Error
	}
	if err := s.deps.Store.AdvanceSyncPeerCursors(ctx, p.ID, out.PullCursor, out.PushCursor, time.Now(), status); err != nil {
		s.log().Error("sync: could not record round outcome", "peer_id", p.ID, "err", err)
	}
}

// pushToPeer hands this node's ops to the peer, page by page.
//
// The cursor advances past ops the peer REFUSED as well as ops it took, and that
// is a deliberate trade with a stated recovery path. Stopping at a refusal would
// wedge the cursor forever on one op the peer will never accept (a claim about an
// event it does not hold), and replication would stop for every other event too.
// Refusals are counted, reported, and written to the peer row; replaying them
// means deleting and re-enrolling the peer, which resets the cursors to zero.
func (s *server) pushToPeer(ctx context.Context, client *http.Client, p store.SyncPeer,
	signer kotvasync.Signer, out *syncRoundResult) error {

	for page := 0; page < maxRoundPages; page++ {
		ops, err := s.deps.Store.SyncOpsForOrgAfter(ctx, p.OrgID, out.PushCursor, maxSyncOpsPerPush)
		if err != nil {
			s.log().Error("sync: reading ops to push", "peer_id", p.ID, "err", err)
			return errors.New("could not read this node's op log")
		}
		if len(ops) == 0 {
			return nil
		}

		req := syncPushRequest{Ops: make([]syncOpWire, 0, len(ops))}
		for _, op := range ops {
			req.Ops = append(req.Ops, syncOpWire{
				EventID: op.EventID,
				Cose:    base64.StdEncoding.EncodeToString(op.Cose),
			})
		}
		var resp syncPushResponse
		if err := s.callPeer(ctx, client, p, signer, http.MethodPost, "/api/sync/ops", nil, req, &resp); err != nil {
			return err
		}
		if resp.Algebra != substrate.AlgebraName {
			return fmt.Errorf("peer merges under algebra %q, this node speaks %q", resp.Algebra, substrate.AlgebraName)
		}
		// Checked BEFORE the outcomes are counted: per-op results are positional,
		// so a peer answering with a different number of them is not one whose
		// bools can be matched to anything, and counting them first would file
		// some other op's outcome against this one.
		if len(resp.Results) != len(req.Ops) {
			return fmt.Errorf("peer reported %d outcomes for %d ops", len(resp.Results), len(req.Ops))
		}
		for _, r := range resp.Results {
			switch {
			case r.Reason != "":
				out.PushRefused++
			case r.Stored:
				out.Pushed++
			default:
				out.PushHeld++
			}
		}
		out.PushCursor = ops[len(ops)-1].Seq

		if len(ops) < maxSyncOpsPerPush {
			return nil
		}
		if page == maxRoundPages-1 {
			out.Complete = false
		}
	}
	return nil
}

// pullFromPeer fetches the peer's ops, page by page, and ingests each one through
// the same verification a pushed op gets.
func (s *server) pullFromPeer(ctx context.Context, client *http.Client, p store.SyncPeer,
	signer kotvasync.Signer, out *syncRoundResult) error {

	allowed := map[string]struct{}{p.OrgID: {}}
	for page := 0; page < maxRoundPages; page++ {
		query := map[string]string{
			"after": fmt.Sprintf("%d", out.PullCursor),
			"limit": fmt.Sprintf("%d", defaultSyncPullLimit),
		}
		var resp syncOpsResponse
		if err := s.callPeer(ctx, client, p, signer, http.MethodGet, "/api/sync/ops", query, nil, &resp); err != nil {
			return err
		}
		if resp.Algebra != substrate.AlgebraName {
			return fmt.Errorf("peer merges under algebra %q, this node speaks %q", resp.Algebra, substrate.AlgebraName)
		}
		if len(resp.Ops) == 0 {
			if resp.NextAfter > out.PullCursor {
				out.PullCursor = resp.NextAfter
			}
			return nil
		}

		results, err := s.ingestPeerOps(ctx, resp.Ops, allowed, p.PublicKey, signer)
		if err != nil {
			s.log().Error("sync: ingesting pulled ops", "peer_id", p.ID, "err", err)
			return errors.New("could not store the ops this peer sent")
		}
		for i, r := range results {
			out.Pulled++
			switch {
			case r.Reason != "":
				out.PullRefused++
				s.log().Warn("sync: refused a pulled op",
					"peer_id", p.ID, "event_id", resp.Ops[i].EventID, "reason", r.Reason)
			case r.Stored:
				out.Stored++
			}
		}
		if resp.NextAfter > out.PullCursor {
			out.PullCursor = resp.NextAfter
		}
		if resp.Complete {
			return nil
		}
		if page == maxRoundPages-1 {
			out.Complete = false
		}
	}
	return nil
}

// callPeer performs one signed request against a peer and verifies the signed
// answer.
//
// The response signature is checked against the PINNED key. A node answering at
// the right address under a different key is refused here, and nothing is
// re-pinned, nothing is stored, and no cursor moves — the operator gets a refusal
// naming both keys. That is the discipline the whole peer model rests on: a key
// change is a refusal, never a silent re-pin.
func (s *server) callPeer(ctx context.Context, client *http.Client, p store.SyncPeer,
	signer kotvasync.Signer, method, path string, query map[string]string, in, outBody any) error {

	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return errors.New("could not encode the request for this peer")
		}
		body = b
	}

	target := p.URL + path
	if len(query) > 0 {
		first := true
		for _, k := range []string{"after", "limit"} { // fixed order: the signature covers the raw query
			v, ok := query[k]
			if !ok {
				continue
			}
			if first {
				target += "?"
				first = false
			} else {
				target += "&"
			}
			target += k + "=" + v
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, peerRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return errors.New("peer URL is not usable")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	nonce, err := substrate.SignRequest(req, signer, body, time.Now())
	if err != nil {
		return errors.New("could not sign the request for this peer")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("peer is unreachable: %s", cleanDialError(err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSyncBody+1))
	if err != nil {
		return errors.New("could not read the peer's answer")
	}
	if len(respBody) > maxSyncBody {
		return fmt.Errorf("peer's answer exceeds %d bytes", maxSyncBody)
	}
	if resp.StatusCode != http.StatusOK {
		// A non-200 is not signed (error responses go through the ordinary error
		// helpers), so it is read as a REPORT and never as data. The worst a
		// forged one can do is make this round report a failure.
		return fmt.Errorf("peer answered %d: %s", resp.StatusCode, peerErrorMessage(respBody))
	}
	if err := substrate.VerifyResponse(resp.Header, p.PublicKey, nonce, respBody, time.Now()); err != nil {
		return err
	}
	if outBody != nil {
		if err := json.Unmarshal(respBody, outBody); err != nil {
			return errors.New("peer's answer is not the expected JSON")
		}
	}
	return nil
}

// peerErrorMessage pulls the message out of Cackle's standard error envelope, or
// falls back to a short, bounded excerpt. A peer's error text is data from
// another machine: it is length-capped before it goes anywhere near a log or a
// response.
func peerErrorMessage(body []byte) string {
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Message != "" {
		return truncate(env.Error.Message, 200)
	}
	return truncate(string(body), 200)
}

// cleanDialError keeps a transport failure readable without pasting a Go error
// chain (which carries the full URL, including the query, into operator-facing
// text) verbatim.
func cleanDialError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return truncate(urlErr.Err.Error(), 160)
	}
	return truncate(err.Error(), 160)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
