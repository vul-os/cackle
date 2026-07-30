package main

// The wiring test for third-party sign-in.
//
// internal/auth already proves FromEnvGoogle's three outcomes. What is
// asserted here is the thing only cmd/cackle decides: that a half-configured
// provider stops the process, rather than being quietly swallowed into a box
// that boots fine and shows no button.
//
// It reaches run() with a deliberately impossible listen address so the
// process never actually serves. What matters is WHICH error comes back —
// the config refusal must arrive before anything that would open a socket.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vul-os/cackle/internal/auth"
)

// runHeadless calls run() with a throwaway database and no server, and
// returns whatever error it stopped on.
func runHeadless(t *testing.T) error {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CACKLE_KEY_PASSPHRASE", "test-only-passphrase-for-wiring-test")
	// Port 0 on a loopback address the OS will refuse to bind is not
	// portable; instead the error we care about is raised long before the
	// listener, so a real address is fine — run() returns on the first
	// failure and we only ever assert on failures.
	return run([]string{
		"-db", filepath.Join(dir, "cackle.db"),
		"-addr", "256.256.256.256:1",
		"-media-dir", filepath.Join(dir, "media"),
	}, os.Stdout, os.Stderr)
}

func TestRun_RefusesToStartWithHalfConfiguredGoogleSignIn(t *testing.T) {
	for _, tc := range []struct {
		name, id, secret string
	}{
		{"client id without a secret", "1234-abc.apps.googleusercontent.com", ""},
		{"secret without a client id", "", "GOCSPX-not-real"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv(auth.EnvGoogleClientID)
			os.Unsetenv(auth.EnvGoogleClientSecret)
			if tc.id != "" {
				t.Setenv(auth.EnvGoogleClientID, tc.id)
			}
			if tc.secret != "" {
				t.Setenv(auth.EnvGoogleClientSecret, tc.secret)
			}

			err := runHeadless(t)
			if err == nil {
				t.Fatal("a half-configured provider started anyway — an operator would never find out the button is missing")
			}
			if !strings.Contains(err.Error(), "google sign-in") {
				t.Fatalf("the refusal does not name what was misconfigured: %v", err)
			}
			// It must name the variable that is MISSING, so the fix is
			// obvious without reading the source.
			missing := auth.EnvGoogleClientSecret
			if tc.id == "" {
				missing = auth.EnvGoogleClientID
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("the refusal does not name the missing variable %s: %v", missing, err)
			}
			// And it must never echo the credential that WAS set.
			for _, secret := range []string{tc.secret} {
				if secret != "" && strings.Contains(err.Error(), secret) {
					t.Fatalf("the refusal leaked the client secret: %v", err)
				}
			}
		})
	}
}
