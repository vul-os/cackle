package main

import (
	"strings"
	"testing"

	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/store"
	"github.com/vul-os/cackle/internal/store/keyvault"
)

// TestChooseKeySourceRefusesWithNoMaterial is the boot-level fail-closed test:
// with nothing configured there is no Source to be had, and the error must be
// something an operator can act on without going looking for the plaintext mode
// that used to exist.
func TestChooseKeySourceRefusesWithNoMaterial(t *testing.T) {
	cases := []struct {
		name   string
		status store.KeyVaultStatus
		// must appear in the message
		wants []string
	}{
		{
			name:   "fresh database",
			status: store.KeyVaultStatus{},
			wants: []string{
				"refusing to start",
				"CACKLE_KEY_PASSPHRASE",
				"CACKLE_KEY_FILE",
				"There is no plaintext mode",
			},
		},
		{
			name:   "already-encrypted database",
			status: store.KeyVaultStatus{Initialised: true, SourceKind: keyvault.KindPassphrase},
			wants: []string{
				"refusing to start",
				"encrypted at rest",
				"CACKLE_KEY_PASSPHRASE",
			},
		},
		{
			name:   "database still holding plaintext keys",
			status: store.KeyVaultStatus{LegacyPlaintextKeys: 3},
			wants: []string{
				"refusing to start",
				"3 event signing key(s)",
				"PLAINTEXT",
				"will not half-migrate",
				"No key has been touched",
				"back up",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			src, origin, err := chooseKeySource(cfg, tc.status)
			if err == nil {
				t.Fatalf("no key material configured but chooseKeySource returned a source (kind %q, origin %q)", src.Kind(), origin)
			}
			if src.Valid() {
				t.Fatal("a usable Source came back alongside the error")
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message does not contain %q:\n%s", want, err)
				}
			}
		})
	}
}

// TestChooseKeySourcePrefersOperatorMaterialOverDemo: --demo must never
// override a real passphrase, so demoing against a real database cannot
// silently re-seal it with the public demo key.
func TestChooseKeySourcePrefersOperatorMaterialOverDemo(t *testing.T) {
	real, err := keyvault.Passphrase("an-operator-passphrase")
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	cfg := &config.Config{Demo: true, KeySource: real, KeySourceOrigin: "CACKLE_KEY_PASSPHRASE"}

	src, origin, err := chooseKeySource(cfg, store.KeyVaultStatus{})
	if err != nil {
		t.Fatalf("chooseKeySource: %v", err)
	}
	if src.Kind() != keyvault.KindPassphrase {
		t.Fatalf("kind = %q, want %q — --demo must not shadow real key material", src.Kind(), keyvault.KindPassphrase)
	}
	if origin != "CACKLE_KEY_PASSPHRASE" {
		t.Fatalf("origin = %q", origin)
	}
}

// TestChooseKeySourceDemoOnlyWithDemoFlag pins the one no-material path that
// exists, and that it is reachable only by an explicit --demo.
func TestChooseKeySourceDemoOnlyWithDemoFlag(t *testing.T) {
	src, origin, err := chooseKeySource(&config.Config{Demo: true}, store.KeyVaultStatus{})
	if err != nil {
		t.Fatalf("chooseKeySource(--demo): %v", err)
	}
	if src.Kind() != keyvault.KindDemo {
		t.Fatalf("kind = %q, want %q", src.Kind(), keyvault.KindDemo)
	}
	if !strings.Contains(origin, "demo") {
		t.Fatalf("origin = %q, want it to name demo mode so the boot log is unambiguous", origin)
	}

	if _, _, err := chooseKeySource(&config.Config{Demo: false}, store.KeyVaultStatus{}); err == nil {
		t.Fatal("the demo key was handed out without --demo")
	}
}

// TestNoKeyMaterialMessageNeverSuggestsAWayAround guards the wording itself.
// The message is the last thing between an operator and a workaround, so it
// must not hint that one exists.
func TestNoKeyMaterialMessageNeverSuggestsAWayAround(t *testing.T) {
	for _, status := range []store.KeyVaultStatus{
		{},
		{Initialised: true, SourceKind: keyvault.KindKeyfile},
		{LegacyPlaintextKeys: 1},
	} {
		msg := noKeyMaterialMessage(status)
		for _, forbidden := range []string{
			"--insecure",
			"disable",
			"skip",
			"CACKLE_ALLOW_PLAINTEXT",
		} {
			if strings.Contains(strings.ToLower(msg), strings.ToLower(forbidden)) {
				t.Errorf("refusal message mentions %q, which reads as an escape hatch:\n%s", forbidden, msg)
			}
		}
		// It must always say what to do instead.
		if !strings.Contains(msg, "CACKLE_KEY_PASSPHRASE") {
			t.Errorf("refusal message does not tell the operator what to set:\n%s", msg)
		}
	}
}
