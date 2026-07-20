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
