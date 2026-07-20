.PHONY: build build-frontend build-backend dev run test lint check screenshots notices clean

BIN := cackle

# Scoped Go package patterns — NEVER bare `./...`. web/node_modules can (and
# does) contain nested npm packages that ship a stray *.go file (e.g. the
# `flatted` package's golang/ port); a bare `./...` walks into web/ and tries
# to build/vet/test that, which is both wrong and fragile.
GO_PKGS := ./cmd/... ./internal/...

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
	rm -rf $(BIN) cmd/cackle/dist web/dist web/node_modules node_modules docs/screenshots
