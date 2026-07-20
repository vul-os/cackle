package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthFlow_SignupLoginMeLogout(t *testing.T) {
	h := newTestHarness(t)

	// Signup.
	rec := h.do(http.MethodPost, "/api/auth/signup", "", signupRequest{
		Email: "alice@example.com", Password: "correct-horse-battery", Name: "Alice",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("signup: status %d body %s", rec.Code, rec.Body.String())
	}
	signup := decodeBody[struct {
		User  userView `json:"user"`
		Token string   `json:"token"`
	}](t, rec)
	if signup.Token == "" {
		t.Fatal("signup: expected a non-empty token")
	}
	if signup.User.Email != "alice@example.com" {
		t.Fatalf("signup: got email %q", signup.User.Email)
	}

	// Duplicate signup is rejected.
	rec = h.do(http.MethodPost, "/api/auth/signup", "", signupRequest{
		Email: "alice@example.com", Password: "another-password", Name: "Alice 2",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate signup: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}

	// GET /api/auth/me without a token is unauthorized.
	rec = h.do(http.MethodGet, "/api/auth/me", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without token: expected 401, got %d", rec.Code)
	}
	assertErrorShape(t, rec)

	// GET /api/auth/me with the signup token works.
	rec = h.do(http.MethodGet, "/api/auth/me", signup.Token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: status %d body %s", rec.Code, rec.Body.String())
	}
	me := decodeBody[struct {
		User userView            `json:"user"`
		Orgs []orgMembershipView `json:"orgs"`
	}](t, rec)
	if me.User.Email != "alice@example.com" {
		t.Fatalf("me: got email %q", me.User.Email)
	}
	if len(me.Orgs) != 0 {
		t.Fatalf("me: expected no org memberships yet, got %d", len(me.Orgs))
	}

	// Wrong password is rejected without revealing which part was wrong.
	rec = h.do(http.MethodPost, "/api/auth/login", "", loginRequest{Email: "alice@example.com", Password: "wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: expected 401, got %d", rec.Code)
	}

	// Correct login works and issues a fresh token.
	rec = h.do(http.MethodPost, "/api/auth/login", "", loginRequest{Email: "alice@example.com", Password: "correct-horse-battery"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status %d body %s", rec.Code, rec.Body.String())
	}
	login := decodeBody[struct {
		Token string `json:"token"`
	}](t, rec)

	// Logout revokes the session; a subsequent /me with the same token is
	// unauthorized again.
	rec = h.do(http.MethodPost, "/api/auth/logout", login.Token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", rec.Code)
	}
	rec = h.do(http.MethodGet, "/api/auth/me", login.Token, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout: expected 401, got %d", rec.Code)
	}

	// Logout is idempotent even with no session at all.
	rec = h.do(http.MethodPost, "/api/auth/logout", "", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("anonymous logout: expected 204, got %d", rec.Code)
	}
}

func TestAuthSignup_WeakPasswordAndBadEmailRejected(t *testing.T) {
	h := newTestHarness(t)

	rec := h.do(http.MethodPost, "/api/auth/signup", "", signupRequest{Email: "short@example.com", Password: "abc", Name: "Short"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)

	rec = h.do(http.MethodPost, "/api/auth/signup", "", signupRequest{Email: "not-an-email", Password: "longenoughpassword", Name: "Bad"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad email: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

// assertErrorShape checks the response body matches {"error":{"code":...,
// "message":...}}, per the house error contract, and that neither field is
// empty.
func assertErrorShape(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	env := decodeBody[errorEnvelope](t, rec)
	if env.Error.Code == "" || env.Error.Message == "" {
		t.Fatalf("expected {error:{code,message}} shape, got %s", rec.Body.String())
	}
}
