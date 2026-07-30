// Package substrate expresses Cackle's cross-gate admission ledger in the
// shared DMTAP Sync algebra (KOTVA `substrate/SYNC.md`, capability ③), so the
// rule by which two gates' admission logs come back together is the suite's
// rather than a second one written here.
//
// The engine is `github.com/vul-os/kotva/bindings/go` — the same compiled Rust
// core SlipScan, BeepBite, FlowStock and Diwan run, executed under wazero.
// wazero is pure Go, so CGO_ENABLED=0 and single-static-binary
// cross-compilation both survive; Cackle's measured cost is written down in
// docs/OFFLINE-GATES.md.
//
// # What this package is NOT
//
// It is not in the admission path, and it must never become so. A gate's
// decision to open the door is `internal/scan.Decide` /
// `DecideWithBundle`: local Ed25519 verification against a pinned key ring,
// a pinned ticket index, and a local dedupe claim. No network, no WebAssembly,
// no this-package. `internal/scan` does not import this package and cannot —
// the dependency runs one way only, which is what keeps a bare offline gate
// binary free of the engine entirely. A WASM call on the hot path would put
// several hundred milliseconds of first-call module compilation between an
// attendee presenting a QR code and a door opening, in exchange for nothing:
// the decision it would inform is one this replica already has the data to
// make locally.
//
// Reconciliation is a background concern and this is where it lives.
//
// # What is achievable here, and what is not
//
// Two gates partitioned from each other CANNOT be prevented from admitting the
// same ticket twice. That needs coordination they do not have, and no CRDT
// creates coordination — a merge rule decides what a set contains once the
// messages arrive, never what happened before they did. Nothing in this
// package, and nothing in Cackle's documentation, should be read as claiming
// otherwise; a venue that planned around a prevention guarantee that cannot
// exist would be worse off than one that planned around the real one.
//
// What this package does provide:
//
//   - each gate admits on its own local view, offline, immediately (that is
//     internal/scan's job and it already did it);
//   - the two views CONVERGE when the gates can talk again — union merge,
//     nothing dropped, order-independent, and the same answer whichever gate
//     you ask;
//   - a double admission that slipped through during the partition is
//     SURFACED afterwards, as two surviving claims on one ticket, rather than
//     collapsed into one.
//
// # The mapping, and the selection test for each choice
//
// KOTVA `substrate/SYNC.md` §4.10 requires an implementation to write down,
// per modelled object, which primitive it chose and how it answered the
// selection test. Cackle models exactly one thing here.
//
// ## The admission ledger → §4.3 OR-Set (kind set_add)
//
//	ns      the event id
//	target  "admission/<ticket_id>"
//	value   bstr, wall(8) ‖ counter(4) ‖ deviceTag(32) ‖ canonical claim JSON
//
// An admission is an immutable fact: at this moment, this device, at this gate,
// let this ticket through (or refused it). Facts are added and never retracted,
// merge is plain union, and the interesting read — "how many devices believed
// they admitted this ticket?" — is a count over that union at read time, never
// a stored counter. §4.3 is the faithful primitive and the selection test is
// answered without ambiguity:
//
//   - §4.4 (LWW register) is WRONG here and wrong in the exact direction that
//     matters. A register keeps one writer's value and discards the other's,
//     so the second gate's admission — the one that describes an extra human
//     being inside the venue — would converge to invisible on every replica,
//     with no error anywhere. That is the whole failure this package exists to
//     prevent.
//
//   - §4.5 (death certificate) has nothing to model: an admission is never
//     un-made. There is no operation, and no user action, that retracts "this
//     person walked through the door".
//
//   - §4.6 (PN-counter) would converge on the right admitted TOTAL and throw
//     away which gates and which devices produced it, which is the entire
//     audit. §4.6's own closing note permits exactly the shape used here: a
//     counter that is a sum of immutable facts MAY be modelled as set-add of an
//     immutable record with a read-side count.
//
// No set-remove is ever minted. An OR-Set with no removes is a grow-only set
// whose merge is plain union, which is precisely the append-only semantics the
// `admissions` table already has.
//
// ## Why the element carries a stamp, and why getting this wrong is silent
//
// §4.3 identifies an element BY ITS VALUE. Two adds carrying identical bytes
// are one element carrying two add-tags. BeepBite learned this the expensive
// way: two identical stock movements collapsed into one and turned a −2 into a
// −1, converged and silent on every replica.
//
// Cackle's exposure is the inverse and worse. The two claims this package
// exists to surface are claims about THE SAME TICKET — so if the element were
// just the ticket id, or just "ticket admitted", the union would collapse them
// into one element and hide exactly the duplicate being looked for. The report
// would come back clean while two people were inside on one ticket. A
// prevention guarantee Cackle never made would have been replaced by a
// detection guarantee it did make and silently broke.
//
// So the element carries the scanning device and the scan time, and both
// survive the merge as distinct elements. Concretely:
//
//   - wall is the claim's own ScannedAt in milliseconds — NOT a fresh clock
//     reading. This is load-bearing for idempotence: folding the same stored
//     admission row twice must produce byte-identical element bytes, hence the
//     same §4.1 content address, hence one element. A clock tick per fold
//     would make every re-read of the log grow the set.
//
//   - counter is always 0. Cackle has no HLC counter to carry: a claim's
//     identity is (ticket_id, device_id, scanned_at), which is already exactly
//     the (ticket_id, device_id, scanned_at) idempotency key
//     `POST /api/scan/sync` has always been keyed on. Two scans by one device
//     of one ticket in one millisecond ARE the same scan event by that
//     contract, and collapsing them is correct rather than lossy.
//
//   - deviceTag is SHA-256(device_id), fixed width so the framing needs no
//     length prefix. It is a TAG, not a key, and the distinction is not
//     cosmetic — see "What a claim's authorship does and does not prove"
//     below.
//
// The three leading fields are fixed-width, so the payload boundary is
// unambiguous, and readback re-derives deviceTag from the payload's own
// device_id and refuses a mismatch rather than trusting either half.
//
// # What a claim's authorship does and does not prove
//
// Every op this package mints is signed by THIS REPLICA's key, and the §3
// author is this replica. A Cackle gate is a browser holding a device id in
// localStorage; it has no keypair and cannot sign anything. So:
//
//	device_id is REPORTED DATA, not an authenticated identity.
//
// A gate that lies about its device_id, or an attacker who can reach
// `POST /api/scan/sync` with a scanner's credentials, can attribute a claim to
// another device. What the signature on a ledger op attests is narrower and
// should not be read as more: that THIS replica recorded this claim, and that
// the claim has not been altered since. Attributing an admission to a physical
// scanner rests on the RBAC check on the sync route and on operational control
// of the devices — not on cryptography. Per-gate keys would change that, and
// they are not built.
//
// # Replay, and the §3 skew check
//
// Because wall comes from the claim rather than from now, every op this package
// mints is stamped in the past — usually hours in the past, since a
// reconciliation report is read after an event. The engine's §3 skew check
// refuses an op whose HLC sits more than its published bound from the
// RECEIVER's clock, and it takes that receiver reading as a parameter. Fold
// and Ingest therefore take a `receiverNow` for the caller to supply the log's
// own era, exactly as replaying any stored log must.
//
// Stated plainly: that makes the engine's skew check vacuous for a replayed
// claim. It is not the check doing the work here, so this package applies the
// bound the replay path still needs — a claim stamped in the FUTURE relative
// to the caller's real clock is refused before it reaches the engine. That is
// the poisoning direction that matters: HLC walls are monotonic
// non-decreasing, so one far-future claim, once folded, out-ranks every honest
// claim in every later comparison and can never come back down.
//
// # Server-to-server replication
//
// Two Cackle servers exchange ops over the transport in peerauth.go and
// internal/httpapi's `/api/sync` routes: `Fold` mints the envelope a peer
// receives, `VerifyOp` is how the receiving node checks one on its own before
// anything is stored, and `IngestVerified` merges it. `internal/store`'s
// `sync_op` table is the durable log the transport pages through. See
// docs/CLUSTERING.md.
//
// Replication does not change what is achievable, and this is the sentence to
// keep: it makes a cross-gate double admission VISIBLE SOONER and on MORE
// NODES. It still cannot prevent one. The gates that produced the duplicate
// could not see each other at the moment of the scan, and nothing that happens
// afterwards — no merge rule, no transport, no number of nodes — reaches back to
// that moment.
package substrate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	kotvasync "github.com/vul-os/kotva/bindings/go"

	"github.com/vul-os/cackle/internal/scan"
)

// AlgebraName is the merge algebra a Cackle deployment running this package
// advertises to a peer.
//
// It is versioned and it is checked rather than assumed, because two replicas
// that merge under different total orders converge to different states and
// both report success. It travels on every `/api/sync` response, and a peer
// answering with a name this node does not speak is refused rather than merged
// on the hope that the rules match.
const AlgebraName = "dmtap-sync-v0"

// SkewRefusalCode is the §12 registry code the engine returns when an op's HLC
// sits further from the receiver's clock than §3 allows.
//
// Named rather than matched on message text: a fail-closed path matched on
// prose eventually takes the wrong branch when the prose is reworded.
const SkewRefusalCode = "0x0A05"

// DefaultMaxFutureSkew bounds how far ahead of the caller's real clock a
// claim's ScannedAt may sit before Fold and Ingest refuse it.
//
// This is Cackle's bound, not the engine's, and it exists because the engine's
// own §3 check is vacuous on the replay path this package uses — see the
// package doc. Five minutes is generous enough to absorb ordinary clock skew
// on a venue's scanner tablets (docs/OFFLINE-GATES.md already tells operators
// their gate clocks must be right, because `nbf`/`exp` verification depends on
// it) and far tighter than the "permanently poisons every later comparison"
// failure it is there to stop.
const DefaultMaxFutureSkew = 5 * time.Minute

// ErrFutureClaim is returned when a claim is stamped further in the future
// than MaxFutureSkew allows. It is a refusal, not a warning: nothing is folded.
var ErrFutureClaim = errors.New("substrate: claim is stamped in the future")

// targetPrefix addresses one ticket's admission ledger.
//
// A ticket id containing the separator would make the split ambiguous on the
// way back, so Fold refuses one rather than round-tripping to a different
// address. Ticket ids are ULIDs (store.NewID), so this has never been
// reachable — which is the reason to check it now rather than to assume it
// stays that way.
const targetPrefix = "admission/"

func targetOf(ticketID string) string { return targetPrefix + ticketID }

func ticketOf(target string) (string, error) {
	id, ok := strings.CutPrefix(target, targetPrefix)
	if !ok || id == "" {
		return "", fmt.Errorf("substrate: %q is not an admission target", target)
	}
	return id, nil
}

// Options configures a Ledger.
type Options struct {
	// EventID is the §7 namespace. Scoping by event means a claim for another
	// event is not merely filtered by a WHERE clause — it lands in a different
	// namespace in the algebra, and §7 refuses a cross-namespace reference
	// rather than merging it. Required.
	EventID string

	// Signer holds this replica's Ed25519 key. Required. The key never crosses
	// into the WebAssembly module: the engine emits the RFC 9052
	// Sig_structure, this signs it where the key lives, and the engine
	// verifies the result before it will assemble an envelope.
	//
	// Use NewEphemeralSigner for a derived, single-replica reconciliation view
	// (which is what Cackle's reporting path is today) — see its doc for why
	// that is sufficient there and what it is not sufficient for.
	Signer kotvasync.Signer

	// MaxFutureSkew overrides DefaultMaxFutureSkew. Zero means the default.
	MaxFutureSkew time.Duration

	// CacheDir, if set, persists wazero's compiled machine code so a restart
	// does not pay module compilation again. Optional; compilation is a few
	// hundred milliseconds, once per process.
	CacheDir string
}

// NewEphemeralSigner returns a Signer over a freshly generated Ed25519 key
// that is never written anywhere.
//
// This is the right choice for Cackle's reconciliation view TODAY, and saying
// why matters more than the convenience. That view is DERIVED: it is folded on
// demand out of the `admissions` table, which is the authoritative local log,
// and it is discarded when the report is served. Nothing persists an envelope,
// no peer ever receives one, and no other replica needs to recognise the
// author — so a durable node identity would be key material to protect on an
// internet-facing host in exchange for no property anybody reads.
//
// It is NOT sufficient for a server-to-server mesh, and must never be used for
// one. Two Cackle instances exchanging admission claims need stable,
// mutually-known node keys, because then the author IS load-bearing: it is how a
// replica decides whether an envelope came from a peer whose key an operator
// pinned. An op minted under an ephemeral key is unattributable by
// construction — every peer would refuse it, and correctly. Replication uses
// NewSigner over the durable identity in `store.NodeIdentity`.
//
// What survives an ephemeral identity, verified rather than assumed (see
// TestEphemeralSignerProducesStableClaims): op IDs differ between folds,
// because the author is part of the op's canonical bytes and therefore part of
// its §4.1 content address. Claims, Conflicts AND StateRoot do not. Every
// element field is derived from the claim itself, and the §6.1 observable state
// is over the merged elements rather than over the add-tags that carry them —
// so two folds of the same table under two different keys agree on the root
// byte for byte. That is why StateRoot is usable as a convergence check here at
// all.
func NewEphemeralSigner() (kotvasync.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("substrate: generating an ephemeral identity: %w", err)
	}
	return kotvasync.InMemorySigner{PrivateKey: priv}, nil
}

// NewSigner wraps a node's durable Ed25519 identity as a Signer.
//
// This is the identity replication runs under: the key a peer's operator pinned
// by hand, and the key every op this node publishes is signed by. Unlike
// NewEphemeralSigner, the author it produces is load-bearing — it is what makes
// a claim attributable to a node another operator decided to trust.
//
// The key stays on the Go side of the WebAssembly boundary; the engine receives
// a signature and never a seed (see kotvasync.Signer's doc for why that is not a
// stylistic choice). The size check is here rather than at first use because a
// short key produces signatures that verify nowhere, and the useful place to
// learn that is at startup rather than mid-round on a peer's ingest path.
func NewSigner(priv ed25519.PrivateKey) (kotvasync.Signer, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("substrate: node private key is %d bytes, want %d",
			len(priv), ed25519.PrivateKeySize)
	}
	return kotvasync.InMemorySigner{PrivateKey: priv}, nil
}

// Compiler is one compiled copy of the engine module, shared by every Ledger
// opened from it.
//
// Compiling the WebAssembly module is by far the most expensive step — a few
// hundred milliseconds — and it is per-process work, not per-event work. A
// server that reconciles one event per request must not pay it per request, and
// a Ledger is per-event because the §7 namespace is the event id. Compiler is
// the split between those two lifetimes: hold one for the process's life, open
// a Ledger per event from it.
//
// It is safe for concurrent use; the binding's Runtime is.
type Compiler struct {
	rt *kotvasync.Runtime
}

// NewCompiler compiles the engine once.
//
// cacheDir, if non-empty, persists wazero's compiled machine code so a restart
// costs milliseconds instead of a few hundred. It is optional and a missing or
// unwritable directory is not fatal to correctness — only to startup speed.
func NewCompiler(ctx context.Context, cacheDir string) (*Compiler, error) {
	var opts []kotvasync.Option
	if cacheDir != "" {
		opts = append(opts, kotvasync.WithCompilationCacheDir(cacheDir))
	}
	rt, err := kotvasync.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("substrate: compiling the engine: %w", err)
	}
	return &Compiler{rt: rt}, nil
}

// Close releases the compiled module. Ledgers opened from this Compiler must be
// closed first.
func (c *Compiler) Close(ctx context.Context) error {
	if c.rt == nil {
		return nil
	}
	rt := c.rt
	c.rt = nil
	return rt.Close(ctx)
}

// Ledger is one replica of the admission ledger: one instance, one merged set,
// one event.
//
// Every method takes the mutex. A wazero instance's linear memory is shared
// mutable state and the binding's contract is that an Instance is correct but
// serialized, so this is the binding's own concurrency model made explicit
// rather than a lock invented on top of one.
type Ledger struct {
	mu  sync.Mutex
	rt  *kotvasync.Runtime // nil unless this Ledger owns its runtime (see Open)
	in  *kotvasync.Instance
	eng *kotvasync.Engine

	signer  kotvasync.Signer
	author  string // this replica's key, lowercase hex — the §3 author
	ns      string
	setAdd  uint8
	maxSkew time.Duration

	folded   int
	ingested int
	refused  int
}

// Open compiles the engine and creates a standalone replica that owns it.
//
// Convenient for a one-shot caller and for tests. A long-lived server should
// use NewCompiler once and Compiler.Ledger per event instead, so module
// compilation is paid once for the process rather than once per event.
func Open(ctx context.Context, opt Options) (*Ledger, error) {
	c, err := NewCompiler(ctx, opt.CacheDir)
	if err != nil {
		return nil, err
	}
	l, err := c.Ledger(ctx, opt)
	if err != nil {
		_ = c.Close(ctx)
		return nil, err
	}
	// This Ledger owns the runtime, so closing it closes the module too.
	l.rt = c.rt
	return l, nil
}

// Ledger creates a replica for one event on the already-compiled engine.
//
// Options.CacheDir is ignored here — compilation already happened in
// NewCompiler, which is the only place a cache directory can matter.
func (c *Compiler) Ledger(ctx context.Context, opt Options) (*Ledger, error) {
	if c.rt == nil {
		return nil, errors.New("substrate: the compiler is closed")
	}
	if opt.EventID == "" {
		return nil, errors.New("substrate: an event id is required as the namespace")
	}
	if opt.Signer == nil {
		return nil, errors.New("substrate: a signer is required")
	}
	pub := opt.Signer.Public()
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("substrate: signer public key is %d bytes, want %d",
			len(pub), ed25519.PublicKeySize)
	}
	maxSkew := opt.MaxFutureSkew
	if maxSkew <= 0 {
		maxSkew = DefaultMaxFutureSkew
	}

	in, err := c.rt.Instance(ctx)
	if err != nil {
		return nil, fmt.Errorf("substrate: instantiating the engine: %w", err)
	}

	l := &Ledger{
		in:      in,
		signer:  opt.Signer,
		author:  hex.EncodeToString(pub),
		ns:      opt.EventID,
		maxSkew: maxSkew,
	}
	if l.eng, err = in.NewEngine(); err != nil {
		_ = l.Close(ctx)
		return nil, fmt.Errorf("substrate: creating the replica: %w", err)
	}
	// Never hard-code the §4.2 kind numbers — ask the engine which is which.
	// set_add happens to be 1 in the linked engine and that is exactly the
	// sort of coincidence a hard-coded constant survives until it doesn't.
	k, err := in.OpKinds()
	if err != nil {
		_ = l.Close(ctx)
		return nil, fmt.Errorf("substrate: reading op kinds: %w", err)
	}
	l.setAdd = k.SetAdd

	return l, nil
}

// Close releases this replica.
//
// It closes the compiled module only when this Ledger owns it — i.e. when it
// came from Open rather than from Compiler.Ledger. Closing one event's replica
// must never take the shared module away from another event's.
func (l *Ledger) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.eng != nil {
		_ = l.eng.Close()
		l.eng = nil
	}
	if l.in != nil {
		_ = l.in.Close(ctx)
		l.in = nil
	}
	if l.rt != nil {
		rt := l.rt
		l.rt = nil
		return rt.Close(ctx)
	}
	return nil
}

// Author returns this replica's §3 author key, lowercase hex.
func (l *Ledger) Author() string { return l.author }

// AlgebraVersion reports the engine revision and §3 skew bound the linked
// engine speaks, read from the engine rather than written down here.
func (l *Ledger) AlgebraVersion() (kotvasync.Version, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.in.Version()
}

// Record is one folded claim: the signed envelope that replicates it, and the
// §4.1 content address the engine identifies it by.
type Record struct {
	// ID is the §4.1 content address, lowercase hex.
	ID string
	// Cose is the COSE_Sign1 envelope — the replicable unit.
	Cose []byte
}

// Fold admits one locally-held admission claim into this replica.
//
// It is idempotent by construction: every field of the resulting op is derived
// from the claim, so folding the same claim twice produces byte-identical
// bytes, the same content address, and one element. That is what lets the
// reconciliation view be recomputed from the `admissions` table on every
// request without the set growing.
//
// receiverNow is the clock reading the engine's §3 skew check is measured
// against. Pass the claim's own era when replaying a stored log — which is
// what folding the admissions table is. See the package doc for why that makes
// the engine's check vacuous here and what replaces it.
//
// now is the caller's REAL clock, used for the one bound that still bites: a
// claim stamped further than MaxFutureSkew ahead of it is refused with
// ErrFutureClaim before anything is minted, so a future-stamped claim cannot
// poison this replica's ordering.
func (l *Ledger) Fold(a scan.QueuedAdmission, receiverNow, now time.Time) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if a.ScannedAt.After(now.Add(l.maxSkew)) {
		l.refused++
		return Record{}, fmt.Errorf("%w: %s is more than %s ahead of %s (ticket %s, device %s)",
			ErrFutureClaim, a.ScannedAt.UTC(), l.maxSkew, now.UTC(), a.TicketID, a.DeviceID)
	}

	sop, err := l.syncOp(a)
	if err != nil {
		l.refused++
		return Record{}, err
	}
	raw, err := l.in.EncodeOp(sop)
	if err != nil {
		l.refused++
		return Record{}, fmt.Errorf("substrate: encoding claim: %w", err)
	}
	id, err := l.in.OpID(raw)
	if err != nil {
		l.refused++
		return Record{}, fmt.Errorf("substrate: addressing claim: %w", err)
	}
	cose, err := l.in.SignOp(raw, l.signer)
	if err != nil {
		l.refused++
		return Record{}, fmt.Errorf("substrate: signing claim: %w", err)
	}
	// Admit our own op through the same path a peer's takes. Minting an op
	// this replica would refuse to ingest is a divergence between what it
	// publishes and what it believes, and it should fail here rather than on
	// some other replica's ingest path after the event.
	if _, err := l.eng.IngestSigned(cose, msOf(receiverNow)); err != nil {
		l.refused++
		return Record{}, fmt.Errorf("substrate: ingesting our own claim: %w", err)
	}
	l.folded++
	return Record{ID: hex.EncodeToString(id), Cose: cose}, nil
}

// VerifiedOp is one peer's envelope after it has been checked on its own
// merits, and before anything has been stored or merged.
//
// It exists because a replicated admission must be trusted for what it PROVES,
// not for the connection it arrived over. An authenticated transport says "this
// enrolled peer sent me these bytes"; it says nothing about whether the bytes
// are a well-formed, correctly-signed, in-namespace claim, and a transport that
// conflated the two would let one compromised peer inject anything at all.
type VerifiedOp struct {
	// ID is the §4.1 content address, lowercase hex. Two nodes that hold the
	// same envelope agree on it, which is what makes storage idempotent.
	ID string
	// Author is the §3 author: the node key that signed this op, lowercase hex.
	// The caller decides whether that key is one an operator pinned — this
	// package verifies the signature, it does not decide who is trusted.
	Author string
	// EventID is the §7 namespace, equal to this Ledger's event.
	EventID string
	// Claim is the admission claim, read out of the VERIFIED envelope rather
	// than out of anything a peer asserted alongside it.
	Claim scan.QueuedAdmission
	// Cose is the envelope exactly as received. It is the replicable unit and it
	// is stored and forwarded byte for byte — re-encoding it would change its
	// content address and break every other node's idempotency.
	Cose []byte
}

// VerifyOp checks one envelope completely, and merges nothing.
//
// Everything that can refuse an op is applied here: the signature (0x0A02), the
// structure and causality (0x0A03), the namespace (§7), the op kind, the
// element/op stamp agreement, and this package's own future-claim bound. What is
// NOT applied is the engine's §3 skew check, which needs the receiver clock a
// merge is performed against — that happens in IngestVerified.
//
// Split out from Ingest so a transport can verify, then decide whether the
// author is a key its operator pinned, then merge — in that order, with nothing
// stored until all three pass. A node that merged first and checked provenance
// afterwards would have already accepted the claim.
func (l *Ledger) VerifyOp(cose []byte, now time.Time) (VerifiedOp, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.verifyOpLocked(cose, now)
}

func (l *Ledger) verifyOpLocked(cose []byte, now time.Time) (VerifiedOp, error) {
	// Decode from the VERIFIED bytes: VerifySignedOp returns the op payload the
	// signature actually covers, so nothing below reads a field a peer could
	// have set outside the signature's reach.
	raw, err := l.in.VerifySignedOp(cose)
	if err != nil {
		l.refused++
		return VerifiedOp{}, err
	}
	sop, err := l.in.DecodeOp(raw)
	if err != nil {
		l.refused++
		return VerifiedOp{}, fmt.Errorf("substrate: decoding a verified claim: %w", err)
	}
	if sop.NS != l.ns {
		// §7: a claim about another event's door is not this replica's to
		// merge, and coercing it into this namespace would put an admission
		// for one event into another event's audit.
		l.refused++
		return VerifiedOp{}, fmt.Errorf("substrate: claim is in namespace %q, not %q", sop.NS, l.ns)
	}
	if sop.Kind != l.setAdd {
		l.refused++
		return VerifiedOp{}, fmt.Errorf(
			"substrate: claim has op kind %d, which Cackle does not model", sop.Kind)
	}
	a, err := l.claimOf(sop)
	if err != nil {
		l.refused++
		return VerifiedOp{}, err
	}
	if a.ScannedAt.After(now.Add(l.maxSkew)) {
		l.refused++
		return VerifiedOp{}, fmt.Errorf("%w: %s is more than %s ahead of %s (ticket %s, device %s)",
			ErrFutureClaim, a.ScannedAt.UTC(), l.maxSkew, now.UTC(), a.TicketID, a.DeviceID)
	}
	id, err := l.in.OpID(raw)
	if err != nil {
		l.refused++
		return VerifiedOp{}, fmt.Errorf("substrate: addressing a verified claim: %w", err)
	}
	return VerifiedOp{
		ID:      hex.EncodeToString(id),
		Author:  sop.HLC.Author,
		EventID: sop.NS,
		Claim:   a,
		Cose:    cose,
	}, nil
}

// IngestVerified merges an op that VerifyOp already checked, and reports whether
// it was new to this replica.
//
// receiverNow is the clock reading the engine's §3 skew check is measured
// against; see the package doc on why a replayed claim makes that check vacuous
// and what VerifyOp applies instead.
func (l *Ledger) IngestVerified(v VerifiedOp, receiverNow time.Time) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if v.EventID != l.ns {
		l.refused++
		return false, fmt.Errorf("substrate: verified claim is in namespace %q, not %q", v.EventID, l.ns)
	}
	if len(v.Cose) == 0 {
		l.refused++
		return false, errors.New("substrate: verified claim carries no envelope")
	}
	fresh, err := l.eng.IngestSigned(v.Cose, msOf(receiverNow))
	if err != nil {
		l.refused++
		return false, err
	}
	if fresh {
		l.ingested++
	}
	return fresh, nil
}

// Ingest admits a claim authored by another replica, from the envelope that
// replica minted, and reports whether it was new.
//
// It fails closed. The signature (0x0A02), the structure and causality
// (0x0A03), the namespace (§7), this package's own future-claim bound and the
// engine's skew check (SkewRefusalCode) are all applied before any state is
// touched, so a refused claim leaves this replica exactly as it was.
//
// It does NOT check who authored the op — that is a trust decision, and this
// package has no pin store. A caller replicating between servers must use
// VerifyOp, check the author against its enrolled peers, and then
// IngestVerified; internal/httpapi's sync routes do exactly that.
func (l *Ledger) Ingest(cose []byte, receiverNow, now time.Time) (scan.QueuedAdmission, bool, error) {
	l.mu.Lock()
	v, err := l.verifyOpLocked(cose, now)
	l.mu.Unlock()
	if err != nil {
		return scan.QueuedAdmission{}, false, err
	}
	fresh, err := l.IngestVerified(v, receiverNow)
	if err != nil {
		return scan.QueuedAdmission{}, false, err
	}
	return v.Claim, fresh, nil
}

// Claims returns every surviving claim for one ticket, union-merged, in a
// deterministic order.
//
// The order is (ScannedAt, DeviceID, GateID, Result, Note) — total, and
// independent of the order the claims arrived in or of which replica is asked,
// which is the property that makes a reconciliation report reproducible.
//
// It is deliberately the WHOLE set and not a winner. §4.3 has no notion of a
// winner, by design: it is a set, and the answer to "was this ticket
// double-admitted" is a count over the members. Naming one claim canonical is
// a read-side policy, and Cackle's server already has one that is enforced by
// the database rather than by a merge rule — `idx_admissions_admitted_once`
// gives exactly one `result='admitted'` row per ticket. Inventing a second
// winner here that could disagree with it would be a second algebra, which is
// the failure mode this package was adopted to avoid.
func (l *Ledger) Claims(ticketID string) ([]scan.QueuedAdmission, error) {
	byTicket, err := l.members()
	if err != nil {
		return nil, err
	}
	out := byTicket[ticketID]
	if out == nil {
		out = []scan.QueuedAdmission{}
	}
	return out, nil
}

// Conflict is one ticket that more than one device believed it admitted.
type Conflict struct {
	// TicketID is the ticket two or more gates each let through.
	TicketID string
	// Claims is every surviving claim on this ticket, in Claims' deterministic
	// order — including the non-admitting ones, because "device C also scanned
	// it and correctly refused it" is part of describing what happened.
	Claims []scan.QueuedAdmission
	// Admitted is the subset of Claims whose Result is scan.Admitted: the
	// claims that each independently believed they were the first and only
	// admission. len(Admitted) is how many people got through on this one
	// ticket, as far as the claims this replica has seen can show.
	Admitted []scan.QueuedAdmission
	// Devices is the number of distinct devices in Admitted. It is always at
	// least 2 for a Conflict — a single device cannot double-admit, because
	// its own local dedupe claim is atomic (internal/scan.SeenSet).
	Devices int
}

// Conflicts returns every ticket in this replica's merged view that more than
// one DEVICE claimed to admit.
//
// "More than one device" rather than "more than one claim" is the exact
// predicate, and the distinction is the whole point. A single device
// re-presenting a ticket to itself is refused locally and atomically, so a
// second admitting claim from the same device_id cannot be a partition
// artefact. Two DIFFERENT devices each holding an admitting claim is the
// signature of the thing that can only happen while they cannot see each
// other, and it means an extra person is inside.
//
// The result is ordered by ticket id so a report is stable.
//
// This is a record of something that already happened. It is not a guard, it
// cannot become one, and it is only as complete as the claims that reached this
// replica — a gate whose log never synced contributes nothing, and a conflict
// it was part of looks from here like a clean single admission.
func (l *Ledger) Conflicts() ([]Conflict, error) {
	byTicket, err := l.members()
	if err != nil {
		return nil, err
	}

	out := make([]Conflict, 0)
	for ticketID, claims := range byTicket {
		var admitted []scan.QueuedAdmission
		devices := make(map[string]struct{}, 2)
		for _, c := range claims {
			if c.Result == scan.Admitted {
				admitted = append(admitted, c)
				devices[c.DeviceID] = struct{}{}
			}
		}
		if len(devices) < 2 {
			continue
		}
		out = append(out, Conflict{
			TicketID: ticketID,
			Claims:   claims,
			Admitted: admitted,
			Devices:  len(devices),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TicketID < out[j].TicketID })
	return out, nil
}

// StateRoot is the §6.1 content address of this replica's whole observable
// state, lowercase hex.
//
// Two replicas that have converged agree on it byte for byte, which is a far
// stronger check than comparing rendered rows: it covers every element,
// including the ones no report displays.
//
// It addresses the OBSERVABLE state — the merged elements — and not the causal
// metadata behind them. Two replicas holding the same claims agree on the root
// even if they folded them under different identities and in different orders,
// which is what makes it a usable convergence check for a derived view whose
// author is ephemeral. What it therefore does NOT distinguish is who added an
// element; if that matters, read VersionVector, which is per author by
// construction.
func (l *Ledger) StateRoot() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	root, err := l.eng.StateRoot()
	if err != nil {
		return "", fmt.Errorf("substrate: reading the state root: %w", err)
	}
	return hex.EncodeToString(root), nil
}

// VersionVector reports the highest §3 stamp this replica has applied per
// author — what a pull request would advertise if Cackle had one.
func (l *Ledger) VersionVector() ([]kotvasync.Mark, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	marks, err := l.eng.VersionVector()
	if err != nil {
		return nil, fmt.Errorf("substrate: reading the version vector: %w", err)
	}
	return marks, nil
}

// Stats reports what this replica has seen.
type Stats struct {
	Folded   int `json:"folded"`
	Ingested int `json:"ingested"`
	Refused  int `json:"refused"`
}

// Stats returns a snapshot of the counters.
func (l *Ledger) Stats() Stats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Stats{Folded: l.folded, Ingested: l.ingested, Refused: l.refused}
}

// --- the mapping ---------------------------------------------------------

// claimPayload is the canonical, explicitly-ordered encoding of a claim's
// non-stamp fields.
//
// The field order is written out rather than inherited from struct order so
// the wire contract is visible and cannot drift with a future field
// reordering — the same technique internal/scan/allocation.go's signingBytes
// uses, for the same reason. ScannedAt is absent because it is the element's
// wall stamp; duplicating it would create two places for it to disagree.
type claimPayload struct {
	TicketID string `json:"tid"`
	EventID  string `json:"eid"`
	GateID   string `json:"gate"`
	DeviceID string `json:"dev"`
	Result   string `json:"result"`
	Note     string `json:"note"`
}

// deviceTag is the fixed-width device identifier the element stamp carries.
//
// SHA-256 of the reported device id: fixed width so the stamp needs no length
// prefix, and derived so readback can re-check it against the payload. It is a
// TAG, not a key — see the package doc on what authorship does and does not
// prove.
func deviceTag(deviceID string) [sha256.Size]byte { return sha256.Sum256([]byte(deviceID)) }

const stampLen = 8 + 4 + sha256.Size

// element builds the §4.3 OR-Set element for one claim.
func element(a scan.QueuedAdmission) (json.RawMessage, error) {
	payload, err := json.Marshal(claimPayload{
		TicketID: a.TicketID,
		EventID:  a.EventID,
		GateID:   a.GateID,
		DeviceID: a.DeviceID,
		Result:   string(a.Result),
		Note:     a.Note,
	})
	if err != nil {
		return nil, fmt.Errorf("substrate: encoding claim payload: %w", err)
	}

	b := make([]byte, 0, stampLen+len(payload))
	var wall [8]byte
	binary.BigEndian.PutUint64(wall[:], uint64(a.ScannedAt.UTC().UnixMilli()))
	b = append(b, wall[:]...)
	// counter is always 0 — see the package doc.
	b = append(b, 0, 0, 0, 0)
	tag := deviceTag(a.DeviceID)
	b = append(b, tag[:]...)
	b = append(b, payload...)
	return kotvasync.Bytes(b), nil
}

// decodeElement reads back what element wrote.
//
// It re-derives the device tag from the payload's own device id and refuses a
// mismatch. The two are one fact recorded twice; a disagreement means the
// element was built by something other than element(), and accepting it would
// put a claim in the ledger under an identity no other replica would compute.
func decodeElement(tagged json.RawMessage) (scan.QueuedAdmission, error) {
	raw, err := untagBytes(tagged)
	if err != nil {
		return scan.QueuedAdmission{}, err
	}
	if len(raw) < stampLen {
		return scan.QueuedAdmission{}, fmt.Errorf(
			"substrate: element is %d bytes, too short to carry its stamp", len(raw))
	}
	wall := binary.BigEndian.Uint64(raw[0:8])
	if counter := binary.BigEndian.Uint32(raw[8:12]); counter != 0 {
		return scan.QueuedAdmission{}, fmt.Errorf(
			"substrate: element carries HLC counter %d; Cackle only mints 0", counter)
	}
	if wall > 1<<62 {
		// Guarded before the int64 conversion below: silently wrapping a
		// value this large negative would invert the order for every later
		// comparison.
		return scan.QueuedAdmission{}, fmt.Errorf("substrate: element carries an out-of-range wall clock")
	}

	var p claimPayload
	if err := json.Unmarshal(raw[stampLen:], &p); err != nil {
		return scan.QueuedAdmission{}, fmt.Errorf("substrate: decoding claim payload: %w", err)
	}
	if got, want := raw[12:stampLen], deviceTag(p.DeviceID); string(got) != string(want[:]) {
		return scan.QueuedAdmission{}, fmt.Errorf(
			"substrate: element stamps device tag %x but its payload names %q", got, p.DeviceID)
	}

	return scan.QueuedAdmission{
		TicketID:  p.TicketID,
		EventID:   p.EventID,
		GateID:    p.GateID,
		DeviceID:  p.DeviceID,
		ScannedAt: time.UnixMilli(int64(wall)).UTC(),
		Result:    scan.Status(p.Result),
		Note:      p.Note,
	}, nil
}

// untagBytes decodes a §4.1 tagged byte-string value.
//
// It refuses any other tag rather than coercing. A value the engine spells as
// text where this package expects bytes is a mapping bug, and guessing would
// make the bug converge instead of surface.
func untagBytes(tagged json.RawMessage) ([]byte, error) {
	var v struct {
		Bstr *string `json:"bstr"`
	}
	if err := json.Unmarshal(tagged, &v); err != nil || v.Bstr == nil {
		return nil, fmt.Errorf("substrate: engine value is not a tagged byte string: %s", tagged)
	}
	b, err := hex.DecodeString(*v.Bstr)
	if err != nil {
		return nil, fmt.Errorf("substrate: engine value is not hex: %w", err)
	}
	return b, nil
}

// syncOp expresses one claim as a §4.1 SyncOp.
func (l *Ledger) syncOp(a scan.QueuedAdmission) (kotvasync.Op, error) {
	if a.TicketID == "" {
		return kotvasync.Op{}, errors.New("substrate: a claim with no ticket id is not an admission fact")
	}
	if a.DeviceID == "" {
		// Without a device the element cannot distinguish two gates, which is
		// the one thing this mapping exists to do. Refuse rather than fold a
		// claim that would collapse into another gate's.
		return kotvasync.Op{}, fmt.Errorf("substrate: claim for ticket %s has no device id", a.TicketID)
	}
	if strings.Contains(a.TicketID, "/") {
		return kotvasync.Op{}, fmt.Errorf(
			"substrate: ticket id %q contains %q, which would make its target ambiguous", a.TicketID, "/")
	}
	if a.EventID != l.ns {
		return kotvasync.Op{}, fmt.Errorf(
			"substrate: claim is for event %q, not this ledger's %q", a.EventID, l.ns)
	}
	ms := a.ScannedAt.UTC().UnixMilli()
	if ms < 0 {
		return kotvasync.Op{}, fmt.Errorf("substrate: claim for ticket %s predates the epoch", a.TicketID)
	}
	val, err := element(a)
	if err != nil {
		return kotvasync.Op{}, err
	}
	return kotvasync.Op{
		Kind:   l.setAdd,
		NS:     l.ns,
		Target: targetOf(a.TicketID),
		Value:  val,
		// The op's HLC wall is the claim's own scan time, so the stamp inside
		// the element and the stamp on the op are the same reading rather than
		// two that could drift. The author is this replica; see the package
		// doc on what that attests.
		HLC: kotvasync.HLC{Wall: uint64(ms), Counter: 0, Author: l.author},
	}, nil
}

// claimOf reads a decoded SyncOp back as the claim it was folded from, and
// refuses one whose element and op stamps disagree.
func (l *Ledger) claimOf(sop kotvasync.Op) (scan.QueuedAdmission, error) {
	ticketID, err := ticketOf(sop.Target)
	if err != nil {
		return scan.QueuedAdmission{}, err
	}
	a, err := decodeElement(sop.Value)
	if err != nil {
		return scan.QueuedAdmission{}, err
	}
	if a.TicketID != ticketID {
		return scan.QueuedAdmission{}, fmt.Errorf(
			"substrate: claim is addressed to ticket %q but its payload names %q", ticketID, a.TicketID)
	}
	if uint64(a.ScannedAt.UnixMilli()) != sop.HLC.Wall {
		// The element's stamp and the op's own HLC are two recordings of one
		// scan time. A disagreement means the op was built somewhere other
		// than syncOp, and accepting it would place the claim in the ledger
		// under an identity no other replica would compute.
		return scan.QueuedAdmission{}, fmt.Errorf(
			"substrate: claim for ticket %s stamps its element %d but its op claims %d",
			ticketID, a.ScannedAt.UnixMilli(), sop.HLC.Wall)
	}
	if a.EventID != l.ns {
		return scan.QueuedAdmission{}, fmt.Errorf(
			"substrate: claim payload names event %q, not this ledger's %q", a.EventID, l.ns)
	}
	return a, nil
}

// members reads the whole merged set, grouped by ticket and deterministically
// ordered within each ticket.
func (l *Ledger) members() (map[string][]scan.QueuedAdmission, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	pairs, err := l.eng.SetMembers()
	if err != nil {
		return nil, fmt.Errorf("substrate: reading set members: %w", err)
	}

	byTicket := make(map[string][]scan.QueuedAdmission, len(pairs))
	for _, pair := range pairs {
		if len(pair) != 2 {
			return nil, fmt.Errorf("substrate: engine returned a %d-element member pair", len(pair))
		}
		var target string
		if err := json.Unmarshal(pair[0], &target); err != nil {
			return nil, fmt.Errorf("substrate: member target is not a string: %s", pair[0])
		}
		ticketID, err := ticketOf(target)
		if err != nil {
			// A target this package did not mint is refused rather than
			// skipped: a set element under an unrecognised address means
			// something else is writing into this namespace, and quietly
			// dropping it would hide that.
			return nil, err
		}
		a, err := decodeElement(pair[1])
		if err != nil {
			return nil, err
		}
		if a.TicketID != ticketID {
			return nil, fmt.Errorf(
				"substrate: member addressed to ticket %q names %q", ticketID, a.TicketID)
		}
		byTicket[ticketID] = append(byTicket[ticketID], a)
	}

	for _, claims := range byTicket {
		sortClaims(claims)
	}
	return byTicket, nil
}

// sortClaims imposes the total order Claims and Conflicts report in.
//
// Earliest scan first, then device, gate, result and note. It matches the
// order internal/scan.ReconcileTicket sorts by on its first two keys, so a
// report built here and one built there agree on which claim came first — and
// it is total, so the same set of claims always renders the same way.
func sortClaims(claims []scan.QueuedAdmission) {
	sort.Slice(claims, func(i, j int) bool {
		a, b := claims[i], claims[j]
		if !a.ScannedAt.Equal(b.ScannedAt) {
			return a.ScannedAt.Before(b.ScannedAt)
		}
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		if a.GateID != b.GateID {
			return a.GateID < b.GateID
		}
		if a.Result != b.Result {
			return a.Result < b.Result
		}
		return a.Note < b.Note
	})
}

// msOf renders a time as the millisecond reading the engine's §3 checks take.
func msOf(t time.Time) uint64 {
	ms := t.UTC().UnixMilli()
	if ms < 0 {
		return 0
	}
	return uint64(ms)
}
