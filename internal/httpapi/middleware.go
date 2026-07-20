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
// CSP allows only same-origin script/style/connect (Tailwind/shadcn need
// 'unsafe-inline' for style-src; there is no inline script anywhere in the
// build so script-src stays strict). camera is allowed same-origin only,
// for the scanner view's QR camera access.
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

const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
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
		AllowedOrigins:   origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
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
