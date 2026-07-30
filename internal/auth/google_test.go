package auth

// Tests for the Google OAuth provider.
//
// NOTHING HERE CONTACTS GOOGLE. Every exchange runs against a local
// httptest server standing in for the token endpoint, reached through the
// unexported withGoogleEndpoints seam. That is a deliberate limit, not an
// oversight: what these tests prove is this code's protocol handling, not
// Google's behaviour. See the "Verification status" note in google.go.
//
// The ID tokens built here carry a garbage signature on purpose. google.go
// does not verify the RS256 signature and says so at length, with the OIDC
// Core §3.1.3.7 reasoning; a test that fed it a correctly signed token would
// be asserting something the code does not claim.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testClientID     = "1234567890-abcdefg.apps.googleusercontent.com"
	testClientSecret = "GOCSPX-test-secret-never-real"
	testRedirectURI  = "https://tickets.venue.example/api/auth/oauth/google/callback"
)

var testNow = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

// idToken builds a compact JWS whose payload is claims. The signature
// segment is fixed nonsense — see the file comment.
func idToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal jwt segment: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	header := enc(map[string]any{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	payload := enc(claims)
	sig := base64.RawURLEncoding.EncodeToString([]byte("this-signature-is-not-checked-see-google.go"))
	return header + "." + payload + "." + sig
}

// goodClaims is a well-formed Google ID token payload for testNow.
func goodClaims() map[string]any {
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            testClientID,
		"sub":            "108743019283746501928",
		"email":          "organiser@venue.example",
		"email_verified": true,
		"name":           "Venue Organiser",
		"nonce":          "the-nonce-we-asked-for",
		"iat":            testNow.Add(-30 * time.Second).Unix(),
		"exp":            testNow.Add(30 * time.Minute).Unix(),
	}
}

// fakeGoogle stands in for the token endpoint. It records the form it was
// posted so tests can assert on what was actually sent.
type fakeGoogle struct {
	srv        *httptest.Server
	lastForm   url.Values
	status     int
	body       string
	requestLog int
}

func newFakeGoogle(t *testing.T, tokenBody string) *fakeGoogle {
	t.Helper()
	f := &fakeGoogle{status: http.StatusOK, body: tokenBody}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requestLog++
		if err := r.ParseForm(); err != nil {
			t.Errorf("fake google: parse form: %v", err)
		}
		f.lastForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// provider builds a GoogleProvider pointed at the fake, with a frozen clock.
func (f *fakeGoogle) provider(t *testing.T) *GoogleProvider {
	t.Helper()
	p, err := NewGoogleProvider(testClientID, testClientSecret,
		withGoogleEndpoints("https://accounts.google.example/authorize", f.srv.URL),
		withGoogleClock(func() time.Time { return testNow }),
	)
	if err != nil {
		t.Fatalf("NewGoogleProvider: %v", err)
	}
	return p
}

func tokenResponse(t *testing.T, claims map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{
		"access_token": "ya29.a0-fake-access-token",
		"id_token":     idToken(t, claims),
		"token_type":   "Bearer",
	})
	if err != nil {
		t.Fatalf("marshal token response: %v", err)
	}
	return string(b)
}

// exchangeWith runs a full exchange against a fake serving an ID token built
// from claims, and returns whatever the provider decided.
func exchangeWith(t *testing.T, claims map[string]any, wantNonce string) (OAuthUserInfo, error) {
	t.Helper()
	f := newFakeGoogle(t, tokenResponse(t, claims))
	return f.provider(t).ExchangeWithNonce(context.Background(), "auth-code-abc", testRedirectURI, wantNonce)
}

// ── the happy path ──────────────────────────────────────────────────────────

func TestGoogleExchange_ResolvesVerifiedIdentity(t *testing.T) {
	info, err := exchangeWith(t, goodClaims(), "the-nonce-we-asked-for")
	if err != nil {
		t.Fatalf("ExchangeWithNonce: %v", err)
	}
	if info.Provider != ProviderNameGoogle {
		t.Errorf("Provider = %q, want %q", info.Provider, ProviderNameGoogle)
	}
	if info.Subject != "108743019283746501928" {
		t.Errorf("Subject = %q", info.Subject)
	}
	if info.Email != "organiser@venue.example" {
		t.Errorf("Email = %q", info.Email)
	}
	if info.Name != "Venue Organiser" {
		t.Errorf("Name = %q", info.Name)
	}
	if !info.EmailVerified {
		t.Error("EmailVerified = false on a token that says email_verified: true")
	}
}

// Google historically emits email_verified as the STRING "true" on some
// surfaces. Both shapes must count as verified; nothing else may.
func TestGoogleExchange_EmailVerifiedAcceptsBoolAndStringOnly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{"bool true", true, true},
		{"string true", "true", true},
		{"bool false", false, false},
		{"string false", "false", false},
		{"absent", nil, false},
		{"number one", float64(1), false},
		{"string yes", "yes", false},
		{"string True", "True", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := goodClaims()
			if tc.value == nil {
				delete(claims, "email_verified")
			} else {
				claims["email_verified"] = tc.value
			}
			_, err := exchangeWith(t, claims, "the-nonce-we-asked-for")
			if tc.want && err != nil {
				t.Fatalf("expected verified, got %v", err)
			}
			if !tc.want && !errors.Is(err, ErrOAuthEmailUnverified) {
				t.Fatalf("expected ErrOAuthEmailUnverified, got %v", err)
			}
		})
	}
}

// ── GUARD: the nonce ────────────────────────────────────────────────────────
//
// MUTATION: delete the subtle.ConstantTimeCompare nonce check in
// identityFromIDToken and all four subtests below must fail.

func TestGoogleExchange_RefusesNonceThatIsNotTheOneWeAskedFor(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tokenNonce any // nil = claim absent
		wantNonce  string
	}{
		{"replayed token from another sign-in", "some-other-nonce", "the-nonce-we-asked-for"},
		{"token carries no nonce at all", nil, "the-nonce-we-asked-for"},
		{"token carries a nonce we never sent", "injected-nonce", ""},
		{"empty string is not a wildcard", "", "the-nonce-we-asked-for"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := goodClaims()
			if tc.tokenNonce == nil {
				delete(claims, "nonce")
			} else {
				claims["nonce"] = tc.tokenNonce
			}
			_, err := exchangeWith(t, claims, tc.wantNonce)
			if !errors.Is(err, ErrOAuthTokenInvalid) {
				t.Fatalf("expected ErrOAuthTokenInvalid for a bad nonce, got %v", err)
			}
		})
	}
}

// The nonce-free pair (the base OAuthProvider interface) must still be
// consistent: AuthURL sends no nonce, so Exchange insists the token has none.
func TestGoogleExchange_NonceFreePairIsSelfConsistent(t *testing.T) {
	claims := goodClaims()
	delete(claims, "nonce")
	f := newFakeGoogle(t, tokenResponse(t, claims))
	if _, err := f.provider(t).Exchange(context.Background(), "code", testRedirectURI); err != nil {
		t.Fatalf("nonce-free Exchange on a nonce-free token: %v", err)
	}

	withNonce := newFakeGoogle(t, tokenResponse(t, goodClaims()))
	if _, err := withNonce.provider(t).Exchange(context.Background(), "code", testRedirectURI); !errors.Is(err, ErrOAuthTokenInvalid) {
		t.Fatalf("nonce-free Exchange accepted a token carrying a nonce: %v", err)
	}
}

// ── GUARD: the audience ─────────────────────────────────────────────────────
//
// MUTATION: make audienceMatches return true and both subtests must fail.
// This is the check that stops a token minted for a DIFFERENT application —
// one the attacker controls — from signing someone into this box.

func TestGoogleExchange_RefusesForeignAudience(t *testing.T) {
	for _, tc := range []struct {
		name string
		aud  any
	}{
		{"another application's client id", "999-attacker.apps.googleusercontent.com"},
		{"array without us in it", []any{"999-attacker.apps.googleusercontent.com", "other"}},
		{"absent", nil},
		{"prefix of ours", testClientID[:10]},
		{"ours with a suffix", testClientID + ".evil"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := goodClaims()
			if tc.aud == nil {
				delete(claims, "aud")
			} else {
				claims["aud"] = tc.aud
			}
			_, err := exchangeWith(t, claims, "the-nonce-we-asked-for")
			if !errors.Is(err, ErrOAuthTokenInvalid) {
				t.Fatalf("expected ErrOAuthTokenInvalid for aud=%v, got %v", tc.aud, err)
			}
		})
	}
}

func TestGoogleExchange_AcceptsAudienceArrayContainingUs(t *testing.T) {
	claims := goodClaims()
	claims["aud"] = []any{"someone-else", testClientID}
	if _, err := exchangeWith(t, claims, "the-nonce-we-asked-for"); err != nil {
		t.Fatalf("aud array containing our client id was refused: %v", err)
	}
}

// ── GUARD: the issuer ───────────────────────────────────────────────────────

func TestGoogleExchange_RefusesUnexpectedIssuer(t *testing.T) {
	for _, iss := range []string{"https://accounts.google.com.evil.example", "", "https://login.microsoftonline.com"} {
		claims := goodClaims()
		claims["iss"] = iss
		if _, err := exchangeWith(t, claims, "the-nonce-we-asked-for"); !errors.Is(err, ErrOAuthTokenInvalid) {
			t.Fatalf("issuer %q was accepted (err=%v)", iss, err)
		}
	}
	// Both issuer strings Google documents are accepted.
	for _, iss := range googleIssuers {
		claims := goodClaims()
		claims["iss"] = iss
		if _, err := exchangeWith(t, claims, "the-nonce-we-asked-for"); err != nil {
			t.Fatalf("documented issuer %q was refused: %v", iss, err)
		}
	}
}

// ── GUARD: time ─────────────────────────────────────────────────────────────

func TestGoogleExchange_RefusesExpiredOrFutureToken(t *testing.T) {
	expired := goodClaims()
	expired["exp"] = testNow.Add(-time.Second).Unix()
	if _, err := exchangeWith(t, expired, "the-nonce-we-asked-for"); !errors.Is(err, ErrOAuthTokenInvalid) {
		t.Fatalf("expired token accepted: %v", err)
	}

	noExp := goodClaims()
	delete(noExp, "exp")
	if _, err := exchangeWith(t, noExp, "the-nonce-we-asked-for"); !errors.Is(err, ErrOAuthTokenInvalid) {
		t.Fatalf("token with no exp accepted: %v", err)
	}

	future := goodClaims()
	future["iat"] = testNow.Add(googleClockSkew + time.Minute).Unix()
	if _, err := exchangeWith(t, future, "the-nonce-we-asked-for"); !errors.Is(err, ErrOAuthTokenInvalid) {
		t.Fatalf("token issued beyond the skew allowance accepted: %v", err)
	}

	withinSkew := goodClaims()
	withinSkew["iat"] = testNow.Add(googleClockSkew / 2).Unix()
	if _, err := exchangeWith(t, withinSkew, "the-nonce-we-asked-for"); err != nil {
		t.Fatalf("token within the skew allowance refused: %v", err)
	}
}

// ── GUARD: the redirect URI, byte for byte ──────────────────────────────────
//
// MUTATION: change ExchangeWithNonce to send a normalised/derived redirect
// URI instead of the one it was handed, and this fails. Google checks this
// too, but a mismatch that only Google notices is a mismatch this code found
// out about too late.

func TestGoogleExchange_SendsTheExactRedirectURIItWasGiven(t *testing.T) {
	f := newFakeGoogle(t, tokenResponse(t, goodClaims()))
	if _, err := f.provider(t).ExchangeWithNonce(context.Background(), "code-xyz", testRedirectURI, "the-nonce-we-asked-for"); err != nil {
		t.Fatalf("ExchangeWithNonce: %v", err)
	}
	if got := f.lastForm.Get("redirect_uri"); got != testRedirectURI {
		t.Fatalf("redirect_uri sent = %q, want byte-identical %q", got, testRedirectURI)
	}
	if got := f.lastForm.Get("grant_type"); got != "authorization_code" {
		t.Fatalf("grant_type = %q", got)
	}
	if got := f.lastForm.Get("code"); got != "code-xyz" {
		t.Fatalf("code = %q", got)
	}
	if got := f.lastForm.Get("client_id"); got != testClientID {
		t.Fatalf("client_id = %q", got)
	}
}

func TestGoogleAuthURL_CarriesStateNonceAndTheSameRedirectURI(t *testing.T) {
	f := newFakeGoogle(t, "{}")
	raw := f.provider(t).AuthURLWithNonce("state-value-abc", "nonce-value-def", testRedirectURI)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"client_id":     testClientID,
		"redirect_uri":  testRedirectURI,
		"response_type": "code",
		"state":         "state-value-abc",
		"nonce":         "nonce-value-def",
		"scope":         googleScope,
	} {
		if got := q.Get(k); got != want {
			t.Errorf("auth URL %s = %q, want %q", k, got, want)
		}
	}
	// The client SECRET must never travel in a URL the browser follows.
	if strings.Contains(raw, testClientSecret) {
		t.Fatal("auth URL contains the client secret")
	}
}

// ── GUARD: tokens never appear in errors (and so never in a log line) ───────
//
// The provider itself never logs. The way a token could still reach a log
// file is by riding inside an error that a caller logs, so every failure
// path is checked for the credentials that passed through it.

func TestGoogleExchange_ErrorsNeverCarryCredentials(t *testing.T) {
	secrets := []string{testClientSecret, "ya29.a0-fake-access-token", "auth-code-abc"}

	// Failure 1: the token endpoint refuses.
	f := newFakeGoogle(t, `{"error":"invalid_grant","error_description":"Bad Request: `+testClientSecret+`"}`)
	f.status = http.StatusBadRequest
	_, err := f.provider(t).ExchangeWithNonce(context.Background(), "auth-code-abc", testRedirectURI, "n")
	if err == nil {
		t.Fatal("expected an error from a 400 token response")
	}
	for _, s := range secrets {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("error text leaked a credential (%q): %s", s, err)
		}
	}
	// Google's short error code IS useful and carries nothing secret.
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should name the provider's error code, got: %s", err)
	}

	// Failure 2: a token that fails validation. The ID token itself must not
	// be quoted back.
	bad := goodClaims()
	bad["aud"] = "someone-else"
	f2 := newFakeGoogle(t, tokenResponse(t, bad))
	_, err = f2.provider(t).ExchangeWithNonce(context.Background(), "auth-code-abc", testRedirectURI, "the-nonce-we-asked-for")
	if err == nil {
		t.Fatal("expected an error for a foreign audience")
	}
	for _, s := range append(secrets, idToken(t, bad)) {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("validation error leaked %q: %s", s, err)
		}
	}
}

func TestGoogleExchange_RefusesTokenResponseWithoutIDToken(t *testing.T) {
	f := newFakeGoogle(t, `{"access_token":"ya29.only","token_type":"Bearer"}`)
	if _, err := f.provider(t).ExchangeWithNonce(context.Background(), "c", testRedirectURI, ""); !errors.Is(err, ErrOAuthExchangeFailed) {
		t.Fatalf("expected ErrOAuthExchangeFailed, got %v", err)
	}
}

func TestGoogleExchange_RefusesMalformedIDToken(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt", "a.b", "a.!!!not-base64!!!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".c"} {
		body, err := json.Marshal(map[string]string{"id_token": tok})
		if err != nil {
			t.Fatal(err)
		}
		f := newFakeGoogle(t, string(body))
		_, err = f.provider(t).ExchangeWithNonce(context.Background(), "c", testRedirectURI, "")
		if err == nil {
			t.Fatalf("malformed id_token %q was accepted", tok)
		}
	}
}

// ── off unless configured ───────────────────────────────────────────────────

func TestFromEnvGoogle_OffUnlessBothVariablesAreSet(t *testing.T) {
	t.Run("neither set is not an error and yields no provider", func(t *testing.T) {
		t.Setenv(EnvGoogleClientID, "")
		t.Setenv(EnvGoogleClientSecret, "")
		os.Unsetenv(EnvGoogleClientID)
		os.Unsetenv(EnvGoogleClientSecret)
		p, err := FromEnvGoogle()
		if err != nil {
			t.Fatalf("unconfigured should not error: %v", err)
		}
		if p != nil {
			t.Fatal("unconfigured returned a provider — Google sign-in is opt-in")
		}
	})

	t.Run("both set yields a provider", func(t *testing.T) {
		t.Setenv(EnvGoogleClientID, testClientID)
		t.Setenv(EnvGoogleClientSecret, testClientSecret)
		p, err := FromEnvGoogle()
		if err != nil {
			t.Fatalf("FromEnvGoogle: %v", err)
		}
		if p == nil {
			t.Fatal("both variables set but no provider")
		}
		if p.Name() != ProviderNameGoogle {
			t.Fatalf("Name() = %q", p.Name())
		}
	})

	// Half-configured is a refusal, never a silent fallback to "off": an
	// operator who believes they enabled Google sign-in must be told, not
	// left wondering where the button went.
	//
	// SET-BUT-EMPTY and GENUINELY-UNSET are BOTH covered, and they are
	// different code paths — os.LookupEnv can tell them apart, and the first
	// version of this test only exercised the first kind. Running the
	// mutation "if !idSet || !secretSet { return nil, nil }" is what found
	// that: it stayed green, because every case here had both variables SET,
	// one of them to "". The genuinely-unset cases below are the ones that
	// mutation now fails.
	for _, tc := range []struct {
		name             string
		id, secret       string
		idSet, secretSet bool
		wantNamedInError string
	}{
		{"client id set, secret entirely unset", testClientID, "", true, false, EnvGoogleClientSecret},
		{"secret set, client id entirely unset", "", testClientSecret, false, true, EnvGoogleClientID},
		{"client id set, secret set to empty", testClientID, "", true, true, EnvGoogleClientSecret},
		{"secret set, client id set to empty", "", testClientSecret, true, true, EnvGoogleClientID},
		{"client id set, secret is whitespace", testClientID, "   ", true, true, EnvGoogleClientSecret},
		{"secret set, client id is whitespace", "  ", testClientSecret, true, true, EnvGoogleClientID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv first in both cases so its cleanup restores whatever
			// the environment held before, then unset for the "not set at
			// all" half.
			t.Setenv(EnvGoogleClientID, tc.id)
			t.Setenv(EnvGoogleClientSecret, tc.secret)
			if !tc.idSet {
				os.Unsetenv(EnvGoogleClientID)
			}
			if !tc.secretSet {
				os.Unsetenv(EnvGoogleClientSecret)
			}

			p, err := FromEnvGoogle()
			if !errors.Is(err, ErrOAuthMisconfigured) {
				t.Fatalf("expected ErrOAuthMisconfigured, got provider=%v err=%v", p != nil, err)
			}
			if p != nil {
				t.Fatal("a provider was returned alongside the error")
			}
			// The message must name the MISSING variable, so the operator
			// can fix it without reading the source.
			if !strings.Contains(err.Error(), tc.wantNamedInError) {
				t.Fatalf("the error does not name the missing variable %s: %v", tc.wantNamedInError, err)
			}
			// And must never echo the credential that WAS supplied.
			if tc.secret != "" && strings.TrimSpace(tc.secret) != "" && strings.Contains(err.Error(), tc.secret) {
				t.Fatalf("the error leaked the client secret: %v", err)
			}
		})
	}
}

func TestNewGoogleProvider_RequiresBothCredentials(t *testing.T) {
	if _, err := NewGoogleProvider("", testClientSecret); !errors.Is(err, ErrOAuthMisconfigured) {
		t.Fatalf("empty client id accepted: %v", err)
	}
	if _, err := NewGoogleProvider(testClientID, ""); !errors.Is(err, ErrOAuthMisconfigured) {
		t.Fatalf("empty client secret accepted: %v", err)
	}
}

// ── GUARD: the service-layer backstop, tested on its own ────────────────────
//
// GoogleProvider refuses an unverified address in identityFromIDToken, so in
// production a bad token never reaches LoginWithOAuth. That makes the two
// checks capable of masking each other, which is exactly the trap a sibling
// agent hit with requireUser. This test bypasses the provider entirely and
// hands LoginWithOAuth the unverified info directly, so it proves the SERVICE
// layer holds — independently of anything google.go does.
//
// MUTATION: delete the `!info.EmailVerified` check in LoginWithOAuth and this
// fails while every google.go test above still passes.

func TestLoginWithOAuth_RefusesUnverifiedEmailWithNoProviderInvolved(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := NewService(st)

	// The attack this closes: a password account exists, and a provider
	// identity claims the same address without verifying it.
	victim, err := svc.Signup(ctx, "owner@venue.example", "correct horse battery", "Owner")
	if err != nil {
		t.Fatalf("signup victim: %v", err)
	}

	attacker := OAuthUserInfo{
		Provider:      "google",
		Subject:       "attacker-subject",
		Email:         "owner@venue.example",
		Name:          "Not The Owner",
		EmailVerified: false,
	}
	if _, err := svc.LoginWithOAuth(ctx, attacker); !errors.Is(err, ErrOAuthEmailUnverified) {
		t.Fatalf("unverified email was not refused: %v", err)
	}

	// Nothing was linked, and the victim's account is untouched.
	if _, err := st.GetOAuthIdentity(ctx, "google", "attacker-subject"); err == nil {
		t.Fatal("an identity was linked despite the refusal")
	}
	reloaded, err := st.GetUserByID(ctx, victim.ID)
	if err != nil {
		t.Fatalf("reload victim: %v", err)
	}
	if reloaded.PasswordHash != victim.PasswordHash {
		t.Fatal("victim's password hash changed")
	}

	// A VERIFIED assertion of the same address does link — the documented,
	// deliberate half of the decision.
	verified := attacker
	verified.EmailVerified = true
	linked, err := svc.LoginWithOAuth(ctx, verified)
	if err != nil {
		t.Fatalf("verified link refused: %v", err)
	}
	if linked.ID != victim.ID {
		t.Fatal("a verified provider email did not link to the existing account")
	}
}

func TestLoginWithOAuth_RefusesVerifiedFlagWithNoAddress(t *testing.T) {
	svc := NewService(newTestStore(t))
	_, err := svc.LoginWithOAuth(context.Background(), OAuthUserInfo{
		Provider: "google", Subject: "s", Email: "", EmailVerified: true,
	})
	if !errors.Is(err, ErrOAuthEmailUnverified) {
		t.Fatalf("expected ErrOAuthEmailUnverified, got %v", err)
	}
}

// ── the constraint that matters most: the scan path never reaches OAuth ─────
//
// Cackle's defining claim is that the gate works with no internet. This is
// the structural proof that enabling Google sign-in cannot change that: the
// package that decides admissions does not import the package that can make
// an outbound request, so there is no call graph from a scan to a network
// call, regardless of configuration.
//
// A source-level check rather than a runtime one on purpose — a runtime test
// proves one path was not taken on one run; this proves no path exists.

func TestScanPathCannotReachOAuth(t *testing.T) {
	scanDir := filepath.Join("..", "scan")
	if _, err := os.Stat(scanDir); err != nil {
		t.Fatalf("internal/scan not found — this guard is looking at the wrong tree: %v", err)
	}

	examined, admissionFiles := 0, 0
	var reachesAuth, admissionReachesNet []string
	err := filepath.Walk(scanDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		examined++
		if strings.Contains(string(src), `"github.com/vul-os/cackle/internal/auth"`) {
			reachesAuth = append(reachesAuth, path)
		}
		// The ADMISSION decision is internal/scan itself. Its subpackage
		// internal/scan/substrate is the server-to-server ledger sync, which
		// is a different thing that legitimately speaks HTTP (peerauth.go
		// signs outbound peer requests) and is never on the path from "a QR
		// code was presented" to "let them in". So the no-network rule is
		// asserted on the decision package, precisely.
		if filepath.Dir(path) == scanDir {
			admissionFiles++
			if strings.Contains(string(src), `"net/http"`) {
				admissionReachesNet = append(admissionReachesNet, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/scan: %v", err)
	}

	// Fail-closed: a walk that found nothing must not report success.
	if examined < 5 || admissionFiles < 3 {
		t.Fatalf("only %d Go files (%d in the admission package) examined under internal/scan — the guard is looking at an empty tree", examined, admissionFiles)
	}
	if len(reachesAuth) > 0 {
		t.Fatalf("the scan tree imports internal/auth, so OAuth is reachable from the gate:\n  %s", strings.Join(reachesAuth, "\n  "))
	}
	if len(admissionReachesNet) > 0 {
		t.Fatalf("the admission package imports net/http — the gate decision can now go to the network:\n  %s", strings.Join(admissionReachesNet, "\n  "))
	}
	t.Logf("%d files under internal/scan examined (%d in the admission package); none imports internal/auth, and no admission file imports net/http",
		examined, admissionFiles)
}
