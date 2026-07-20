// Package httpapi is Cackle's HTTP surface: a chi router, every handler in
// docs/API.md, and the middleware chain (auth, CSRF, rate limiting,
// security headers, structured logging) that sits in front of them. It
// also serves the embedded React build at "/" with SPA fallback.
//
// See ARCHITECTURE.md's "security bar" section for the non-negotiables
// this package exists to enforce, in particular: RBAC checked server-side
// on every org/event route, and an error shape that never leaks an
// internal error or SQL to the client.
package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/time/rate"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// Deps is everything the router needs. Scan is deliberately absent:
// internal/scan is a pure, store-independent package by design (see its
// package doc) — there is no scan.Service to inject. httpapi wires
// internal/scan's SeenSet/SyncSink interfaces onto Store directly; see
// scan_handlers.go.
type Deps struct {
	Store    *store.Store
	Auth     *auth.Service
	Events   *events.Service
	Orders   *orders.Service
	Payments *payments.Registry
	Config   *config.Config
	WebFS    fs.FS // embedded web/dist build; nil is handled (see spa.go)
	Logger   *slog.Logger
}

type server struct {
	deps        Deps
	webhookSeen *memorySeenStore
}

func (s *server) log() *slog.Logger {
	if s.deps.Logger != nil {
		return s.deps.Logger
	}
	return slog.Default()
}

// New builds the full Cackle HTTP handler: middleware chain, every route
// in docs/API.md, and the embedded frontend with SPA fallback.
func New(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	s := &server{deps: deps, webhookSeen: newMemorySeenStore()}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(requestLogger(s.log()))
	r.Use(recoverer(s.log()))
	r.Use(securityHeaders)
	r.Use(cors.Handler(corsOptions(baseURLOf(deps.Config))))
	r.Use(s.authenticate)
	r.Use(s.requireCSRF)

	// /healthz is intentionally outside /api and outside auth/rate-limit —
	// it's what the Docker HEALTHCHECK polls and must never depend on the
	// database, a session, or any other subsystem being healthy beyond
	// "the process is up and serving".
	r.Get("/healthz", handleHealthz)

	authLimiter := newIPLimiter(rate.Every(2*time.Second), 5) // ~30/min, burst 5
	scanLimiter := newIPLimiter(rate.Limit(10), 30)           // 10/sec sustained, burst 30

	r.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) { notFound(w, "no such route") })
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, codeInvalidRequest, "method not allowed")
		})

		r.Route("/auth", func(r chi.Router) {
			r.With(rateLimit(authLimiter)).Post("/signup", s.handleSignup)
			r.With(rateLimit(authLimiter)).Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.requireUser(s.handleMe))
			r.With(rateLimit(authLimiter)).Post("/password-reset", s.handlePasswordReset)
			r.With(rateLimit(authLimiter)).Post("/password-update", s.handlePasswordUpdate)
		})

		r.Route("/events", func(r chi.Router) {
			r.Get("/", s.handleListPublicEvents)
			r.Post("/", s.requireUser(s.handleCreateEvent))
			r.Get("/{id}", s.handleGetPublicEvent)
			r.Patch("/{id}", s.requireUser(s.handleUpdateEvent))
			r.Post("/{id}/publish", s.requireUser(s.handlePublishEvent))
			r.Get("/{id}/stats", s.requireUser(s.handleEventStats))
			r.Get("/{id}/ticket-types", s.requireUser(s.handleListTicketTypes))
			r.Post("/{id}/ticket-types", s.requireUser(s.handleCreateTicketType))
			r.Get("/{id}/scan-bundle", s.requireUser(s.handleScanBundle))
		})

		r.Route("/ticket-types", func(r chi.Router) {
			r.Patch("/{id}", s.requireUser(s.handleUpdateTicketType))
			r.Delete("/{id}", s.requireUser(s.handleDeleteTicketType))
		})

		r.Route("/orders", func(r chi.Router) {
			r.Post("/", s.handleCreateOrder) // buyer auth optional — see handler doc
			r.Get("/", s.requireUser(s.handleListMyOrders))
			r.Get("/{id}", s.requireUser(s.handleGetOrder))
		})

		r.Route("/payments", func(r chi.Router) {
			r.Post("/verify", s.handleVerifyPayment)
			r.Post("/webhook/{provider}", s.handleWebhook) // no auth: provider-signed
		})

		r.Route("/tickets", func(r chi.Router) {
			r.Get("/", s.requireUser(s.handleListMyTickets))
			r.Get("/{id}", s.requireUser(s.handleGetTicket))
			r.Get("/{id}/pdf", s.requireUser(s.handleTicketPDF))
		})

		r.With(rateLimit(scanLimiter)).Post("/scan", s.requireUser(s.handleScan))
		r.Post("/scan/sync", s.requireUser(s.handleScanSync))
	})

	// Everything else falls through to the embedded SPA (or a "not built"
	// notice), never shadowing /api/* — chi resolves the longest matching
	// static prefix first, so /api/... always hits the subrouter above.
	r.Handle("/*", s.spaHandler())

	return r
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func baseURLOf(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.BaseURL
}
