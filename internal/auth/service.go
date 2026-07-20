// Package auth implements Cackle's account system: signup/login/logout,
// server-side sessions, password reset, RBAC helpers, and an OAuth provider
// seam. It is the only package that hashes or verifies passwords, and the
// only package that mints or checks session/reset tokens.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vul-os/cackle/internal/store"
)

// Sentinel errors. Handlers in internal/httpapi should map these to the
// documented {"error":{"code":...}} shapes; none of them ever leak whether
// an email exists when that would be a privacy/enumeration issue (see
// RequestPasswordReset).
var (
	ErrInvalidCredentials = errors.New("auth: invalid email or password")
	ErrEmailTaken         = errors.New("auth: email already registered")
	ErrInvalidEmail       = errors.New("auth: invalid email")
	ErrWeakPassword       = errors.New("auth: password too short")
	ErrSessionInvalid     = errors.New("auth: session invalid or expired")
	ErrResetTokenInvalid  = errors.New("auth: reset token invalid, expired, or already used")
)

const (
	// MinPasswordLength is the minimum accepted password length at signup
	// and reset time.
	MinPasswordLength = 8

	// DefaultSessionTTL is how long a freshly created session is valid.
	DefaultSessionTTL = 30 * 24 * time.Hour

	// DefaultPasswordResetTTL is how long a password reset token is valid.
	DefaultPasswordResetTTL = 1 * time.Hour
)

// Service is the entrypoint for all auth operations. It holds no state of
// its own beyond configuration — everything durable lives in the Store.
type Service struct {
	store            *store.Store
	sessionTTL       time.Duration
	passwordResetTTL time.Duration
	now              func() time.Time
}

// Option configures a Service.
type Option func(*Service)

// WithSessionTTL overrides DefaultSessionTTL.
func WithSessionTTL(d time.Duration) Option {
	return func(s *Service) { s.sessionTTL = d }
}

// WithPasswordResetTTL overrides DefaultPasswordResetTTL.
func WithPasswordResetTTL(d time.Duration) Option {
	return func(s *Service) { s.passwordResetTTL = d }
}

// WithClock overrides the time source (tests only).
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// NewService constructs an auth Service backed by st.
func NewService(st *store.Store, opts ...Option) *Service {
	s := &Service{
		store:            st,
		sessionTTL:       DefaultSessionTTL,
		passwordResetTTL: DefaultPasswordResetTTL,
		now:              time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// normalizeEmail lower-cases and trims an email for consistent lookups. It
// does not attempt full RFC 5322 validation — just enough to reject empty
// or obviously-malformed input.
func normalizeEmail(email string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" || !strings.Contains(e, "@") || strings.HasPrefix(e, "@") || strings.HasSuffix(e, "@") {
		return "", ErrInvalidEmail
	}
	return e, nil
}

// Signup creates a new user with a hashed password. Returns ErrEmailTaken if
// the email is already registered, ErrInvalidEmail/ErrWeakPassword for bad
// input.
func (s *Service) Signup(ctx context.Context, email, password, name string) (*store.User, error) {
	norm, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if len(password) < MinPasswordLength {
		return nil, ErrWeakPassword
	}

	if _, err := s.store.GetUserByEmail(ctx, norm); err == nil {
		return nil, ErrEmailTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("auth: signup lookup: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("auth: signup hash: %w", err)
	}

	u := &store.User{
		Email:        norm,
		PasswordHash: hash,
		Name:         strings.TrimSpace(name),
	}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("auth: signup create: %w", err)
	}
	return u, nil
}

// Login verifies email/password and returns the user. Returns
// ErrInvalidCredentials for any mismatch — it deliberately does not
// distinguish "no such user" from "wrong password" in the returned error,
// to avoid account enumeration.
func (s *Service) Login(ctx context.Context, email, password string) (*store.User, error) {
	norm, err := normalizeEmail(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	u, err := s.store.GetUserByEmail(ctx, norm)
	if errors.Is(err, store.ErrNotFound) {
		// Still do a (wasted) hash comparison so login timing doesn't
		// reveal whether the email exists.
		_, _ = VerifyPassword(dummyHash, password)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: login lookup: %w", err)
	}

	ok, err := VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

// dummyHash is a fixed argon2id hash used only to equalise login timing
// when no such user exists. It is not a secret and matches no real account.
const dummyHash = "$argon2id$v=19$m=65536,t=1,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// CreateSession mints a new session for userID and persists its hash. The
// returned token is the plaintext value to hand back to the client
// (cookie or bearer) — it is never recoverable from the store afterwards.
func (s *Service) CreateSession(ctx context.Context, userID string) (token string, expiresAt time.Time, err error) {
	plaintext, hash, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = s.now().Add(s.sessionTTL)

	sess := &store.Session{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: s.now(),
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: create session: %w", err)
	}
	return plaintext, expiresAt, nil
}

// ValidateSession resolves a plaintext session token to its user and
// session record. Returns ErrSessionInvalid if the token is unknown or
// expired.
func (s *Service) ValidateSession(ctx context.Context, token string) (*store.User, *store.Session, error) {
	if token == "" {
		return nil, nil, ErrSessionInvalid
	}

	sess, err := s.store.GetSessionByTokenHash(ctx, hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, nil, fmt.Errorf("auth: validate session: %w", err)
	}
	if !sess.ExpiresAt.After(s.now()) {
		return nil, nil, ErrSessionInvalid
	}

	u, err := s.store.GetUserByID(ctx, sess.UserID)
	if errors.Is(err, store.ErrNotFound) {
		// Session outlived its user (shouldn't happen: FK cascade deletes
		// sessions with the user, but fail closed regardless).
		return nil, nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, nil, fmt.Errorf("auth: validate session user lookup: %w", err)
	}
	return u, sess, nil
}

// Logout revokes a single session by its plaintext token. Idempotent.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, hashToken(token))
}

// LogoutAll revokes every session belonging to a user (e.g. "sign out
// everywhere", or forced on password change).
func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.store.DeleteSessionsForUser(ctx, userID)
}

// RequestPasswordReset issues a password reset token for the given email,
// if an account with that email exists. To avoid account enumeration, a
// nonexistent email is not an error: the returned token is simply empty.
// Callers (internal/httpapi) must not distinguish these cases in the HTTP
// response.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) (token string, err error) {
	norm, err := normalizeEmail(email)
	if err != nil {
		return "", nil
	}

	u, err := s.store.GetUserByEmail(ctx, norm)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("auth: password reset lookup: %w", err)
	}

	plaintext, hash, err := newToken()
	if err != nil {
		return "", err
	}
	rec := &store.PasswordResetToken{
		TokenHash: hash,
		UserID:    u.ID,
		ExpiresAt: s.now().Add(s.passwordResetTTL),
		CreatedAt: s.now(),
	}
	if err := s.store.CreatePasswordResetToken(ctx, rec); err != nil {
		return "", fmt.Errorf("auth: password reset create: %w", err)
	}
	return plaintext, nil
}

// ResetPassword consumes a password reset token and sets a new password.
// Tokens are single-use: a second call with the same token fails with
// ErrResetTokenInvalid. All existing sessions for the user are revoked.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < MinPasswordLength {
		return ErrWeakPassword
	}
	if token == "" {
		return ErrResetTokenInvalid
	}

	rec, err := s.store.GetPasswordResetToken(ctx, hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return ErrResetTokenInvalid
	}
	if err != nil {
		return fmt.Errorf("auth: reset lookup: %w", err)
	}
	if rec.UsedAt != nil || !rec.ExpiresAt.After(s.now()) {
		return ErrResetTokenInvalid
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("auth: reset hash: %w", err)
	}
	if err := s.store.UpdateUserPassword(ctx, rec.UserID, hash); err != nil {
		return fmt.Errorf("auth: reset update password: %w", err)
	}
	if err := s.store.MarkPasswordResetTokenUsed(ctx, rec.TokenHash, s.now()); err != nil {
		return fmt.Errorf("auth: reset mark used: %w", err)
	}
	if err := s.store.DeleteSessionsForUser(ctx, rec.UserID); err != nil {
		return fmt.Errorf("auth: reset revoke sessions: %w", err)
	}
	return nil
}

// Store exposes the underlying store, for callers (httpapi, demo seeding)
// that need direct access alongside the auth Service.
func (s *Service) Store() *store.Store { return s.store }
