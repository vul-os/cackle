// Package keyvault is the pure-crypto half of Cackle's at-rest protection
// for event signing keys. It knows nothing about SQL: it derives a
// key-encryption key (KEK) from operator-supplied material, wraps a
// data-encryption key (DEK) under it, and seals/opens individual secrets
// under the DEK.
//
// Key hierarchy (the shape is deliberately the one SlipScan's credential
// vault uses — see that repo's crates/slipscan-core/src/secrets/vault.rs):
//
//	operator ──supplies── passphrase or keyfile  (never on disk in the DB)
//	                       │ Argon2id / HKDF-SHA256
//	                      KEK (32 bytes, derived per boot, never stored)
//	                       │ wraps (XChaCha20-Poly1305, AAD-bound)
//	key_vault    ──holds── DEK ciphertext
//	                       │ seals (XChaCha20-Poly1305, per-secret nonce + AAD)
//	event_keys   ──holds── sealed_private_key
//
// Two properties are the whole point:
//
//   - Copying the SQLite file off the machine yields nothing usable. The KEK
//     is not in it, and the DEK is only ciphertext.
//   - There is NO key material in this package that can be produced without
//     an operator Source. A missing or empty passphrase is an error, never a
//     zero value that happens to work. Callers cannot obtain a usable Vault
//     by accident, which is what keeps the fail-closed guarantee in
//     internal/store from depending on a caller remembering to check a flag.
//
// The envelope (rather than deriving a key straight from the passphrase and
// encrypting each secret with it) exists so the passphrase can be rotated by
// re-wrapping ONE row, without touching — or even reading — a single event
// key's plaintext.
package keyvault

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// Errors returned by this package. They are deliberately specific: an
// operator reading a boot failure needs to know whether they supplied
// nothing, supplied something too weak, or supplied the wrong thing.
var (
	// ErrNoSource means no key material was supplied at all. This is the
	// fail-closed case: it must never be recovered from by falling back to
	// storing plaintext.
	ErrNoSource = errors.New("keyvault: no key material supplied")
	// ErrWeakSource means material was supplied but is too weak to derive a
	// KEK from.
	ErrWeakSource = errors.New("keyvault: key material too weak")
	// ErrWrongKey means an unwrap/open failed authentication — in practice
	// the wrong passphrase or keyfile for this database.
	ErrWrongKey = errors.New("keyvault: key material does not match this database")
)

// Source kinds, as recorded in the key_vault row so a later boot can tell an
// operator what kind of material this database expects.
const (
	KindPassphrase = "passphrase"
	KindKeyfile    = "keyfile"
	// KindDemo is a passphrase Source that Cackle itself minted for
	// `--demo`. It is recorded distinctly so that a NON-demo boot against a
	// demo database refuses instead of quietly working: the material behind
	// it is not secret. See DemoSource.
	KindDemo = "demo"
)

// KDF names, as recorded in the key_vault row.
const (
	KDFArgon2id = "argon2id"
	KDFHKDF     = "hkdf-sha256"
)

// minPassphraseRunes is the shortest passphrase this package will derive a
// KEK from. A short passphrase against a stolen database file is an offline
// guess-as-fast-as-you-like attack; Argon2id raises the cost per guess but
// cannot rescue four characters.
const minPassphraseRunes = 12

// minKeyfileBytes is the shortest keyfile accepted. A keyfile is expected to
// be raw random bytes (`head -c 32 /dev/urandom > cackle.key`), so there is
// no reason for it to be smaller than the key it derives.
const minKeyfileBytes = 32

// kekLen is the derived KEK length; dekLen the generated DEK length. Both
// are XChaCha20-Poly1305 keys.
const (
	kekLen = chacha20poly1305.KeySize
	dekLen = chacha20poly1305.KeySize
)

// saltLen is the per-database KDF salt length.
const saltLen = 16

// Argon2id cost defaults. Tuned to be meaningfully expensive per guess on a
// stolen database while staying acceptable in a container at boot: this runs
// exactly once per process, not per request.
const (
	defaultArgonTime   uint32 = 3
	defaultArgonMemory uint32 = 64 * 1024 // KiB, i.e. 64 MiB
	defaultArgonLanes  uint8  = 4
)

// hkdfInfo domain-separates the keyfile KDF.
var hkdfInfo = []byte("cackle.keyvault.kek.v1")

// Source is operator-supplied key material: a passphrase or the contents of
// a keyfile. It is opaque on purpose — there is no accessor that returns the
// material, so it cannot be logged, serialised, or returned through an API.
// Construct one with Passphrase, Keyfile, or DemoSource; the zero Source is
// unusable and every method on it reports ErrNoSource.
type Source struct {
	kind     string
	material []byte
}

// Passphrase builds a Source from an operator passphrase.
//
// An empty or whitespace-only passphrase is ErrNoSource, NOT a Source that
// derives some degenerate key. This is the single most important line in the
// package: a sibling repo's anti-rollback check silently stopped verifying
// when its passphrase was blank, and its own tests were green because they
// took that path. A blank passphrase here can only ever produce an error.
func Passphrase(p string) (Source, error) {
	if strings.TrimSpace(p) == "" {
		return Source{}, fmt.Errorf("%w: passphrase is empty", ErrNoSource)
	}
	if n := utf8.RuneCountInString(p); n < minPassphraseRunes {
		return Source{}, fmt.Errorf("%w: passphrase is %d characters, need at least %d", ErrWeakSource, n, minPassphraseRunes)
	}
	return Source{kind: KindPassphrase, material: []byte(p)}, nil
}

// Keyfile builds a Source from the raw contents of a keyfile. Trailing
// newlines are trimmed so that a keyfile written by a shell redirect behaves
// the same as one written byte-exactly.
func Keyfile(raw []byte) (Source, error) {
	trimmed := trimTrailingNewlines(raw)
	if len(trimmed) == 0 {
		return Source{}, fmt.Errorf("%w: keyfile is empty", ErrNoSource)
	}
	if len(trimmed) < minKeyfileBytes {
		return Source{}, fmt.Errorf("%w: keyfile is %d bytes, need at least %d", ErrWeakSource, len(trimmed), minKeyfileBytes)
	}
	material := make([]byte, len(trimmed))
	copy(material, trimmed)
	return Source{kind: KindKeyfile, material: material}, nil
}

// DemoSource returns the fixed, PUBLICLY KNOWN Source that `cackle --demo`
// seals its throwaway event keys with.
//
// This is not a security measure and is not pretending to be one. It exists
// so demo mode — which already prints its login password to stdout — needs
// no setup, while still never writing a plaintext private key to disk. The
// material is a constant in this file, so anyone with a demo database can
// unseal it.
//
// The safety property is not secrecy, it is that a demo database ANNOUNCES
// itself: the key_vault row records KindDemo, and internal/store refuses to
// unlock such a database from a non-demo Source (and vice versa). A demo
// database can therefore never be promoted into a real deployment by
// accident — the promotion fails closed with an explanation.
func DemoSource() Source {
	return Source{kind: KindDemo, material: []byte("cackle-demo-vault-not-secret-v1")}
}

// Kind reports which sort of material this Source holds — one of
// KindPassphrase, KindKeyfile, KindDemo. Safe to log.
func (s Source) Kind() string { return s.kind }

// Valid reports whether s holds usable material.
func (s Source) Valid() bool { return s.kind != "" && len(s.material) > 0 }

// Zero wipes the held material. Callers should defer it once the KEK has
// been derived. Go gives no guarantee the runtime kept no copy (a growing
// string, a stack spill), so this reduces the window rather than closing it —
// see the docs/SELF-HOSTING.md threat model, which says so out loud rather
// than implying process memory is protected.
func (s Source) Zero() { wipe(s.material) }

// KDFParams are the stored, per-database parameters needed to re-derive the
// same KEK from the same Source on a later boot. They are not secret: they
// live in the key_vault row in the clear, exactly like a password hash's
// salt and cost parameters.
type KDFParams struct {
	// Name is the KDF: KDFArgon2id for a passphrase/demo Source,
	// KDFHKDF for a keyfile.
	Name string
	Salt []byte
	// Time, Memory and Lanes are the Argon2id cost parameters. They are
	// zero and ignored for KDFHKDF.
	Time   uint32
	Memory uint32
	Lanes  uint8
}

// NewKDFParams mints fresh parameters (including a fresh random salt)
// appropriate to the given Source kind. Used once, when a database's DEK is
// first created or re-wrapped.
func NewKDFParams(kind string) (KDFParams, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return KDFParams{}, fmt.Errorf("keyvault: generate salt: %w", err)
	}
	switch kind {
	case KindPassphrase:
		return KDFParams{
			Name:   KDFArgon2id,
			Salt:   salt,
			Time:   defaultArgonTime,
			Memory: defaultArgonMemory,
			Lanes:  defaultArgonLanes,
		}, nil
	case KindKeyfile, KindDemo:
		// KindDemo uses the cheap KDF deliberately. Argon2id exists to make
		// each guess of a LOW-ENTROPY secret expensive; the demo material is
		// a published constant, so stretching it would buy nothing and would
		// only slow every demo boot (and every test) down. Saying that here
		// is better than a reader later "fixing" it and concluding the demo
		// vault is therefore protected.
		return KDFParams{Name: KDFHKDF, Salt: salt}, nil
	default:
		return KDFParams{}, fmt.Errorf("keyvault: unknown source kind %q", kind)
	}
}

// DeriveKEK derives the 32-byte key-encryption key for this Source under p.
// The result is never stored; it exists only to unwrap the DEK.
func (s Source) DeriveKEK(p KDFParams) ([]byte, error) {
	if !s.Valid() {
		return nil, ErrNoSource
	}
	if len(p.Salt) == 0 {
		return nil, errors.New("keyvault: KDF params have no salt")
	}
	switch p.Name {
	case KDFArgon2id:
		if p.Time == 0 || p.Memory == 0 || p.Lanes == 0 {
			return nil, fmt.Errorf("keyvault: argon2id params incomplete (time=%d memory=%d lanes=%d)", p.Time, p.Memory, p.Lanes)
		}
		return argon2.IDKey(s.material, p.Salt, p.Time, p.Memory, p.Lanes, kekLen), nil
	case KDFHKDF:
		kek := make([]byte, kekLen)
		r := hkdf.New(sha256.New, s.material, p.Salt, hkdfInfo)
		if _, err := io.ReadFull(r, kek); err != nil {
			return nil, fmt.Errorf("keyvault: hkdf: %w", err)
		}
		return kek, nil
	default:
		return nil, fmt.Errorf("keyvault: unknown kdf %q", p.Name)
	}
}

// GenerateDEK returns a fresh random data-encryption key.
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, dekLen)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("keyvault: generate dek: %w", err)
	}
	return dek, nil
}

// Seal encrypts plaintext under key with a fresh random nonce, binding aad
// into the authentication tag. It returns the ciphertext and the nonce, which
// the caller stores alongside it.
func Seal(key, aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, nil, fmt.Errorf("keyvault: aead: %w", err)
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("keyvault: generate nonce: %w", err)
	}
	return aead.Seal(nil, nonce, plaintext, aad), nonce, nil
}

// Open reverses Seal. A failure to authenticate is reported as ErrWrongKey —
// the overwhelmingly likely cause is the wrong passphrase or keyfile, and
// that is the error an operator can act on. It is also what a tampered
// ciphertext or a ciphertext moved between rows produces, since the row's
// identity is bound in via aad.
func Open(key, aad, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("keyvault: aead: %w", err)
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("keyvault: nonce is %d bytes, want %d", len(nonce), aead.NonceSize())
	}
	pt, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrWrongKey
	}
	return pt, nil
}

// Vault holds an unwrapped DEK and seals/opens individual secrets under it.
// A non-nil *Vault is proof that key material was supplied and matched the
// database: there is no way to construct one otherwise.
type Vault struct {
	dek  []byte
	kind string
}

// NewVault wraps an unwrapped DEK. kind is the Source kind the DEK was
// unwrapped with, carried along for diagnostics only.
func NewVault(dek []byte, kind string) (*Vault, error) {
	if len(dek) != dekLen {
		return nil, fmt.Errorf("keyvault: dek is %d bytes, want %d", len(dek), dekLen)
	}
	own := make([]byte, dekLen)
	copy(own, dek)
	return &Vault{dek: own, kind: kind}, nil
}

// SourceKind reports the kind of material this Vault was unlocked with.
func (v *Vault) SourceKind() string {
	if v == nil {
		return ""
	}
	return v.kind
}

// Seal encrypts plaintext under the DEK, binding aad.
func (v *Vault) Seal(aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	if v == nil || len(v.dek) != dekLen {
		return nil, nil, ErrNoSource
	}
	return Seal(v.dek, aad, plaintext)
}

// Open decrypts a ciphertext sealed by Seal with the same aad.
func (v *Vault) Open(aad, nonce, ciphertext []byte) ([]byte, error) {
	if v == nil || len(v.dek) != dekLen {
		return nil, ErrNoSource
	}
	return Open(v.dek, aad, nonce, ciphertext)
}

// Zero wipes the DEK. Same caveat as Source.Zero: it narrows the window, it
// does not make process memory secret.
func (v *Vault) Zero() {
	if v != nil {
		wipe(v.dek)
	}
}

// Fingerprint returns a short, non-reversible label for a secret: the first
// 8 hex characters of a domain-separated SHA-256. Enough to tell "did this
// change" in a log line or an operator report, never enough to recover
// anything. Modelled on SlipScan's vault metadata fingerprint, which exists
// for exactly this reason: an operator needs to confirm a rotation happened
// without ever being shown the secret.
func Fingerprint(label string, secret []byte) string {
	h := sha256.New()
	h.Write([]byte("cackle.keyvault.fingerprint.v1"))
	h.Write([]byte{0})
	h.Write([]byte(label))
	h.Write([]byte{0})
	h.Write(secret)
	return hex.EncodeToString(h.Sum(nil))[:fingerprintHexLen]
}

// fingerprintHexLen is how much of the digest a Fingerprint shows. Short
// enough to be useless for recovering the secret, long enough to distinguish
// one rotation from the next in an operator's eyes.
const fingerprintHexLen = 8

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func trimTrailingNewlines(b []byte) []byte {
	end := len(b)
	for end > 0 && (b[end-1] == '\n' || b[end-1] == '\r') {
		end--
	}
	return b[:end]
}
