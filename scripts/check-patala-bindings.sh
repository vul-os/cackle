#!/usr/bin/env bash
# scripts/check-patala-bindings.sh
#
# Verifies that the currently-generated patala-go bindings (a gitignored,
# locally-generated artifact — see ../patala/patala-go/.gitignore) actually
# expose the surface internal/payments/patala.go needs, BEFORE `go build`/
# `go test` gets anywhere near them.
#
# WHY THIS EXISTS
# ----------------
# internal/payments/patala.go (behind `-tags patala`) calls
# patala.PatalaFiatProviders() and patala.PatalaRailNewFiat(...). Those
# symbols only exist in the generated bindings when patala-py's cdylib was
# built with the fiat adapters compiled in (see PATALA_FEATURES at the repo
# root). A cdylib built with the default empty feature set only exports the
# Mock rail constructors — a perfectly valid, perfectly real build of
# patala-go, just not the one cackle needs.
#
# `make build-patala`/`test-patala` already force a fresh, correctly-featured
# regeneration every time (patala-generate is an unconditional prerequisite),
# so they cannot go stale. But bindings/ is a SHARED, mutable, gitignored
# directory: any OTHER work in the sibling patala checkout — someone running
# patala-go's own bare `make generate` (FEATURES defaults to empty) for an
# unrelated reason — silently overwrites it out from under cackle. The next
# bare `go build -tags patala` or `go test -tags patala` (i.e. NOT through
# this repo's Makefile) then fails with a correct but cryptic
# "undefined: patala.PatalaFiatProviders" deep in a compiler error, with no
# hint that a `make` step exists at all, let alone which one.
#
# This script is the diagnostic layer: run it any time to get a plain,
# actionable answer instead of that compiler error. It is also invoked as the
# last step of `make patala-generate`, so the make-driven path self-verifies
# its own output too (protects against uniffi-bindgen-go itself producing
# something unexpected, not just against a bypassed regeneration).
#
# Usage: scripts/check-patala-bindings.sh [patala-bindings-dir]
#   Defaults to ../patala/patala-go/bindings/patala (matches Makefile's
#   PATALA_BINDINGS with the default PATALA_DIR).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINDINGS_DIR="${1:-$ROOT/../patala/patala-go/bindings/patala}"
FEATURES_FILE="$ROOT/PATALA_FEATURES"

fail() {
  echo "check-patala-bindings: FAIL: $*" >&2
  echo "" >&2
  echo "  Regenerate with the features this repo actually needs:" >&2
  echo "    make -C '$ROOT' patala-generate" >&2
  echo "  (or by hand: cd ../patala/patala-go && make FEATURES=\"\$(grep -v '^#' '$FEATURES_FILE' | head -1)\" generate)" >&2
  exit 1
}

[ -f "$FEATURES_FILE" ] || fail "$FEATURES_FILE not found — this repo's own source of truth for required patala features is missing."
required_features="$(grep -v '^#' "$FEATURES_FILE" | grep -v '^[[:space:]]*$' | head -1)"
[ -n "$required_features" ] || fail "$FEATURES_FILE is empty — no feature set recorded."

echo "check-patala-bindings: required features (from PATALA_FEATURES): $required_features"

[ -d "$BINDINGS_DIR" ] || fail "no generated bindings at $BINDINGS_DIR — patala-go/bindings/ is gitignored build output, never checked in (see its own .gitignore), so it must be generated at least once."

generated_go_files=("$BINDINGS_DIR"/*.go)
[ -e "${generated_go_files[0]}" ] || fail "$BINDINGS_DIR exists but contains no generated .go file."

# The fiat surface is exactly these two exported UniFFI functions — the same
# ones internal/payments/patala.go calls. Checking for their presence in the
# generated source is a direct, cheap proxy for "was this cdylib built with
# the fiat features on", without needing to load the cdylib itself.
missing=()
for symbol in PatalaFiatProviders PatalaRailNewFiat; do
  if ! grep -q "^func ${symbol}(" "$BINDINGS_DIR"/*.go 2>/dev/null; then
    missing+=("$symbol")
  fi
done

if [ "${#missing[@]}" -gt 0 ]; then
  fail "generated bindings at $BINDINGS_DIR are missing: ${missing[*]}
  This is what a fiat-less regeneration (patala-go's bare 'make generate',
  FEATURES empty) produces — it only exports the Mock rail constructors.
  internal/payments/patala.go needs the fiat-all surface (see PATALA_FEATURES)."
fi

echo "check-patala-bindings: OK — fiat surface present (PatalaFiatProviders, PatalaRailNewFiat) in $BINDINGS_DIR"
