// lib/api.js
//
// Thin, typed fetch wrapper for Cackle's HTTP API (see BUILD-SPEC.md "HTTP API").
// Every backend error is shaped { "error": { "code", "message" } } — we surface
// that as an `ApiError` instance so callers can branch on `.code` when useful
// and otherwise just show `.message`.
//
// Auth: the backend accepts either `Authorization: Bearer <session>` or the
// httpOnly `cackle_session` cookie. We keep a bearer token in localStorage
// (so the offline gate scanner can attach it to background sync requests
// without depending on cookies) and also send `credentials: "include"` so the
// cookie works out of the box for normal browser navigation/dev.

const DEFAULT_BASE = '/api';

function resolveBaseUrl() {
    const configured = import.meta.env.VITE_API_URL;
    if (configured && configured.trim()) {
        return configured.replace(/\/+$/, '');
    }
    return DEFAULT_BASE;
}

export const API_BASE_URL = resolveBaseUrl();

const TOKEN_KEY = 'cackle_token';

export function getToken() {
    try {
        return localStorage.getItem(TOKEN_KEY);
    } catch {
        return null;
    }
}

export function setToken(token) {
    try {
        if (token) {
            localStorage.setItem(TOKEN_KEY, token);
        } else {
            localStorage.removeItem(TOKEN_KEY);
        }
    } catch {
        // localStorage unavailable (private mode, etc.) — bearer auth just
        // won't persist across reloads; cookie auth still works.
    }
}

/** Error thrown for every non-2xx response and for network failures. */
export class ApiError extends Error {
    constructor(message, { code = 'unknown', status = 0, cause } = {}) {
        super(message);
        this.name = 'ApiError';
        this.code = code;
        this.status = status;
        if (cause) this.cause = cause;
    }
}

// Subscribers notified whenever a request comes back 401. AuthProvider hooks
// in here to clear local session state and redirect to /login without a full
// page reload.
const unauthorizedListeners = new Set();

export function onUnauthorized(listener) {
    unauthorizedListeners.add(listener);
    return () => unauthorizedListeners.delete(listener);
}

function notifyUnauthorized() {
    for (const listener of unauthorizedListeners) {
        try {
            listener();
        } catch {
            // never let a bad listener break the request pipeline
        }
    }
}

function buildQuery(params) {
    if (!params) return '';
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
        if (value === undefined || value === null || value === '') continue;
        search.set(key, String(value));
    }
    const qs = search.toString();
    return qs ? `?${qs}` : '';
}

/**
 * Core request function. Resolves with the parsed JSON body on success.
 * Throws `ApiError` on any non-2xx response or network failure.
 *
 * @param {string} path - path relative to the API base, e.g. "/events"
 * @param {object} [options]
 * @param {string} [options.method]
 * @param {object} [options.query] - query-string params, falsy values dropped
 * @param {any} [options.body] - JSON-serialisable request body
 * @param {boolean} [options.skipAuthRedirect] - don't fire onUnauthorized for this call
 */
export async function request(path, options = {}) {
    const { method = 'GET', query, body, skipAuthRedirect = false, headers: extraHeaders, ...rest } = options;

    const url = `${API_BASE_URL}${path}${buildQuery(query)}`;
    const headers = { Accept: 'application/json', ...extraHeaders };

    const token = getToken();
    if (token) {
        headers.Authorization = `Bearer ${token}`;
    }

    let payload;
    if (body !== undefined) {
        headers['Content-Type'] = 'application/json';
        payload = JSON.stringify(body);
    }

    let response;
    try {
        response = await fetch(url, {
            method,
            headers,
            body: payload,
            credentials: 'include',
            ...rest,
        });
    } catch (cause) {
        throw new ApiError('Network error — check your connection.', { code: 'network_error', cause });
    }

    if (response.status === 204) {
        return null;
    }

    const contentType = response.headers.get('content-type') || '';
    const isJson = contentType.includes('application/json');
    const data = isJson ? await response.json().catch(() => null) : await response.text().catch(() => null);

    if (!response.ok) {
        const errShape = data && typeof data === 'object' ? data.error : null;
        const message = errShape?.message || (typeof data === 'string' && data) || response.statusText || 'Request failed';
        const code = errShape?.code || `http_${response.status}`;

        if (response.status === 401 && !skipAuthRedirect) {
            notifyUnauthorized();
        }

        throw new ApiError(message, { code, status: response.status });
    }

    return data;
}

const get = (path, query) => request(path, { method: 'GET', query });
const post = (path, body, opts) => request(path, { method: 'POST', body, ...opts });
const patch = (path, body) => request(path, { method: 'PATCH', body });
const del = (path) => request(path, { method: 'DELETE' });

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

export const auth = {
    signup: (data) => post('/auth/signup', data),
    login: (data) => post('/auth/login', data),
    logout: () => post('/auth/logout'),
    me: () => get('/auth/me'),
    passwordReset: (email) => post('/auth/password-reset', { email }),
    passwordUpdate: (token, password) => post('/auth/password-update', { token, password }),
};

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

export const events = {
    /**
     * Public listing (published only) when called anonymously. When called
     * with org auth, the same endpoint is used by the organizer console to
     * list events owned by the caller's org (including drafts) — the backend
     * decides scope from the caller's session, not from any query param here.
     */
    list: (params) => get('/events', params),
    get: (slug) => get(`/events/${encodeURIComponent(slug)}`),
    create: (data) => post('/events', data),
    update: (id, data) => patch(`/events/${id}`, data),
    publish: (id) => post(`/events/${id}/publish`),
    stats: (id) => get(`/events/${id}/stats`),
    scanBundle: (id) => get(`/events/${id}/scan-bundle`),
};

// ---------------------------------------------------------------------------
// Ticket types
// ---------------------------------------------------------------------------

export const ticketTypes = {
    list: (eventId) => get(`/events/${eventId}/ticket-types`),
    create: (eventId, data) => post(`/events/${eventId}/ticket-types`, data),
    update: (id, data) => patch(`/ticket-types/${id}`, data),
    remove: (id) => del(`/ticket-types/${id}`),
};

// ---------------------------------------------------------------------------
// Orders
// ---------------------------------------------------------------------------

export const orders = {
    create: (data) => post('/orders', data),
    list: () => get('/orders'),
    get: (id) => get(`/orders/${id}`),
};

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

export const payments = {
    verify: (reference) => post('/payments/verify', { reference }),
};

// ---------------------------------------------------------------------------
// Tickets
// ---------------------------------------------------------------------------

export const tickets = {
    list: () => get('/tickets'),
    get: (id) => get(`/tickets/${id}`),
    pdfUrl: (id) => `${API_BASE_URL}/tickets/${id}/pdf`,
};

// ---------------------------------------------------------------------------
// Offline gate scan
// ---------------------------------------------------------------------------

export const scan = {
    bundle: (eventId) => get(`/events/${eventId}/scan-bundle`),
    submit: (data) => post('/scan', data),
    sync: (admissions) => post('/scan/sync', { admissions }),
};

export default {
    request,
    auth,
    events,
    ticketTypes,
    orders,
    payments,
    tickets,
    scan,
    getToken,
    setToken,
    onUnauthorized,
    ApiError,
};
