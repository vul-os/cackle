package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// securityHeaders sets a fixed, conservative header set on every response.
// CSP allows only same-origin script/style/connect, and names no remote
// origin anywhere: a Cackle page contacts nobody else.
//
// style-src carries 'unsafe-inline' — not, as this comment used to say,
// because "Tailwind/shadcn need it". Tailwind compiles to a linked
// stylesheet. The two things that actually need it, both measured in a
// browser against the built app, are web/index.html's inline <style> (the
// anti-FOUC background, which must apply before the stylesheet loads) and
// inline style="" attributes — 25 in web/src plus qr-scanner's scan-region
// overlay. script-src needs no such thing: the build emits no inline script.
//
// camera is allowed same-origin only, for the scanner view's QR camera.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=(), payment=()")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-XSS-Protection", "0") // superseded by CSP; explicitly disable legacy filter
		next.ServeHTTP(w, r)
	})
}

// worker-src carries `blob:` because the gate scanner's QR decoder runs in a
// Worker built from a Blob. qr-scanner (a bundled, vendored dependency) does
// exactly this in qr-scanner-worker.min.js:
//
//	export const createWorker = () =>
//	    new Worker(URL.createObjectURL(new Blob([...], {type:"application/javascript"})))
//
// and reaches for it whenever the browser has no usable native
// BarcodeDetector — Firefox and Safari everywhere, plus Chromium on Apple
// Silicon macOS, which qr-scanner routes to the worker on purpose. Without
// this directive worker-src falls back to script-src ('self'), the browser
// refuses the worker ("Creating a worker from 'blob:…' violates … script-src
// 'self'"), and the camera opens but never decodes a ticket — staff fall back
// to typing codes by hand at the door.
//
// `blob:` here is not a widening of what code may be fetched from where. A
// blob: URL is minted by this document at runtime and is same-origin and
// opaque: it names bytes this page already holds, cannot be forged by a
// third party, and is unreachable to any other origin. No remote host becomes
// loadable because of it. script-src stays 'self' precisely so a blob: is
// still refused for a page-level <script>; only the Worker case is allowed.
//
// object-src / frame-src / base-uri are 'none' rather than inheriting
// default-src's 'self'. Each names something the app does not do and should
// never start doing by accident:
//
//	object-src 'none'   there is no <object>/<embed> anywhere in web/src. Left
//	                    at 'self' an injection could <embed> an operator-
//	                    uploaded file from /media/{id} and get a plugin
//	                    document out of it.
//	frame-src 'none'    the app frames nothing. Checkout hands off to a
//	                    payment provider by top-level navigation
//	                    (visitor/checkout/redirect.jsx), never an iframe.
//	                    With 'self' an injection could frame Cackle's own
//	                    authenticated routes for a UI-redress attack from
//	                    inside the origin, which frame-ancestors — an
//	                    outside-in directive — does not stop.
//	base-uri 'none'     no page carries a <base>. 'self' would still let an
//	                    injected <base href="/anything/"> silently re-point
//	                    every relative URL on the page.
//
// img-src keeps `data:`: leaflet's stylesheet ships its three marker/shadow
// PNGs as data: URLs, and the map chunk loads on any deployment that sets
// VITE_CACKLE_MAP_TILE_URL. Verified by loading those exact three URLs under
// a policy without it — all three blocked.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"worker-src 'self' blob:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"frame-src 'none'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'; " +
	"form-action 'self'"

// corsOptions is same-origin by default: browsers never apply CORS to
// same-origin requests at all, so leaving AllowedOrigins empty already
// blocks every cross-origin caller without affecting the SPA served from
// this same binary. The one exception is cfg.BaseURL itself, allowed
// explicitly so a self-hoster fronting Cackle behind a reverse proxy on a
// distinct public origin (BaseURL differs from the listener's own view of
// its origin) still works with credentials.
func corsOptions(baseURL string) cors.Options {
	var origins []string
	if baseURL != "" {
		origins = []string{baseURL}
	}
	return cors.Options{
		AllowedOrigins: origins,
		// PUT belongs here: two routes use it (PUT /api/orgs/{id}/bank-account
		// and PUT /api/events/{id}/page) and neither was reachable from a
		// cross-origin client — the only deployment where CORS applies at all
		// — because the preflight named a method this list did not.
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "Authorization", csrfHeaderName},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}
}

// requestLogger logs one structured line per request: method, path,
// status, duration, request id, remote IP. It NEVER logs headers, cookies,
// query strings, or bodies — those may carry session tokens, passwords, or
// provider secrets, and this is the one place a stray %+v could leak them
// into every deployment's logs.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", chimw.GetReqID(r.Context()),
				"remote_ip", clientIP(r),
			)
		})
	}
}

// recoverer turns a panic anywhere downstream into a clean JSON 500
// instead of a dropped connection or a leaked stack trace to the client.
// The stack/paniced value is only ever logged server-side.
func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if logger != nil {
						logger.Error("panic recovered", "panic", fmt.Sprintf("%v", rec), "path", r.URL.Path, "request_id", chimw.GetReqID(r.Context()))
					}
					writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
