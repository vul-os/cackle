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
	"github.com/vul-os/cackle/internal/orgs"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/scan/substrate"
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
	Orgs     *orgs.Service
	Payments *payments.Registry
	Config   *config.Config
	WebFS    fs.FS // embedded web/dist build; nil is handled (see spa.go)
	Logger   *slog.Logger
	// MediaDir is where uploaded event images are stored on disk (see
	// internal/media). Falls back to cfg.MediaDir via mediaDir() if unset,
	// so existing callers that only set Config still work.
	MediaDir string
}

// mediaDir resolves the effective media storage directory: the explicit
// Deps.MediaDir if set, otherwise cfg.MediaDir, otherwise "./media" — the
// same default config.Load itself falls back to, so a Deps built by hand
// (tests) without going through config.Load still gets a sane directory.
func (d Deps) mediaDir() string {
	if d.MediaDir != "" {
		return d.MediaDir
	}
	if d.Config != nil && d.Config.MediaDir != "" {
		return d.Config.MediaDir
	}
	return "./media"
}

type server struct {
	deps        Deps
	webhookSeen *memorySeenStore
	// ledgers lazily compiles the shared DMTAP sync engine on first use, once
	// per process. Only the cross-gate reconciliation report and the
	// server-to-server sync routes reach it; the admission path never does. See
	// reconcile_handlers.go and sync_handlers.go.
	ledgers ledgerCompiler
	// syncNonces refuses a replayed peer request inside the window in which its
	// timestamp is still fresh. Process-wide, because a peer's nonce must be
	// spent once for this node and not once per handler. See
	// internal/scan/substrate/peerauth.go.
	syncNonces *substrate.NonceCache
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
	s := &server{deps: deps, webhookSeen: newMemorySeenStore(), syncNonces: substrate.NewNonceCache()}

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
			r.Delete("/{id}", s.requireUser(s.handleDeleteEvent))
			r.Post("/{id}/publish", s.requireUser(s.handlePublishEvent))
			r.Get("/{id}/stats", s.requireUser(s.handleEventStats))
			r.Get("/{id}/ticket-types", s.requireUser(s.handleListTicketTypes))
			r.Post("/{id}/ticket-types", s.requireUser(s.handleCreateTicketType))
			r.Get("/{id}/scan-bundle", s.requireUser(s.handleScanBundle))
			// Rate-limited like the scan routes it reports on: this is the
			// most expensive read in the product (it re-folds the event's
			// contested admissions through the WebAssembly merge engine on
			// every call), so on an internet-reachable host it is the one
			// authenticated GET worth bounding.
			r.With(rateLimit(scanLimiter)).Get("/{id}/admission-conflicts", s.requireUser(s.handleAdmissionConflicts))
			r.Get("/{id}/attendees", s.requireUser(s.handleListEventAttendees))
			r.Post("/{id}/images", s.requireUser(s.handleUploadImage))
			r.Get("/{id}/payouts", s.requireUser(s.handleEventPayouts))
			r.Get("/{id}/orders", s.requireUser(s.handleListEventOrders))

			// Host pages (docs/HOST-PAGES.md). GET is public and
			// published-only, exactly like the rendered page at /h/{ref} —
			// it is what a host fetches to render their event on their own
			// site. PUT/DELETE are admin+ on the event's org, the same bar
			// as editing the event itself.
			r.Get("/{id}/page", s.handleGetEventPage)
			r.Put("/{id}/page", s.requireUser(s.handlePutEventPage))
			r.Delete("/{id}/page", s.requireUser(s.handleDeleteEventPage))
		})

		r.Get("/categories", s.handleListCategories)
		r.Get("/currencies", s.handleListCurrencies)

		r.Route("/ticket-types", func(r chi.Router) {
			r.Patch("/{id}", s.requireUser(s.handleUpdateTicketType))
			r.Delete("/{id}", s.requireUser(s.handleDeleteTicketType))
		})

		r.Delete("/images/{id}", s.requireUser(s.handleDeleteImage))

		r.Route("/orgs", func(r chi.Router) {
			// POST /api/orgs is the only org route not gated on an
			// EXISTING membership — there is no org yet to be a member of.
			// requireUser is the whole gate, and the caller becomes the new
			// org's owner. See handleCreateOrg's doc for why that grants no
			// authority over anyone else's org.
			r.Post("/", s.requireUser(s.handleCreateOrg))

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/events", s.requireUser(s.handleListOrgEvents))
				r.Get("/members", s.requireUser(s.handleListOrgMembers))
				r.Patch("/members/{user_id}", s.requireUser(s.handleUpdateOrgMemberRole))
				r.Get("/invites", s.requireUser(s.handleListOrgInvites))
				r.Post("/invites", s.requireUser(s.handleCreateOrgInvite))
				r.Get("/bank-account", s.requireUser(s.handleGetBankAccount))
				r.Put("/bank-account", s.requireUser(s.handleSetBankAccount))
			})
		})
		r.Delete("/invites/{id}", s.requireUser(s.handleDeleteOrgInvite))
		r.Post("/invites/accept", s.requireUser(s.handleAcceptOrgInvite))
		r.Get("/banks", s.requireUser(s.handleListBanks))

		r.Route("/orders", func(r chi.Router) {
			r.Post("/", s.handleCreateOrder) // buyer auth optional — see handler doc
			r.Get("/", s.requireUser(s.handleListMyOrders))
			r.Get("/{id}", s.requireUser(s.handleGetOrder))
			r.Post("/{id}/mark-paid", s.requireUser(s.handleMarkOrderPaid))
			r.Post("/{id}/mark-failed", s.requireUser(s.handleMarkOrderFailed))
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
		// /scan/sync is rate-limited on the same bucket as /scan, which it
		// previously was not. It is an authenticated WRITE that inserts one
		// admissions row per batch item, and a venue's gates reach it from the
		// public internet — an unbounded write endpoint is a strange thing to
		// leave next to a bounded one. A batch carries many scans in a single
		// request, so 10/sec sustained per IP is far more headroom than a real
		// gate needs even with every scanner behind one venue NAT.
		r.With(rateLimit(scanLimiter)).Post("/scan/sync", s.requireUser(s.handleScanSync))

		// Server-to-server replication of the admission ledger. Two groups of
		// routes with two entirely separate credentials, and mixing them up is
		// not possible: /ops is authenticated by a PINNED NODE KEY signing each
		// request (no session, no cookie, no bearer token), everything else by an
		// ordinary operator session with the org owner role.
		//
		// Both /ops routes share the sync limiter. /scan/sync taught this
		// codebase that an authenticated write reachable from the public internet
		// still needs a bound; a cloud node's /ops is the same shape of thing,
		// and it is bounded from its first commit rather than after the fact —
		// size-capped body, count-capped page, page-capped round.
		//
		// Nothing here runs on a node with no peers. There is no poller: a round
		// happens when POST /api/sync/peers/{id}/sync is called.
		syncLimiter := newIPLimiter(rate.Limit(5), 20) // 5/sec sustained, burst 20
		r.Route("/sync", func(r chi.Router) {
			r.With(rateLimit(syncLimiter)).Get("/ops", s.handleSyncPull)
			r.With(rateLimit(syncLimiter)).Post("/ops", s.handleSyncPush)

			// Rate-limited despite being owner-only: it reads the engine's own
			// version, which instantiates the WebAssembly module, so it is the
			// one operator GET here with a real per-call cost.
			r.With(rateLimit(syncLimiter)).Get("/status", s.requireUser(s.handleSyncStatus))
			r.Post("/peers", s.requireUser(s.handleEnrolSyncPeer))
			r.Delete("/peers/{id}", s.requireUser(s.handleDeleteSyncPeer))
			r.With(rateLimit(syncLimiter)).Post("/peers/{id}/sync", s.requireUser(s.handleSyncPeerRound))
		})
	})

	// GET /media/{id} is public (uploaded event images are not secrets) and
	// deliberately outside /api and /healthz's exemptions but still behind
	// the shared middleware chain (security headers, rate-agnostic —
	// there's no auth to rate-limit around here). Registered directly on
	// the root router, so chi's longest-static-prefix-first resolution
	// means it can never be shadowed by the SPA catch-all below.
	r.Get("/media/{id}", s.handleServeMedia)

	// GET /h/{slugOrID} is the public, server-rendered host page — HTML, not
	// API, so it lives beside /media rather than under /api. It is
	// unauthenticated and published-only, serves no script, and answers with
	// its own far stricter Content-Security-Policy than the app-wide one (see
	// page_handlers.go). Registered on the root router for the same reason
	// /media is: chi resolves the longest static prefix first, so the SPA
	// catch-all below can never shadow it.
	r.Get("/h/{ref}", s.handleHostPage)

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
