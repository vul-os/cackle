package httpapi

// The OAuth sign-in routes: start, callback, and the little public endpoint
// that tells the frontend whether either exists.
//
// # Off unless configured
//
// Deps.OAuth is nil unless an operator set both CACKLE_GOOGLE_CLIENT_ID and
// CACKLE_GOOGLE_CLIENT_SECRET (see auth.FromEnvGoogle and cmd/cackle). When
// it is nil:
//
//   - the /oauth/{provider}/* routes are NOT MOUNTED — they 404 like any
//     other unknown route, rather than existing and refusing;
//   - GET /api/auth/providers answers {"providers":[]};
//   - the frontend, seeing an empty list, renders no button.
//
// That chain is the whole point. A button that cannot work is worse than no
// button, and a stock Cackle must issue no external request whatsoever.
//
// # This is organiser login. It is not, and must never become, gate
// infrastructure
//
// Nothing on the admission path touches this file. internal/scan does not
// import internal/auth at all (proved in TestScanPathCannotReachOAuth), the
// scan routes call none of these handlers, and password login stays fully
// functional and fully visible whether or not a provider is configured. An
// operator whose internet is down signs in with a password and works the
// door exactly as before.
//
// # Sessions
//
// A successful callback goes through issueSession — the SAME path signup and
// login use. Server-side session row, only a hash stored, httpOnly cookie
// plus the CSRF companion. There is no second session mechanism here and no
// JWT: the ID token from the provider is inspected, used to answer "who is
// this", and dropped. It is never stored, never re-presented, and never
// handed to the browser.
//
// The session token is deliberately NOT put in the redirect URL. A token in
// a query string lands in access logs, in Referer headers and in browser
// history; this codebase already learned that lesson with invite links. The
// browser gets the session as a cookie and the SPA reads /api/auth/me.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
)

const (
	// oauthStateCookieName holds the CSRF state and the OIDC nonce for one
	// in-flight sign-in. httpOnly, single-use, scoped to the OAuth routes
	// only, and cleared on the way through the callback whatever happens.
	oauthStateCookieName = "cackle_oauth"

	// oauthCookiePath keeps the cookie off every other route in the product.
	// It must stay a prefix of both the start and callback paths.
	oauthCookiePath = "/api/auth/oauth"

	// oauthStateTTL is how long a started sign-in stays completable. Long
	// enough to pick an account and type a password at Google, short enough
	// that a state value lifted from a shared machine is stale.
	oauthStateTTL = 10 * time.Minute

	// oauthCallbackSuffix is appended to the base URL to form the redirect
	// URI. It must match the path the callback route is mounted at, and it
	// is what an operator registers in the Google Cloud Console.
	oauthCallbackSuffix = "/api/auth/oauth/%s/callback"
)

// Reason codes appended to /login?sso=… when a sign-in does not complete.
// They are short, non-secret, and safe in a URL bar and an access log — the
// actual detail goes to the server log and nowhere else.
const (
	ssoReasonDenied     = "denied"     // the person said no at the provider
	ssoReasonState      = "state"      // state missing, stale, or mismatched
	ssoReasonUnverified = "unverified" // the provider would not vouch for the email
	ssoReasonFailed     = "failed"     // everything else
)

// oauthProvider returns the configured provider, or nil.
//
// It requires auth.OIDCProvider rather than the bare auth.OAuthProvider on
// purpose: a provider that cannot carry a nonce cannot be nonce-checked, and
// "this provider does not support nonce" must never quietly become "nonce
// not checked". A configured provider that does not satisfy the interface is
// treated as not configured at all, and cmd/cackle refuses to start rather
// than reaching that state.
func (s *server) oauthProvider() auth.OIDCProvider {
	if s.deps.OAuth == nil {
		return nil
	}
	p, ok := s.deps.OAuth.(auth.OIDCProvider)
	if !ok {
		return nil
	}
	return p
}

// oauthRedirectURI is THE redirect URI, derived once from configuration and
// used byte-identically in the authorization request and in the token
// exchange.
//
// It is built from cfg.BaseURL and never from the incoming request's Host or
// X-Forwarded-Host. Those are attacker-controlled: a request carrying
// Host: evil.example would otherwise make Cackle ask Google to send the
// authorization code somewhere else. The cost of that choice is that
// CACKLE_BASE_URL must be right — which is exactly what the operator also
// pastes into the Google Cloud Console, so a mismatch fails loudly at Google
// on the very first attempt instead of silently later.
func (s *server) oauthRedirectURI(provider string) string {
	base := strings.TrimRight(baseURLOf(s.deps.Config), "/")
	return base + strings.Replace(oauthCallbackSuffix, "%s", provider, 1)
}

// oauthProviderView is one entry in the providers list.
type oauthProviderView struct {
	// ID is the provider name, e.g. "google".
	ID string `json:"id"`
	// Label is what a button should say.
	Label string `json:"label"`
	// StartPath is where the button should send the browser. Relative on
	// purpose: the app never holds an absolute URL to anywhere off-origin.
	StartPath string `json:"start_path"`
}

// handleListOAuthProviders serves GET /api/auth/providers — public, and the
// only route in this file that exists whether or not anything is configured.
//
// It answers {"providers":[]} on a stock box. That empty list is what stops
// the frontend rendering a button, and it is a cheap, honest answer: the
// client asks "what sign-in methods work here", and the server says "just
// the password form".
func (s *server) handleListOAuthProviders(w http.ResponseWriter, r *http.Request) {
	out := make([]oauthProviderView, 0, 1)
	if p := s.oauthProvider(); p != nil {
		name := p.Name()
		out = append(out, oauthProviderView{
			ID:        name,
			Label:     oauthLabelFor(name),
			StartPath: oauthCookiePath + "/" + name + "/start",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

func oauthLabelFor(name string) string {
	if name == auth.ProviderNameGoogle {
		return "Google"
	}
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// handleOAuthStart serves GET /api/auth/oauth/{provider}/start: mint a state
// and a nonce, remember them in an httpOnly cookie, and redirect to the
// provider.
//
// This is the ONLY place in Cackle that sends a browser off-origin, and it
// only runs on a box where an operator configured a provider.
func (s *server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	p := s.oauthProvider()
	if p == nil || chi.URLParam(r, "provider") != p.Name() {
		notFound(w, "no such sign-in provider")
		return
	}

	state, err := randomToken()
	if err != nil {
		internalError(w, s.log(), "oauth state", err)
		return
	}
	nonce, err := randomToken()
	if err != nil {
		internalError(w, s.log(), "oauth nonce", err)
		return
	}

	setOAuthStateCookie(w, state, nonce, isHTTPSRequest(r, false))
	http.Redirect(w, r, p.AuthURLWithNonce(state, nonce, s.oauthRedirectURI(p.Name())), http.StatusFound)
}

// handleOAuthCallback serves GET /api/auth/oauth/{provider}/callback.
//
// Order matters and is deliberate: the state cookie is cleared FIRST, so a
// sign-in is single-use no matter which way the rest of this function exits,
// including a panic caught by the recoverer. Then state is checked, before
// the code is spent — an exchange is a real request to a third party and
// must not be reachable by an unauthenticated caller replaying a URL.
func (s *server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	p := s.oauthProvider()
	if p == nil || chi.URLParam(r, "provider") != p.Name() {
		notFound(w, "no such sign-in provider")
		return
	}

	wantState, wantNonce, haveCookie := readOAuthStateCookie(r)
	clearOAuthStateCookie(w, isHTTPSRequest(r, false))

	// The person pressed "cancel" (or the provider refused). Not an error
	// worth a stack trace; send them back to a sign-in page that still works.
	if errCode := r.URL.Query().Get("error"); errCode != "" {
		s.log().Info("oauth sign-in not completed at provider", "provider", p.Name(), "provider_error", errCode)
		s.redirectToSignIn(w, r, ssoReasonDenied)
		return
	}

	// ── GUARD: state (CSRF) ────────────────────────────────────────────────
	//
	// Without this, anyone can feed a victim's browser a callback URL
	// carrying THEIR authorization code and silently sign the victim into
	// the attacker's account — a login CSRF, which on a box like this means
	// the victim's next action happens in an org they do not own.
	//
	// There is no other layer holding this up. The callback is a GET, so
	// requireCSRF (mutating methods only) does not apply, and there is no
	// requireUser in front of it — by necessity, since the caller is not
	// signed in yet. Removing the comparison below removes the only check.
	gotState := r.URL.Query().Get("state")
	if !haveCookie || wantState == "" || subtle.ConstantTimeCompare([]byte(gotState), []byte(wantState)) != 1 {
		s.log().Warn("oauth callback refused: state did not match", "provider", p.Name(), "had_cookie", haveCookie)
		s.redirectToSignIn(w, r, ssoReasonState)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		s.redirectToSignIn(w, r, ssoReasonFailed)
		return
	}

	// ── GUARD: exact redirect URI, and the nonce ───────────────────────────
	//
	// The same string handleOAuthStart sent, recomputed from configuration
	// rather than reconstructed from this request. wantNonce came out of the
	// httpOnly cookie; the provider refuses any ID token that does not echo
	// it back exactly.
	info, err := p.ExchangeWithNonce(r.Context(), code, s.oauthRedirectURI(p.Name()), wantNonce)
	if err != nil {
		// The provider's errors carry no tokens (asserted in
		// TestGoogleExchange_ErrorsNeverCarryCredentials), so this is safe to
		// log. The user gets a reason code, never the detail.
		if errors.Is(err, auth.ErrOAuthEmailUnverified) {
			s.log().Warn("oauth sign-in refused: provider did not verify the email address", "provider", p.Name())
			s.redirectToSignIn(w, r, ssoReasonUnverified)
			return
		}
		s.log().Warn("oauth exchange failed", "provider", p.Name(), "error", err)
		s.redirectToSignIn(w, r, ssoReasonFailed)
		return
	}

	user, err := s.deps.Auth.LoginWithOAuth(r.Context(), info)
	if err != nil {
		// The backstop half of the linking rule. The provider already
		// refuses an unverified address; if that check were ever removed,
		// this one still stops the takeover — which is why both exist and
		// why they are tested apart.
		if errors.Is(err, auth.ErrOAuthEmailUnverified) {
			s.log().Warn("oauth sign-in refused at the service layer: unverified email", "provider", p.Name())
			s.redirectToSignIn(w, r, ssoReasonUnverified)
			return
		}
		s.log().Error("oauth login", "provider", p.Name(), "error", err)
		s.redirectToSignIn(w, r, ssoReasonFailed)
		return
	}

	// The existing session path. Not a new one.
	token, expiresAt, err := s.deps.Auth.CreateSession(r.Context(), user.ID)
	if err != nil {
		internalError(w, s.log(), "oauth create session", err)
		return
	}
	setSessionCookies(w, token, expiresAt, s.deps.Config.SessionSecret, isHTTPSRequest(r, false))

	// Back to the sign-in page, which sees an established session and sends
	// the user wherever password sign-in would have. No token in this URL.
	s.redirectToSignIn(w, r, "ok")
}

// redirectToSignIn sends the browser to the SPA's sign-in page with a short
// reason code. 303 rather than 302: the callback is a GET but the browser
// should treat the destination as a fresh navigation.
func (s *server) redirectToSignIn(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/login?sso="+reason, http.StatusSeeOther)
}

// randomToken mints 32 bytes of URL-safe randomness for a state or nonce.
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// setOAuthStateCookie stores state and nonce for the round trip.
//
// SameSite=Lax and not Strict: the callback arrives as a top-level
// navigation from Google, and Strict would withhold the cookie there,
// breaking every sign-in. Lax sends it on exactly that navigation and
// withholds it from cross-site subresource requests, which is the property
// that matters.
func setOAuthStateCookie(w http.ResponseWriter, state, nonce string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state + "." + nonce,
		Path:     oauthCookiePath,
		MaxAge:   int(oauthStateTTL / time.Second),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthStateCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookieName, Value: "", Path: oauthCookiePath,
		Expires: time.Unix(0, 0), MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

// readOAuthStateCookie returns the state and nonce from the cookie. ok is
// false when the cookie is absent or does not hold both halves — an
// unparseable cookie is treated as no cookie, never as an empty state that
// an empty query parameter could then match.
func readOAuthStateCookie(r *http.Request) (state, nonce string, ok bool) {
	c, err := r.Cookie(oauthStateCookieName)
	if err != nil || c.Value == "" {
		return "", "", false
	}
	state, nonce, found := strings.Cut(c.Value, ".")
	if !found || state == "" || nonce == "" {
		return "", "", false
	}
	return state, nonce, true
}
