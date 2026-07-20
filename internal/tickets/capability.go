// Package tickets implements Cackle's core differentiator: Ed25519-signed
// ticket capabilities that a gate scanner can verify with zero network
// access, against a per-event public key pinned ahead of time.
//
// The format is deliberately compact so it fits in a QR code:
//
//	cackle.<base64url(payload_json)>.<base64url(ed25519_signature)>
//
// See README.md in this directory for the full format spec and a worked
// example.
package tickets

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CurrentVersion is the only payload version this build knows how to
// verify. Bump deliberately, and keep old versions verifiable for as long
// as tickets signed under them may still be in the wild.
const CurrentVersion = 1

// tokenPrefix is the literal first segment of every capability token.
const tokenPrefix = "cackle"

// Sentinel errors returned by Verify. Callers (in particular internal/scan)
// should switch on these with errors.Is rather than string-matching.
var (
	// ErrMalformed covers structurally broken tokens: wrong number of
	// segments, bad base64, invalid JSON.
	ErrMalformed = errors.New("tickets: malformed capability token")
	// ErrUnsupportedVersion is returned when payload.V is not a version
	// this build understands.
	ErrUnsupportedVersion = errors.New("tickets: unsupported payload version")
	// ErrBadSignature covers both a tampered payload and a signature made
	// with the wrong key — verification cannot and should not distinguish
	// the two, to avoid leaking an oracle.
	ErrBadSignature = errors.New("tickets: signature verification failed")
	// ErrNotYetValid is returned when now < nbf.
	ErrNotYetValid = errors.New("tickets: ticket not yet valid (nbf)")
	// ErrExpired is returned when now >= exp.
	ErrExpired = errors.New("tickets: ticket expired (exp)")
	// ErrUnknownKID is returned by VerifyWithRing when the token's kid is
	// not present in the supplied KeyRing.
	ErrUnknownKID = errors.New("tickets: unknown key id (kid)")
)

// Payload is the JSON object embedded (base64url, no padding) as the second
// dot-separated segment of a capability token.
//
// Struct tags fix the exact compact key names from the wire format — do not
// rename them, older/other verifiers depend on this shape.
type Payload struct {
	V    int    `json:"v"`              // payload version, must equal CurrentVersion to be accepted
	TID  string `json:"tid"`            // ticket ULID
	EID  string `json:"eid"`            // event ULID
	TT   string `json:"tt"`             // ticket_type ULID
	KID  string `json:"kid"`            // event issuer key id this token is signed with
	Sub  string `json:"sub"`            // holder user ULID
	Name string `json:"nm"`             // holder display name
	IAT  int64  `json:"iat"`            // issued-at, unix seconds
	NBF  int64  `json:"nbf,omitempty"`  // not-before, unix seconds (0 = no lower bound)
	EXP  int64  `json:"exp,omitempty"`  // expiry, unix seconds (0 = no upper bound)
	Seat string `json:"seat,omitempty"` // optional seat/section label
}

// Issue signs payload with privkey and returns the compact capability
// token. payload.V is set to CurrentVersion regardless of its current
// value — callers should not set it themselves.
func Issue(payload Payload, privkey ed25519.PrivateKey) (string, error) {
	if len(privkey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("tickets: invalid private key size %d", len(privkey))
	}
	payload.V = CurrentVersion

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("tickets: marshal payload: %w", err)
	}

	sig := ed25519.Sign(privkey, body)

	encPayload := base64.RawURLEncoding.EncodeToString(body)
	encSig := base64.RawURLEncoding.EncodeToString(sig)

	return tokenPrefix + "." + encPayload + "." + encSig, nil
}

// Verify checks a capability token against pub and returns the decoded
// payload iff the token is structurally valid, correctly signed by pub, and
// currently within its nbf/exp window relative to now.
//
// Verify is PURE: it performs no I/O, touches no database, and makes no
// network call. It never calls time.Now() itself — the caller supplies now.
// This purity is the entire point: it is what lets a gate scanner verify a
// ticket with the network unplugged. Do not add any hidden dependency here
// (no package-level clock, no global key store, no logging with side
// effects) — anything added breaks the offline guarantee for every caller.
func Verify(token string, pub ed25519.PublicKey, now time.Time) (*Payload, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: invalid public key size %d", ErrMalformed, len(pub))
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: expected 3 dot-separated segments, got %d", ErrMalformed, len(parts))
	}
	if parts[0] != tokenPrefix {
		return nil, fmt.Errorf("%w: bad prefix %q", ErrMalformed, parts[0])
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: payload base64: %v", ErrMalformed, err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: signature base64: %v", ErrMalformed, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: invalid signature length %d", ErrMalformed, len(sig))
	}

	// Verify the signature over the *raw* decoded bytes before we even
	// attempt to parse JSON out of them, and use ed25519.Verify (which is
	// constant-time in the comparison) rather than hand-rolled compare.
	if !ed25519.Verify(pub, body, sig) {
		return nil, ErrBadSignature
	}

	var payload Payload
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: payload json: %v", ErrMalformed, err)
	}
	// Reject trailing garbage after the JSON object.
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after payload json", ErrMalformed)
	}

	if payload.V != CurrentVersion {
		return nil, fmt.Errorf("%w: got version %d, want %d", ErrUnsupportedVersion, payload.V, CurrentVersion)
	}

	nowUnix := now.Unix()
	if payload.NBF != 0 && nowUnix < payload.NBF {
		return nil, ErrNotYetValid
	}
	if payload.EXP != 0 && nowUnix >= payload.EXP {
		return nil, ErrExpired
	}

	return &payload, nil
}

// peekKID extracts the kid field from a capability token's payload WITHOUT
// verifying its signature. It exists solely so VerifyWithRing can select
// which pinned public key to verify against — exactly analogous to reading
// an unverified `kid` header in a JWT. The returned kid must never be
// trusted for anything beyond a map lookup into a KeyRing the caller
// already pinned out-of-band; the actual trust decision still happens
// inside Verify via the Ed25519 signature check.
func peekKID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenPrefix {
		return "", ErrMalformed
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("%w: payload base64: %v", ErrMalformed, err)
	}
	var partial struct {
		KID string `json:"kid"`
	}
	if err := json.Unmarshal(body, &partial); err != nil {
		return "", fmt.Errorf("%w: payload json: %v", ErrMalformed, err)
	}
	return partial.KID, nil
}
