# Dockerfile — Cackle deploy image (single static Go binary + embedded React UI).
#
# Produces the image CI pushes as
#   ghcr.io/vul-os/cackle:latest
# For local/self-host use, tag it however you like — the README documents:
#
#   docker build -t vulos/cackle .
#   docker run -d --name cackle -p 8080:8080 -v cackle-data:/srv/data vulos/cackle
#   # open http://localhost:8080
#
# ── BUILD CONTEXT ─────────────────────────────────────────────────────────────
# Plain clone-and-build, no sibling repos required:
#   docker build -t ghcr.io/vul-os/cackle:latest .
#
# ── EMBED CONTRACT (read before touching cmd/cackle) ──────────────────────────
# cmd/cackle embeds the built web UI via Go's embed.FS. Since `go:embed` cannot
# traverse into a parent/sibling directory (no `../web/dist`), the web build
# output is copied into cmd/cackle/dist immediately before compiling, gated
# behind the `embed_frontend` build tag (mirrors the wede convention):
#
#   cmd/cackle/embed_frontend.go   (+build embed_frontend)  //go:embed all:dist
#   cmd/cackle/embed_dev.go        (!embed_frontend)         empty/dev fallback
#
# `go build -tags embed_frontend ./cmd/cackle` therefore REQUIRES cmd/cackle/dist
# to exist and contain the built SPA at build time; it is never committed and is
# removed again after the build (see Makefile `build` target for the same
# sequence used locally).

# ── Stage 1: build the SPA ─────────────────────────────────────────────────────
FROM node:22-bookworm-slim AS web
WORKDIR /build/web
COPY web/package*.json ./
# Falls back to `npm install` if the lockfile is missing/out of sync (e.g. web/
# is still being scaffolded) so the image build never blocks on that alone.
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ── Stage 2: build the static Go binary ────────────────────────────────────────
FROM golang:1.25-bookworm AS build
ARG VERSION=docker
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# Overlay the freshly-built SPA so //go:embed all:dist (behind the
# embed_frontend tag) bakes in real assets. See the EMBED CONTRACT above.
COPY --from=web /build/web/dist ./cmd/cackle/dist
# CGO_ENABLED=0: modernc.org/sqlite is pure Go, so the binary stays fully
# static and the final image can be distroless/alpine with no libc games.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -tags embed_frontend \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/cackle ./cmd/cackle

# ── Stage 3: minimal non-root runtime ──────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget \
 && adduser -D -u 10001 cackle
COPY --from=build /out/cackle /usr/local/bin/cackle
# CACKLE_DB defaults to ./cackle.db (relative to CWD) — run from /srv/data so
# the SQLite file + generated session secret persist across container restarts
# in a single, obvious volume.
WORKDIR /srv/data
RUN chown -R cackle:cackle /srv/data
USER cackle
VOLUME ["/srv/data"]

ENV CACKLE_ADDR=":8080" \
    CACKLE_DB="/srv/data/cackle.db"

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/cackle"]
