// The invite link, and why it is a link rather than an email.
//
// Cackle has no mail code. Not "mail is misconfigured" — there is no SMTP
// client, no provider SDK, no sender of any kind in the repository. For a
// while the team page pretended otherwise: it created an invite, threw the
// only copy of the token away, and toasted "Invite sent". Nothing was sent,
// nobody could accept, and so nobody could be added as a `scanner` — which
// means nobody could staff a door, which is the entire product.
//
// The fix is the same one the product already made for money. Cackle's
// default payment provider is `manual`: no API key, no compliance surface,
// works in every country, because the operator is standing right there.
// Invites work the same way. The owner gets the link and sends it however
// they already talk to their staff — WhatsApp, SMS, or reading it out over
// the bar. Zero configuration, no third party, and it works on a venue
// laptop with no internet beyond the local network.
//
// This module is the pure half of that, kept out of the component so it can
// be tested without a DOM.

/** The client-side route that redeems an invite. Must match routes.tsx. */
export const INVITE_ACCEPT_PATH = '/accept-invite';

/** The query parameter AcceptInvitePage reads the token from. */
export const INVITE_TOKEN_PARAM = 'token';

/**
 * Build the URL an invited person opens.
 *
 * @param {string} origin - normally `window.location.origin`. Whatever host
 *   the owner is using is the host their staff can reach: a venue laptop on
 *   `http://192.168.1.20:8080` produces a LAN link that works at the door,
 *   which a server-side CACKLE_BASE_URL would get wrong.
 * @param {string} token - the plaintext invite token, returned exactly once
 *   by `POST /api/orgs/{id}/invites`.
 * @returns {string} an absolute URL.
 *
 * Throws on a blank token rather than returning a half-formed URL. A link
 * with no token renders "Missing invite link" for the recipient, and the
 * owner has no way to tell that from a working one by looking at it — so
 * fail here, loudly, while someone is still watching.
 */
export function buildInviteUrl(origin, token) {
    if (typeof token !== 'string' || token.trim() === '') {
        throw new TypeError('buildInviteUrl: refusing to build an invite link with no token');
    }
    if (typeof origin !== 'string' || origin.trim() === '') {
        throw new TypeError('buildInviteUrl: an invite link needs an origin');
    }
    const base = origin.trim().replace(/\/+$/, '');
    // encodeURIComponent is not decoration: today's tokens are base64url
    // (`auth.NewOpaqueToken`) and pass through untouched, but a future token
    // alphabet containing `+`, `/` or `&` would otherwise produce a link
    // that looks right and silently decodes to the wrong value.
    return `${base}${INVITE_ACCEPT_PATH}?${INVITE_TOKEN_PARAM}=${encodeURIComponent(token)}`;
}

/**
 * Copy text to the clipboard, returning true only if it actually landed.
 *
 * The async Clipboard API needs a secure context, and a self-hosted Cackle
 * on a venue LAN is very often plain `http://` — exactly the deployment this
 * product is built for. So the failure path matters more than usual: fall
 * back to the legacy selection copy, and if that fails too, say so instead
 * of claiming success. The link is always displayed and selectable, so a
 * failed copy is an inconvenience, never a dead end.
 */
export async function copyTextToClipboard(text, doc = globalThis.document, nav = globalThis.navigator) {
    if (typeof text !== 'string' || text === '') return false;

    if (nav?.clipboard?.writeText) {
        try {
            await nav.clipboard.writeText(text);
            return true;
        } catch {
            // fall through to the legacy path
        }
    }

    if (!doc?.createElement || !doc?.execCommand) return false;
    try {
        const scratch = doc.createElement('textarea');
        scratch.value = text;
        scratch.setAttribute('readonly', '');
        scratch.style.position = 'fixed';
        scratch.style.opacity = '0';
        doc.body.appendChild(scratch);
        scratch.select();
        const ok = doc.execCommand('copy');
        doc.body.removeChild(scratch);
        return ok === true;
    } catch {
        return false;
    }
}
