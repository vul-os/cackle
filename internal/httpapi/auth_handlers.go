package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/store"
)

// userView is what a user looks like over the wire — never PasswordHash.
type userView struct {
	ID              string  `json:"id"`
	Email           string  `json:"email"`
	Name            string  `json:"name"`
	CreatedAt       string  `json:"created_at"`
	EmailVerifiedAt *string `json:"email_verified_at,omitempty"`
}

func toUserView(u *store.User) userView {
	v := userView{ID: u.ID, Email: u.Email, Name: u.Name, CreatedAt: nowText(u.CreatedAt)}
	if u.EmailVerifiedAt != nil {
		t := nowText(*u.EmailVerifiedAt)
		v.EmailVerifiedAt = &t
	}
	return v
}

type orgMembershipView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// handleSignup serves POST /api/auth/signup.
func (s *server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	user, err := s.deps.Auth.Signup(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailTaken):
			conflict(w, "an account with that email already exists")
		case errors.Is(err, auth.ErrInvalidEmail):
			badRequest(w, "invalid email address")
		case errors.Is(err, auth.ErrWeakPassword):
			badRequest(w, "password too short")
		default:
			internalError(w, s.log(), "signup", err)
		}
		return
	}

	s.issueSession(w, r, user)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleLogin serves POST /api/auth/login.
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	user, err := s.deps.Auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			unauthorized(w, "invalid email or password")
			return
		}
		internalError(w, s.log(), "login", err)
		return
	}

	s.issueSession(w, r, user)
}

// issueSession mints a session for user, sets the cookie pair, and writes
// {user, token} — the shared tail of signup and login.
func (s *server) issueSession(w http.ResponseWriter, r *http.Request, user *store.User) {
	token, expiresAt, err := s.deps.Auth.CreateSession(r.Context(), user.ID)
	if err != nil {
		internalError(w, s.log(), "create session", err)
		return
	}
	secure := isHTTPSRequest(r, false)
	setSessionCookies(w, token, expiresAt, s.deps.Config.SessionSecret, secure)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(user), "token": token})
}

// handleLogout serves POST /api/auth/logout. Idempotent: calling it
// without a valid (or any) session is not an error.
func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token, _ := sessionTokenFromContext(r.Context())
	if token != "" {
		if err := s.deps.Auth.Logout(r.Context(), token); err != nil {
			internalError(w, s.log(), "logout", err)
			return
		}
	}
	clearSessionCookies(w, isHTTPSRequest(r, false))
	w.WriteHeader(http.StatusNoContent)
}

// handleMe serves GET /api/auth/me (auth required).
func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())

	memberships, err := s.deps.Store.ListOrgsForUser(r.Context(), user.ID)
	if err != nil {
		internalError(w, s.log(), "list orgs for user", err)
		return
	}
	orgs := make([]orgMembershipView, 0, len(memberships))
	for _, m := range memberships {
		orgs = append(orgs, orgMembershipView{ID: m.ID, Name: m.Name, Role: m.Role})
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(user), "orgs": orgs})
}

type passwordResetRequest struct {
	Email string `json:"email"`
}

// handlePasswordReset serves POST /api/auth/password-reset. Never reveals
// whether the email exists — internal/auth already returns an empty token
// for an unknown address; the HTTP response is identical either way.
func (s *server) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	if _, err := s.deps.Auth.RequestPasswordReset(r.Context(), req.Email); err != nil {
		internalError(w, s.log(), "request password reset", err)
		return
	}
	// The token itself is delivered out-of-band (email) in a real
	// deployment; it is never echoed in this response.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type passwordUpdateRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// handlePasswordUpdate serves POST /api/auth/password-update.
func (s *server) handlePasswordUpdate(w http.ResponseWriter, r *http.Request) {
	var req passwordUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Token == "" || req.Password == "" {
		badRequest(w, "token and password are required")
		return
	}

	if err := s.deps.Auth.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		switch {
		case errors.Is(err, auth.ErrResetTokenInvalid):
			badRequest(w, "reset token invalid, expired, or already used")
		case errors.Is(err, auth.ErrWeakPassword):
			badRequest(w, "password too short")
		default:
			internalError(w, s.log(), "reset password", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
