package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/store"
)

// Password reset on a box with no mail server.
//
// Cackle has no mail code: no SMTP client, no provider SDK, no sender of
// any kind. For a long time the UI hid that behind "Reset email sent —
// check your inbox", which is the worst of both worlds: the user waits
// for something that is never coming, and ends up locked out of an
// account they could have recovered in thirty seconds.
//
// The honest answer for a self-hosted single binary is that the person
// who can fix it is standing right there. This subcommand mints the same
// single-use, one-hour reset token the HTTP route mints, and prints the
// link. The operator passes it to whoever asked — the same copy-a-link
// motion the team-invite flow uses, for the same reason.
//
// It prints a LINK rather than setting a password directly on purpose:
//
//   - the operator never learns, types, or leaves in their shell history
//     a password belonging to somebody else;
//   - the user picks their own, so it is not a shared secret from the
//     moment it exists;
//   - it reuses the reset machinery that already exists and is already
//     tested (single-use, expiring, revokes every session on use) rather
//     than adding a second, less careful way to change a password;
//   - and it makes /update-password reachable, which until now it was
//     not, by anybody, ever.

// resetPasswordCmd implements `cackle reset-password`.
func resetPasswordCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("cackle reset-password", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var email, dbPath, baseURL string
	fs.StringVar(&email, "email", "", "email address of the account to reset (required)")
	fs.StringVar(&dbPath, "db", "", "path to SQLite database file (env CACKLE_DB, default ./cackle.db)")
	fs.StringVar(&baseURL, "base-url", "", "externally-visible base URL the link should use (env CACKLE_BASE_URL)")
	fs.Usage = func() {
		fmt.Fprint(stderr, `usage: cackle reset-password -email <address> [-db <path>] [-base-url <url>]

Mints a single-use password reset link for an existing account and prints
it. Cackle sends no email — hand the link to the account holder yourself.
The link expires in one hour, works once, and signs that account out
everywhere when it is used.

`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(email) == "" {
		fs.Usage()
		return errors.New("reset-password: -email is required")
	}

	cfg, err := config.Load(config.Flags{DB: dbPath, BaseURL: baseURL})
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := store.Open(cfg.DB)
	if err != nil {
		return fmt.Errorf("open store %s: %w", cfg.DB, err)
	}
	defer st.Close()

	// No key vault is unlocked here, deliberately: resetting a password
	// touches no event signing key, and a subcommand that demanded the
	// vault would fail on a box whose keyfile lives somewhere the
	// operator's shell cannot see.
	token, expiresAt, err := auth.NewService(st).MintPasswordResetToken(context.Background(), email)
	switch {
	case errors.Is(err, auth.ErrNoSuchUser):
		return fmt.Errorf("no account on this server uses %s — check the address, or list accounts in the database", email)
	case errors.Is(err, auth.ErrInvalidEmail):
		return fmt.Errorf("%q is not a valid email address", email)
	case err != nil:
		return fmt.Errorf("mint reset token: %w", err)
	}

	link := strings.TrimRight(cfg.BaseURL, "/") + "/update-password?token=" + token

	fmt.Fprintf(stdout, "Password reset link for %s:\n\n  %s\n\n", email, link)
	fmt.Fprintf(stdout, "Give this link to them yourself — Cackle does not send email.\n")
	fmt.Fprintf(stdout, "It works once, expires at %s, and signs that account out everywhere when used.\n",
		expiresAt.Format("2006-01-02 15:04:05 MST"))
	if baseURL == "" {
		fmt.Fprintf(stdout, "\nThe link uses %s. If that is not the address they reach this server on,\nre-run with -base-url or set CACKLE_BASE_URL.\n", cfg.BaseURL)
	}
	return nil
}
