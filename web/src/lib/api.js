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

// `/media/{id}` is a public, unauthenticated route that sits beside `/api`,
// not under it (see docs/API.md) — strip a trailing "/api" from whatever
// base we resolved so media URLs still point at the right origin when
// VITE_API_URL is an absolute cross-origin URL, and stay root-relative in
// the common same-origin case.
const MEDIA_BASE_URL = API_BASE_URL.replace(/\/api\/?$/, '');

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
const put = (path, body) => request(path, { method: 'PUT', body });
const del = (path) => request(path, { method: 'DELETE' });

/**
 * Multipart upload via XMLHttpRequest — `fetch` has no upload-progress
 * event, and the image uploader needs one. Mirrors `request()`'s auth +
 * error-shape handling (bearer token, ApiError, 401 -> onUnauthorized) so
 * callers get the same contract regardless of transport.
 */
function uploadFile(path, file, { onProgress } = {}) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open('POST', `${API_BASE_URL}${path}`, true);
        xhr.withCredentials = true;
        xhr.setRequestHeader('Accept', 'application/json');
        const token = getToken();
        if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`);

        if (xhr.upload && typeof onProgress === 'function') {
            xhr.upload.onprogress = (e) => {
                if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100));
            };
        }

        xhr.onload = () => {
            let data = null;
            try {
                data = xhr.responseText ? JSON.parse(xhr.responseText) : null;
            } catch {
                // non-JSON body — data stays null, message falls back below
            }
            if (xhr.status >= 200 && xhr.status < 300) {
                resolve(data);
                return;
            }
            const errShape = data && typeof data === 'object' ? data.error : null;
            const message = errShape?.message || xhr.statusText || 'Upload failed';
            const code = errShape?.code || `http_${xhr.status}`;
            if (xhr.status === 401) notifyUnauthorized();
            reject(new ApiError(message, { code, status: xhr.status }));
        };
        xhr.onerror = () => reject(new ApiError('Network error — check your connection.', { code: 'network_error' }));

        const form = new FormData();
        form.append('file', file);
        xhr.send(form);
    });
}

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
     * GET /api/events — published events only, regardless of caller. As of
     * this wave the backend does NOT branch on org auth to include the
     * caller's own drafts (internal/events.Service has a ListByOrg for
     * that, but nothing in internal/httpapi routes to it yet) — so the
     * organiser console's own draft events are invisible here. See
     * pages/organizers/events/pending-draft.js for the frontend-only
     * stopgap (remembering an in-progress draft's id locally) until a
     * proper org-scoped listing route exists.
     */
    list: (params) => get('/events', params),
    get: (slug) => get(`/events/${encodeURIComponent(slug)}`),
    create: (data) => post('/events', data),
    update: (id, data) => patch(`/events/${id}`, data),
    /**
     * Not part of the documented API as of this wave — no confirmed
     * DELETE /api/events/{id} route exists yet. Kept as a thin wrapper so
     * the delete-confirmation UI has one call site to update the moment
     * the backend lands it; callers must handle the "not implemented"
     * shape (404/405) rather than assume success.
     */
    remove: (id) => del(`/events/${id}`),
    publish: (id) => post(`/events/${id}/publish`),
    stats: (id) => get(`/events/${id}/stats`),
    scanBundle: (id) => get(`/events/${id}/scan-bundle`),
    /**
     * Organizer/scanner-only attendee roster: { attendees, total, limit, offset }.
     * params may include { q, status, limit, offset } — all optional.
     */
    attendees: (id, params) => get(`/events/${id}/attendees`, params),
};

// ---------------------------------------------------------------------------
// Event images (cover + gallery)
// ---------------------------------------------------------------------------

/**
 * `POST /api/events/{id}/images` multipart upload -> {id,url,width,height}.
 * `DELETE /api/images/{id}` removes a stored image. `url(id)` is the public,
 * unauthenticated `/media/{id}` byte-serving route — safe to drop straight
 * into an <img src>.
 */
export const images = {
    upload: (eventId, file, opts) => uploadFile(`/events/${eventId}/images`, file, opts),
    remove: (id) => del(`/images/${id}`),
    url: (id) => (id ? `${MEDIA_BASE_URL}/media/${id}` : null),
};

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

/**
 * Event categories, derived server-side from published events:
 * [{ slug, label, count }]. Used to drive the landing page's category tabs
 * and the browse page's ?category= filter. Callers should treat a failure
 * here as "no categories to show" rather than a page-level error — category
 * tabs are a filter convenience, not critical path.
 */
export const categories = {
    list: () => get('/categories'),
};

/**
 * The full ISO-4217 currency table Cackle knows about (internal/money):
 * [{ code, name, exponent }]. Drives the event-creation/edit currency
 * picker — Cackle has no privileged currency, so this is deliberately the
 * whole table, not a hardcoded handful of "common" ones.
 */
export const currencies = {
    list: () => get('/currencies'),
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
// Org members & invites
// ---------------------------------------------------------------------------

export const orgMembers = {
    list: (orgId) => get(`/orgs/${orgId}/members`),
    invites: (orgId) => get(`/orgs/${orgId}/invites`),
    invite: (orgId, data) => post(`/orgs/${orgId}/invites`, data),
    revokeInvite: (inviteId) => del(`/invites/${inviteId}`),
    acceptInvite: (token) => post('/invites/accept', { token }),
    /**
     * Not part of the documented API as of this wave — only listing members
     * is confirmed. Kept as a thin wrapper so the role-change control has one
     * call site to update the moment the backend lands it; callers must
     * handle the "not implemented" shape (404/405) rather than assume success.
     */
    updateRole: (orgId, userId, role) => patch(`/orgs/${orgId}/members/${userId}`, { role }),
};

// ---------------------------------------------------------------------------
// Payouts & bank details
// ---------------------------------------------------------------------------

/**
 * Bank account numbers are masked on read (see docs/API.md) — a GET here
 * is for display only, never pre-fill an edit form's account-number field
 * from it. `banks.list()` is the provider's (Paystack) bank list, used to
 * populate the bank-code select.
 */
export const payoutsApi = {
    bankAccount: (orgId) => get(`/orgs/${orgId}/bank-account`),
    setBankAccount: (orgId, data) => put(`/orgs/${orgId}/bank-account`, data),
    banks: () => get('/banks'),
    forEvent: (eventId) => get(`/events/${eventId}/payouts`),
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
    categories,
    images,
    orgMembers,
    payoutsApi,
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
