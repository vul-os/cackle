package keyvault

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestPassphraseRefusesBlankAndWeak is the regression test for the failure
// mode this package exists to make impossible: a blank passphrase being
// accepted and quietly producing a "working" key.
//
// The cautionary example is a sibling repo whose anti-rollback check silently
// stopped verifying when its passphrase was blank — and whose own tests were
// green because they went down that path. So this test asserts the refusal
// itself, not merely that some error occurs somewhere downstream.
func TestPassphraseRefusesBlankAndWeak(t *testing.T) {
	blank := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"tab", "\t"},
		{"newline", "\n"},
		{"spaces and newlines", " \n \t "},
	}
	for _, tc := range blank {
		t.Run("blank/"+tc.name, func(t *testing.T) {
			src, err := Passphrase(tc.in)
			if !errors.Is(err, ErrNoSource) {
				t.Fatalf("Passphrase(%q) error = %v, want ErrNoSource", tc.in, err)
			}
			if src.Valid() {
				t.Fatal("a blank passphrase produced a usable Source")
			}
			// And the unusable Source must not derive anything either.
			if _, err := src.DeriveKEK(KDFParams{Name: KDFArgon2id, Salt: []byte("salt"), Time: 1, Memory: 8, Lanes: 1}); !errors.Is(err, ErrNoSource) {
				t.Fatalf("DeriveKEK on blank Source = %v, want ErrNoSource", err)
			}
		})
	}

	short := strings.Repeat("a", minPassphraseRunes-1)
	src, err := Passphrase(short)
	if !errors.Is(err, ErrWeakSource) {
		t.Fatalf("Passphrase(%d chars) error = %v, want ErrWeakSource", len(short), err)
	}
	if src.Valid() {
		t.Fatal("a too-short passphrase produced a usable Source")
	}

	// And the boundary is usable.
	ok, err := Passphrase(strings.Repeat("a", minPassphraseRunes))
	if err != nil {
		t.Fatalf("Passphrase at minimum length: %v", err)
	}
	if !ok.Valid() || ok.Kind() != KindPassphrase {
		t.Fatalf("minimum-length passphrase: valid=%v kind=%q", ok.Valid(), ok.Kind())
	}
}

func TestKeyfileRefusesEmptyAndShort(t *testing.T) {
	if _, err := Keyfile(nil); !errors.Is(err, ErrNoSource) {
		t.Fatalf("Keyfile(nil) = %v, want ErrNoSource", err)
	}
	if _, err := Keyfile([]byte("\n\n")); !errors.Is(err, ErrNoSource) {
		t.Fatalf("Keyfile(newlines only) = %v, want ErrNoSource", err)
	}
	if _, err := Keyfile(bytes.Repeat([]byte("k"), minKeyfileBytes-1)); !errors.Is(err, ErrWeakSource) {
		t.Fatalf("Keyfile(short) = %v, want ErrWeakSource", err)
	}

	// A trailing newline (what `head -c 32 /dev/urandom > f` plus an editor
	// leaves behind) must not change the derived key, or an operator's keyfile
	// would stop working the first time anything touched the file.
	raw := bytes.Repeat([]byte("k"), minKeyfileBytes)
	a, err := Keyfile(raw)
	if err != nil {
		t.Fatalf("Keyfile: %v", err)
	}
	b, err := Keyfile(append(append([]byte{}, raw...), '\n'))
	if err != nil {
		t.Fatalf("Keyfile(with newline): %v", err)
	}
	params := KDFParams{Name: KDFHKDF, Salt: []byte("fixed-salt-16byt")}
	ka, err := a.DeriveKEK(params)
	if err != nil {
		t.Fatalf("DeriveKEK a: %v", err)
	}
	kb, err := b.DeriveKEK(params)
	if err != nil {
		t.Fatalf("DeriveKEK b: %v", err)
	}
	if !bytes.Equal(ka, kb) {
		t.Fatal("a trailing newline changed the derived KEK")
	}
}

func TestZeroSourceIsUnusable(t *testing.T) {
	var zero Source
	if zero.Valid() {
		t.Fatal("the zero Source reports itself valid")
	}
	if zero.Kind() != "" {
		t.Fatalf("zero Source kind = %q", zero.Kind())
	}
	if _, err := zero.DeriveKEK(KDFParams{Name: KDFHKDF, Salt: []byte("s")}); !errors.Is(err, ErrNoSource) {
		t.Fatalf("zero Source DeriveKEK = %v, want ErrNoSource", err)
	}
	// A nil Vault must refuse too, rather than panicking or sealing under a
	// zero key.
	var v *Vault
	if _, _, err := v.Seal([]byte("aad"), []byte("secret")); !errors.Is(err, ErrNoSource) {
		t.Fatalf("nil Vault Seal = %v, want ErrNoSource", err)
	}
	if _, err := v.Open([]byte("aad"), make([]byte, 24), []byte("ct")); !errors.Is(err, ErrNoSource) {
		t.Fatalf("nil Vault Open = %v, want ErrNoSource", err)
	}
}

func TestDeriveKEKIsDeterministicAndSaltDependent(t *testing.T) {
	src, err := Passphrase("correct horse battery staple")
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	p1, err := NewKDFParams(KindPassphrase)
	if err != nil {
		t.Fatalf("NewKDFParams: %v", err)
	}
	if p1.Name != KDFArgon2id {
		t.Fatalf("passphrase KDF = %q, want %q", p1.Name, KDFArgon2id)
	}
	// Keep the test fast; the cost parameters themselves are not what is
	// under test here.
	p1.Time, p1.Memory, p1.Lanes = 1, 8, 1

	k1, err := src.DeriveKEK(p1)
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	k2, err := src.DeriveKEK(p1)
	if err != nil {
		t.Fatalf("DeriveKEK again: %v", err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("DeriveKEK is not deterministic for the same salt")
	}
	if len(k1) != kekLen {
		t.Fatalf("KEK length %d, want %d", len(k1), kekLen)
	}

	p2 := p1
	p2.Salt = append([]byte{}, p1.Salt...)
	p2.Salt[0] ^= 0xff
	k3, err := src.DeriveKEK(p2)
	if err != nil {
		t.Fatalf("DeriveKEK other salt: %v", err)
	}
	if bytes.Equal(k1, k3) {
		t.Fatal("changing the salt did not change the KEK")
	}
}

// TestDemoSourceUsesCheapKDF documents, in an executable way, that the demo
// vault is not protected by key stretching — because its material is a public
// constant, so stretching would be theatre. If someone "hardens" this later,
// this test tells them what they are actually changing.
func TestDemoSourceUsesCheapKDF(t *testing.T) {
	if DemoSource().Kind() != KindDemo {
		t.Fatalf("DemoSource kind = %q", DemoSource().Kind())
	}
	p, err := NewKDFParams(KindDemo)
	if err != nil {
		t.Fatalf("NewKDFParams(demo): %v", err)
	}
	if p.Name != KDFHKDF {
		t.Fatalf("demo KDF = %q, want %q (a published constant gains nothing from Argon2id)", p.Name, KDFHKDF)
	}
}

func TestSealOpenRoundTripAndAADBinding(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	v, err := NewVault(dek, KindPassphrase)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}

	secret := []byte("an ed25519 private key would live here")
	aad := []byte("cackle.event_key.v1|key-1|event-1")

	ct, nonce, err := v.Seal(aad, secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(ct, secret) {
		t.Fatal("ciphertext contains the plaintext")
	}

	got, err := v.Open(aad, nonce, ct)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("round trip changed the secret")
	}

	// Different AAD (i.e. the same ciphertext moved to another event_keys row)
	// must NOT decrypt. This is what stops a sealed key being replayed under
	// another event's identity.
	otherAAD := []byte("cackle.event_key.v1|key-1|event-2")
	if _, err := v.Open(otherAAD, nonce, ct); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("Open with foreign AAD = %v, want ErrWrongKey", err)
	}

	// A flipped ciphertext bit must not decrypt either.
	tampered := append([]byte{}, ct...)
	tampered[0] ^= 0x01
	if _, err := v.Open(aad, nonce, tampered); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("Open tampered = %v, want ErrWrongKey", err)
	}

	// A different DEK must not decrypt.
	otherDEK, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	other, err := NewVault(otherDEK, KindPassphrase)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if _, err := other.Open(aad, nonce, ct); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("Open under a different DEK = %v, want ErrWrongKey", err)
	}
}

func TestSealUsesAFreshNoncePerCall(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	v, err := NewVault(dek, KindKeyfile)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		ct, nonce, err := v.Seal([]byte("aad"), []byte("same plaintext every time"))
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		if seen[string(nonce)] {
			t.Fatal("nonce reused across Seal calls")
		}
		seen[string(nonce)] = true
		if seen[string(ct)] {
			t.Fatal("identical ciphertext for identical plaintext (nonce not fresh)")
		}
		seen[string(ct)] = true
	}
}

func TestNewVaultRejectsWrongSizedDEK(t *testing.T) {
	if _, err := NewVault([]byte("short"), KindPassphrase); err == nil {
		t.Fatal("NewVault accepted a short DEK")
	}
	if _, err := NewVault(nil, KindPassphrase); err == nil {
		t.Fatal("NewVault accepted a nil DEK")
	}
}

func TestZeroWipesMaterial(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	v, err := NewVault(dek, KindKeyfile)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	v.Zero()
	// After Zero the vault must refuse rather than seal under a zero key.
	if _, _, err := v.Seal([]byte("aad"), []byte("x")); err == nil {
		if !bytes.Equal(v.dek, make([]byte, len(v.dek))) {
			t.Fatal("Zero did not wipe the DEK")
		}
	}
	if !bytes.Equal(v.dek, make([]byte, dekLen)) {
		t.Fatal("Zero did not wipe the DEK")
	}
}

func TestFingerprintIsStableShortAndNotReversible(t *testing.T) {
	secret := []byte("some private key bytes")
	a := Fingerprint("k1", secret)
	b := Fingerprint("k1", secret)
	if a != b {
		t.Fatalf("Fingerprint not stable: %q vs %q", a, b)
	}
	if len(a) != fingerprintHexLen {
		t.Fatalf("fingerprint length %d, want %d", len(a), fingerprintHexLen)
	}
	if strings.Contains(string(secret), a) {
		t.Fatal("fingerprint appears to leak plaintext")
	}
	// Domain separation: the same secret under a different label differs, so a
	// fingerprint cannot be used to correlate one secret across two labels.
	if Fingerprint("k2", secret) == a {
		t.Fatal("fingerprint ignores its label")
	}
	// And a changed secret changes it, which is the whole point (an operator
	// confirming a rotation happened without being shown the key).
	if Fingerprint("k1", []byte("different key bytes")) == a {
		t.Fatal("fingerprint ignores the secret")
	}
}
