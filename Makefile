.PHONY: build build-frontend build-backend dev run test lint check screenshots notices clean \
	patala-generate build-patala test-patala run-patala

BIN := cackle

# Scoped Go package patterns — NEVER bare `./...`. web/node_modules can (and
# does) contain nested npm packages that ship a stray *.go file (e.g. the
# `flatted` package's golang/ port); a bare `./...` walks into web/ and tries
# to build/vet/test that, which is both wrong and fragile.
GO_PKGS := ./cmd/... ./internal/...

# Where the sibling patala repo lives, relative to this Makefile — matches
# go.mod's `replace github.com/vul-os/patala/patala-go => ../patala/patala-go`.
# Override with `make PATALA_DIR=/path/to/patala patala-generate` if your
# checkout lives somewhere else (go.mod's replace path would need to match).
PATALA_DIR := ../patala
PATALA_GO_DIR := $(PATALA_DIR)/patala-go
PATALA_BINDINGS := $(PATALA_GO_DIR)/bindings/patala
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	PATALA_LIB := $(PATALA_BINDINGS)/libpatala_py.dylib
else
	PATALA_LIB := $(PATALA_BINDINGS)/libpatala_py.so
endif

# ── Frontend ───────────────────────────────────────────────────────────────
# web/ is its own npm package (own package.json, vite config, index.html).
build-frontend:
	cd web && npm ci && npm run build

# ── Backend ────────────────────────────────────────────────────────────────
# Plain `go build` (no embed_frontend tag) compiles cmd/cackle on its own,
# serving whatever dev/stub UI cmd/cackle falls back to when no built SPA is
# present. Use `make build` for the real single-binary artifact with the web
# UI embedded.
build-backend:
	go build -o $(BIN) ./cmd/cackle

# Full single-binary build: build the SPA, copy it into cmd/cackle/dist so
# `//go:embed all:dist` (behind the embed_frontend build tag) can see it, build
# the Go binary, then remove the copy so it never lingers in the working tree.
# See Dockerfile's "EMBED CONTRACT" comment for the full contract.
build: build-frontend
	rm -rf cmd/cackle/dist
	cp -r web/dist cmd/cackle/dist
	CGO_ENABLED=0 go build -tags embed_frontend -o $(BIN) ./cmd/cackle
	rm -rf cmd/cackle/dist

# ── Dev loop ───────────────────────────────────────────────────────────────
# Two servers: Vite dev server (HMR) on :5173 proxying API calls to the Go
# backend on :8080 (run separately with `make run` in another shell).
dev:
	cd web && npm run dev

run:
	go run ./cmd/cackle --demo

# ── Verification ───────────────────────────────────────────────────────────
test:
	go test $(GO_PKGS)

lint:
	go vet $(GO_PKGS)
	cd web && npm run lint

# Single gate to run at the end of every change — mirrors CI.
check: lint test build

# ── Docs / release housekeeping ─────────────────────────────────────────────
screenshots:
	npm run screenshots

notices:
	npm run notices

clean:
	rm -rf $(BIN) cmd/cackle/dist web/dist web/node_modules node_modules
	@# NOTE: docs/screenshots/ is deliberately NOT removed. Those PNGs are
	@# committed deliverables the README renders on GitHub, not build
	@# artifacts — `make clean` once deleted them. Regenerate deliberately
	@# with `make screenshots`, never as a side effect of cleaning.

# ── The patala path (real payment processors) ───────────────────────────────
# See docs/PAYMENTS.md "The patala path" and internal/payments/patala.go's
# own build comment for the full explanation. The DEFAULT `build`/`test`
# targets above are completely unaffected by any of this — they never pass
# `-tags patala`, so they never import patala-go, need no cgo, and stay
# exactly the pure-Go, CGO_ENABLED=0 build they always were (the offline
# gate/scanner's own guarantee — see docs/OFFLINE-GATES.md). Everything
# below is opt-in, for a self-hoster who wants a real processor (Stripe,
# Paystack, Adyen, ...) instead of (or alongside) the always-available
# native `manual` provider.
#
# Prerequisites, once, before any target below works:
#   1. Clone patala next to this repo (so ../patala/patala-go exists — see
#      PATALA_DIR above to override).
#   2. Install uniffi-bindgen-go pinned to this workspace's uniffi version
#      (see ../patala/patala-go/README.md's exact `cargo install` command).
#   3. A Rust toolchain (cargo/rustc) and a C toolchain (cc/clang/gcc).

# Regenerate patala-go's Go bindings from a cdylib built with every
# patala-fiat processor compiled in. Delegates entirely to patala-go's own
# Makefile — this target exists so `make build-patala`/`test-patala` don't
# silently build against stale bindings.
patala-generate:
	$(MAKE) -C $(PATALA_GO_DIR) FEATURES=fiat-all generate

# Build cackle WITH the patala path linked in (no embedded frontend — pair
# with `build-frontend`/`cmd/cackle/dist` by hand if you want both; see
# `build`'s own recipe for that dance). Requires CGO_ENABLED=1 and a C
# toolchain — see internal/payments/patala.go's build comment.
build-patala: patala-generate
	CGO_ENABLED=1 CGO_LDFLAGS="-lpatala_py -L$(PATALA_BINDINGS)" \
		go build -tags patala -o $(BIN) ./cmd/cackle

# Run the patala-tagged test suite (internal/payments/patala_test.go +
# every other package's own tests, unaffected). DYLD/LD_LIBRARY_PATH point
# the dynamic linker at the freshly generated cdylib at RUN time (macOS/
# Linux — same reasoning as patala-go's own Makefile).
test-patala: patala-generate
	CGO_ENABLED=1 CGO_LDFLAGS="-lpatala_py -L$(PATALA_BINDINGS)" \
		DYLD_LIBRARY_PATH="$(PATALA_BINDINGS):$$DYLD_LIBRARY_PATH" \
		LD_LIBRARY_PATH="$(PATALA_BINDINGS):$$LD_LIBRARY_PATH" \
		go test -tags patala $(GO_PKGS)

# Run cackle --demo with the patala path linked in, e.g. to smoke-test a
# real processor's credentials:
#   CACKLE_STRIPE_SECRET_KEY=sk_test_... CACKLE_STRIPE_WEBHOOK_SECRET=whsec_... make run-patala
run-patala: build-patala
	DYLD_LIBRARY_PATH="$(PATALA_BINDINGS):$$DYLD_LIBRARY_PATH" \
		LD_LIBRARY_PATH="$(PATALA_BINDINGS):$$LD_LIBRARY_PATH" \
		./$(BIN) --demo
