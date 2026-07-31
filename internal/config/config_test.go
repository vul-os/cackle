package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vul-os/cackle/internal/store/keyvault"
)

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"", "", "x"}, "x"},
		{[]string{"a", "b"}, "a"},
		{[]string{"", "b", "c"}, "b"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := firstNonEmpty(c.in...); got != c.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEnvBool(t *testing.T) {
	const key = "CACKLE_TEST_BOOL"
	for _, v := range []string{"1", "true", "yes", "on", "TRUE", "On", "  yes  "} {
		t.Setenv(key, v)
		if !envBool(key) {
			t.Errorf("envBool(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"0", "false", "no", "off", "", "garbage", "2"} {
		t.Setenv(key, v)
		if envBool(key) {
			t.Errorf("envBool(%q) = true, want false", v)
		}
	}
}

// defaultBaseURLFor is the wave-that-fixed-it behaviour: the public URL follows
// the listen address instead of being pinned to :8080, and wildcard/empty hosts
// collapse to localhost.
func TestDefaultBaseURLFor(t *testing.T) {
	cases := map[string]string{
		":8080":           "http://localhost:8080",
		"127.0.0.1:9999":  "http://127.0.0.1:9999",
		"0.0.0.0:8080":    "http://localhost:8080",
		"example.com:443": "http://example.com:443",
		"localhost":       "http://localhost:8080", // no port -> SplitHostPort errors -> default
		"":                "http://localhost:8080", // errors -> default
	}
	for addr, want := range cases {
		if got := defaultBaseURLFor(addr); got != want {
			t.Errorf("defaultBaseURLFor(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestDefaultMediaDirFor(t *testing.T) {
	cases := map[string]string{
		"":                    "./media",
		":memory:":            "./media",
		"/var/data/cackle.db": filepath.Join("/var/data", "media"),
		"./cackle.db":         filepath.Join(".", "media"),
	}
	for db, want := range cases {
		if got := defaultMediaDirFor(db); got != want {
			t.Errorf("defaultMediaDirFor(%q) = %q, want %q", db, got, want)
		}
	}
}

func TestSecretFilePath(t *testing.T) {
	cases := map[string]string{
		"":                    ".cackle_session_secret",
		":memory:":            ".cackle_session_secret",
		"/var/data/cackle.db": filepath.Join("/var/data", ".cackle_session_secret"),
	}
	for db, want := range cases {
		if got := secretFilePath(db); got != want {
			t.Errorf("secretFilePath(%q) = %q, want %q", db, got, want)
		}
	}
}

// isolateEnv clears every CACKLE_* key Load reads so an ambient value in the
// developer's shell can't leak into a Load test.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		envAddr, envDB, envBaseURL, envSessionSecret, envMediaDir, envDemo, envPaystackKey,
		envHostScope, envHostOrg, envHostName,
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_DefaultsDeriveBaseURLAndPersistSecret(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cackle.db")

	cfg, err := Load(Flags{DB: dbPath, Addr: ":8080"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want http://localhost:8080", cfg.BaseURL)
	}
	if cfg.MediaDir != filepath.Join(dir, "media") {
		t.Errorf("MediaDir = %q, want %q", cfg.MediaDir, filepath.Join(dir, "media"))
	}
	if len(cfg.SessionSecret) < 16 {
		t.Errorf("SessionSecret too short: %q", cfg.SessionSecret)
	}
	// The generated secret is persisted to a file beside the DB (not the DB).
	if _, err := os.Stat(filepath.Join(dir, ".cackle_session_secret")); err != nil {
		t.Errorf("secret file not created beside the DB: %v", err)
	}
	// A second Load against the same DB dir reuses that persisted secret.
	cfg2, err := Load(Flags{DB: dbPath, Addr: ":8080"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.SessionSecret != cfg.SessionSecret {
		t.Error("second Load produced a different secret — persistence is broken")
	}
}

func TestLoad_FlagBeatsEnvBeatsDefault(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv(envAddr, ":7777")                                     // env
	t.Setenv(envSessionSecret, "an-explicit-32-char-long-secret!") // >= 16

	cfg, err := Load(Flags{Addr: ":9999", DB: filepath.Join(dir, "x.db")}) // flag
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999 (flag beats env)", cfg.Addr)
	}
	if cfg.BaseURL != "http://localhost:9999" {
		t.Errorf("BaseURL = %q, want http://localhost:9999 (derived from --addr flag)", cfg.BaseURL)
	}
	if cfg.SessionSecret != "an-explicit-32-char-long-secret!" {
		t.Errorf("SessionSecret = %q, want the explicit env value", cfg.SessionSecret)
	}
}

func TestLoad_ShortSecretRejected(t *testing.T) {
	isolateEnv(t)
	t.Setenv(envSessionSecret, "tooshort") // 8 chars < 16
	t.Setenv(envDB, filepath.Join(t.TempDir(), "x.db"))
	if _, err := Load(Flags{}); err == nil {
		t.Error("Load accepted a <16-char session secret; want an error")
	}
}

// --- event-key material ---------------------------------------------------

// clearKeyEnv removes every key-material variable. It uses os.Unsetenv rather
// than t.Setenv(k, "") — which is how the rest of this file spells "unset" —
// because for THIS group of variables an empty value is a deliberate error and
// not a synonym for absent. See resolveKeySource.
func clearKeyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{envKeyPassphrase, envKeyPassphraseFile, envKeyFile} {
		if old, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { os.Setenv(k, old) })
		} else {
			t.Cleanup(func() { os.Unsetenv(k) })
		}
		os.Unsetenv(k)
	}
}

func TestResolveKeySource_AbsentIsNotAnError(t *testing.T) {
	clearKeyEnv(t)

	src, origin, err := resolveKeySource()
	if err != nil {
		t.Fatalf("resolveKeySource with nothing set: %v", err)
	}
	if src.Valid() {
		t.Fatal("no key material configured, but a valid Source came back")
	}
	if origin != "" {
		t.Fatalf("origin = %q, want empty", origin)
	}

	// Load must also succeed: refusing belongs to cmd/cackle, which can name
	// the operation being refused. What Load must NOT do is invent material.
	isolateEnv(t)
	clearKeyEnv(t)
	cfg, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")})
	if err != nil {
		t.Fatalf("Load with no key material: %v", err)
	}
	if cfg.HasKeySource() {
		t.Fatal("Load fabricated key material out of nothing")
	}
	if cfg.KeySourceOrigin != "" {
		t.Fatalf("KeySourceOrigin = %q, want empty", cfg.KeySourceOrigin)
	}
}

// TestResolveKeySource_BlankIsAnError is the config-level half of the
// blank-passphrase lesson: an operator who writes CACKLE_KEY_PASSPHRASE= in a
// compose file believes they configured a passphrase. Silently reading that as
// "no encryption configured" is how a plaintext path stays alive.
func TestResolveKeySource_BlankIsAnError(t *testing.T) {
	for _, name := range []string{envKeyPassphrase, envKeyPassphraseFile, envKeyFile} {
		for _, val := range []string{"", "   ", "\t"} {
			t.Run(name+"/"+strconv.Quote(val), func(t *testing.T) {
				clearKeyEnv(t)
				t.Setenv(name, val)

				_, _, err := resolveKeySource()
				if err == nil {
					t.Fatalf("%s=%q was accepted", name, val)
				}
				if !strings.Contains(err.Error(), name) {
					t.Fatalf("error does not name the variable: %v", err)
				}
				if !strings.Contains(err.Error(), "set but empty") {
					t.Fatalf("error does not explain the problem: %v", err)
				}
			})
		}
	}
}

// TestResolveKeySource_AmbiguityIsAnError: resolving two configured sources by
// precedence would mean a deployment is unlocked by material its operator
// believes is unused, and a later rotation would target the wrong one.
func TestResolveKeySource_AmbiguityIsAnError(t *testing.T) {
	clearKeyEnv(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "keyfile")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envKeyPassphrase, "a-perfectly-good-passphrase")
	t.Setenv(envKeyFile, keyPath)

	_, _, err := resolveKeySource()
	if err == nil {
		t.Fatal("two configured key sources were accepted")
	}
	for _, want := range []string{envKeyPassphrase, envKeyFile, "exactly one"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

func TestResolveKeySource_Passphrase(t *testing.T) {
	clearKeyEnv(t)
	t.Setenv(envKeyPassphrase, "correct horse battery staple")

	src, origin, err := resolveKeySource()
	if err != nil {
		t.Fatalf("resolveKeySource: %v", err)
	}
	if !src.Valid() || src.Kind() != keyvault.KindPassphrase {
		t.Fatalf("kind = %q valid = %v", src.Kind(), src.Valid())
	}
	if origin != envKeyPassphrase {
		t.Fatalf("origin = %q, want %q", origin, envKeyPassphrase)
	}
}

func TestResolveKeySource_ShortPassphraseRefused(t *testing.T) {
	clearKeyEnv(t)
	t.Setenv(envKeyPassphrase, "hunter2")

	if _, _, err := resolveKeySource(); err == nil {
		t.Fatal("a 7-character passphrase was accepted for the crown jewels")
	} else if !errors.Is(err, keyvault.ErrWeakSource) {
		t.Fatalf("error = %v, want keyvault.ErrWeakSource", err)
	}
}

func TestResolveKeySource_PassphraseFileTrimsTrailingNewline(t *testing.T) {
	clearKeyEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "passphrase")
	// A trailing newline is what every editor and `echo` leaves behind; it must
	// not become part of the passphrase, or the operator's documented secret
	// would not open their own database.
	if err := os.WriteFile(path, []byte("a-file-based-passphrase\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envKeyPassphraseFile, path)

	src, origin, err := resolveKeySource()
	if err != nil {
		t.Fatalf("resolveKeySource: %v", err)
	}
	if origin != envKeyPassphraseFile {
		t.Fatalf("origin = %q", origin)
	}

	// Derive from both and compare: the file-sourced Source must equal the
	// literal passphrase without the newline.
	params := keyvault.KDFParams{Name: keyvault.KDFArgon2id, Salt: []byte("0123456789abcdef"), Time: 1, Memory: 8, Lanes: 1}
	fromFile, err := src.DeriveKEK(params)
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	literal, err := keyvault.Passphrase("a-file-based-passphrase")
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	fromLiteral, err := literal.DeriveKEK(params)
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	if !bytes.Equal(fromFile, fromLiteral) {
		t.Fatal("the trailing newline changed the derived key")
	}
}

func TestResolveKeySource_Keyfile(t *testing.T) {
	clearKeyEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "keyfile")
	if err := os.WriteFile(path, bytes.Repeat([]byte{0xa5}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envKeyFile, path)

	src, origin, err := resolveKeySource()
	if err != nil {
		t.Fatalf("resolveKeySource: %v", err)
	}
	if src.Kind() != keyvault.KindKeyfile {
		t.Fatalf("kind = %q, want %q", src.Kind(), keyvault.KindKeyfile)
	}
	if origin != envKeyFile {
		t.Fatalf("origin = %q", origin)
	}
}

func TestResolveKeySource_UnreadableFileIsAnError(t *testing.T) {
	for _, name := range []string{envKeyPassphraseFile, envKeyFile} {
		t.Run(name, func(t *testing.T) {
			clearKeyEnv(t)
			missing := filepath.Join(t.TempDir(), "nope")
			t.Setenv(name, missing)

			_, _, err := resolveKeySource()
			if err == nil {
				t.Fatal("a missing key material file was accepted")
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("error does not name the path: %v", err)
			}
		})
	}
}

func TestResolveKeySource_ShortKeyfileRefused(t *testing.T) {
	clearKeyEnv(t)
	path := filepath.Join(t.TempDir(), "keyfile")
	if err := os.WriteFile(path, []byte("too short"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envKeyFile, path)

	if _, _, err := resolveKeySource(); !errors.Is(err, keyvault.ErrWeakSource) {
		t.Fatalf("error = %v, want keyvault.ErrWeakSource", err)
	}
}

// TestConfigNeverRendersKeyMaterial: Config is passed around and is exactly the
// sort of struct that ends up in a debug log line.
func TestConfigNeverRendersKeyMaterial(t *testing.T) {
	isolateEnv(t)
	clearKeyEnv(t)
	const secret = "a-very-secret-operator-passphrase"
	t.Setenv(envKeyPassphrase, secret)

	cfg, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.HasKeySource() {
		t.Fatal("key material was configured but Config does not report it")
	}
	for _, rendered := range []string{
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", *cfg),
	} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("Config rendering leaks the passphrase: %s", rendered)
		}
	}
}

// --- host display scope ---------------------------------------------------

// The default has to be the honest single-tenant one. A box that shows every
// organisation's events by default, on a product that is self-hosted by one
// organiser, is a marketplace page nobody asked for.
func TestLoad_HostScopeDefaultsToOwn(t *testing.T) {
	isolateEnv(t)
	clearKeyEnv(t)

	cfg, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HostScope != HostScopeOwn {
		t.Errorf("HostScope = %q, want %q", cfg.HostScope, HostScopeOwn)
	}
	if cfg.HostOrg != "" || cfg.HostName != "" {
		t.Errorf("HostOrg/HostName = %q/%q, want both empty", cfg.HostOrg, cfg.HostName)
	}
}

func TestLoad_HostScopeSingle(t *testing.T) {
	isolateEnv(t)
	clearKeyEnv(t)
	t.Setenv(envHostScope, "  Single  ") // case and whitespace tolerated
	t.Setenv(envHostOrg, " the-bijou ")
	t.Setenv(envHostName, " The Bijou ")

	cfg, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HostScope != HostScopeSingle {
		t.Errorf("HostScope = %q, want %q", cfg.HostScope, HostScopeSingle)
	}
	if cfg.HostOrg != "the-bijou" {
		t.Errorf("HostOrg = %q, want %q", cfg.HostOrg, "the-bijou")
	}
	if cfg.HostName != "The Bijou" {
		t.Errorf("HostName = %q, want %q", cfg.HostName, "The Bijou")
	}
}

// A typo in a tenancy setting must stop the process, not fall back to the
// widest behaviour.
func TestLoad_HostScopeUnknownIsRefused(t *testing.T) {
	isolateEnv(t)
	clearKeyEnv(t)
	t.Setenv(envHostScope, "singel")

	_, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")})
	if err == nil {
		t.Fatal("Load accepted an unknown host scope; want an error")
	}
	if !strings.Contains(err.Error(), envHostScope) {
		t.Errorf("error does not name %s: %v", envHostScope, err)
	}
}

func TestLoad_HostScopeSingleNeedsAnOrg(t *testing.T) {
	isolateEnv(t)
	clearKeyEnv(t)
	t.Setenv(envHostScope, "single")

	_, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")})
	if err == nil {
		t.Fatal("Load accepted single scope with no organisation named; want an error")
	}
	if !strings.Contains(err.Error(), envHostOrg) {
		t.Errorf("error does not name %s: %v", envHostOrg, err)
	}
}

// CACKLE_HOST_ORG next to a scope that ignores it is an operator who believes
// their box presents as one organisation when it does not.
func TestLoad_HostOrgWithoutSingleScopeIsRefused(t *testing.T) {
	isolateEnv(t)
	clearKeyEnv(t)
	for _, scope := range []string{"", "own", "peers"} {
		t.Setenv(envHostScope, scope)
		t.Setenv(envHostOrg, "the-bijou")
		if _, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")}); err == nil {
			t.Errorf("scope=%q with %s set was accepted; want an error", scope, envHostOrg)
		}
	}
}

// Borrowed listings are displayed under the peers scope and under no other.
//
// This test used to assert IncludesPeerEvents() was false for EVERY scope,
// which was accurate while the scope was a name with nothing behind it. Once
// peer event feeds landed it stopped being accurate and started pinning a
// defect: CACKLE_HOST_SCOPE=peers did nothing at all, and the listing API
// told every client that borrowed listings were not included while the browse
// page was able to display them.
func TestHostScopePeersDisplaysBorrowedListings(t *testing.T) {
	isolateEnv(t)
	clearKeyEnv(t)
	t.Setenv(envHostScope, "peers")

	cfg, err := Load(Flags{DB: filepath.Join(t.TempDir(), "cackle.db")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HostScope != HostScopePeers {
		t.Fatalf("HostScope = %q, want %q", cfg.HostScope, HostScopePeers)
	}
	if !cfg.HostScope.IncludesPeerEvents() {
		t.Error("peers.IncludesPeerEvents() = false; the setting an operator wrote would do nothing")
	}
	// Every other scope, including the default, shows none. Enrolling a
	// publisher and pulling its programme is a decision about what this box
	// reads; putting it on the public front page is a second decision, and
	// this is the switch for it.
	for _, s := range []HostScope{HostScopeOwn, HostScopeSingle, "", "nonsense"} {
		if s.IncludesPeerEvents() {
			t.Errorf("%q.IncludesPeerEvents() = true; only %q displays borrowed listings", s, HostScopePeers)
		}
	}
}

func TestHostScopeValid(t *testing.T) {
	for _, s := range []HostScope{HostScopeOwn, HostScopeSingle, HostScopePeers} {
		if !s.Valid() {
			t.Errorf("%q.Valid() = false, want true", s)
		}
	}
	for _, s := range []HostScope{"", "OWN", "peer", "all", "global"} {
		if s.Valid() {
			t.Errorf("%q.Valid() = true, want false", s)
		}
	}
}
