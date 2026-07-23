package config

import (
	"os"
	"path/filepath"
	"testing"
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
	t.Setenv(envAddr, ":7777")                                    // env
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
