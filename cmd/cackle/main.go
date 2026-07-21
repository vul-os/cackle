// Command cackle is the Cackle events + ticketing platform: one static Go
// binary, embedded SQLite, embedded React UI. See the module README for the
// product pitch; this file just wires flags/env into a running server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/demo"
	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/httpapi"
	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/orgs"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "cackle:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("cackle", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		addr        string
		dbPath      string
		baseURL     string
		demoFlag    bool
		showVersion bool
		mediaDir    string
	)
	fs.StringVar(&addr, "addr", "", "listen address (env CACKLE_ADDR, default :8080)")
	fs.StringVar(&dbPath, "db", "", "path to SQLite database file (env CACKLE_DB, default ./cackle.db)")
	fs.StringVar(&baseURL, "base-url", "", "externally-visible base URL (env CACKLE_BASE_URL)")
	fs.BoolVar(&demoFlag, "demo", false, "boot fully seeded with demo data, no setup required")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.StringVar(&mediaDir, "media-dir", "", "directory uploaded event images are stored under (env CACKLE_MEDIA_DIR, default <db dir>/media)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if showVersion {
		fmt.Fprintln(stdout, "cackle "+version)
		return nil
	}

	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// NOTE: never log secrets. cfg.SessionSecret and cfg.PaystackSecretKey
	// must never appear in a log line, error message, or panic dump.

	cfg, err := config.Load(config.Flags{
		Addr:     addr,
		DB:       dbPath,
		BaseURL:  baseURL,
		Demo:     demoFlag,
		DemoSet:  demoFlag || flagWasSet(fs, "demo"),
		MediaDir: mediaDir,
	})
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.MediaDir, 0o700); err != nil {
		return fmt.Errorf("create media dir: %w", err)
	}

	st, err := store.Open(cfg.DB)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("close store", "error", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authSvc := auth.NewService(st)
	eventsSvc := events.New(st)

	registry := payments.NewRegistry()
	recordStore := paymentRecordStoreAdapter{store: st}

	// manual is Cackle's DEFAULT payment provider: zero network calls,
	// zero API keys, works in every country. It is ALWAYS registered —
	// self-hosting Cackle with no payment configuration at all must still
	// be able to run a real event. Its state (including the audit trail
	// of who marked an order paid, and when) is backed by the database so
	// it survives a restart.
	manual, err := payments.NewManualWithStore(ctx, nil, recordStore)
	if err != nil {
		return fmt.Errorf("configure manual payment provider: %w", err)
	}
	if err := registry.Register(manual); err != nil {
		return fmt.Errorf("configure manual payment provider: %w", err)
	}

	// The stub provider auto-settles every order the instant it's begun
	// and refuses to register at all if a real Paystack secret is
	// configured (see internal/payments/stub.go) — it is only ever wired
	// up for --demo/CACKLE_DEMO, an explicit operator choice, never a
	// default.
	if cfg.Demo {
		stub, err := payments.NewStub(true)
		if err != nil {
			return fmt.Errorf("demo: register stub payment provider: %w", err)
		}
		if err := registry.Register(stub); err != nil {
			return fmt.Errorf("demo: register stub payment provider: %w", err)
		}
	}
	// bankingProvider stays nil unless a real Paystack secret is
	// configured — orgs.New treats that as a valid, supported
	// configuration (see orgs.BankingProvider's doc comment): the bank
	// list falls back to a small built-in one and bank account details are
	// stored locally without a live recipient registration.
	var bankingProvider orgs.BankingProvider
	if cfg.PaystackSecretKey != "" {
		ps, err := payments.NewPaystack()
		if err != nil {
			return fmt.Errorf("configure paystack: %w", err)
		}
		if err := registry.Register(ps); err != nil {
			return fmt.Errorf("configure paystack: %w", err)
		}
		bankingProvider = ps
	}
	// Every other real processor (Stripe, Paystack's own charge/verify
	// path, Adyen, BTCPay, lnbits, ...) is served through the patala
	// substrate rather than a hand-rolled Cackle adapter — see
	// docs/PAYMENTS.md "The patala path" and internal/payments/patala.go.
	// registerPatalaProviders is a real implementation ONLY in a binary
	// built with `-tags patala` (requires CGO_ENABLED=1 and the sibling
	// patala repo's Go bindings generated); the default, pure-Go build
	// wires in a no-op stub (cmd/cackle/patala_register_stub.go) so the
	// offline gate / scanner and every other pure-Go build stay completely
	// unaffected. A provider with no CACKLE_<NAME>_* environment variables
	// set at all is silently skipped (not configured); once at least one
	// is set, any error is a real misconfiguration and fails startup.
	if err := registerPatalaProviders(registry, recordStore); err != nil {
		return fmt.Errorf("configure patala payment providers: %w", err)
	}
	ordersSvc := orders.New(st, eventsSvc, registry)
	orgsSvc := orgs.New(st, bankingProvider)

	if cfg.Demo {
		if err := demo.Seed(ctx, st, eventsSvc, ordersSvc, orgsSvc); err != nil {
			return fmt.Errorf("demo: seed: %w", err)
		}
		fmt.Fprintf(stdout, "\ncackle --demo is seeded and ready:\n")
		fmt.Fprintf(stdout, "  URL:      %s\n", cfg.BaseURL)
		fmt.Fprintf(stdout, "  Login:    %s\n", demo.DemoEmail)
		fmt.Fprintf(stdout, "  Password: %s\n\n", demo.DemoPassword)
	}

	webFS, err := embeddedWebFS()
	if err != nil {
		logger.Warn("no embedded frontend in this binary", "detail", err)
	}

	handler := httpapi.New(httpapi.Deps{
		Store:    st,
		Auth:     authSvc,
		Events:   eventsSvc,
		Orders:   ordersSvc,
		Orgs:     orgsSvc,
		Payments: registry,
		Config:   cfg,
		MediaDir: cfg.MediaDir,
		WebFS:    webFS,
		Logger:   logger,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("cackle listening", "addr", cfg.Addr, "version", version, "demo", cfg.Demo)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}
}

// flagWasSet reports whether a flag was explicitly passed on the command
// line, so a bare `--demo=false` (or its absence) doesn't shadow
// CACKLE_DEMO=1 from the environment.
func flagWasSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
