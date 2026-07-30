package httpapi

// Tests for the OAuth sign-in routes.
//
// NOTHING HERE CONTACTS GOOGLE, or anything else. The provider is a local
// fake that records what it was asked and answers from a fixture, so every
// assertion is about THIS package's protocol handling — state, nonce,
// redirect URI, session issuance, and what does and does not get logged.
// Google's own behaviour is not, and cannot be, tested here.
//
// Each security guard below carries a MUTATION note naming the single edit
// that must turn it red. Where a guard could be held up by a second layer,
// the test is written so only one layer is in play — see the note on the
// unverified-email test in particular.

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/auth"
)

// ── the fake provider ───────────────────────────────────────────────────────

// fakeOIDC implements auth.OIDCProvider with no network and no validation of
// its own. That absence is the point: it makes this file's assertions about
// httpapi's guards, not about a provider's.
type fakeOIDC struct {
	name string
	info auth.OAuthUserInfo
	err  error

	// what it was handed, for assertions
	exchanges       int
	lastCode        string
	lastRedirectURI string
	lastNonce       string
	lastAuthNonce   string
	lastAuthState   string
}

func (f *fakeOIDC) Name() string { return f.name }

func (f *fakeOIDC) AuthURL(state, redirectURI string) string {
	return f.AuthURLWithNonce(state, "", redirectURI)
}

func (f *fakeOIDC) AuthURLWithNonce(state, nonce, redirectURI string) string {
	f.lastAuthState, f.lastAuthNonce = state, nonce
	q := url.Values{}
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("redirect_uri", redirectURI)
	return "https://provider.invalid/authorize?" + q.Encode()
}

func (f *fakeOIDC) Exchange(ctx context.Context, code, redirectURI string) (auth.OAuthUserInfo, error) {
	return f.ExchangeWithNonce(ctx, code, redirectURI, "")
}

func (f *fakeOIDC) ExchangeWithNonce(_ context.Context, code, redirectURI, nonce string) (auth.OAuthUserInfo, error) {
	f.exchanges++
	f.lastCode, f.lastRedirectURI, f.lastNonce = code, redirectURI, nonce
	if f.err != nil {
		return auth.OAuthUserInfo{}, f.err
	}
	return f.info, nil
}

func verifiedFake() *fakeOIDC {
	return &fakeOIDC{
		name: auth.ProviderNameGoogle,
		info: auth.OAuthUserInfo{
			Provider:      auth.ProviderNameGoogle,
			Subject:       "provider-subject-1",
			Email:         "organiser@venue.example",
			Name:          "Venue Organiser",
			EmailVerified: true,
		},
	}
}

// ── harness ─────────────────────────────────────────────────────────────────

// withOAuth rebuilds the harness's router with an OAuth provider wired in,
// reusing every service the plain harness already built. Deliberately not a
// change to newTestHarness: the DEFAULT harness must stay a box with no
// provider configured, because that is the default deployment.
func (h *testHarness) withOAuth(p auth.OAuthProvider) *testHarness {
	h.t.Helper()
	h.handler = New(Deps{
		Store:    h.store,
		Auth:     h.auth,
		Events:   h.events,
		Orders:   h.orders,
		Orgs:     h.orgs,
		Payments: h.payments,
		Config:   h.cfg,
		MediaDir: h.mediaDir,
		OAuth:    p,
		Logger:   slog.New(slog.NewTextHandler(h.logs, nil)),
	})
	return h
}

// get issues a GET carrying the given cookies (h.do only speaks bearer).
func (h *testHarness) getWithCookies(path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "203.0.113.10:12345"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

// cookieNamed picks a Set-Cookie out of a response.
func cookieNamed(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range (&http.Response{Header: rec.Header()}).Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func ssoReasonOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse Location %q: %v", loc, err)
	}
	if u.Path != "/login" {
		t.Fatalf("expected a redirect to /login, got %q", loc)
	}
	return u.Query().Get("sso")
}

// startSignIn walks the start route and returns the state cookie the server
// set, plus the state and nonce it minted.
func (h *testHarness) startSignIn(t *testing.T) (*http.Cookie, string, string) {
	t.Helper()
	rec := h.getWithCookies("/api/auth/oauth/google/start")
	if rec.Code != http.StatusFound {
		t.Fatalf("start: status %d body %s", rec.Code, rec.Body.String())
	}
	c := cookieNamed(t, rec, oauthStateCookieName)
	if c == nil {
		t.Fatal("start set no state cookie")
	}
	state, nonce, ok := strings.Cut(c.Value, ".")
	if !ok {
		t.Fatalf("state cookie %q does not hold state.nonce", c.Value)
	}
	return c, state, nonce
}

// ── OFF unless configured ───────────────────────────────────────────────────
//
// MUTATION: mount the OAuth routes unconditionally in deps.go and the 404
// assertions fail. MUTATION: make handleListOAuthProviders always list
// google and the empty-list assertion fails.

func TestOAuth_NotConfigured_NothingMountedNothingOffered(t *testing.T) {
	h := newTestHarness(t) // the default box: no provider

	rec := h.do(http.MethodGet, "/api/auth/providers", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("providers: status %d body %s", rec.Code, rec.Body.String())
	}
	got := decodeBody[struct {
		Providers []oauthProviderView `json:"providers"`
	}](t, rec)
	if len(got.Providers) != 0 {
		t.Fatalf("an unconfigured box offered %d sign-in providers: %+v", len(got.Providers), got.Providers)
	}
	// The list must be [] and not null: a JSON null makes a frontend
	// `.map` throw, and a page that throws is a page that cannot render
	// the password form either.
	if !strings.Contains(rec.Body.String(), `"providers":[]`) {
		t.Fatalf("expected an empty JSON array, got %s", rec.Body.String())
	}

	for _, path := range []string{
		"/api/auth/oauth/google/start",
		"/api/auth/oauth/google/callback?code=x&state=y",
		"/api/auth/oauth/github/start",
	} {
		rec := h.getWithCookies(path)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s on an unconfigured box: status %d, want 404 (body %s)", path, rec.Code, rec.Body.String())
		}
	}
}

func TestOAuth_Configured_IsOfferedUnderARelativePath(t *testing.T) {
	h := newTestHarness(t).withOAuth(verifiedFake())

	rec := h.do(http.MethodGet, "/api/auth/providers", "", nil)
	got := decodeBody[struct {
		Providers []oauthProviderView `json:"providers"`
	}](t, rec)
	if len(got.Providers) != 1 {
		t.Fatalf("expected exactly one provider, got %+v", got.Providers)
	}
	p := got.Providers[0]
	if p.ID != "google" || p.Label != "Google" {
		t.Fatalf("unexpected provider view: %+v", p)
	}
	// The path the button uses must be relative — the shipped app holds no
	// absolute URL to anywhere off-origin (scripts/check-app.mjs).
	if !strings.HasPrefix(p.StartPath, "/") || strings.Contains(p.StartPath, "://") {
		t.Fatalf("start_path is not a relative path: %q", p.StartPath)
	}
}

// A provider that cannot carry a nonce is treated as not configured at all,
// so "this provider has no nonce support" can never become "nonce not
// checked".
//
// MUTATION: relax oauthProvider() to accept a bare auth.OAuthProvider and
// this fails.
func TestOAuth_NonOIDCProviderIsNotMounted(t *testing.T) {
	stub := auth.NewStubOAuthProvider("google", auth.OAuthUserInfo{
		Subject: "s", Email: "e@example.com", EmailVerified: true,
	})
	h := newTestHarness(t).withOAuth(stub)

	rec := h.do(http.MethodGet, "/api/auth/providers", "", nil)
	got := decodeBody[struct {
		Providers []oauthProviderView `json:"providers"`
	}](t, rec)
	if len(got.Providers) != 0 {
		t.Fatalf("a provider with no nonce support was offered: %+v", got.Providers)
	}
	if rec := h.getWithCookies("/api/auth/oauth/google/start"); rec.Code != http.StatusNotFound {
		t.Fatalf("start mounted for a non-OIDC provider: status %d", rec.Code)
	}
}

// ── start ───────────────────────────────────────────────────────────────────

func TestOAuthStart_MintsStateAndNonceIntoAnHttpOnlyCookie(t *testing.T) {
	fake := verifiedFake()
	h := newTestHarness(t).withOAuth(fake)

	cookie, state, nonce := h.startSignIn(t)

	if !cookie.HttpOnly {
		t.Error("state cookie is not httpOnly — script on the page can read the state")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("state cookie SameSite = %v, want Lax (Strict withholds it on the callback navigation)", cookie.SameSite)
	}
	if cookie.Path != oauthCookiePath {
		t.Errorf("state cookie Path = %q, want %q — it must not travel to every route", cookie.Path, oauthCookiePath)
	}
	if cookie.MaxAge <= 0 || cookie.MaxAge > 900 {
		t.Errorf("state cookie MaxAge = %d, want a short positive TTL", cookie.MaxAge)
	}
	if len(state) < 32 || len(nonce) < 32 {
		t.Errorf("state/nonce too short to be unguessable: %d/%d chars", len(state), len(nonce))
	}
	if state == nonce {
		t.Error("state and nonce are the same value — they defend different things and must be independent")
	}

	// The provider was handed exactly what went into the cookie.
	if fake.lastAuthState != state || fake.lastAuthNonce != nonce {
		t.Fatalf("provider got state/nonce %q/%q, cookie holds %q/%q",
			fake.lastAuthState, fake.lastAuthNonce, state, nonce)
	}

	// Two sign-ins never share a state.
	_, state2, nonce2 := h.startSignIn(t)
	if state2 == state || nonce2 == nonce {
		t.Fatal("a second sign-in reused the first one's state or nonce")
	}
}

// ── GUARD: state (CSRF) ─────────────────────────────────────────────────────
//
// MUTATION: delete the subtle.ConstantTimeCompare state comparison in
// handleOAuthCallback (or replace it with `if false`) and every subtest here
// must fail.
//
// NO BACKSTOP HOLDS THIS UP, and that is checked rather than assumed: the
// callback is a GET, so requireCSRF (mutating methods only) never runs on it,
// and there is no requireUser in front of it — there cannot be, since the
// caller is not signed in yet. The final subtest proves the point by showing
// a callback with a MATCHING state does reach the provider, i.e. nothing else
// in the chain is refusing these requests for its own reasons.

func TestOAuthCallback_RefusesAStateThatIsNotTheOneWeIssued(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cookieValue string // "" = send no cookie
		queryState  string
	}{
		{"no cookie at all — a callback URL fed to a stranger's browser", "", "attacker-state"},
		{"cookie present, state does not match", "our-state.our-nonce", "attacker-state"},
		{"no state in the query", "our-state.our-nonce", ""},
		{"empty cookie is not a wildcard", ".", ""},
		{"cookie with no nonce half", "our-state", "our-state"},
		{"state is a prefix of ours", "our-state.our-nonce", "our"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := verifiedFake()
			h := newTestHarness(t).withOAuth(fake)

			var cookies []*http.Cookie
			if tc.cookieValue != "" {
				cookies = append(cookies, &http.Cookie{Name: oauthStateCookieName, Value: tc.cookieValue})
			}
			rec := h.getWithCookies("/api/auth/oauth/google/callback?code=stolen-code&state="+url.QueryEscape(tc.queryState), cookies...)

			if got := ssoReasonOf(t, rec); got != ssoReasonState {
				t.Fatalf("sso reason = %q, want %q", got, ssoReasonState)
			}
			// The strongest assertion: the authorization code was never
			// spent. A state check that refuses AFTER exchanging is a state
			// check that still let an unauthenticated caller drive an
			// outbound request.
			if fake.exchanges != 0 {
				t.Fatalf("the provider was called %d times despite a bad state", fake.exchanges)
			}
			if c := cookieNamed(t, rec, sessionCookieName); c != nil && c.Value != "" {
				t.Fatal("a session cookie was issued on a refused callback")
			}
		})
	}

	// Control: the same request shape with the RIGHT state does get through.
	// Without this, a broken route would pass every subtest above.
	fake := verifiedFake()
	h := newTestHarness(t).withOAuth(fake)
	cookie, state, _ := h.startSignIn(t)
	rec := h.getWithCookies("/api/auth/oauth/google/callback?code=good-code&state="+url.QueryEscape(state), cookie)
	if fake.exchanges != 1 {
		t.Fatalf("control: a matching state did NOT reach the provider (%d exchanges) — the subtests above prove nothing", fake.exchanges)
	}
	if got := ssoReasonOf(t, rec); got != "ok" {
		t.Fatalf("control: sso reason = %q, want ok", got)
	}
}

// The state cookie is single-use: the callback clears it on the way through,
// on every exit path, so a replayed callback URL has nothing to match.
func TestOAuthCallback_ClearsTheStateCookieOnEveryPath(t *testing.T) {
	for _, tc := range []struct{ name, query string }{
		{"success", "?code=c&state=%s"},
		{"bad state", "?code=c&state=nope"},
		{"provider said no", "?error=access_denied&state=%s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t).withOAuth(verifiedFake())
			cookie, state, _ := h.startSignIn(t)
			rec := h.getWithCookies("/api/auth/oauth/google/callback"+strings.Replace(tc.query, "%s", state, 1), cookie)
			cleared := cookieNamed(t, rec, oauthStateCookieName)
			if cleared == nil {
				t.Fatal("callback did not clear the state cookie")
			}
			if cleared.Value != "" || cleared.MaxAge >= 0 {
				t.Fatalf("state cookie not expired: value=%q MaxAge=%d", cleared.Value, cleared.MaxAge)
			}
		})
	}
}

// ── GUARD: the nonce ────────────────────────────────────────────────────────
//
// MUTATION: pass "" instead of wantNonce to ExchangeWithNonce in
// handleOAuthCallback, and this fails. (google.go's own nonce comparison is
// mutation-tested separately in google_test.go — the two layers are never
// exercised together here, so neither can mask the other.)

func TestOAuthCallback_HandsTheProviderTheNonceFromTheCookie(t *testing.T) {
	fake := verifiedFake()
	h := newTestHarness(t).withOAuth(fake)

	cookie, state, nonce := h.startSignIn(t)
	h.getWithCookies("/api/auth/oauth/google/callback?code=c&state="+url.QueryEscape(state), cookie)

	if fake.lastNonce == "" {
		t.Fatal("the provider was asked to exchange with NO expected nonce — nonce validation cannot happen")
	}
	if fake.lastNonce != nonce {
		t.Fatalf("provider got nonce %q, the cookie issued at start held %q", fake.lastNonce, nonce)
	}
	if fake.lastNonce != fake.lastAuthNonce {
		t.Fatalf("the nonce sent to the provider at start (%q) is not the one required at exchange (%q)",
			fake.lastAuthNonce, fake.lastNonce)
	}
}

// ── GUARD: exact redirect URI ───────────────────────────────────────────────
//
// MUTATION: build the redirect URI from r.Host (or honour X-Forwarded-Host)
// in oauthRedirectURI and the spoofing subtest fails.

func TestOAuthCallback_UsesTheExactConfiguredRedirectURIInBothHalves(t *testing.T) {
	fake := verifiedFake()
	h := newTestHarness(t).withOAuth(fake)
	want := h.cfg.BaseURL + "/api/auth/oauth/google/callback"

	// Half one: the authorization request.
	rec := h.getWithCookies("/api/auth/oauth/google/start")
	authURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if got := authURL.Query().Get("redirect_uri"); got != want {
		t.Fatalf("authorization redirect_uri = %q, want %q", got, want)
	}

	// Half two: the token exchange, byte-identical.
	cookie := cookieNamed(t, rec, oauthStateCookieName)
	state, _, _ := strings.Cut(cookie.Value, ".")
	h.getWithCookies("/api/auth/oauth/google/callback?code=c&state="+url.QueryEscape(state), cookie)
	if fake.lastRedirectURI != want {
		t.Fatalf("exchange redirect_uri = %q, want the identical %q", fake.lastRedirectURI, want)
	}
}

func TestOAuthStart_IgnoresASpoofedHostHeader(t *testing.T) {
	h := newTestHarness(t).withOAuth(verifiedFake())
	want := h.cfg.BaseURL + "/api/auth/oauth/google/callback"

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google/start", nil)
	req.Host = "evil.example"
	req.Header.Set("X-Forwarded-Host", "evil.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)

	authURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	got := authURL.Query().Get("redirect_uri")
	if got != want {
		t.Fatalf("a spoofed Host changed the redirect_uri to %q (want %q) — the authorization code would be delivered elsewhere", got, want)
	}
	if strings.Contains(got, "evil.example") {
		t.Fatalf("redirect_uri names an attacker-supplied host: %q", got)
	}
}

// ── GUARD: unverified email ─────────────────────────────────────────────────
//
// MUTATION: delete the `!info.EmailVerified` check in auth.LoginWithOAuth and
// this test fails.
//
// The fake provider here does NO validation of its own — it hands back
// EmailVerified:false directly. That isolates the SERVICE-layer check, which
// is the backstop; google.go's own refusal is mutation-tested separately over
// in google_test.go. Neither layer can mask the other, because neither test
// has both layers in it.

func TestOAuthCallback_RefusesAnUnverifiedEmailAndLinksNothing(t *testing.T) {
	fake := verifiedFake()
	fake.info.EmailVerified = false
	h := newTestHarness(t).withOAuth(fake)

	// A password account already exists at the address the provider claims.
	// This is the takeover being refused.
	_, victimID := h.signupUser("organiser@venue.example", "correct-horse-battery", "Real Organiser")

	cookie, state, _ := h.startSignIn(t)
	rec := h.getWithCookies("/api/auth/oauth/google/callback?code=c&state="+url.QueryEscape(state), cookie)

	if got := ssoReasonOf(t, rec); got != ssoReasonUnverified {
		t.Fatalf("sso reason = %q, want %q", got, ssoReasonUnverified)
	}
	if c := cookieNamed(t, rec, sessionCookieName); c != nil && c.Value != "" {
		t.Fatal("a session was issued for an unverified provider email")
	}
	if _, err := h.store.GetOAuthIdentity(t.Context(), "google", "provider-subject-1"); err == nil {
		t.Fatal("the identity was linked despite the refusal")
	}

	// And the real owner still owns the account.
	victim, err := h.store.GetUserByID(t.Context(), victimID)
	if err != nil {
		t.Fatalf("reload victim: %v", err)
	}
	if victim.Email != "organiser@venue.example" {
		t.Fatalf("victim account changed: %+v", victim)
	}
}

// ── the session is the existing one ─────────────────────────────────────────

func TestOAuthCallback_IssuesTheSameServerSideSessionAsPasswordLogin(t *testing.T) {
	fake := verifiedFake()
	h := newTestHarness(t).withOAuth(fake)

	cookie, state, _ := h.startSignIn(t)
	rec := h.getWithCookies("/api/auth/oauth/google/callback?code=c&state="+url.QueryEscape(state), cookie)

	if got := ssoReasonOf(t, rec); got != "ok" {
		t.Fatalf("sso reason = %q, want ok", got)
	}

	sess := cookieNamed(t, rec, sessionCookieName)
	if sess == nil || sess.Value == "" {
		t.Fatal("no session cookie issued")
	}
	if !sess.HttpOnly {
		t.Error("session cookie is not httpOnly")
	}
	// The CSRF companion the rest of the app expects — proof this went
	// through setSessionCookies and not some parallel mechanism.
	if csrf := cookieNamed(t, rec, csrfCookieName); csrf == nil || csrf.Value == "" {
		t.Fatal("no CSRF companion cookie — this did not use the shared session path")
	} else if csrf.HttpOnly {
		t.Error("the CSRF cookie must be readable by script")
	}

	// It is a real session, usable on the real routes.
	me := h.getWithCookies("/api/auth/me", sess)
	if me.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/me with the OAuth session: status %d body %s", me.Code, me.Body.String())
	}
	user := decodeBody[struct {
		User userView `json:"user"`
	}](t, me).User
	if user.Email != "organiser@venue.example" {
		t.Fatalf("session belongs to %q", user.Email)
	}

	// Server-side, and only a HASH is stored. A token that could be read
	// back out of the database would be a different security posture from
	// the one the rest of auth documents.
	var stored string
	if err := h.store.DB().QueryRowContext(t.Context(), `SELECT token_hash FROM sessions WHERE user_id = ?`, user.ID).Scan(&stored); err != nil {
		t.Fatalf("read session row: %v", err)
	}
	if stored == "" {
		t.Fatal("no session row")
	}
	if stored == sess.Value {
		t.Fatal("the session table holds the raw token, not a hash")
	}

	// No second session mechanism: no JWT anywhere in the response, and no
	// token in the URL the browser will keep in its history.
	loc := rec.Header().Get("Location")
	if strings.Contains(loc, sess.Value) || strings.Contains(loc, "token") {
		t.Fatalf("the redirect URL carries a credential: %q", loc)
	}
	for _, h := range rec.Header().Values("Set-Cookie") {
		if strings.Contains(h, "eyJ") {
			t.Fatalf("something JWT-shaped was set as a cookie: %s", h)
		}
	}
}

// A second sign-in with the same provider identity resolves to the same
// account rather than making a new one every time.
func TestOAuthCallback_IsIdempotentAcrossSignIns(t *testing.T) {
	h := newTestHarness(t).withOAuth(verifiedFake())

	ids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		cookie, state, _ := h.startSignIn(t)
		rec := h.getWithCookies("/api/auth/oauth/google/callback?code=c&state="+url.QueryEscape(state), cookie)
		sess := cookieNamed(t, rec, sessionCookieName)
		if sess == nil {
			t.Fatalf("sign-in %d issued no session", i)
		}
		me := h.getWithCookies("/api/auth/me", sess)
		ids = append(ids, decodeBody[struct {
			User userView `json:"user"`
		}](t, me).User.ID)
	}
	if ids[0] != ids[1] {
		t.Fatalf("two sign-ins produced two accounts: %s and %s", ids[0], ids[1])
	}
}

// ── GUARD: nothing secret reaches the log ───────────────────────────────────

func TestOAuthCallback_LogsNoCodeNoTokenNoSession(t *testing.T) {
	fake := verifiedFake()
	h := newTestHarness(t).withOAuth(fake)

	cookie, state, nonce := h.startSignIn(t)
	rec := h.getWithCookies("/api/auth/oauth/google/callback?code=super-secret-authorization-code&state="+url.QueryEscape(state), cookie)
	sess := cookieNamed(t, rec, sessionCookieName)
	if sess == nil {
		t.Fatal("no session issued")
	}

	logs := h.logs.String()
	for label, secret := range map[string]string{
		"the authorization code": "super-secret-authorization-code",
		"the session token":      sess.Value,
		"the OIDC nonce":         nonce,
		"the CSRF state":         state,
	} {
		if strings.Contains(logs, secret) {
			t.Fatalf("%s reached the log:\n%s", label, logs)
		}
	}
}

// ── password login is never displaced ───────────────────────────────────────

func TestOAuthEnabled_PasswordLoginStillWorks(t *testing.T) {
	h := newTestHarness(t).withOAuth(verifiedFake())

	if _, id := h.signupUser("door@venue.example", "a-real-password-1", "Door Staff"); id == "" {
		t.Fatal("signup produced no user")
	}
	rec := h.do(http.MethodPost, "/api/auth/login", "", loginRequest{
		Email: "door@venue.example", Password: "a-real-password-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("password login with a provider configured: status %d body %s", rec.Code, rec.Body.String())
	}
}

// ── THE constraint: the gate is untouched ───────────────────────────────────
//
// Configuring Google sign-in must not put a network dependency anywhere near
// admission. This runs the real scan and offline-sync routes on a box with
// the provider enabled and asserts the provider was never consulted — and
// the whole test, like every test in this package, runs with no network
// available to it at all.
//
// auth.TestScanPathCannotReachOAuth proves the same thing structurally (no
// import path exists); this proves it behaviourally on the routes an actual
// door uses.

func TestOAuthEnabled_ScanAndOfflineSyncAreUnaffected(t *testing.T) {
	fake := verifiedFake()
	h := newTestHarness(t).withOAuth(fake)
	fx := h.newPublishedEvent(t, "oauth-gate")

	buyerToken, _ := h.signupUser("buyer-oauth-gate@example.com", "buyer-password-1", "Buyer")
	tks := h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID, "buyer-oauth-gate@example.com", "Buyer", 1)

	// The scanner fetches its offline bundle...
	if rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/scan-bundle", fx.ownerToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("scan bundle: status %d body %s", rec.Code, rec.Body.String())
	}
	// ...admits a ticket...
	rec := h.do(http.MethodPost, "/api/scan", fx.ownerToken, scanRequest{
		EventID: fx.eventID, Capability: tks[0].Capability,
		DeviceID: "gate-device-1", GateID: "main-gate", ScannedAt: time.Now(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("scan: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := decodeBody[scanResponse](t, rec).Result; got != "admitted" {
		t.Fatalf("scan result = %q, want admitted", got)
	}
	// ...and catches up its offline log.
	if rec := h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, scanSyncRequest{Admissions: nil}); rec.Code != http.StatusOK {
		t.Fatalf("scan sync: status %d body %s", rec.Code, rec.Body.String())
	}

	if fake.exchanges != 0 {
		t.Fatalf("the gate path consulted the OAuth provider %d times", fake.exchanges)
	}
}
