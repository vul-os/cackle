package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersPresentOnEveryResponse(t *testing.T) {
	h := newTestHarness(t)
	rec := h.do(http.MethodGet, "/api/events", "", nil)

	for _, header := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy", "Permissions-Policy"} {
		if rec.Header().Get(header) == "" {
			t.Errorf("expected %s header to be set, got none", header)
		}
	}
}

func TestHealthzIsPublicAndOutsideAPI(t *testing.T) {
	h := newTestHarness(t)
	rec := h.do(http.MethodGet, "/healthz", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: expected 200, got %d", rec.Code)
	}
}

func TestAPINotFoundReturnsJSONNotHTML(t *testing.T) {
	h := newTestHarness(t)
	rec := h.do(http.MethodGet, "/api/this-route-does-not-exist", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	assertErrorShape(t, rec)
}

// TestCSRF_CookieAuthMutationRequiresToken proves the double-submit CSRF
// check: a cookie-authenticated mutation without the matching
// X-CSRF-Token header is rejected, and with it, succeeds. A
// Bearer-authenticated mutation needs no CSRF token at all (no browser
// attaches Authorization automatically, so there's no cross-site vector).
func TestCSRF_CookieAuthMutationRequiresToken(t *testing.T) {
	h := newTestHarness(t)

	rec := h.do(http.MethodPost, "/api/auth/signup", "", signupRequest{
		Email: "csrf@example.com", Password: "csrf-password-123", Name: "CSRF Test",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("signup: status %d body %s", rec.Code, rec.Body.String())
	}

	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case sessionCookieName:
			sessionCookie = c
		case csrfCookieName:
			csrfCookie = c
		}
	}
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatalf("expected both %s and %s cookies to be set on login", sessionCookieName, csrfCookieName)
	}

	// Cookie auth, no CSRF header: rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.RemoteAddr = "203.0.113.30:1"
	req.AddCookie(sessionCookie)
	rec2 := httptest.NewRecorder()
	h.handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("cookie-auth mutation without CSRF token: expected 403, got %d body %s", rec2.Code, rec2.Body.String())
	}

	// Cookie auth, wrong CSRF header: still rejected.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.RemoteAddr = "203.0.113.30:1"
	req.AddCookie(sessionCookie)
	req.Header.Set(csrfHeaderName, "not-the-right-token")
	rec3 := httptest.NewRecorder()
	h.handler.ServeHTTP(rec3, req)
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("cookie-auth mutation with wrong CSRF token: expected 403, got %d", rec3.Code)
	}

	// Cookie auth, correct CSRF header: succeeds.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.RemoteAddr = "203.0.113.30:1"
	req.AddCookie(sessionCookie)
	req.Header.Set(csrfHeaderName, csrfCookie.Value)
	rec4 := httptest.NewRecorder()
	h.handler.ServeHTTP(rec4, req)
	if rec4.Code != http.StatusNoContent {
		t.Fatalf("cookie-auth mutation with correct CSRF token: expected 204, got %d body %s", rec4.Code, rec4.Body.String())
	}
}

// TestRateLimit_AuthEndpointsThrottlePerIP proves repeated rapid requests
// from the same client IP eventually get a 429, per the house security bar
// ("rate-limit auth + scan").
func TestRateLimit_AuthEndpointsThrottlePerIP(t *testing.T) {
	h := newTestHarness(t)

	var sawTooManyRequests bool
	for i := 0; i < 20; i++ {
		rec := h.do(http.MethodPost, "/api/auth/login", "", loginRequest{Email: "nobody@example.com", Password: "wrong"})
		if rec.Code == http.StatusTooManyRequests {
			sawTooManyRequests = true
			assertErrorShape(t, rec)
			break
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status %d on attempt %d: %s", rec.Code, i, rec.Body.String())
		}
	}
	if !sawTooManyRequests {
		t.Fatal("expected to hit a 429 rate limit within 20 rapid login attempts from the same IP")
	}
}
