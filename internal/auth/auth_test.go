package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cackle.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestHashAndVerifyPassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{"simple", "correct horse battery staple"},
		{"short-ish", "p4ssword"},
		{"unicode", "pâsswördé🔒"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := HashPassword(tc.password)
			if err != nil {
				t.Fatalf("HashPassword: %v", err)
			}
			if hash == tc.password {
				t.Fatal("hash must not equal plaintext")
			}
			ok, err := VerifyPassword(hash, tc.password)
			if err != nil {
				t.Fatalf("VerifyPassword: %v", err)
			}
			if !ok {
				t.Fatal("expected password to verify")
			}
			ok, err = VerifyPassword(hash, tc.password+"x")
			if err != nil {
				t.Fatalf("VerifyPassword (wrong): %v", err)
			}
			if ok {
				t.Fatal("expected wrong password to fail verification")
			}
		})
	}
}

func TestHashPasswordUniqueSaltPerCall(t *testing.T) {
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("hash 1: %v", err)
	}
	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("hash 2: %v", err)
	}
	if h1 == h2 {
		t.Fatal("expected different hashes for same password (random salt)")
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=1,p=4$not base64!$not base64!",
		"$bcrypt$v=1$abc$def",
	}
	for _, c := range cases {
		if _, err := VerifyPassword(c, "whatever"); err == nil {
			t.Fatalf("expected error verifying malformed hash %q", c)
		}
	}
}

func TestSignupAndLogin(t *testing.T) {
	st := newTestStore(t)
	svc := NewService(st)
	ctx := context.Background()

	u, err := svc.Signup(ctx, "Alice@Example.com", "hunter22", "Alice")
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email not normalized: got %q", u.Email)
	}

	// Duplicate signup fails.
	if _, err := svc.Signup(ctx, "alice@example.com", "otherpassword", "Alice2"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}

	// Weak password rejected.
	if _, err := svc.Signup(ctx, "bob@example.com", "short", "Bob"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}

	// Correct login.
	got, err := svc.Login(ctx, "ALICE@example.com", "hunter22")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("login returned different user: %s vs %s", got.ID, u.ID)
	}

	// Wrong password.
	if _, err := svc.Login(ctx, "alice@example.com", "wrongpassword"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// Nonexistent user gives the same error (no enumeration).
	if _, err := svc.Login(ctx, "nobody@example.com", "hunter22"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for unknown user, got %v", err)
	}
}

func TestSessionLifecycleAndExpiry(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	now := time.Now()
	clock := func() time.Time { return now }
	svc := NewService(st, WithClock(clock), WithSessionTTL(time.Hour))

	u, err := svc.Signup(ctx, "carol@example.com", "hunter22", "Carol")
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}

	token, expiresAt, err := svc.CreateSession(ctx, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if !expiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("expiresAt = %v, want %v", expiresAt, now.Add(time.Hour))
	}

	gotUser, gotSess, err := svc.ValidateSession(ctx, token)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if gotUser.ID != u.ID {
		t.Fatalf("validated user mismatch: %s vs %s", gotUser.ID, u.ID)
	}
	if gotSess.TokenHash == token {
		t.Fatal("session record must store a hash, not the plaintext token")
	}

	// Garbage/unknown token.
	if _, _, err := svc.ValidateSession(ctx, "not-a-real-token"); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expected ErrSessionInvalid, got %v", err)
	}

	// Advance the clock past expiry.
	now = now.Add(2 * time.Hour)
	if _, _, err := svc.ValidateSession(ctx, token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expected ErrSessionInvalid after expiry, got %v", err)
	}

	// Roll clock back, logout, confirm revoked.
	now = now.Add(-2 * time.Hour)
	if _, _, err := svc.ValidateSession(ctx, token); err != nil {
		t.Fatalf("session should be valid again once clock rewound: %v", err)
	}
	if err := svc.Logout(ctx, token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, _, err := svc.ValidateSession(ctx, token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expected ErrSessionInvalid after logout, got %v", err)
	}

	// Logout is idempotent.
	if err := svc.Logout(ctx, token); err != nil {
		t.Fatalf("Logout (idempotent): %v", err)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	svc := NewService(st, WithClock(func() time.Time { return now }), WithPasswordResetTTL(time.Hour))

	u, err := svc.Signup(ctx, "dave@example.com", "oldpassword", "Dave")
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	sessionToken, _, err := svc.CreateSession(ctx, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Unknown email doesn't error and doesn't leak existence via a token.
	tok, err := svc.RequestPasswordReset(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset (unknown): %v", err)
	}
	if tok != "" {
		t.Fatal("expected empty token for unknown email")
	}

	resetToken, err := svc.RequestPasswordReset(ctx, "dave@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if resetToken == "" {
		t.Fatal("expected a reset token for a real user")
	}

	if err := svc.ResetPassword(ctx, resetToken, "newpassword1"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// Old password no longer works, new one does.
	if _, err := svc.Login(ctx, "dave@example.com", "oldpassword"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected old password to fail, got %v", err)
	}
	if _, err := svc.Login(ctx, "dave@example.com", "newpassword1"); err != nil {
		t.Fatalf("expected new password to work: %v", err)
	}

	// Resetting revokes existing sessions.
	if _, _, err := svc.ValidateSession(ctx, sessionToken); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expected prior session revoked after reset, got %v", err)
	}

	// Token is single-use.
	if err := svc.ResetPassword(ctx, resetToken, "anotherpassword2"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("expected ErrResetTokenInvalid on reuse, got %v", err)
	}

	// Expired token rejected.
	tok2, err := svc.RequestPasswordReset(ctx, "dave@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset 2: %v", err)
	}
	now = now.Add(2 * time.Hour)
	if err := svc.ResetPassword(ctx, tok2, "yetanother3"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("expected ErrResetTokenInvalid on expired token, got %v", err)
	}
}

func TestCanManageEventRBAC(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := NewService(st)

	owner, err := svc.Signup(ctx, "owner@example.com", "hunter22", "Owner")
	if err != nil {
		t.Fatalf("signup owner: %v", err)
	}
	scanner, err := svc.Signup(ctx, "scanner@example.com", "hunter22", "Scanner")
	if err != nil {
		t.Fatalf("signup scanner: %v", err)
	}
	outsider, err := svc.Signup(ctx, "outsider@example.com", "hunter22", "Outsider")
	if err != nil {
		t.Fatalf("signup outsider: %v", err)
	}

	org := &store.Org{Name: "Acme", Slug: "acme"}
	if err := st.CreateOrg(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := st.AddOrgMember(ctx, &store.OrgMember{OrgID: org.ID, UserID: owner.ID, Role: "owner"}); err != nil {
		t.Fatalf("add owner member: %v", err)
	}
	if err := st.AddOrgMember(ctx, &store.OrgMember{OrgID: org.ID, UserID: scanner.ID, Role: "scanner"}); err != nil {
		t.Fatalf("add scanner member: %v", err)
	}

	eventID := store.NewID()
	if _, err := st.DB().ExecContext(ctx, `
		INSERT INTO events (id, org_id, slug, title, starts_at, ends_at, created_at, updated_at)
		VALUES (?, ?, 'test-event', 'Test Event', '2026-08-01T00:00:00Z', '2026-08-01T04:00:00Z', '2026-07-20T00:00:00Z', '2026-07-20T00:00:00Z')`,
		eventID, org.ID); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	// Owner can manage (admin-level action).
	ok, err := svc.CanManageEvent(ctx, owner.ID, eventID, RoleAdmin)
	if err != nil {
		t.Fatalf("CanManageEvent(owner): %v", err)
	}
	if !ok {
		t.Fatal("expected owner to be able to manage event")
	}

	// Scanner cannot perform an admin-level action — RBAC denial.
	ok, err = svc.CanManageEvent(ctx, scanner.ID, eventID, RoleAdmin)
	if err != nil {
		t.Fatalf("CanManageEvent(scanner, admin): %v", err)
	}
	if ok {
		t.Fatal("expected scanner to be denied admin-level event management")
	}

	// Scanner CAN meet the scanner-level bar (e.g. for scanning admissions).
	ok, err = svc.CanManageEvent(ctx, scanner.ID, eventID, RoleScanner)
	if err != nil {
		t.Fatalf("CanManageEvent(scanner, scanner): %v", err)
	}
	if !ok {
		t.Fatal("expected scanner to meet the scanner-level bar")
	}

	// Outsider (no membership at all) is denied, no error.
	ok, err = svc.CanManageEvent(ctx, outsider.ID, eventID, RoleScanner)
	if err != nil {
		t.Fatalf("CanManageEvent(outsider): %v", err)
	}
	if ok {
		t.Fatal("expected outsider with no membership to be denied")
	}

	// Nonexistent event is denied, no error (fails closed, not a crash).
	ok, err = svc.CanManageEvent(ctx, owner.ID, "nonexistent-event", RoleScanner)
	if err != nil {
		t.Fatalf("CanManageEvent(nonexistent event): %v", err)
	}
	if ok {
		t.Fatal("expected nonexistent event to deny access")
	}
}

func TestRoleMeets(t *testing.T) {
	cases := []struct {
		actual, min Role
		want        bool
	}{
		{RoleOwner, RoleScanner, true},
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleOwner, true},
		{RoleAdmin, RoleOwner, false},
		{RoleAdmin, RoleAdmin, true},
		{RoleScanner, RoleAdmin, false},
		{Role("bogus"), RoleScanner, false},
		{RoleOwner, Role("bogus"), false},
	}
	for _, tc := range cases {
		if got := RoleMeets(tc.actual, tc.min); got != tc.want {
			t.Errorf("RoleMeets(%q, %q) = %v, want %v", tc.actual, tc.min, got, tc.want)
		}
	}
}

func TestOAuthStubLoginCreatesAndLinksUser(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := NewService(st)

	provider := NewStubOAuthProvider("google", OAuthUserInfo{
		Subject: "google-subject-123",
		Email:   "oauth@example.com",
		Name:    "OAuth User",
	})

	info, err := provider.Exchange(ctx, "any-code", "https://cackle.example/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	u1, err := svc.LoginWithOAuth(ctx, info)
	if err != nil {
		t.Fatalf("LoginWithOAuth (create): %v", err)
	}
	if u1.Email != "oauth@example.com" {
		t.Fatalf("unexpected email: %q", u1.Email)
	}

	// Second login with the same identity resolves to the same user.
	u2, err := svc.LoginWithOAuth(ctx, info)
	if err != nil {
		t.Fatalf("LoginWithOAuth (repeat): %v", err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("expected same user on repeat oauth login: %s vs %s", u2.ID, u1.ID)
	}

	// A native account with the same email, then a *different* provider
	// subject with that email, links to the existing account rather than
	// creating a duplicate.
	native, err := svc.Signup(ctx, "linked@example.com", "hunter22", "Linked")
	if err != nil {
		t.Fatalf("signup native: %v", err)
	}
	linkedInfo := OAuthUserInfo{Provider: "github", Subject: "gh-456", Email: "linked@example.com", Name: "Linked"}
	linkedUser, err := svc.LoginWithOAuth(ctx, linkedInfo)
	if err != nil {
		t.Fatalf("LoginWithOAuth (link): %v", err)
	}
	if linkedUser.ID != native.ID {
		t.Fatalf("expected oauth login to link to existing native account, got different user")
	}
}

func TestAuthURLIsStubOnly(t *testing.T) {
	provider := NewStubOAuthProvider("google", OAuthUserInfo{Subject: "s", Email: "e@example.com"})
	url := provider.AuthURL("state123", "https://cackle.example/callback")
	if url == "" {
		t.Fatal("expected non-empty auth URL")
	}
}
