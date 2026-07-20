// Package config loads Cackle's runtime configuration. Every setting is
// env-first (prefixed CACKLE_) with sane defaults, so the binary boots with
// zero setup. Flags (wired in cmd/cackle) may override the environment.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the fully-resolved runtime configuration for a Cackle process.
type Config struct {
	// Addr is the address the HTTP server listens on, e.g. ":8080".
	Addr string
	// DB is the path to the SQLite database file (or ":memory:").
	DB string
	// BaseURL is the externally-visible base URL, used for links (password
	// reset emails, payment redirect callbacks, etc).
	BaseURL string
	// SessionSecret is 32+ bytes of key material used to authenticate
	// anything that needs a server-side secret beyond the per-session
	// random token (e.g. CSRF double-submit tokens). It is never logged.
	SessionSecret string
	// PaystackSecretKey is the Paystack API secret key. Empty means the
	// Paystack provider is unavailable; there is no default and it is
	// never persisted or logged.
	PaystackSecretKey string
	// Demo indicates the process should boot fully seeded with demo data
	// (internal/demo), with no external dependencies required.
	Demo bool
	// MediaDir is the directory uploaded event images are stored under (see
	// internal/media and internal/httpapi's image handlers). Defaults to a
	// "media" directory beside the SQLite database file.
	MediaDir string
}

// Flags carries CLI flag values, mirroring the environment variables below.
// Zero values mean "not explicitly set on the command line" and fall back to
// the environment, then to the default.
type Flags struct {
	Addr     string
	DB       string
	BaseURL  string
	Demo     bool
	MediaDir string
	// DemoSet records whether --demo was explicitly passed, so a false
	// zero value doesn't shadow CACKLE_DEMO=1.
	DemoSet bool
}

const (
	envAddr          = "CACKLE_ADDR"
	envDB            = "CACKLE_DB"
	envBaseURL       = "CACKLE_BASE_URL"
	envSessionSecret = "CACKLE_SESSION_SECRET"
	envPaystackKey   = "CACKLE_PAYSTACK_SECRET_KEY"
	envDemo          = "CACKLE_DEMO"
	envMediaDir      = "CACKLE_MEDIA_DIR"

	defaultAddr    = ":8080"
	defaultDB      = "./cackle.db"
	defaultBaseURL = "http://localhost:8080"

	// sessionSecretByteLen is the amount of random key material generated
	// when no secret is configured.
	sessionSecretByteLen = 32
)

// defaultBaseURLFor derives a sensible public base URL from the listen
// address, so that running on a non-default port does not silently emit
// links pointing at port 8080. A wildcard or empty host becomes localhost;
// anything genuinely public should be set via CACKLE_BASE_URL.
func defaultBaseURLFor(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return defaultBaseURL
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	if port == "" {
		return "http://" + host
	}
	return "http://" + net.JoinHostPort(host, port)
}

// Load resolves configuration from flags, then the environment, then
// defaults. If no session secret is configured anywhere, one is generated
// and persisted next to the database file so restarts keep sessions valid.
func Load(f Flags) (*Config, error) {
	cfg := &Config{
		Addr:              firstNonEmpty(f.Addr, os.Getenv(envAddr), defaultAddr),
		DB:                firstNonEmpty(f.DB, os.Getenv(envDB), defaultDB),
		PaystackSecretKey: os.Getenv(envPaystackKey),
	}

	// BaseURL defaults to whatever we are actually listening on. Pinning it to
	// port 8080 regardless of --addr meant password-reset links and payment
	// callbacks pointed at a port nothing was serving.
	cfg.BaseURL = firstNonEmpty(f.BaseURL, os.Getenv(envBaseURL), defaultBaseURLFor(cfg.Addr))

	if f.DemoSet {
		cfg.Demo = f.Demo
	} else {
		cfg.Demo = envBool(envDemo)
	}

	cfg.MediaDir = firstNonEmpty(f.MediaDir, os.Getenv(envMediaDir), defaultMediaDirFor(cfg.DB))

	secret := os.Getenv(envSessionSecret)
	if secret == "" {
		s, err := loadOrCreatePersistedSecret(cfg.DB)
		if err != nil {
			return nil, fmt.Errorf("config: session secret: %w", err)
		}
		secret = s
	}
	if len(secret) < 16 {
		return nil, errors.New("config: session secret too short (must be at least 16 characters)")
	}
	cfg.SessionSecret = secret

	return cfg, nil
}

// firstNonEmpty returns the first non-empty string among candidates, in
// order, falling through to the final value as the default.
func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return c
		}
	}
	return ""
}

// envBool parses a boolean-ish environment variable. Unset or unrecognised
// values are false.
func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		return false
	}
}

// defaultMediaDirFor derives the default uploaded-image storage directory
// from the database path: a "media" sibling directory next to the DB file,
// so a self-hoster pointing --db somewhere specific gets their uploads
// stored alongside it with zero extra configuration. An in-memory or empty
// DB path (tests, ephemeral runs) falls back to "./media".
func defaultMediaDirFor(dbPath string) string {
	if dbPath == "" || dbPath == ":memory:" {
		return "./media"
	}
	return filepath.Join(filepath.Dir(dbPath), "media")
}

// secretFilePath derives a persisted-secret path from the database path so
// the generated secret survives restarts without requiring extra config.
func secretFilePath(dbPath string) string {
	if dbPath == "" || dbPath == ":memory:" {
		return ".cackle_session_secret"
	}
	dir := filepath.Dir(dbPath)
	return filepath.Join(dir, ".cackle_session_secret")
}

// loadOrCreatePersistedSecret reads a previously-generated secret from disk,
// or generates and persists a new one (mode 0600) if none exists yet.
func loadOrCreatePersistedSecret(dbPath string) (string, error) {
	path := secretFilePath(dbPath)

	if b, err := os.ReadFile(path); err == nil {
		secret := strings.TrimSpace(string(b))
		if len(secret) >= 16 {
			return secret, nil
		}
		// Fall through and regenerate a corrupt/short secret file.
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	buf := make([]byte, sessionSecretByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	secret := hex.EncodeToString(buf)

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist secret: %w", err)
	}
	return secret, nil
}
