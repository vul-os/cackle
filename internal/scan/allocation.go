package scan

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Allocation is a signed capacity grant that lets a specific, possibly
// disconnected, device mint up to Count tickets of one ticket type without
// contacting the server — the delegated-issuance half of the offline
// story. It mirrors the `allocations` table in the store schema
// (id, event_id, device_id, ticket_type_id, count, issued_at, expires_at,
// signature); this package owns the struct, its signing/verification, and
// a local cap-enforcement counter, but never touches the table itself.
//
// Full delegation (a sub-issuer minting genuinely independent capability
// tokens under its own keypair, cross-signed by the event issuer) is
// roadmap work — see ROADMAP.md's "capacity delegation to sub-issuers for
// disconnected/remote sites". What is built here is the piece needed now:
// proof that a device's claimed capacity was genuinely granted by the
// event's issuer key, and a local mechanism to enforce it stays within
// that cap while offline.
type Allocation struct {
	ID           string    `json:"id"`
	EventID      string    `json:"event_id"`
	DeviceID     string    `json:"device_id"`
	TicketTypeID string    `json:"ticket_type_id"`
	Count        int       `json:"count"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	KID          string    `json:"kid"` // which event issuer key signed this allocation
}

// ErrAllocationExpired is returned by VerifyAllocation when now is at or
// after ExpiresAt.
var ErrAllocationExpired = errors.New("scan: allocation expired")

// ErrAllocationBadSignature is returned by VerifyAllocation when the
// signature does not match — tampered fields or signed by the wrong key.
var ErrAllocationBadSignature = errors.New("scan: allocation signature verification failed")

// signingBytes returns the canonical byte representation of a signed over
// by Sign/VerifyAllocation — deliberately excluding any signature field
// (Allocation has none; the signature travels alongside, never inside, the
// struct that's signed).
func (a Allocation) signingBytes() ([]byte, error) {
	// A stable, explicit field order (rather than relying on struct JSON
	// tag order, which IS stable in Go but this makes the wire contract
	// obvious) keeps signing deterministic and independent of any future
	// struct field reordering.
	type canonical struct {
		ID           string `json:"id"`
		EventID      string `json:"event_id"`
		DeviceID     string `json:"device_id"`
		TicketTypeID string `json:"ticket_type_id"`
		Count        int    `json:"count"`
		IssuedAt     int64  `json:"issued_at"`
		ExpiresAt    int64  `json:"expires_at"`
		KID          string `json:"kid"`
	}
	c := canonical{
		ID:           a.ID,
		EventID:      a.EventID,
		DeviceID:     a.DeviceID,
		TicketTypeID: a.TicketTypeID,
		Count:        a.Count,
		IssuedAt:     a.IssuedAt.Unix(),
		ExpiresAt:    a.ExpiresAt.Unix(),
		KID:          a.KID,
	}
	return json.Marshal(c)
}

// SignAllocation signs a with the event issuer's private key and returns
// the base64url-encoded Ed25519 signature to store/transmit alongside it
// (the `signature` column in the `allocations` table).
func SignAllocation(a Allocation, priv ed25519.PrivateKey) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("scan: sign allocation: invalid private key size %d", len(priv))
	}
	body, err := a.signingBytes()
	if err != nil {
		return "", fmt.Errorf("scan: sign allocation: %w", err)
	}
	sig := ed25519.Sign(priv, body)
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifyAllocation checks that sig is a valid Ed25519 signature over a's
// canonical bytes under pub, and that now falls within [IssuedAt,
// ExpiresAt). Like tickets.Verify, this is pure: no I/O, no clock of its
// own — now is always supplied by the caller, so a disconnected sub-issuer
// device can check its own allocation is still good with zero network
// access, exactly like a gate verifies a ticket capability.
func VerifyAllocation(a Allocation, sig string, pub ed25519.PublicKey, now time.Time) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("scan: verify allocation: invalid public key size %d", len(pub))
	}
	rawSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("scan: verify allocation: bad signature encoding: %w", err)
	}
	if len(rawSig) != ed25519.SignatureSize {
		return fmt.Errorf("scan: verify allocation: invalid signature length %d", len(rawSig))
	}
	body, err := a.signingBytes()
	if err != nil {
		return fmt.Errorf("scan: verify allocation: %w", err)
	}
	if !ed25519.Verify(pub, body, rawSig) {
		return ErrAllocationBadSignature
	}
	if !a.ExpiresAt.IsZero() && !now.Before(a.ExpiresAt) {
		return ErrAllocationExpired
	}
	return nil
}

// AllocationCounter enforces, locally and offline, that a disconnected
// sub-issuer never mints more than its Allocation.Count grant — the
// "mint within a signed cap" half of delegated issuance. It has no notion
// of signatures itself; callers are expected to have already called
// VerifyAllocation once (e.g. when the allocation was first loaded) before
// trusting the Count it enforces here.
type AllocationCounter struct {
	mu       sync.Mutex
	cap      int
	consumed int
}

// NewAllocationCounter returns a counter enforcing the cap in alloc.Count.
func NewAllocationCounter(alloc Allocation) *AllocationCounter {
	return &AllocationCounter{cap: alloc.Count}
}

// TryConsume attempts to consume n units of capacity (n is normally 1, one
// per ticket minted). It reports whether the consumption was allowed; on
// false, no state changes — the caller must not mint.
func (c *AllocationCounter) TryConsume(n int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n <= 0 {
		return true
	}
	if c.consumed+n > c.cap {
		return false
	}
	c.consumed += n
	return true
}

// Remaining reports how much capacity is left.
func (c *AllocationCounter) Remaining() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cap - c.consumed
}
