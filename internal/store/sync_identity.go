package store

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// This file, sync_peers.go and sync_ops.go are the durable side of
// server-to-server replication of the admission ledger. The algebra they carry
// is not written here: an admission claim is a §4.3 add-only-set element in the
// shared DMTAP Sync algebra, and internal/scan/substrate owns that mapping. This
// package owns the bytes on disk — a node's identity, the peers a human pinned,
// and the op log.
//
// None of it runs unless an operator enrols a peer. A Cackle node with no peers
// is the default: no key is generated, no row is written, no socket is opened.
// See docs/CLUSTERING.md.

// NodeIdentity is this node's replication identity: the Ed25519 keypair that
// signs the ops it mints and the requests and responses it exchanges with a
// peer.
//
// The public half is what another operator pins by hand when enrolling this
// node. There is no certificate authority, no directory and no trust-on-first-
// use anywhere in this path — a key is either pinned or refused.
type NodeIdentity struct {
	// PublicKey is the Ed25519 public key, lowercase hex. This is the value an
	// operator reads off one node and types into another.
	PublicKey string
	// PrivateKey is the signing key. It is never logged, never served over
	// HTTP, and never leaves this process.
	PrivateKey ed25519.PrivateKey
	// CreatedAt is when this node first needed an identity.
	CreatedAt time.Time
}

// NodeIdentity reads this node's replication identity, or returns ErrNotFound
// if it has never needed one.
//
// ErrNotFound is the ordinary state of a node with no peers, not a fault. A
// caller that wants an identity to exist calls EnsureNodeIdentity; a caller
// that only wants to report on one (a status view) uses this and treats absence
// as "replication has never been configured here".
func (s *Store) NodeIdentity(ctx context.Context) (NodeIdentity, error) {
	var (
		out       NodeIdentity
		priv      []byte
		createdAt string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT public_key, private_key, created_at FROM sync_node_identity WHERE id = 1`,
	).Scan(&out.PublicKey, &priv, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeIdentity{}, ErrNotFound
	}
	if err != nil {
		return NodeIdentity{}, fmt.Errorf("store: read node identity: %w", err)
	}
	if len(priv) != ed25519.PrivateKeySize {
		// Fail closed and loudly. A truncated key would produce signatures no
		// peer can verify, and silently regenerating one here would rotate this
		// node's identity behind the operator's back — which every peer would
		// then refuse, with no explanation anywhere.
		return NodeIdentity{}, fmt.Errorf(
			"store: node identity private key is %d bytes, want %d — the row is corrupt; "+
				"delete it to mint a new identity and re-enrol this node with every peer",
			len(priv), ed25519.PrivateKeySize)
	}
	out.PrivateKey = ed25519.PrivateKey(priv)
	if want := hex.EncodeToString(out.PrivateKey.Public().(ed25519.PublicKey)); want != out.PublicKey {
		// The two halves disagree. Trusting either one would mean signing under
		// a key that is not the one peers pinned, or advertising a key this node
		// cannot sign with.
		return NodeIdentity{}, errors.New(
			"store: node identity public key does not match its private key — the row is corrupt")
	}
	t, err := textToTime(createdAt)
	if err != nil {
		return NodeIdentity{}, fmt.Errorf("store: node identity created_at %q: %w", createdAt, err)
	}
	out.CreatedAt = t
	return out, nil
}

// EnsureNodeIdentity returns this node's replication identity, generating it on
// first use.
//
// Lazy on purpose. Generating a keypair at boot would put signing key material
// on every Cackle deployment, including the overwhelming majority that will
// never enrol a peer, in exchange for nothing. It is created the first time an
// operator asks to see this node's key or enrols a peer — i.e. the first time
// somebody actually intends to cluster.
//
// It is safe against a concurrent caller: the insert is a no-op if a row already
// exists, and the row is then read back, so two simultaneous callers agree on
// one identity rather than racing to overwrite each other's.
func (s *Store) EnsureNodeIdentity(ctx context.Context) (NodeIdentity, error) {
	if id, err := s.NodeIdentity(ctx); err == nil {
		return id, nil
	} else if !errors.Is(err, ErrNotFound) {
		return NodeIdentity{}, err
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return NodeIdentity{}, fmt.Errorf("store: generate node identity: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_node_identity (id, public_key, private_key, created_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		hex.EncodeToString(pub), []byte(priv), timeToText(time.Now()),
	); err != nil {
		return NodeIdentity{}, fmt.Errorf("store: create node identity: %w", err)
	}
	return s.NodeIdentity(ctx)
}

// NormalizeNodeKey parses an operator-supplied Ed25519 public key and returns
// it in the canonical form the pin store and every wire header use: lowercase
// hex, exactly 32 bytes.
//
// Canonicalising at the boundary is what makes "a key change is a refusal"
// implementable at all. If "AB…" and "ab…" could both be stored, a pin
// comparison would be a string compare against an unknown spelling, and the
// safe answer (refuse) and the unsafe answer (accept) would depend on which
// case the operator happened to paste.
func NormalizeNodeKey(key string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(key))
	raw, err := hex.DecodeString(k)
	if err != nil {
		return "", fmt.Errorf("store: node key is not hex: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("store: node key is %d bytes, want %d",
			len(raw), ed25519.PublicKeySize)
	}
	return k, nil
}
