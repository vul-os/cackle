package tickets

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IssuerKey is one event's Ed25519 signing key, as stored server-side
// (in `event_keys`). The private key is only ever held by the issuing
// server (or, for delegated issuance, a sub-issuer holding an allocation —
// see internal/scan). Scanners only ever see the public half, via KeyRing.
type IssuerKey struct {
	KID        string             `json:"kid"`
	EventID    string             `json:"event_id"`
	PublicKey  ed25519.PublicKey  `json:"public_key"`
	PrivateKey ed25519.PrivateKey `json:"private_key,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	RevokedAt  *time.Time         `json:"revoked_at,omitempty"`
}

// GenerateIssuerKey creates a fresh Ed25519 keypair for eventID and derives
// a stable, collision-resistant key id (kid) from the public key.
func GenerateIssuerKey(eventID string) (IssuerKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return IssuerKey{}, fmt.Errorf("tickets: generate key: %w", err)
	}
	return IssuerKey{
		KID:        KeyID(pub),
		EventID:    eventID,
		PublicKey:  pub,
		PrivateKey: priv,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// KeyID derives a deterministic key id from a public key: the first 16
// bytes of SHA-256(pub), base32-ish base64url encoded, prefixed "k_". Two
// calls with the same public key always produce the same kid, and it is
// safe to use as a stable, non-secret identifier embedded in tickets and
// exchanged with scanners.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "k_" + base64.RawURLEncoding.EncodeToString(sum[:16])
}

// Revoke marks the key revoked at t. A revoked key's public half should be
// removed from (or flagged in) any KeyRing built for a scanner going
// forward, but Verify itself has no notion of revocation — that policy
// belongs to whoever builds the KeyRing/Bundle, deliberately, so Verify
// stays pure.
func (k *IssuerKey) Revoke(t time.Time) {
	tt := t.UTC()
	k.RevokedAt = &tt
}

// KeyRing is the pinned, serialisable set of per-event public keys a
// scanner trusts. It is fetched once while online (as part of a scan
// Bundle) and cached to disk; from then on every Verify call is answered
// purely from this in-memory/JSON structure with zero network access.
//
// KeyRing is intentionally dumb: it holds public keys only, keyed by kid.
// It never holds a private key.
type KeyRing struct {
	EventID string                       `json:"event_id"`
	Keys    map[string]ed25519.PublicKey `json:"keys"`
}

// NewKeyRing returns an empty ring scoped to eventID.
func NewKeyRing(eventID string) KeyRing {
	return KeyRing{EventID: eventID, Keys: make(map[string]ed25519.PublicKey)}
}

// Add pins pub under kid. It overwrites any existing entry for the same
// kid (callers should not normally reuse a kid across different keys —
// KeyID makes that vanishingly unlikely for distinct keys).
func (r *KeyRing) Add(kid string, pub ed25519.PublicKey) {
	if r.Keys == nil {
		r.Keys = make(map[string]ed25519.PublicKey)
	}
	r.Keys[kid] = pub
}

// AddKey is a convenience wrapper that pins an IssuerKey's public half
// under its own kid.
func (r *KeyRing) AddKey(k IssuerKey) {
	r.Add(k.KID, k.PublicKey)
}

// Lookup returns the pinned public key for kid, if any.
func (r KeyRing) Lookup(kid string) (ed25519.PublicKey, bool) {
	if r.Keys == nil {
		return nil, false
	}
	pub, ok := r.Keys[kid]
	return pub, ok
}

// VerifyWithRing is a convenience wrapper around Verify that dispatches to
// the correct pinned public key by reading the token's (unverified) kid,
// looking it up in ring, and only then running the real, signature-checked
// Verify against that specific key. It is still pure — no I/O, no clock —
// same as Verify itself; ring is expected to already be resident in memory
// (e.g. loaded from disk once while online).
//
// If kid is missing from ring, ErrUnknownKID is returned before any
// signature check is attempted.
func VerifyWithRing(token string, ring KeyRing, now time.Time) (*Payload, error) {
	kid, err := peekKID(token)
	if err != nil {
		return nil, err
	}
	pub, ok := ring.Lookup(kid)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKID, kid)
	}
	return Verify(token, pub, now)
}

// MarshalJSON gives KeyRing a stable, explicit wire shape: kid -> base64url
// (no padding) public key. encoding/json would otherwise base64-std-encode
// []byte map values by default, which is also fine to decode but this makes
// the choice explicit and independent of Go's default encoding behaviour.
func (r KeyRing) MarshalJSON() ([]byte, error) {
	type wire struct {
		EventID string            `json:"event_id"`
		Keys    map[string]string `json:"keys"`
	}
	w := wire{EventID: r.EventID, Keys: make(map[string]string, len(r.Keys))}
	for kid, pub := range r.Keys {
		w.Keys[kid] = base64.RawURLEncoding.EncodeToString(pub)
	}
	return json.Marshal(w)
}

// UnmarshalJSON is the inverse of MarshalJSON.
func (r *KeyRing) UnmarshalJSON(data []byte) error {
	type wire struct {
		EventID string            `json:"event_id"`
		Keys    map[string]string `json:"keys"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	keys := make(map[string]ed25519.PublicKey, len(w.Keys))
	for kid, enc := range w.Keys {
		raw, err := base64.RawURLEncoding.DecodeString(enc)
		if err != nil {
			return fmt.Errorf("tickets: keyring: bad public key for kid %q: %w", kid, err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return fmt.Errorf("tickets: keyring: wrong public key size for kid %q: got %d want %d", kid, len(raw), ed25519.PublicKeySize)
		}
		keys[kid] = ed25519.PublicKey(raw)
	}
	r.EventID = w.EventID
	r.Keys = keys
	return nil
}

// SaveToFile writes the ring as indented JSON to path, creating parent
// directories as needed. This is the "cache on disk" half of the offline
// scanner story: a gate device calls this once while online and never
// needs to again for the rest of the event.
func (r KeyRing) SaveToFile(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("tickets: marshal keyring: %w", err)
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("tickets: mkdir %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("tickets: write keyring %s: %w", path, err)
	}
	return nil
}

// LoadKeyRingFromFile reads back a ring written by SaveToFile.
func LoadKeyRingFromFile(path string) (KeyRing, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return KeyRing{}, fmt.Errorf("tickets: read keyring %s: %w", path, err)
	}
	var r KeyRing
	if err := json.Unmarshal(data, &r); err != nil {
		return KeyRing{}, fmt.Errorf("tickets: unmarshal keyring %s: %w", path, err)
	}
	return r, nil
}
