// lib/capability.js
//
// Browser-side mirror of internal/tickets/capability.go's Verify(). This is
// the piece that makes offline gate scanning possible: it is PURE (no
// network, no IndexedDB, no wall-clock access beyond an injected `now`) so it
// can run entirely inside the scanner page with the device unplugged from
// the network.
//
// Wire format: cackle.<base64url(payload_json)>.<base64url(ed25519_sig)>
// The signature covers the raw decoded payload bytes, verified with the
// event's Ed25519 public key pinned ahead of time from the scan bundle.

import { ed25519 } from '@noble/curves/ed25519';

export const CURRENT_VERSION = 1;
const TOKEN_PREFIX = 'cackle';

export const CapabilityErrorCode = {
    MALFORMED: 'malformed',
    UNSUPPORTED_VERSION: 'unsupported_version',
    BAD_SIGNATURE: 'bad_signature',
    NOT_YET_VALID: 'not_yet_valid',
    EXPIRED: 'expired',
    UNKNOWN_KID: 'unknown_kid',
};

export class CapabilityError extends Error {
    constructor(code, message) {
        super(message);
        this.name = 'CapabilityError';
        this.code = code;
    }
}

function base64UrlToBytes(b64url) {
    let base64 = b64url.replace(/-/g, '+').replace(/_/g, '/');
    const pad = base64.length % 4;
    if (pad === 2) base64 += '==';
    else if (pad === 3) base64 += '=';
    else if (pad !== 0) throw new CapabilityError(CapabilityErrorCode.MALFORMED, 'invalid base64url length');

    const binary = atob(base64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
}

function bytesToUtf8(bytes) {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
}

function hexToBytes(hex) {
    const clean = hex.trim();
    const bytes = new Uint8Array(clean.length / 2);
    for (let i = 0; i < bytes.length; i++) {
        bytes[i] = parseInt(clean.substr(i * 2, 2), 16);
    }
    return bytes;
}

/**
 * Normalises a pinned public key (as delivered in a scan bundle's
 * `issuer_keys.keys[kid]`, a map of key id to base64url-encoded public key —
 * see tickets.KeyRing.MarshalJSON on the backend) into raw bytes. Also
 * tolerates hex or standard base64 in case that encoding ever changes.
 */
export function publicKeyToBytes(encoded) {
    if (encoded instanceof Uint8Array) return encoded;
    if (typeof encoded !== 'string') {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, 'public key must be a string or byte array');
    }
    // hex is exactly 64 chars for a 32-byte Ed25519 key and only contains hex digits
    if (/^[0-9a-fA-F]{64}$/.test(encoded)) {
        return hexToBytes(encoded);
    }
    try {
        return base64UrlToBytes(encoded.replace(/=+$/, ''));
    } catch {
        // fall through to standard base64 (with padding, +/ alphabet)
    }
    const binary = atob(encoded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
}

/**
 * Verify a capability token against a pinned Ed25519 public key.
 *
 * @param {string} token
 * @param {Uint8Array} publicKey - 32-byte Ed25519 public key
 * @param {Date} [now] - defaults to `new Date()`; pass explicitly in tests
 * @returns {{v:number,tid:string,eid:string,tt:string,kid:string,sub:string,nm:string,iat:number,nbf?:number,exp?:number,seat?:string}}
 */
export function verifyCapability(token, publicKey, now = new Date()) {
    if (typeof token !== 'string') {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, 'token must be a string');
    }
    const parts = token.split('.');
    if (parts.length !== 3) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, `expected 3 dot-separated segments, got ${parts.length}`);
    }
    const [prefix, encPayload, encSig] = parts;
    if (prefix !== TOKEN_PREFIX) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, `bad prefix "${prefix}"`);
    }

    let body;
    let sig;
    try {
        body = base64UrlToBytes(encPayload);
        sig = base64UrlToBytes(encSig);
    } catch (err) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, `bad base64: ${err.message}`);
    }
    if (sig.length !== 64) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, `invalid signature length ${sig.length}`);
    }
    if (publicKey.length !== 32) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, `invalid public key length ${publicKey.length}`);
    }

    let ok = false;
    try {
        ok = ed25519.verify(sig, body, publicKey);
    } catch {
        ok = false;
    }
    if (!ok) {
        throw new CapabilityError(CapabilityErrorCode.BAD_SIGNATURE, 'signature verification failed');
    }

    let payload;
    try {
        payload = JSON.parse(bytesToUtf8(body));
    } catch (err) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, `bad payload json: ${err.message}`);
    }

    if (payload.v !== CURRENT_VERSION) {
        throw new CapabilityError(CapabilityErrorCode.UNSUPPORTED_VERSION, `got version ${payload.v}, want ${CURRENT_VERSION}`);
    }

    const nowUnix = Math.floor(now.getTime() / 1000);
    if (payload.nbf && nowUnix < payload.nbf) {
        throw new CapabilityError(CapabilityErrorCode.NOT_YET_VALID, 'ticket not yet valid');
    }
    if (payload.exp && nowUnix >= payload.exp) {
        throw new CapabilityError(CapabilityErrorCode.EXPIRED, 'ticket expired');
    }

    return payload;
}

/** Read the `kid` field without verifying the signature — used only to pick
 * which pinned key to verify against, exactly like reading an unverified JWT
 * header. Never trust this for anything beyond a map lookup. */
export function peekKid(token) {
    const parts = typeof token === 'string' ? token.split('.') : [];
    if (parts.length !== 3 || parts[0] !== TOKEN_PREFIX) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, 'malformed token');
    }
    try {
        const body = base64UrlToBytes(parts[1]);
        const partial = JSON.parse(bytesToUtf8(body));
        return partial.kid;
    } catch (err) {
        throw new CapabilityError(CapabilityErrorCode.MALFORMED, `bad payload json: ${err.message}`);
    }
}

/**
 * Verify a token against a KeyRing (kid -> public key bytes), mirroring
 * Go's VerifyWithRing.
 */
export function verifyWithRing(token, keyRing, now = new Date()) {
    const kid = peekKid(token);
    const pub = keyRing[kid];
    if (!pub) {
        throw new CapabilityError(CapabilityErrorCode.UNKNOWN_KID, `unknown key id "${kid}"`);
    }
    return verifyCapability(token, pub, now);
}
