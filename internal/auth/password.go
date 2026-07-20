package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters. These are deliberately conservative defaults for a
// self-hosted single-binary service; tune via NewService options if a
// deployment needs different cost.
const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024 // KiB (64 MiB)
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// ErrInvalidPasswordHash is returned when a stored hash is not a
// well-formed argon2id PHC string.
var ErrInvalidPasswordHash = errors.New("auth: invalid password hash")

// HashPassword hashes a plaintext password with argon2id, returning a
// self-describing PHC-format string:
//
//	$argon2id$v=19$m=<mem>,t=<time>,p=<threads>$<salt-b64>$<hash-b64>
//
// The salt is random per call. Never log the return value's input.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword reports whether password matches the given argon2id PHC
// hash, in constant time. A malformed hash is treated as a non-match (with
// an error), never a panic.
func VerifyPassword(encodedHash, password string) (bool, error) {
	params, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(hash)))
	if subtle.ConstantTimeCompare(candidate, hash) == 1 {
		return true, nil
	}
	return false, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// Expect: "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrInvalidPasswordHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("%w: %v", ErrInvalidPasswordHash, err)
	}
	if version != argon2.Version {
		return argonParams{}, nil, nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidPasswordHash, version)
	}

	var p argonParams
	var mem, t int
	var threads int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &threads); err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("%w: %v", ErrInvalidPasswordHash, err)
	}
	p.memory, p.time, p.threads = uint32(mem), uint32(t), uint8(threads)

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("%w: bad salt encoding", ErrInvalidPasswordHash)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("%w: bad hash encoding", ErrInvalidPasswordHash)
	}

	return p, salt, hash, nil
}
