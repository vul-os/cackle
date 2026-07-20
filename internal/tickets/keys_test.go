package tickets

import (
	"crypto/ed25519"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateIssuerKey(t *testing.T) {
	k1, err := GenerateIssuerKey("event-1")
	if err != nil {
		t.Fatalf("GenerateIssuerKey: %v", err)
	}
	k2, err := GenerateIssuerKey("event-1")
	if err != nil {
		t.Fatalf("GenerateIssuerKey: %v", err)
	}

	if len(k1.PublicKey) != ed25519.PublicKeySize {
		t.Fatalf("unexpected public key size %d", len(k1.PublicKey))
	}
	if len(k1.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatalf("unexpected private key size %d", len(k1.PrivateKey))
	}
	if k1.KID == k2.KID {
		t.Fatalf("two independently generated keys produced the same kid: %s", k1.KID)
	}
	if k1.EventID != "event-1" {
		t.Fatalf("unexpected event id %s", k1.EventID)
	}
	if k1.RevokedAt != nil {
		t.Fatalf("freshly generated key should not be revoked")
	}
}

func TestKeyID_Deterministic(t *testing.T) {
	k, err := GenerateIssuerKey("e1")
	if err != nil {
		t.Fatalf("GenerateIssuerKey: %v", err)
	}
	id1 := KeyID(k.PublicKey)
	id2 := KeyID(k.PublicKey)
	if id1 != id2 {
		t.Fatalf("KeyID not deterministic: %s != %s", id1, id2)
	}
	if id1[:2] != "k_" {
		t.Fatalf("expected kid prefix k_, got %s", id1)
	}
}

func TestIssuerKey_Revoke(t *testing.T) {
	k, err := GenerateIssuerKey("e1")
	if err != nil {
		t.Fatalf("GenerateIssuerKey: %v", err)
	}
	if k.RevokedAt != nil {
		t.Fatalf("expected nil RevokedAt before Revoke")
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	k.Revoke(now)
	if k.RevokedAt == nil {
		t.Fatalf("expected non-nil RevokedAt after Revoke")
	}
	if !k.RevokedAt.Equal(now) {
		t.Fatalf("RevokedAt = %v, want %v", k.RevokedAt, now)
	}
}

func TestKeyRing_AddLookup(t *testing.T) {
	ring := NewKeyRing("event-1")
	k, _ := GenerateIssuerKey("event-1")
	ring.AddKey(k)

	pub, ok := ring.Lookup(k.KID)
	if !ok {
		t.Fatalf("expected to find kid %s", k.KID)
	}
	if !pub.Equal(k.PublicKey) {
		t.Fatalf("looked-up public key does not match")
	}

	if _, ok := ring.Lookup("nonexistent"); ok {
		t.Fatalf("expected lookup miss for nonexistent kid")
	}
}

func TestKeyRing_ZeroValueLookupDoesNotPanic(t *testing.T) {
	var ring KeyRing // zero value, Keys is nil
	if _, ok := ring.Lookup("anything"); ok {
		t.Fatalf("expected miss on zero-value ring")
	}
}

func TestKeyRing_JSONRoundTrip(t *testing.T) {
	ring := NewKeyRing("event-1")
	k1, _ := GenerateIssuerKey("event-1")
	k2, _ := GenerateIssuerKey("event-1")
	ring.AddKey(k1)
	ring.AddKey(k2)

	data, err := ring.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var decoded KeyRing
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if decoded.EventID != ring.EventID {
		t.Fatalf("event id mismatch: got %s want %s", decoded.EventID, ring.EventID)
	}
	for _, k := range []IssuerKey{k1, k2} {
		pub, ok := decoded.Lookup(k.KID)
		if !ok {
			t.Fatalf("kid %s missing after round trip", k.KID)
		}
		if !pub.Equal(k.PublicKey) {
			t.Fatalf("public key mismatch after round trip for kid %s", k.KID)
		}
	}
}

func TestKeyRing_UnmarshalJSON_RejectsBadKeySize(t *testing.T) {
	bad := []byte(`{"event_id":"e1","keys":{"k_x":"YWJj"}}`) // "abc" decoded, only 3 bytes
	var ring KeyRing
	if err := ring.UnmarshalJSON(bad); err == nil {
		t.Fatalf("expected error for undersized public key")
	}
}

func TestKeyRing_UnmarshalJSON_RejectsBadBase64(t *testing.T) {
	bad := []byte(`{"event_id":"e1","keys":{"k_x":"not-valid-base64!!!"}}`)
	var ring KeyRing
	if err := ring.UnmarshalJSON(bad); err == nil {
		t.Fatalf("expected error for invalid base64 public key")
	}
}

func TestKeyRing_SaveLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "keyring.json")

	ring := NewKeyRing("event-1")
	k, _ := GenerateIssuerKey("event-1")
	ring.AddKey(k)

	if err := ring.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	loaded, err := LoadKeyRingFromFile(path)
	if err != nil {
		t.Fatalf("LoadKeyRingFromFile: %v", err)
	}
	pub, ok := loaded.Lookup(k.KID)
	if !ok || !pub.Equal(k.PublicKey) {
		t.Fatalf("loaded ring does not match saved ring")
	}
}

func TestLoadKeyRingFromFile_MissingFile(t *testing.T) {
	_, err := LoadKeyRingFromFile(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

// TestEndToEnd_GenerateIssueVerifyViaRing exercises the full realistic
// flow: generate an event key, issue a ticket, build the scanner's
// KeyRing (as it would come down in a Bundle), round-trip it through disk,
// and verify offline against the loaded ring.
func TestEndToEnd_GenerateIssueVerifyViaRing(t *testing.T) {
	issuerKey, err := GenerateIssuerKey("event-123")
	if err != nil {
		t.Fatalf("GenerateIssuerKey: %v", err)
	}

	p := samplePayload()
	p.EID = "event-123"
	p.KID = issuerKey.KID
	token, err := Issue(p, issuerKey.PrivateKey)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	ring := NewKeyRing("event-123")
	ring.AddKey(issuerKey)

	dir := t.TempDir()
	path := filepath.Join(dir, "ring.json")
	if err := ring.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	scannerRing, err := LoadKeyRingFromFile(path)
	if err != nil {
		t.Fatalf("LoadKeyRingFromFile: %v", err)
	}

	got, err := VerifyWithRing(token, scannerRing, time.Unix(p.IAT, 0))
	if err != nil {
		t.Fatalf("VerifyWithRing: %v", err)
	}
	if got.TID != p.TID {
		t.Fatalf("unexpected tid %s", got.TID)
	}

	// A tampered token must still be rejected after the round trip.
	tamperedToken := token[:len(token)-1] + "x"
	if tamperedToken == token {
		t.Skip("could not construct a distinct tampered token")
	}
	if _, err := VerifyWithRing(tamperedToken, scannerRing, time.Unix(p.IAT, 0)); err == nil {
		t.Fatalf("expected tampered token to be rejected")
	} else if !errors.Is(err, ErrBadSignature) && !errors.Is(err, ErrMalformed) {
		t.Fatalf("expected ErrBadSignature or ErrMalformed, got %v", err)
	}
}
