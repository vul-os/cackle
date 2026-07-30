package main

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/store"
)

// The bar for these tests: they must fail if `cackle reset-password`
// stops being a real way to get back into an account. Printing a
// plausible-looking URL is not the feature — the feature is that the
// person holding that URL can set a password and sign in with it. So the
// happy-path test redeems the token it printed and then logs in.
//
// This matters more than usual here because the thing being replaced was
// a UI that CLAIMED an email had been sent. Anything that only checks
// "some output appeared" would have passed for that too.

func isolateCackleEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"CACKLE_ADDR", "CACKLE_DB", "CACKLE_BASE_URL",
		"CACKLE_SESSION_SECRET", "CACKLE_MEDIA_DIR", "CACKLE_DEMO",
		"CACKLE_PAYSTACK_SECRET_KEY",
	} {
		t.Setenv(k, "")
	}
}

// newAccountDB creates a database with one real account in it and returns
// its path.
func newAccountDB(t *testing.T, email, password string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cackle.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if _, err := auth.NewService(st).Signup(t.Context(), email, password, "Test Person"); err != nil {
		t.Fatalf("signup: %v", err)
	}
	return dbPath
}

var linkPattern = regexp.MustCompile(`https?://\S+/update-password\?token=\S+`)

func TestResetPasswordCmd_PrintsALinkThatActuallyResetsThePassword(t *testing.T) {
	isolateCackleEnv(t)
	const email, oldPassword, newPassword = "locked-out@example.com", "the-old-password-1", "the-new-password-2"
	dbPath := newAccountDB(t, email, oldPassword)

	var stdout, stderr bytes.Buffer
	if err := resetPasswordCmd([]string{"-email", email, "-db", dbPath, "-base-url", "http://gate.example.org:8080"}, &stdout, &stderr); err != nil {
		t.Fatalf("reset-password: %v (stderr %s)", err, stderr.String())
	}

	out := stdout.String()
	link := linkPattern.FindString(out)
	if link == "" {
		t.Fatalf("no /update-password link in the output:\n%s", out)
	}
	if !strings.HasPrefix(link, "http://gate.example.org:8080/update-password?token=") {
		t.Fatalf("link does not use the base URL it was given: %s", link)
	}
	// The copy must not reintroduce the lie this replaced.
	for _, forbidden := range []string{"inbox", "sent you", "email sent", "emailed"} {
		if strings.Contains(strings.ToLower(out), forbidden) {
			t.Fatalf("output implies a message was delivered (%q):\n%s", forbidden, out)
		}
	}
	if !strings.Contains(out, "does not send email") {
		t.Fatalf("output does not say Cackle sends no email, so an operator could still assume it did:\n%s", out)
	}

	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	token := u.Query().Get("token")
	if token == "" {
		t.Fatalf("link carries no token: %s", link)
	}

	// Redeem it exactly the way /update-password does, then prove the
	// account is genuinely recovered.
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st.Close()
	svc := auth.NewService(st)

	if err := svc.ResetPassword(t.Context(), token, newPassword); err != nil {
		t.Fatalf("reset password with the printed token: %v", err)
	}
	if _, err := svc.Login(t.Context(), email, newPassword); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	if _, err := svc.Login(t.Context(), email, oldPassword); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("the old password still works after a reset: %v", err)
	}

	// Single-use: the printed link cannot be replayed. An operator pastes
	// these into chat apps; a reusable one would be a standing key.
	if err := svc.ResetPassword(t.Context(), token, "yet-another-password-3"); !errors.Is(err, auth.ErrResetTokenInvalid) {
		t.Fatalf("a spent reset token was accepted again: %v", err)
	}
}

func TestResetPasswordCmd_RefusesAnAddressWithNoAccount(t *testing.T) {
	isolateCackleEnv(t)
	dbPath := newAccountDB(t, "real@example.com", "the-real-password-1")

	var stdout, stderr bytes.Buffer
	err := resetPasswordCmd([]string{"-email", "typo@example.com", "-db", dbPath}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected an error for an address with no account")
	}
	if !strings.Contains(err.Error(), "no account") {
		t.Fatalf("unhelpful error for an operator who mistyped: %v", err)
	}
	// Crucially: no link. An operator who is told "here is your link" for
	// a typo'd address will send it, and the recipient will be stuck.
	if linkPattern.MatchString(stdout.String()) {
		t.Fatalf("printed a link for an account that does not exist:\n%s", stdout.String())
	}
}

func TestResetPasswordCmd_RequiresAnEmail(t *testing.T) {
	isolateCackleEnv(t)
	var stdout, stderr bytes.Buffer
	if err := resetPasswordCmd(nil, &stdout, &stderr); err == nil {
		t.Fatal("expected an error when -email is omitted")
	}
	if !strings.Contains(stderr.String(), "-email") {
		t.Fatalf("usage did not name the missing flag:\n%s", stderr.String())
	}
}

// TestRun_SubcommandDispatchDoesNotShadowFlags is the regression guard for
// the dispatch itself: adding subcommands must not change what a bare
// `cackle -flag ...` does. Every existing deployment invokes this binary
// with flags only.
func TestRun_SubcommandDispatchDoesNotShadowFlags(t *testing.T) {
	isolateCackleEnv(t)

	// -version still short-circuits, exactly as before.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(stdoutR)
		done <- buf.String()
	}()
	if err := run([]string{"-version"}, stdoutW, os.Stderr); err != nil {
		t.Fatalf("run -version: %v", err)
	}
	stdoutW.Close()
	if got := <-done; !strings.HasPrefix(got, "cackle ") {
		t.Fatalf("run -version printed %q", got)
	}

	// An unknown subcommand is refused by name rather than silently
	// treated as a flag parse (which would boot a server).
	err = run([]string{"definitely-not-a-subcommand"}, os.Stdout, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("expected an unknown-subcommand error, got %v", err)
	}
	if !strings.Contains(err.Error(), "reset-password") {
		t.Fatalf("the error should list what IS available, got %v", err)
	}
}

// TestMintPasswordResetToken_TellsTheOperatorTheTruthAndTheWebNothing pins
// the asymmetry the two entry points deliberately have: the operator at
// the console is told an address is unknown, and the public HTTP route
// must never be able to tell. Losing either half is a bug — the first
// makes the CLI useless, the second turns POST /api/auth/password-reset
// into an account-enumeration oracle.
func TestMintPasswordResetToken_TellsTheOperatorTheTruthAndTheWebNothing(t *testing.T) {
	isolateCackleEnv(t)
	dbPath := newAccountDB(t, "known@example.com", "a-known-password-1")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	svc := auth.NewService(st)

	if _, _, err := svc.MintPasswordResetToken(t.Context(), "unknown@example.com"); !errors.Is(err, auth.ErrNoSuchUser) {
		t.Fatalf("operator path: expected ErrNoSuchUser for an unknown address, got %v", err)
	}
	token, expiresAt, err := svc.MintPasswordResetToken(t.Context(), "known@example.com")
	if err != nil {
		t.Fatalf("operator path: %v", err)
	}
	if token == "" || expiresAt.IsZero() {
		t.Fatalf("operator path returned token=%q expiresAt=%v", token, expiresAt)
	}

	// The web path stays silent, and silent in the SAME way for both.
	unknownTok, err := svc.RequestPasswordReset(t.Context(), "unknown@example.com")
	if err != nil || unknownTok != "" {
		t.Fatalf("web path leaked something for an unknown address: token=%q err=%v", unknownTok, err)
	}
	knownTok, err := svc.RequestPasswordReset(t.Context(), "known@example.com")
	if err != nil {
		t.Fatalf("web path for a known address: %v", err)
	}
	if knownTok == "" {
		t.Fatal("web path minted no token for a real account")
	}
	// A malformed address must also be indistinguishable over HTTP.
	badTok, err := svc.RequestPasswordReset(t.Context(), "not-an-email")
	if err != nil || badTok != "" {
		t.Fatalf("web path leaked something for a malformed address: token=%q err=%v", badTok, err)
	}
}
