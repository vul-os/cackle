package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName = "cackle_session"
	csrfCookieName    = "cackle_csrf"
	csrfHeaderName    = "X-CSRF-Token"
)

// extractToken pulls a session token from an Authorization: Bearer header
// (preferred — never subject to CSRF, since browsers don't attach it
// automatically) or, failing that, the httpOnly session cookie.
func extractToken(r *http.Request) (token string, viaCookie bool) {
	if h := r.Header.Get("Authorization"); h != "" {
		if rest, ok := strings.CutPrefix(h, "Bearer "); ok && rest != "" {
			return rest, false
		}
	}
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value, true
	}
	return "", false
}

// authenticate resolves the caller's session (if any) and stores the user,
// raw token, and auth-method on the request context. It never rejects a
// request itself — that is requireUser's job — so public routes (event
// listing, signup, login) can still run through the same middleware chain.
func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, viaCookie := extractToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, _, err := s.deps.Auth.ValidateSession(r.Context(), token)
		if err != nil {
			// Invalid/expired session: treat exactly like anonymous. Never
			// leak *why* via the request — a stale cookie just means the
			// user isn't logged in anymore.
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyUser, user)
		ctx = context.WithValue(ctx, ctxKeySessionToken, token)
		ctx = context.WithValue(ctx, ctxKeyAuthViaCookie, viaCookie)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireUser wraps a handler that must not run for an anonymous caller.
func (s *server) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := userFromContext(r.Context()); !ok {
			unauthorized(w, "authentication required")
			return
		}
		next(w, r)
	}
}

// csrfTokenFor derives the expected CSRF token for a session token: an
// HMAC-SHA256 keyed by the server's session secret. Cookie-authenticated
// clients read this value back from the (non-httpOnly) csrf cookie set at
// login and must echo it in the X-CSRF-Token header on every mutation —
// classic double-submit, but bound to the actual session via HMAC rather
// than a bare random value, so it can be verified statelessly.
func csrfTokenFor(sessionToken, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sessionToken))
	return hex.EncodeToString(mac.Sum(nil))
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// requireCSRF enforces CSRF protection on every mutating request that
// authenticated via the session cookie. Bearer-token requests are exempt:
// a browser never attaches an Authorization header to a cross-site request
// on its own, so there is no CSRF vector to defend against there — only
// the cookie path needs the double-submit check.
func (s *server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		token, viaCookie := sessionTokenFromContext(r.Context())
		if !viaCookie || token == "" {
			next.ServeHTTP(w, r)
			return
		}
		want := csrfTokenFor(token, s.deps.Config.SessionSecret)
		got := r.Header.Get(csrfHeaderName)
		if got == "" || !hmac.Equal([]byte(got), []byte(want)) {
			forbidden(w, "missing or invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// setSessionCookies issues the httpOnly session cookie and its companion,
// JS-readable CSRF cookie after a successful login/signup. secure controls
// the Secure flag — always true outside of explicit local-dev/tests.
func setSessionCookies(w http.ResponseWriter, token string, expiresAt time.Time, secret string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    csrfTokenFor(token, secret),
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookies(w http.ResponseWriter, secure bool) {
	expired := time.Unix(0, 0)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", Expires: expired,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: "", Path: "/", Expires: expired,
		HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// isHTTPSRequest is a best-effort check for whether this request arrived
// over TLS or via a reverse proxy that terminated TLS, used only to decide
// the Secure flag on cookies issued in response. It fails safe: anything
// ambiguous is treated as non-secure only when explicitly running in a
// configured dev/http mode.
func isHTTPSRequest(r *http.Request, forceSecure bool) bool {
	if forceSecure {
		return true
	}
	if r.TLS != nil {
		return true
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
		return true
	}
	return false
}

var errNoSession = errors.New("httpapi: no session")
