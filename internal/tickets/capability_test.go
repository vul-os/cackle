package tickets

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func mustKeyPair(t testing.TB) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

// signRawBody signs arbitrary bytes directly (bypassing Payload/JSON
// entirely) and wraps them as a capability token. Test-only: it lets us
// construct a token whose signature is valid over bytes that are NOT
// well-formed Payload JSON, isolating "malformed JSON" rejection from
// "bad signature" rejection.
func signRawBody(priv ed25519.PrivateKey, body []byte) string {
	sig := ed25519.Sign(priv, body)
	return "cackle." + base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// issueRaw signs payload exactly as given, WITHOUT forcing V to
// CurrentVersion the way the public Issue() does. Test-only: it exists so
// we can construct a validly-signed-but-wrong-version token to prove
// Verify rejects it on version grounds specifically, rather than Issue's
// override masking the case.
func issueRaw(p Payload, priv ed25519.PrivateKey) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, body)
	return "cackle." + base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func samplePayload() Payload {
	return Payload{
		TID:  "01J8ZK8T0M8N0P0Q0R0S0T0U0V",
		EID:  "01J8ZK8T0M8N0P0Q0R0S0T0U0E",
		TT:   "01J8ZK8T0M8N0P0Q0R0S0T0U0T",
		KID:  "k_test",
		Sub:  "01J8ZK8T0M8N0P0Q0R0S0T0U0S",
		Name: "Ada Lovelace",
		IAT:  1750000000,
		NBF:  1750000000,
		EXP:  1750086400,
		Seat: "A14",
	}
}

func TestIssueVerify_RoundTrip(t *testing.T) {
	pub, priv := mustKeyPair(t)
	p := samplePayload()

	token, err := Issue(p, priv)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if !strings.HasPrefix(token, "cackle.") {
		t.Fatalf("token missing cackle. prefix: %q", token)
	}
	if parts := strings.Split(token, "."); len(parts) != 3 {
		t.Fatalf("expected 3 segments, got %d: %q", len(parts), token)
	}

	now := time.Unix(1750000001, 0)
	got, err := Verify(token, pub, now)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if got.TID != p.TID || got.EID != p.EID || got.TT != p.TT || got.KID != p.KID ||
		got.Sub != p.Sub || got.Name != p.Name || got.IAT != p.IAT || got.NBF != p.NBF ||
		got.EXP != p.EXP || got.Seat != p.Seat {
		t.Fatalf("round-tripped payload mismatch: got %+v, want %+v", got, p)
	}
	if got.V != CurrentVersion {
		t.Fatalf("got version %d, want %d", got.V, CurrentVersion)
	}
}

func TestIssue_ForcesCurrentVersion(t *testing.T) {
	_, priv := mustKeyPair(t)
	p := samplePayload()
	p.V = 99 // caller-set garbage must be overwritten

	token, err := Issue(p, priv)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	parts := strings.Split(token, ".")
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var decoded Payload
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.V != CurrentVersion {
		t.Fatalf("Issue did not force version: got %d want %d", decoded.V, CurrentVersion)
	}
}

// TestVerify_Rejections is the heart of the security test suite: every one
// of the rejection paths called out in the build spec, table-driven.
func TestVerify_Rejections(t *testing.T) {
	pub, priv := mustKeyPair(t)
	otherPub, _ := mustKeyPair(t)

	basePayload := samplePayload()
	validToken, err := Issue(basePayload, priv)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	validNow := time.Unix(1750000001, 0)

	// tamperPayload mutates the payload but keeps the ORIGINAL signature —
	// it must always trigger ErrBadSignature, since the signed bytes no
	// longer match what the signature covers. Used for tamper-detection
	// tests.
	tamperPayload := func(mutate func(p *Payload)) string {
		parts := strings.Split(validToken, ".")
		body, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var p Payload
		_ = json.Unmarshal(body, &p)
		mutate(&p)
		newBody, _ := json.Marshal(p)
		return "cackle." + base64.RawURLEncoding.EncodeToString(newBody) + "." + parts[2]
	}

	// mutateAndResign mutates the payload and re-signs it with the real
	// event private key, so the signature check passes and whatever field
	// was changed (exp/nbf/version/etc.) is the thing under test in
	// isolation.
	mutateAndResign := func(mutate func(p *Payload)) string {
		p := samplePayload()
		p.V = CurrentVersion
		mutate(&p)
		tok, err := issueRaw(p, priv)
		if err != nil {
			t.Fatalf("mutateAndResign: %v", err)
		}
		return tok
	}

	tests := []struct {
		name    string
		token   string
		pub     ed25519.PublicKey
		now     time.Time
		wantErr error // checked with errors.Is; nil means "just want non-nil error"
	}{
		{
			name:  "valid token passes",
			token: validToken,
			pub:   pub,
			now:   validNow,
		},
		{
			name: "tampered payload (seat changed) fails signature",
			token: tamperPayload(func(p *Payload) {
				p.Seat = "Z99"
			}),
			pub:     pub,
			now:     validNow,
			wantErr: ErrBadSignature,
		},
		{
			name: "tampered payload (sub swapped) fails signature",
			token: tamperPayload(func(p *Payload) {
				p.Sub = "attacker-user-id"
			}),
			pub:     pub,
			now:     validNow,
			wantErr: ErrBadSignature,
		},
		{
			name:    "wrong key rejects a validly-formed, validly-signed-by-someone-else token",
			token:   validToken,
			pub:     otherPub,
			now:     validNow,
			wantErr: ErrBadSignature,
		},
		{
			name: "expired: now == exp",
			token: mutateAndResign(func(p *Payload) {
				p.EXP = validNow.Unix()
			}),
			pub:     pub,
			now:     validNow,
			wantErr: ErrExpired,
		},
		{
			name: "not yet valid: now < nbf",
			token: mutateAndResign(func(p *Payload) {
				p.NBF = validNow.Unix() + 1
			}),
			pub:     pub,
			now:     validNow,
			wantErr: ErrNotYetValid,
		},
		{
			name:    "truncated token (missing signature segment)",
			token:   strings.Join(strings.Split(validToken, ".")[:2], "."),
			pub:     pub,
			now:     validNow,
			wantErr: ErrMalformed,
		},
		{
			name:    "truncated token (chopped mid-base64)",
			token:   validToken[:len(validToken)-10],
			pub:     pub,
			now:     validNow,
			wantErr: nil, // may be ErrMalformed or ErrBadSignature depending on where it breaks
		},
		{
			name:    "empty string",
			token:   "",
			pub:     pub,
			now:     validNow,
			wantErr: ErrMalformed,
		},
		{
			name:    "wrong prefix",
			token:   "notcackle." + strings.SplitN(validToken, ".", 2)[1],
			pub:     pub,
			now:     validNow,
			wantErr: ErrMalformed,
		},
		{
			name:    "too many segments",
			token:   validToken + ".extra",
			pub:     pub,
			now:     validNow,
			wantErr: ErrMalformed,
		},
		{
			name:    "bad base64 in payload segment",
			token:   "cackle.not-valid-base64!!!." + strings.Split(validToken, ".")[2],
			pub:     pub,
			now:     validNow,
			wantErr: ErrMalformed,
		},
		{
			name:    "bad base64 in signature segment",
			token:   strings.Split(validToken, ".")[0] + "." + strings.Split(validToken, ".")[1] + ".not-valid-base64!!!",
			pub:     pub,
			now:     validNow,
			wantErr: ErrMalformed,
		},
		{
			name: "wrong version",
			token: mutateAndResign(func(p *Payload) {
				p.V = 2
			}),
			pub:     pub,
			now:     validNow,
			wantErr: ErrUnsupportedVersion,
		},
		{
			// Signature check runs before JSON parsing (by design — never
			// run a parser over bytes that aren't authenticated yet), so
			// to isolate "malformed JSON" from "bad signature" we sign
			// this arbitrary non-object body directly with the real
			// private key.
			name:    "malformed json payload (not an object, correctly signed)",
			token:   signRawBody(priv, []byte("[1,2,3]")),
			pub:     pub,
			now:     validNow,
			wantErr: ErrMalformed,
		},
		{
			// An empty payload segment decodes as valid (zero-length)
			// base64, so this is rejected on signature grounds (the
			// original signature was computed over the real payload
			// bytes, not zero bytes) rather than at the base64/JSON
			// parsing stage. Still must never panic or succeed.
			name:    "empty payload segment",
			token:   "cackle.." + strings.Split(validToken, ".")[2],
			pub:     pub,
			now:     validNow,
			wantErr: ErrBadSignature,
		},
		{
			name:    "invalid pubkey size",
			token:   validToken,
			pub:     pub[:16],
			now:     validNow,
			wantErr: ErrMalformed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Verify(tc.token, tc.pub, tc.now)
			if tc.name == "valid token passes" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if got == nil {
					t.Fatalf("expected non-nil payload")
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got success with payload %+v", got)
			}
			if got != nil {
				t.Fatalf("expected nil payload on error, got %+v", got)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error wrapping %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestVerify_ExpiredExactBoundary(t *testing.T) {
	pub, priv := mustKeyPair(t)
	p := samplePayload()
	p.NBF = 1000
	p.EXP = 2000
	token, err := Issue(p, priv)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	cases := []struct {
		name    string
		now     int64
		wantErr error
	}{
		{"one before nbf: not yet valid", 999, ErrNotYetValid},
		{"exactly nbf: valid", 1000, nil},
		{"between nbf and exp: valid", 1500, nil},
		{"exactly exp: expired (exp is exclusive upper bound)", 2000, ErrExpired},
		{"one after exp: expired", 2001, ErrExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Verify(token, pub, time.Unix(c.now, 0))
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("expected valid, got error: %v", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("expected %v, got %v", c.wantErr, err)
			}
		})
	}
}

func TestVerify_NoExpiryNoNBF_MeansUnbounded(t *testing.T) {
	pub, priv := mustKeyPair(t)
	p := samplePayload()
	p.NBF = 0
	p.EXP = 0
	token, err := Issue(p, priv)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	// Any time at all should verify, since 0 means "no bound".
	for _, ts := range []int64{0, 1, 1750000000, 9999999999} {
		if _, err := Verify(token, pub, time.Unix(ts, 0)); err != nil {
			t.Fatalf("time %d: expected valid (unbounded), got %v", ts, err)
		}
	}
}

func TestVerify_TrailingGarbageAfterJSON(t *testing.T) {
	pub, priv := mustKeyPair(t)
	p := samplePayload()
	p.V = CurrentVersion
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Append trailing bytes after the valid JSON object, then sign THIS
	// exact (corrupted) body directly so the signature check passes and
	// we isolate the decoder's "reject trailing data" behaviour
	// specifically (dec.More() in Verify).
	corrupted := append(append([]byte{}, body...), []byte(`{"x":1}`)...)
	token := signRawBody(priv, corrupted)

	_, err = Verify(token, pub, time.Unix(p.IAT, 0))
	if err == nil {
		t.Fatalf("expected error for trailing garbage after JSON payload")
	}
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestVerify_UnknownFieldRejected(t *testing.T) {
	// A payload containing a field Payload doesn't know about must be
	// rejected outright (DisallowUnknownFields), rather than silently
	// ignored -- this keeps the format strict and prevents smuggling data
	// that later code might trust without it ever being validated here.
	pub, priv := mustKeyPair(t)
	type payloadWithExtra struct {
		Payload
		Extra string `json:"extra"`
	}
	p := payloadWithExtra{Payload: samplePayload(), Extra: "smuggled"}
	p.Payload.V = CurrentVersion
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sig := ed25519.Sign(priv, body)
	token := "cackle." + base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig)

	if _, err := Verify(token, pub, time.Unix(p.Payload.IAT, 0)); err == nil {
		t.Fatalf("expected rejection of payload with unknown field")
	} else if !errors.Is(err, ErrMalformed) {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestVerifyWithRing(t *testing.T) {
	pub, priv := mustKeyPair(t)
	p := samplePayload()
	p.KID = "k_event1"
	token, err := Issue(p, priv)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	now := time.Unix(p.IAT, 0)

	t.Run("known kid verifies", func(t *testing.T) {
		ring := NewKeyRing("eid")
		ring.Add("k_event1", pub)
		got, err := VerifyWithRing(token, ring, now)
		if err != nil {
			t.Fatalf("VerifyWithRing: %v", err)
		}
		if got.KID != "k_event1" {
			t.Fatalf("unexpected kid: %s", got.KID)
		}
	})

	t.Run("unknown kid rejected before signature check", func(t *testing.T) {
		ring := NewKeyRing("eid")
		ring.Add("k_someone_else", pub)
		_, err := VerifyWithRing(token, ring, now)
		if !errors.Is(err, ErrUnknownKID) {
			t.Fatalf("expected ErrUnknownKID, got %v", err)
		}
	})

	t.Run("empty ring rejected", func(t *testing.T) {
		ring := NewKeyRing("eid")
		_, err := VerifyWithRing(token, ring, now)
		if !errors.Is(err, ErrUnknownKID) {
			t.Fatalf("expected ErrUnknownKID, got %v", err)
		}
	})

	t.Run("malformed token surfaces ErrMalformed not a panic", func(t *testing.T) {
		ring := NewKeyRing("eid")
		_, err := VerifyWithRing("garbage", ring, now)
		if !errors.Is(err, ErrMalformed) {
			t.Fatalf("expected ErrMalformed, got %v", err)
		}
	})
}

func TestIssue_RejectsBadKeySize(t *testing.T) {
	_, err := Issue(samplePayload(), ed25519.PrivateKey([]byte("too-short")))
	if err == nil {
		t.Fatalf("expected error for bad private key size")
	}
}

// FuzzVerify feeds arbitrary bytes into Verify (with a real key so a
// nonzero fraction of inputs at least get past prefix parsing) and asserts
// only that it never panics and never returns a payload alongside an
// error — the offline scan path calls this on untrusted QR-scanned input
// all day, so panics here are a denial-of-service in production.
func FuzzVerify(f *testing.F) {
	pub, priv := mustKeyPair(f)
	valid, err := Issue(samplePayload(), priv)
	if err != nil {
		f.Fatalf("Issue: %v", err)
	}

	seeds := []string{
		valid,
		"",
		"cackle..",
		"cackle.a.b",
		"cackle." + strings.Repeat("A", 1000) + "." + strings.Repeat("B", 1000),
		"cackle.\x00\x01\x02.\x03\x04\x05",
		valid[:len(valid)/2],
		valid + valid,
		strings.ReplaceAll(valid, "cackle", "CACKLE"),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, token string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Verify panicked on input %q: %v", token, r)
			}
		}()
		payload, err := Verify(token, pub, time.Unix(1750000001, 0))
		if err == nil && payload == nil {
			t.Fatalf("Verify returned nil error and nil payload for input %q", token)
		}
		if err != nil && payload != nil {
			t.Fatalf("Verify returned both a non-nil payload and an error for input %q", token)
		}
	})
}
