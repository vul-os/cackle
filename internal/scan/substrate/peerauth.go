package substrate

// Mutual per-request key authentication for the server-to-server admission
// ledger transport.
//
// # Why this is here and not in internal/httpapi
//
// The canonical bytes two nodes sign are part of the wire contract, exactly like
// the §4.3 element encoding above them: if the two ends disagree about what is
// covered by a signature, every request fails and the failure looks like a
// broken key. Both belong in one place, and this is the package that owns what
// goes over the wire. internal/httpapi owns the routes and the socket; it does
// not get its own opinion about what a signature covers.
//
// # The shape of it, and what each part is for
//
// Every request carries four headers: the caller's public key, a timestamp, a
// nonce, and a signature over a canonical envelope. Every response carries the
// same four, over an envelope that includes the REQUEST's nonce. So both
// directions are authenticated per message, and neither direction depends on the
// other having been authenticated first.
//
//   - The key is the identity. A Cackle node's name in the mesh IS its Ed25519
//     public key, so the header that carries the caller's claimed identity is the
//     same key the signature is verified against — there is no separate node id
//     to look up and no way for the two to disagree.
//
//   - The signature covers the METHOD, the PATH, the RAW QUERY, the SHA-256 of
//     the body, the timestamp and the nonce. The query string is in there because
//     this transport puts the replication cursor and the page size there: a
//     signature that did not cover it would let anything on the path rewrite
//     `after=0` to `after=999999` and quietly starve a node of history it thinks
//     it received.
//
//   - The timestamp bounds a replay to ±MaxClockSkew, and the nonce refuses one
//     inside that window. Both are needed: the timestamp alone leaves a
//     five-minute replay window, and the nonce alone would require remembering
//     every nonce ever seen.
//
//   - The response signature is bound to the request's nonce, so a recorded
//     response cannot be replayed as the answer to a later question.
//
// # What this deliberately does NOT have
//
// No pairing secret, no bootstrap token, no trust-on-first-use, no "accept an
// unsigned request if a shared secret is presented" fallback. Cackle's enrolment
// is a human typing another node's URL and its public key, which means there is
// no first-contact problem to solve and therefore no reason to keep a weaker path
// open beside the strong one. An unsigned request is refused; a key nobody
// pinned is refused; a key that stops matching its pin is refused rather than
// re-pinned.
//
// That last one is the discipline propfix's own transport_auth.go was picked out
// for, and it is the property to preserve: a key change is a REFUSAL, and the fix
// is a human deleting the enrolment and making a new one. Nothing here can be
// talked into updating a pin, because nothing here writes one.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	kotvasync "github.com/vul-os/kotva/bindings/go"
)

// Peer authentication headers. Prefixed to keep them clearly distinct from
// anything a browser sends: this transport is node-to-node and never shares a
// credential with the session/cookie world.
const (
	HeaderKey       = "X-Cackle-Node"
	HeaderTimestamp = "X-Cackle-Timestamp"
	HeaderNonce     = "X-Cackle-Nonce"
	HeaderSig       = "X-Cackle-Sig"
)

// MaxClockSkew is how far a request or response timestamp may sit from the
// receiver's clock.
//
// Five minutes each way, matching the bound Cackle already applies to a claim's
// own scan time (DefaultMaxFutureSkew) so an operator has one number to keep
// their clocks inside rather than two.
const MaxClockSkew = 5 * time.Minute

// domain separation tags. Two envelopes that a signature could be moved between
// must not share a preimage shape; a request signature is not a response
// signature and cannot be replayed as one.
const (
	requestDomain  = "cackle-sync-request-v1"
	responseDomain = "cackle-sync-response-v1"
)

// ErrUnsigned is returned when a request or response carries no signed envelope
// at all. It is a distinct error because it is the one refusal that means
// "misconfigured", not "hostile" — most often a peer's URL pointed at something
// that is not a Cackle node.
var ErrUnsigned = errors.New("substrate: request carries no signed envelope")

// BodyHash is the body digest bound into a signature. An empty body hashes to
// the SHA-256 of no bytes rather than to a special case, so "no body" and "a
// body I chose not to cover" cannot be confused.
func BodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// RequestSigBase is the canonical preimage for a request signature.
//
// Newline-separated with a leading domain tag. Every component is bound in, so a
// signature cannot be lifted onto a different method, path, query, body, or
// freshness window.
func RequestSigBase(method, path, rawQuery, bodyHash, ts, nonce string) []byte {
	return []byte(strings.Join([]string{
		requestDomain, method, path, rawQuery, bodyHash, ts, nonce,
	}, "\n"))
}

// ResponseSigBase is the canonical preimage for a response signature.
//
// It binds the REQUEST's nonce, which is what makes a response un-replayable:
// the answer to one question cannot be presented as the answer to another, even
// by the peer that legitimately produced it.
func ResponseSigBase(requestNonce, bodyHash, ts string) []byte {
	return []byte(strings.Join([]string{
		responseDomain, requestNonce, bodyHash, ts,
	}, "\n"))
}

// NewNonce returns a fresh 128-bit random nonce, hex-encoded.
func NewNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("substrate: generating a request nonce: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// NonceCache refuses a nonce it has already seen, within the only window in
// which a replay could still be fresh.
//
// Entries are kept for twice MaxClockSkew and then dropped: past that point the
// timestamp check refuses the replay on its own, so remembering longer would
// grow without bound in exchange for nothing. It is safe for concurrent use.
type NonceCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// NewNonceCache returns an empty cache.
func NewNonceCache() *NonceCache { return &NonceCache{seen: make(map[string]time.Time)} }

// CheckAndAdd records key and reports whether it was unseen. A false return is a
// replay.
func (c *NonceCache) CheckAndAdd(key string, now time.Time) bool {
	ttl := 2 * MaxClockSkew
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = make(map[string]time.Time)
	}
	for k, exp := range c.seen { // lazy prune
		if now.After(exp) {
			delete(c.seen, k)
		}
	}
	if exp, ok := c.seen[key]; ok && now.Before(exp) {
		return false
	}
	c.seen[key] = now.Add(ttl)
	return true
}

// SignRequest attaches this node's signed envelope to req.
//
// body must be the exact bytes sent — nil for a bodyless GET. The nonce is
// returned so the caller can bind the peer's response signature to it.
func SignRequest(req *http.Request, signer kotvasync.Signer, body []byte, now time.Time) (nonce string, err error) {
	if signer == nil {
		return "", errors.New("substrate: signing a peer request needs this node's identity")
	}
	nonce, err = NewNonce()
	if err != nil {
		return "", err
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	base := RequestSigBase(req.Method, req.URL.Path, req.URL.RawQuery, BodyHash(body), ts, nonce)
	sig, err := signer.Sign(base)
	if err != nil {
		return "", fmt.Errorf("substrate: signing a peer request: %w", err)
	}
	req.Header.Set(HeaderKey, hex.EncodeToString(signer.Public()))
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSig, hex.EncodeToString(sig))
	return nonce, nil
}

// PresentedKey reports the public key an inbound request claims, normalized, or
// "" if it presents none or presents a malformed one.
//
// It is only ever used to LOOK UP a pin. A caller must not treat a non-empty
// result as authenticated — VerifyRequest is what does that.
func PresentedKey(h http.Header) string {
	key, err := normalizeKey(h.Get(HeaderKey))
	if err != nil {
		return ""
	}
	return key
}

// VerifyRequest authenticates an inbound peer request against body.
//
// wantKey is the pinned key this request must have been signed by — the caller
// looks it up from its own enrolled peers, keyed on PresentedKey, and passes it
// in. Passing a key that differs from the presented one is a refusal, not a
// re-pin: this function has no way to write a pin and no way to be persuaded to.
//
// It refuses in this order, and each refusal leaves nothing behind: no envelope,
// a timestamp outside MaxClockSkew, a bad signature, then a replayed nonce. The
// nonce is recorded only AFTER the signature verifies, so an attacker cannot
// burn a legitimate peer's nonces by guessing them.
func VerifyRequest(r *http.Request, body []byte, wantKey string, nonces *NonceCache, now time.Time) error {
	key := r.Header.Get(HeaderKey)
	ts := r.Header.Get(HeaderTimestamp)
	nonce := r.Header.Get(HeaderNonce)
	sig := r.Header.Get(HeaderSig)
	if key == "" || ts == "" || nonce == "" || sig == "" {
		return ErrUnsigned
	}
	presented, err := normalizeKey(key)
	if err != nil {
		return fmt.Errorf("substrate: presented node key: %w", err)
	}
	pinned, err := normalizeKey(wantKey)
	if err != nil {
		return fmt.Errorf("substrate: pinned node key: %w", err)
	}
	if presented != pinned {
		return fmt.Errorf("substrate: request is signed by %s, which is not the pinned key", short(presented))
	}
	if err := checkFreshness(ts, now); err != nil {
		return err
	}
	base := RequestSigBase(r.Method, r.URL.Path, r.URL.RawQuery, BodyHash(body), ts, nonce)
	if err := verifySig(pinned, base, sig); err != nil {
		return err
	}
	if nonces != nil && !nonces.CheckAndAdd(pinned+"|"+nonce, now) {
		return errors.New("substrate: replayed request nonce")
	}
	return nil
}

// SignResponse attaches this node's signed envelope to a response, bound to the
// request's nonce.
//
// This is the half that lets a CALLER detect that the node answering is not the
// one whose key it pinned. Without it, a node could authenticate every request it
// sends and still be talking to an impostor.
func SignResponse(h http.Header, signer kotvasync.Signer, requestNonce string, body []byte, now time.Time) error {
	if signer == nil {
		return errors.New("substrate: signing a peer response needs this node's identity")
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	sig, err := signer.Sign(ResponseSigBase(requestNonce, BodyHash(body), ts))
	if err != nil {
		return fmt.Errorf("substrate: signing a peer response: %w", err)
	}
	h.Set(HeaderKey, hex.EncodeToString(signer.Public()))
	h.Set(HeaderTimestamp, ts)
	h.Set(HeaderNonce, requestNonce)
	h.Set(HeaderSig, hex.EncodeToString(sig))
	return nil
}

// VerifyResponse checks that a response came from the peer whose key is pinned,
// and answers the request that carried requestNonce.
//
// A mismatch here is the case the whole pin discipline exists for: the address is
// answering, but with a different key than the operator enrolled. It returns an
// error, and the caller's contract is to record the refusal and change NOTHING —
// not the pin, not the cursors. A node whose key changed is indistinguishable
// from an impostor at that address, and only a human can tell the two apart.
func VerifyResponse(h http.Header, wantKey, requestNonce string, body []byte, now time.Time) error {
	key := h.Get(HeaderKey)
	ts := h.Get(HeaderTimestamp)
	sig := h.Get(HeaderSig)
	if key == "" || ts == "" || sig == "" {
		return ErrUnsigned
	}
	pinned, err := normalizeKey(wantKey)
	if err != nil {
		return fmt.Errorf("substrate: pinned node key: %w", err)
	}
	presented, err := normalizeKey(key)
	if err != nil {
		return fmt.Errorf("substrate: peer answered with a malformed node key: %w", err)
	}
	if presented != pinned {
		return fmt.Errorf(
			"substrate: peer answered as %s but %s is pinned — refusing, and the pin is unchanged",
			short(presented), short(pinned))
	}
	if got := h.Get(HeaderNonce); got != requestNonce {
		return fmt.Errorf("substrate: peer's response is bound to nonce %q, not this request's", got)
	}
	if err := checkFreshness(ts, now); err != nil {
		return err
	}
	return verifySig(pinned, ResponseSigBase(requestNonce, BodyHash(body), ts), sig)
}

func checkFreshness(ts string, now time.Time) error {
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("substrate: timestamp %q is not a unix second count", ts)
	}
	d := now.Sub(time.Unix(sec, 0))
	if d > MaxClockSkew || d < -MaxClockSkew {
		return fmt.Errorf("substrate: timestamp is %s from this node's clock, beyond ±%s",
			d.Round(time.Second), MaxClockSkew)
	}
	return nil
}

func verifySig(keyHex string, base []byte, sigHex string) error {
	pub, err := hex.DecodeString(keyHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("substrate: node key is not a 32-byte hex Ed25519 key")
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return errors.New("substrate: signature is not hex")
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("substrate: signature is %d bytes, want %d", len(sig), ed25519.SignatureSize)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), base, sig) {
		return errors.New("substrate: signature does not verify against the pinned key")
	}
	return nil
}

// normalizeKey canonicalises a hex Ed25519 public key. Comparing pins as raw
// strings would make case a security-relevant detail; comparing them normalized
// makes a mismatch mean a different key and nothing else.
func normalizeKey(key string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(key))
	raw, err := hex.DecodeString(k)
	if err != nil {
		return "", fmt.Errorf("not hex: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("%d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	return k, nil
}

// short abbreviates a key for a message a human reads. Keys are public, so this
// is legibility rather than redaction — a full 64 hex characters in an error
// tells an operator less than the first twelve do.
func short(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12] + "…"
}
