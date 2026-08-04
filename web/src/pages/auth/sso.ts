// Single sign-on: the small, pure half.
//
// Cackle ships with NO sign-in provider configured. `GET /api/auth/providers`
// answers `{"providers": []}` on a stock box, the OAuth routes are not even
// mounted, and this module turns that empty list into an empty button row.
//
// The rule the rest of this file exists to keep: NEVER RENDER A BUTTON THAT
// CANNOT WORK. A "Sign in with Google" button on a box whose operator never
// configured Google is a dead end that looks like a feature — the person
// presses it, something fails, and they conclude the product is broken rather
// than that a setting is missing.
//
// Nothing here holds an absolute URL. The button navigates to a path on this
// same box (`/api/auth/oauth/google/start`) and the server does the
// redirecting, so the shipped app still reaches out to nobody. That is what
// keeps `scripts/check-app.mjs`'s external-fetch rule true and, more to the
// point, keeps the promise the rule is about.

/**
 * Messages for the `?sso=` reason code the callback redirects back with.
 * Short, plain, and never blaming the person: each one says what happened
 * and what to do instead. Password sign-in is always the thing to do
 * instead, because it always works.
 */
export const SSO_MESSAGES = {
    denied: 'Sign-in was cancelled. You can try again, or sign in with your email and password.',
    state: 'That sign-in attempt took too long or was started somewhere else. Please try again.',
    unverified:
        'Your provider would not confirm that this email address is yours, so we did not sign you in. Sign in with your email and password instead.',
    failed: 'Sign-in did not finish. Please try again, or sign in with your email and password.',
};

type SsoReason = keyof typeof SSO_MESSAGES;

function isSsoReason(value: string): value is SsoReason {
    return value in SSO_MESSAGES;
}

/**
 * Turn a `?sso=` value into something to show. Returns null for `ok` (a
 * success needs no notice) and for anything unrecognised — an unknown code is
 * not an excuse to invent a message.
 */
export function ssoMessageFor(reason: string | null | undefined): string | null {
    if (!reason || reason === 'ok') return null;
    return isSsoReason(reason) ? SSO_MESSAGES[reason] : SSO_MESSAGES.failed;
}

/** A single entry from the raw `{providers: [...]}` payload, before validation. */
export interface SsoProviderPayloadEntry {
    id?: string;
    label?: string;
    start_path?: string;
}

/** The raw `GET /api/auth/providers` response shape. */
export interface SsoProvidersPayload {
    providers?: SsoProviderPayloadEntry[];
}

/** A provider entry that has been validated and is safe to render a button for. */
export interface SsoProvider {
    id: string;
    label: string;
    startPath: string;
}

/**
 * Normalise the `{providers: [...]}` payload into the list the button row
 * renders. Anything malformed collapses to an empty list: an entry with no
 * start path is a button that cannot work, so it is not a button.
 *
 * Fails closed by construction — every non-answer (a network error the caller
 * turned into null, an unconfigured box's empty array, a half-formed entry)
 * ends in the same place: no buttons, password form untouched.
 */
export function signInProvidersFrom(payload: SsoProvidersPayload | null | undefined): SsoProvider[] {
    const raw = payload?.providers;
    if (!Array.isArray(raw)) return [];
    return raw
        .filter(
            (p): p is SsoProviderPayloadEntry & { id: string; start_path: string } =>
                !!p && typeof p.id === 'string' && !!p.id && typeof p.start_path === 'string' && p.start_path.startsWith('/'),
        )
        .map((p) => ({
            id: p.id,
            label: typeof p.label === 'string' && p.label ? p.label : p.id,
            startPath: p.start_path,
        }));
}
