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

	"github.com/vul-os/cackle/internal/store/keyvault"
)

// Config is the fully-resolved runtime configuration for a Cackle process.
type Config struct {
	// Addr is the address the HTTP server listens on, e.g. ":8080".
	Addr string
	// DB is the path to the SQLite database file (or ":memory:").
	DB string
	// BaseURL is the externally-visible base URL, used for links: payment
	// redirect callbacks, and the password-reset link `cackle
	// reset-password` prints for an operator to pass on by hand. It is
	// not used for email — Cackle sends none.
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

	// KeySource is the operator-supplied material that unlocks the event
	// signing keys in the database (internal/store/keyvault). It is opaque:
	// there is no accessor that yields the passphrase back, so it cannot be
	// logged or serialised even by accident.
	//
	// The zero value means NO key material is configured. That is not a mode
	// of operation — it is the state in which cmd/cackle refuses to start
	// anything that would need a private key. There is deliberately no
	// default, no generated-and-persisted file (as with SessionSecret) and no
	// fallback: a signing key that protects every ticket for every event must
	// be unlocked by a human decision, not by a convenience path that a
	// future reader would find already in place and assume was considered.
	KeySource keyvault.Source

	// KeySourceOrigin names the environment variable KeySource came from, for
	// boot log lines and error messages. Empty when none is configured. Safe
	// to log — it is a variable name, never a value.
	KeySourceOrigin string

	// HostScope decides whose events the public listing at GET /api/events
	// shows. See HostScope's own doc — this is a tenancy setting, not a
	// display preference.
	HostScope HostScope
	// HostOrg names the one organisation this box presents as, when
	// HostScope is HostScopeSingle. It is an org SLUG (preferred) or an org
	// id, resolved at request time against the database — config cannot
	// reach the database, and refusing to boot on a typo would also refuse
	// to boot before the org has been created. Empty for every other scope.
	HostOrg string
	// HostName is the operator's own name for this box ("The Bijou",
	// "Kloof Street Sessions"), shown as "Events at <HostName>". Empty means
	// unset: the UI falls back to the organisation's own name (single scope)
	// or to a name-free heading, and never invents one.
	HostName string
}

// HostScope decides whose events the public listing shows. Cackle is
// self-hosted by an organiser, so a root page that reads "browse all events"
// implies a global marketplace that does not exist. This setting makes the
// truth explicit and configurable.
type HostScope string

const (
	// HostScopeOwn — the default — shows the published events of every
	// organisation on THIS box, and nothing else. On a box that hosts one
	// organisation this is that organisation's events; on a box that hosts
	// several it is all of theirs, labelled per organisation.
	HostScopeOwn HostScope = "own"
	// HostScopeSingle presents the box as ONE organisation, named by
	// HostOrg. Events belonging to any other organisation on the same box
	// are not listed publicly. This is the common case for a single venue.
	HostScopeSingle HostScope = "single"
	// HostScopePeers is reserved for opt-in peer feeds — listing events from
	// other Cackle boxes an operator has explicitly chosen to federate with.
	//
	// NO PEER DATA EXISTS TODAY. There is no peer event source in this
	// binary, so this scope behaves EXACTLY as HostScopeOwn: it lists this
	// box's own organisations' events and nothing else. It is accepted as a
	// value so that the setting an operator writes now keeps its meaning
	// when peer feeds land, and so the seam has one named place rather than
	// being invented twice. The listing response reports
	// "peers_included": false so a client can never assume otherwise.
	HostScopePeers HostScope = "peers"
)

// Valid reports whether s is one of the three known scopes.
func (s HostScope) Valid() bool {
	switch s {
	case HostScopeOwn, HostScopeSingle, HostScopePeers:
		return true
	}
	return false
}

// IncludesPeerEvents reports whether the listing may contain events from
// another box. It is FALSE for every scope, including HostScopePeers,
// because no peer event source exists in this binary. It is a method rather
// than a literal so that the day one does exist, there is exactly one place
// that has to start saying true — and every caller (API response, docs
// assertions, tests) follows.
func (s HostScope) IncludesPeerEvents() bool { return false }

// Key-material environment variables. Exactly one may be set; see
// resolveKeySource.
const (
	envKeyPassphrase     = "CACKLE_KEY_PASSPHRASE"
	envKeyPassphraseFile = "CACKLE_KEY_PASSPHRASE_FILE"
	envKeyFile           = "CACKLE_KEY_FILE"
)

// HasKeySource reports whether operator key material was configured.
func (c *Config) HasKeySource() bool {
	return c != nil && c.KeySource.Valid()
}

// resolveKeySource turns the environment into a keyvault.Source.
//
// Rules, each of which exists to close a way this could silently do nothing:
//
//   - Nothing set: no Source, NO error. Absence is reported to the caller as
//     absence; it is cmd/cackle's job to refuse to start, with a message that
//     can name the specific operation being refused.
//   - Set but empty (`CACKLE_KEY_PASSPHRASE=`): an ERROR, not absence.
//     os.LookupEnv can tell these apart and they mean different things — an
//     empty value is an operator who believes they configured a passphrase.
//     Treating it as "unset" is precisely the blank-passphrase-silently-
//     accepted failure this whole change exists to avoid.
//   - More than one set: an ERROR. Resolving the ambiguity by precedence
//     would mean a deployment could be unlocked by material its operator
//     thinks is unused, and rotation would silently target the wrong one.
//   - A file that cannot be read: an ERROR naming the path.
func resolveKeySource() (keyvault.Source, string, error) {
	type candidate struct {
		env   string
		value string
	}
	var set []candidate
	for _, name := range []string{envKeyPassphrase, envKeyPassphraseFile, envKeyFile} {
		v, ok := os.LookupEnv(name)
		if !ok {
			continue
		}
		if strings.TrimSpace(v) == "" {
			return keyvault.Source{}, "", fmt.Errorf("%s is set but empty; unset it or give it a value (a blank passphrase is refused, never treated as 'no encryption')", name)
		}
		set = append(set, candidate{env: name, value: v})
	}

	switch len(set) {
	case 0:
		return keyvault.Source{}, "", nil
	case 1:
	default:
		names := make([]string, 0, len(set))
		for _, c := range set {
			names = append(names, c.env)
		}
		return keyvault.Source{}, "", fmt.Errorf("%s are all set; exactly one source of event-key material may be configured", strings.Join(names, ", "))
	}

	c := set[0]
	switch c.env {
	case envKeyPassphrase:
		src, err := keyvault.Passphrase(c.value)
		if err != nil {
			return keyvault.Source{}, "", fmt.Errorf("%s: %w", c.env, err)
		}
		return src, c.env, nil

	case envKeyPassphraseFile:
		raw, err := os.ReadFile(c.value)
		if err != nil {
			return keyvault.Source{}, "", fmt.Errorf("%s: read %s: %w", c.env, c.value, err)
		}
		// A passphrase file is a text file; a trailing newline from an editor
		// or a shell redirect is not part of the passphrase.
		src, err := keyvault.Passphrase(strings.TrimRight(string(raw), "\r\n"))
		if err != nil {
			return keyvault.Source{}, "", fmt.Errorf("%s (%s): %w", c.env, c.value, err)
		}
		return src, c.env, nil

	case envKeyFile:
		raw, err := os.ReadFile(c.value)
		if err != nil {
			return keyvault.Source{}, "", fmt.Errorf("%s: read %s: %w", c.env, c.value, err)
		}
		src, err := keyvault.Keyfile(raw)
		if err != nil {
			return keyvault.Source{}, "", fmt.Errorf("%s (%s): %w", c.env, c.value, err)
		}
		return src, c.env, nil
	}

	return keyvault.Source{}, "", fmt.Errorf("config: unhandled key material variable %q", c.env)
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
	envHostScope     = "CACKLE_HOST_SCOPE"
	envHostOrg       = "CACKLE_HOST_ORG"
	envHostName      = "CACKLE_HOST_NAME"

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

	// Event-key material. A misconfiguration here is fatal at Load; a total
	// ABSENCE is not, because "no key material" is a legitimate configuration
	// for a process that only serves gates read-only, and because refusing
	// with a message about the specific thing being refused is cmd/cackle's
	// job (see requireKeyVault there).
	keySrc, keyOrigin, err := resolveKeySource()
	if err != nil {
		return nil, fmt.Errorf("config: event key material: %w", err)
	}
	cfg.KeySource = keySrc
	cfg.KeySourceOrigin = keyOrigin

	scope, org, name, err := resolveHostScope()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg.HostScope = scope
	cfg.HostOrg = org
	cfg.HostName = name

	return cfg, nil
}

// resolveHostScope reads the three CACKLE_HOST_* variables.
//
// Both failure modes here are refusals to start, because both are an
// operator who believes their box is narrower than it is:
//
//   - An unrecognised scope. Falling back to the default would mean a
//     typo ("singel") silently publishes every organisation's events on a
//     box the operator meant to present as one venue.
//   - CACKLE_HOST_ORG set alongside a scope that ignores it, or absent when
//     the scope requires it. Same reasoning: the setting names a tenancy
//     boundary, and a tenancy boundary that is quietly not applied is the
//     failure this whole setting exists to prevent.
//
// CACKLE_HOST_ORG is NOT resolved against the database here — config has no
// database, and the org may not have been created yet on first boot. The
// listing route resolves it per request and fails closed (an error, never
// "show everything") if it names no organisation.
func resolveHostScope() (HostScope, string, string, error) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(envHostScope)))
	scope := HostScopeOwn
	if raw != "" {
		scope = HostScope(raw)
		if !scope.Valid() {
			return "", "", "", fmt.Errorf("%s=%q is not a known scope (want one of: %s, %s, %s)",
				envHostScope, raw, HostScopeOwn, HostScopeSingle, HostScopePeers)
		}
	}

	org := strings.TrimSpace(os.Getenv(envHostOrg))
	switch {
	case scope == HostScopeSingle && org == "":
		return "", "", "", fmt.Errorf("%s=%s requires %s to name the one organisation this host presents as (its URL name, e.g. the-bijou)",
			envHostScope, HostScopeSingle, envHostOrg)
	case scope != HostScopeSingle && org != "":
		return "", "", "", fmt.Errorf("%s is set but %s=%s ignores it; use %s=%s to present this host as one organisation, or unset %s",
			envHostOrg, envHostScope, scope, envHostScope, HostScopeSingle, envHostOrg)
	}

	return scope, org, strings.TrimSpace(os.Getenv(envHostName)), nil
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

// ScanCacheDir is where the shared DMTAP sync engine's compiled WebAssembly
// machine code is cached, derived from the database path so a self-hoster gets
// it beside their data with zero extra configuration.
//
// It holds NOTHING secret and nothing durable: only wazero's compilation
// output, which is a pure function of a module embedded in the binary. Deleting
// it costs a few hundred milliseconds on the next reconciliation report and
// nothing else, which is why an unwritable or missing directory is not an error
// anywhere — it degrades startup speed, never correctness. It is deliberately
// NOT under MediaDir, which is served over HTTP.
//
// An in-memory or empty DB path (tests, ephemeral runs) returns "", meaning "do
// not cache at all" rather than scattering cache directories through whatever
// the working directory happens to be.
func (c *Config) ScanCacheDir() string {
	if c == nil || c.DB == "" || c.DB == ":memory:" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.DB), "wasm-cache")
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
