package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenByteLen is the amount of random material in a session or password
// reset token: 32 random bytes, per the house security bar.
const tokenByteLen = 32

// newToken generates a new random, URL-safe token and returns both the
// plaintext token (to hand to the caller, e.g. as a cookie or bearer value)
// and its sha256 hex digest (the only form ever persisted).
func newToken() (plaintext string, hash string, err error) {
	buf := make([]byte, tokenByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, hashToken(plaintext), nil
}

// hashToken returns the sha256 hex digest of a plaintext token. Session and
// password-reset tokens are always looked up by this hash — the plaintext
// is never stored and never logged.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// NewOpaqueToken mints a token using the exact same scheme as sessions and
// password resets (32 random bytes, URL-safe base64 plaintext, sha256 hex
// digest for at-rest storage) for any OTHER single-use, hashed-at-rest
// secret elsewhere in the system — currently org invites (see
// internal/store's org_invites table and internal/orgs). Exported so
// callers outside this package never have to (and never should)
// reimplement token generation themselves; this remains the only package
// that mints or checks any such token.
func NewOpaqueToken() (plaintext, hash string, err error) {
	return newToken()
}

// HashOpaqueToken returns the sha256 hex digest of a plaintext token minted
// by NewOpaqueToken, for looking up its persisted hash.
func HashOpaqueToken(plaintext string) string {
	return hashToken(plaintext)
}
