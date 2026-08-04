// Password reset on a box that cannot send mail.
//
// Cackle has no mail code — no SMTP client, no provider SDK, no sender of
// any kind. The forgot-password page used to hide that behind a toast
// telling the user a message had gone out and to go and look for it, which
// is the worst possible failure: the person waits for something that is
// never coming and stays locked out of an account that could have been
// recovered in under a minute.
//
// A self-hosted single binary has an operator standing right there. They
// run `cackle reset-password`, which mints the same single-use, one-hour
// token the HTTP route mints, and prints a link. This module builds the
// exact command to hand them, with the address already filled in.

/** The subcommand an operator runs. Must match cmd/cackle/resetpassword.go. */
export const RESET_SUBCOMMAND = 'cackle reset-password';

const EMAIL_PLACEHOLDER = 'their@email.address';

/**
 * Quote an email for a POSIX shell.
 *
 * This is not paranoia about a hostile input — the string is typed by the
 * person who will paste it — it is about the command being CORRECT. An
 * address with a shell metacharacter in it (a `'`, a `$`, a space from a
 * fat-fingered paste) that is displayed unquoted produces a command that
 * either fails confusingly or, worse, runs something else. Wrap it in
 * single quotes and escape any single quote the POSIX way.
 */
function shellQuote(value: string): string {
    return `'${String(value).replace(/'/g, `'\\''`)}'`;
}

/**
 * Build the command to give whoever runs this Cackle server.
 *
 * A blank address yields a clearly-a-placeholder command rather than a
 * broken one, so the page reads sensibly before anything is typed.
 */
export function resetCommandFor(email: string | null | undefined): string {
    const address = typeof email === 'string' && email.trim() !== '' ? email.trim() : EMAIL_PLACEHOLDER;
    return `${RESET_SUBCOMMAND} -email ${shellQuote(address)}`;
}
