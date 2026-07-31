package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestCSP_AllowsBlobWorkerForGateScanner pins the one directive the gate
// scanner cannot work without. qr-scanner's decode engine is a Worker built
// from a Blob (qr-scanner-worker.min.js: `new
// Worker(URL.createObjectURL(new Blob([...])))`), used on every browser with
// no usable native BarcodeDetector. worker-src has NO fallback of its own —
// it falls back to script-src, which is 'self' — so deleting this directive
// silently reverts the scanner to "camera opens, nothing ever decodes" and
// the door falls back to manual entry. Observed as:
//
//	Creating a worker from 'blob:…' violates the following Content Security
//	Policy directive: "script-src 'self'". Note that 'worker-src' was not
//	explicitly set, so 'script-src' is used as a fallback.
//
// MUTATION: delete the `worker-src 'self' blob:` line from
// contentSecurityPolicy and this fails, naming the script-src fallback.
// MUTATION: "fix" it instead by widening script-src to `'self' blob:` — the
// tempting wrong answer, which does make the worker load — and it still
// fails, because blob: belongs in worker-src alone. MUTATION: keep
// worker-src but drop blob: from it and this fails. MUTATION: add
// 'unsafe-inline'/'unsafe-eval' to script-src and this fails.
func TestCSP_AllowsBlobWorkerForGateScanner(t *testing.T) {
	h := newTestHarness(t)
	rec := h.do(http.MethodGet, "/api/events", "", nil)

	directives := parseCSP(t, rec.Header().Get("Content-Security-Policy"))

	workerSrc, ok := directives["worker-src"]
	if !ok {
		t.Fatalf("CSP has no worker-src directive, so it falls back to script-src (%q) and the gate scanner's blob: Worker is blocked; policy = %q",
			strings.Join(directives["script-src"], " "), rec.Header().Get("Content-Security-Policy"))
	}
	if !containsSource(workerSrc, "blob:") {
		t.Fatalf("worker-src = %q, want it to allow blob: — qr-scanner builds its decode Worker from a Blob URL", strings.Join(workerSrc, " "))
	}
	if !containsSource(workerSrc, "'self'") {
		t.Errorf("worker-src = %q, want 'self' kept alongside blob: so same-origin workers stay loadable", strings.Join(workerSrc, " "))
	}

	// The narrow fix must stay narrow: blob: belongs in worker-src only.
	if containsSource(directives["script-src"], "blob:") {
		t.Errorf("script-src = %q allows blob:; the scanner needs it in worker-src only, never for page-level script", strings.Join(directives["script-src"], " "))
	}
	for _, name := range []string{"script-src", "style-src", "default-src", "worker-src"} {
		for _, unsafe := range []string{"'unsafe-eval'", "'unsafe-hashes'"} {
			if containsSource(directives[name], unsafe) {
				t.Errorf("%s = %q contains %s", name, strings.Join(directives[name], " "), unsafe)
			}
		}
	}
	if containsSource(directives["script-src"], "'unsafe-inline'") {
		t.Errorf("script-src = %q contains 'unsafe-inline'; the build emits no inline script", strings.Join(directives["script-src"], " "))
	}
}

// parseCSP splits a policy into directive name -> source list. Names are
// ASCII case-insensitive per CSP3; source expressions are not.
func parseCSP(t *testing.T, policy string) map[string][]string {
	t.Helper()
	if policy == "" {
		t.Fatal("no Content-Security-Policy header on the response")
	}
	out := map[string][]string{}
	for _, part := range strings.Split(policy, ";") {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		out[strings.ToLower(fields[0])] = fields[1:]
	}
	return out
}

func containsSource(sources []string, want string) bool {
	for _, s := range sources {
		if s == want {
			return true
		}
	}
	return false
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
