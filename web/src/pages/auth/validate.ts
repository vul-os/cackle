// Small, pure field-level checks shared by the three auth forms that submit
// something (sign-in, sign-up, update-password). Forgot-password has no
// submit to validate — it only ever builds a shell command locally.
//
// These exist so a person sees "enter your email" the instant they leave the
// field empty and tab to Sign In, rather than after a round trip to the
// server tells them the same thing a moment later. They deliberately do NOT
// replace server-side validation — the server is still the source of truth
// (see internal/auth/service.go's MinPasswordLength) — they just say the same
// thing sooner, in the same place the person is already looking.

/** A permissive shape check, not a validator for what an inbox will accept.
 * Its only job is to catch "obviously not an email" before a submit, not to
 * second-guess the server. */
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Matches internal/auth/service.go's MinPasswordLength. If that changes,
 * this must change with it — see operator-reset.test.ts for the pattern this
 * repo uses to pin a UI number to the Go source it has to agree with. */
export const MIN_PASSWORD_LENGTH = 8;

export function isEmailShaped(value: string | null | undefined): boolean {
    return EMAIL_SHAPE.test(String(value || '').trim());
}

export function emailError(value: string | null | undefined): string | null {
    const trimmed = String(value || '').trim();
    if (!trimmed) return 'Enter your email address.';
    if (!isEmailShaped(trimmed)) return "That doesn't look like a full email address.";
    return null;
}

export function requiredError(value: string | null | undefined, label: string): string | null {
    return String(value || '').trim() ? null : `Enter ${label}.`;
}

export function newPasswordError(value: string | null | undefined): string | null {
    if (!value) return 'Enter a password.';
    if (value.length < MIN_PASSWORD_LENGTH) return `Use at least ${MIN_PASSWORD_LENGTH} characters.`;
    return null;
}

export function confirmPasswordError(password: string | null | undefined, confirmation: string | null | undefined): string | null {
    if (!confirmation) return 'Type your password again.';
    if (password !== confirmation) return "Passwords don't match.";
    return null;
}
