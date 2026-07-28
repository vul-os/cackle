package tickets

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Conformance vectors: the wire format, pinned as bytes.
//
// docs/TICKET-FORMAT.md specifies the capability token so that an
// implementation in another language can be written from the prose alone.
// docs/ticket-format-vectors.json is the executable half of that promise —
// a frozen corpus of issued tokens and verification outcomes that ANY
// implementation must reproduce exactly. This file is Go's side of it; the
// same corpus is run against the browser verifier by
// web/src/lib/capability.conformance.test.js, which is what stops the two
// implementations that ship in this repo from quietly disagreeing.
//
// These vectors are deliberately NOT regenerable by a `-update` flag. If a
// change here is needed, the wire format changed, and that is a deliberate
// act requiring a CurrentVersion bump — not something a test run should be
// able to rubber-stamp.

const vectorsPath = "../../docs/ticket-format-vectors.json"

type vectorFile struct {
	Format         string   `json:"format"`
	PayloadVersion int      `json:"payload_version"`
	FieldOrder     []string `json:"field_order"`
	OmitWhenZero   []string `json:"omit_when_zero"`
	ErrorCodes     []string `json:"error_codes"`

	Keys map[string]struct {
		SeedHex         string `json:"seed_hex"`
		PublicKeyB64URL string `json:"public_key_b64url"`
		KID             string `json:"kid"`
	} `json:"keys"`

	Issue []struct {
		Name        string          `json:"name"`
		Payload     json.RawMessage `json:"payload"`
		PayloadJSON string          `json:"payload_json"`
		Token       string          `json:"token"`
	} `json:"issue"`

	Verify []struct {
		Name    string `json:"name"`
		Token   string `json:"token"`
		Key     string `json:"key"`
		NowUnix int64  `json:"now_unix"`
		Result  string `json:"result"`
		Error   string `json:"error"`
		TID     string `json:"tid"`
	} `json:"verify"`

	VerifyWithRing []struct {
		Name    string            `json:"name"`
		Token   string            `json:"token"`
		Ring    map[string]string `json:"ring"`
		NowUnix int64             `json:"now_unix"`
		Result  string            `json:"result"`
		Error   string            `json:"error"`
	} `json:"verify_with_ring"`
}

// Minimum vector counts. These are floors, not targets: they exist so that a
// truncated, mis-parsed or accidentally-emptied vectors file fails loudly
// instead of passing by running nothing. Raise them when the corpus grows.
const (
	minIssueVectors  = 3
	minVerifyVectors = 25
	minRingVectors   = 5
)

// errorCodeSentinels maps the language-neutral error code names used in the
// vectors file onto this package's sentinel errors. Every code listed in the
// vectors file's own "error_codes" array must appear here, and the test
// below asserts that.
var errorCodeSentinels = map[string]error{
	"malformed":           ErrMalformed,
	"unsupported_version": ErrUnsupportedVersion,
	"bad_signature":       ErrBadSignature,
	"not_yet_valid":       ErrNotYetValid,
	"expired":             ErrExpired,
	"unknown_kid":         ErrUnknownKID,
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	abs, err := filepath.Abs(vectorsPath)
	if err != nil {
		t.Fatalf("resolve vectors path: %v", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read conformance vectors at %s: %v\n"+
			"This file is the frozen wire-format contract; it must exist.", abs, err)
	}
	var vf vectorFile
	if err := json.Unmarshal(data, &vf); err != nil {
		t.Fatalf("parse conformance vectors: %v", err)
	}
	return vf
}

func decodeB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode base64url %q: %v", s, err)
	}
	return b
}

// TestConformanceVectors_Structure guards the corpus itself: the right
// format, enough vectors to be meaningful, and no error code named in the
// file that this package cannot map to a sentinel.
func TestConformanceVectors_Structure(t *testing.T) {
	vf := loadVectors(t)

	if vf.Format != "cackle-capability-token" {
		t.Fatalf("vectors declare format %q, want %q", vf.Format, "cackle-capability-token")
	}
	if vf.PayloadVersion != CurrentVersion {
		t.Fatalf("vectors declare payload_version %d, but this build's CurrentVersion is %d.\n"+
			"If the format version really changed, the vectors must be regenerated deliberately.",
			vf.PayloadVersion, CurrentVersion)
	}
	if len(vf.Issue) < minIssueVectors {
		t.Fatalf("only %d issue vectors, want at least %d", len(vf.Issue), minIssueVectors)
	}
	if len(vf.Verify) < minVerifyVectors {
		t.Fatalf("only %d verify vectors, want at least %d", len(vf.Verify), minVerifyVectors)
	}
	if len(vf.VerifyWithRing) < minRingVectors {
		t.Fatalf("only %d verify_with_ring vectors, want at least %d", len(vf.VerifyWithRing), minRingVectors)
	}

	// Every documented error code must be mappable, and every one must be
	// exercised by at least one vector — otherwise a rejection path could be
	// deleted from the format and no vector would notice.
	exercised := make(map[string]bool, len(vf.ErrorCodes))
	for _, v := range vf.Verify {
		if v.Error != "" {
			exercised[v.Error] = true
		}
	}
	for _, v := range vf.VerifyWithRing {
		if v.Error != "" {
			exercised[v.Error] = true
		}
	}
	for _, code := range vf.ErrorCodes {
		if _, ok := errorCodeSentinels[code]; !ok {
			t.Errorf("vectors name error code %q, which maps to no sentinel in this package", code)
		}
		if !exercised[code] {
			t.Errorf("error code %q is documented but no vector exercises it", code)
		}
	}

	// The declared field order must match what Payload actually marshals to.
	// This is the thing another implementation reads off the spec, so it must
	// not be able to drift from the struct.
	body, err := json.Marshal(Payload{
		V: CurrentVersion, TID: "t", EID: "e", TT: "tt", KID: "k",
		Sub: "s", Name: "n", IAT: 1, NBF: 2, EXP: 3, Seat: "A1",
	})
	if err != nil {
		t.Fatalf("marshal probe payload: %v", err)
	}
	var order []string
	dec := json.NewDecoder(bytes.NewReader(body))
	if _, err := dec.Token(); err != nil { // opening '{'
		t.Fatalf("probe payload: %v", err)
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			t.Fatalf("probe payload key: %v", err)
		}
		order = append(order, key.(string))
		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			t.Fatalf("probe payload value: %v", err)
		}
	}
	if len(order) != len(vf.FieldOrder) {
		t.Fatalf("Payload marshals %d fields %v, vectors declare %d %v", len(order), order, len(vf.FieldOrder), vf.FieldOrder)
	}
	for i := range order {
		if order[i] != vf.FieldOrder[i] {
			t.Fatalf("field order mismatch at %d: Payload emits %q, vectors declare %q\n(full: %v vs %v)",
				i, order[i], vf.FieldOrder[i], order, vf.FieldOrder)
		}
	}
}

// TestConformanceVectors_Issue proves Issue reproduces every frozen token
// byte for byte from its payload — canonical field order, omitempty
// behaviour, JSON string escaping and base64url encoding all included.
func TestConformanceVectors_Issue(t *testing.T) {
	vf := loadVectors(t)
	issuer := vf.Keys["issuer"]
	seed, err := hex.DecodeString(issuer.SeedHex)
	if err != nil {
		t.Fatalf("decode issuer seed: %v", err)
	}
	priv := ed25519.NewKeyFromSeed(seed)

	// The published public key and kid must be the ones this seed produces,
	// or every downstream vector is verifying against the wrong thing.
	pub := priv.Public().(ed25519.PublicKey)
	if got := base64.RawURLEncoding.EncodeToString(pub); got != issuer.PublicKeyB64URL {
		t.Fatalf("issuer public key mismatch: seed derives %q, vectors publish %q", got, issuer.PublicKeyB64URL)
	}
	if got := KeyID(pub); got != issuer.KID {
		t.Fatalf("issuer kid mismatch: KeyID derives %q, vectors publish %q", got, issuer.KID)
	}

	ran := 0
	for _, vec := range vf.Issue {
		t.Run(vec.Name, func(t *testing.T) {
			var p Payload
			if err := json.Unmarshal(vec.Payload, &p); err != nil {
				t.Fatalf("unmarshal vector payload: %v", err)
			}
			token, err := Issue(p, priv)
			if err != nil {
				t.Fatalf("Issue: %v", err)
			}
			if token != vec.Token {
				t.Fatalf("token mismatch\n got: %s\nwant: %s", token, vec.Token)
			}
			// The payload_json field is what an implementer reads to see the
			// exact bytes signed; it must equal the middle segment verbatim.
			segments := strings.Split(token, ".")
			if len(segments) != 3 {
				t.Fatalf("issued token has %d segments, want 3", len(segments))
			}
			body := decodeB64(t, segments[1])
			if string(body) != vec.PayloadJSON {
				t.Fatalf("payload bytes mismatch\n got: %s\nwant: %s", body, vec.PayloadJSON)
			}
			// And the token must verify against the published public key.
			if _, err := Verify(token, pub, time.Unix(p.IAT, 0)); err != nil {
				t.Fatalf("frozen token does not verify against the published key: %v", err)
			}
		})
		ran++
	}
	if ran != len(vf.Issue) {
		t.Fatalf("ran %d issue vectors, file has %d", ran, len(vf.Issue))
	}
}

// TestConformanceVectors_Verify runs every accept/reject vector through
// Verify and asserts the outcome, including which sentinel error class the
// rejection falls into.
func TestConformanceVectors_Verify(t *testing.T) {
	vf := loadVectors(t)

	keyFor := func(t *testing.T, name string) ed25519.PublicKey {
		t.Helper()
		k, ok := vf.Keys[name]
		if !ok {
			t.Fatalf("vector references unknown key %q", name)
		}
		return ed25519.PublicKey(decodeB64(t, k.PublicKeyB64URL))
	}

	ran := 0
	for _, vec := range vf.Verify {
		t.Run(vec.Name, func(t *testing.T) {
			pub := keyFor(t, vec.Key)
			payload, err := Verify(vec.Token, pub, time.Unix(vec.NowUnix, 0))

			switch vec.Result {
			case "ok":
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				if payload == nil {
					t.Fatal("expected a payload on success, got nil")
				}
				if vec.TID != "" && payload.TID != vec.TID {
					t.Fatalf("tid = %q, want %q", payload.TID, vec.TID)
				}
			case "error":
				if err == nil {
					t.Fatalf("expected error %q, got success with %+v", vec.Error, payload)
				}
				if payload != nil {
					t.Fatalf("expected nil payload alongside an error, got %+v", payload)
				}
				want, ok := errorCodeSentinels[vec.Error]
				if !ok {
					t.Fatalf("vector names unmappable error code %q", vec.Error)
				}
				if !errors.Is(err, want) {
					t.Fatalf("expected error wrapping %v, got %v", want, err)
				}
			default:
				t.Fatalf("vector has unknown result %q (want \"ok\" or \"error\")", vec.Result)
			}
		})
		ran++
	}
	if ran != len(vf.Verify) {
		t.Fatalf("ran %d verify vectors, file has %d", ran, len(vf.Verify))
	}
}

// TestConformanceVectors_VerifyWithRing covers kid dispatch: the ring lookup
// happens before any signature check, and a kid bound to the wrong key still
// fails on the signature rather than being waved through.
func TestConformanceVectors_VerifyWithRing(t *testing.T) {
	vf := loadVectors(t)

	ran := 0
	for _, vec := range vf.VerifyWithRing {
		t.Run(vec.Name, func(t *testing.T) {
			ring := NewKeyRing("")
			for kid, enc := range vec.Ring {
				ring.Add(kid, ed25519.PublicKey(decodeB64(t, enc)))
			}
			payload, err := VerifyWithRing(vec.Token, ring, time.Unix(vec.NowUnix, 0))

			switch vec.Result {
			case "ok":
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				if payload == nil {
					t.Fatal("expected a payload on success, got nil")
				}
			case "error":
				if err == nil {
					t.Fatalf("expected error %q, got success with %+v", vec.Error, payload)
				}
				want, ok := errorCodeSentinels[vec.Error]
				if !ok {
					t.Fatalf("vector names unmappable error code %q", vec.Error)
				}
				if !errors.Is(err, want) {
					t.Fatalf("expected error wrapping %v, got %v", want, err)
				}
			default:
				t.Fatalf("vector has unknown result %q", vec.Result)
			}
		})
		ran++
	}
	if ran != len(vf.VerifyWithRing) {
		t.Fatalf("ran %d ring vectors, file has %d", ran, len(vf.VerifyWithRing))
	}
}
