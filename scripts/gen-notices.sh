#!/usr/bin/env bash
# Regenerate THIRD-PARTY-NOTICES.txt from the ACTUAL dependency graph.
#
# Cackle redistributes third-party code two ways: Go modules compiled into
# the single static binary (chi, modernc.org/sqlite, golang.org/x/crypto,
# oklog/ulid, ...) and the npm graph bundled into the embedded React UI
# (React, Vite, Tailwind, shadcn/radix primitives, ...). MIT, BSD, ISC and
# Apache-2.0 all require the copyright notice and licence text to accompany
# every copy — this script reproduces those from the real module/package
# graphs rather than hand-maintaining a list that drifts.
#
#     ./scripts/gen-notices.sh           regenerate THIRD-PARTY-NOTICES.txt
#     ./scripts/gen-notices.sh --check   exit 1 if the committed file is stale
#
# THIRD-PARTY-NOTICES.txt is committed at the repo root. It is NOT
# hand-maintained — re-run this after changing go.mod or web/package.json.
# --check resolves the same real dependency graph and diffs it against the
# committed file WITHOUT writing anything — the working tree is left exactly
# as it was found, pass or fail. That is the mode CI runs: this file is a
# legal-compliance artifact (licence attribution), and unlike source code
# nothing else notices when it silently falls out of sync with a dependency
# bump, so the check has to be a real diff of the real graph, not a lint.
#
# Degrades gracefully for ENVIRONMENT problems: if web/ has no package.json
# yet (frontend not scaffolded), or a tool can't be installed (no network),
# that section is skipped with an explanatory note rather than failing the
# whole run — this script must never be the reason `make check` is red for
# reasons unrelated to licensing.
#
# It does NOT degrade for LICENSING problems, and the difference is the whole
# point of the distinction. A dependency whose licence cannot be determined is
# not an environment hiccup: it means this binary redistributes code whose
# attribution requirements are unknown, and writing a notices file that simply
# omits the entire Go section would produce a shorter, cleaner, WRONG file that
# looks exactly like a correct one. Attribution is a redistribution obligation,
# not a nice-to-have, so that case exits non-zero and leaves the committed
# file untouched. Fix the dependency (or record an override with evidence —
# see scripts/notices/go-licence-overrides.json) and re-run.
set -uo pipefail

# Exit code 4 means "the graph resolved but a module's licence is unknown" —
# distinct from a tool/network failure, which is still a soft skip.
#
# ── Overrides, and the rule for adding one ──────────────────────────────────
# scripts/notices/go-licence-overrides.json, if present, is passed to
# go-licence-detector as -overrides. Its format is ONE JSON OBJECT PER LINE
# (not a JSON document, and it tolerates no comments):
#
#   {"name": "example.com/mod", "licenceType": "Apache-2.0", "url": "https://..."}
#
# Record EVIDENCE, not a guess. An override is an assertion about someone
# else's licence terms, shipped to everyone who receives this binary, and a
# wrong entry is worse than a missing one because it looks authoritative.
# Acceptable evidence: a LICENSE file in the upstream repository at the pinned
# version, a clear SPDX identifier there, or a written statement from the
# copyright holder. Cite it in the commit message, since the file itself
# cannot carry comments.
#
# ── KNOWN GAP, unresolved as of this writing ────────────────────────────────
# github.com/vul-os/kotva/bindings/go@v0.2.1 — the shared DMTAP sync engine,
# a direct dependency of internal/scan/substrate — ships NO licence file:
#
#   unzip -l "$(go env GOMODCACHE)"/cache/download/github.com/vul-os/kotva/bindings/go/@v/v0.2.1.zip \
#     | grep -i licen        → no matches
#
# and there is no SPDX identifier or licence statement in its README.md, its
# go.mod, or any source file in the module. Its terms therefore cannot be
# established from what it distributes, and NO override is recorded — assuming
# it matches Cackle's own "MIT OR Apache-2.0" would be an assertion with
# nothing behind it.
#
# Consequence, stated plainly: this script exits 4 and
# THIRD-PARTY-NOTICES.txt cannot be regenerated until the licence is
# established. That is a visible blocker instead of a notices file that
# quietly stopped listing every Go dependency. The fix belongs upstream —
# publish a LICENSE file in the kotva repository and it is picked up
# automatically, at which point this whole note can be deleted.
EXIT_UNKNOWN_LICENCE=4

CHECK=0
for arg in "$@"; do
  case "$arg" in
    --check) CHECK=1 ;;
    *)
      echo "gen-notices.sh: unrecognised argument: $arg" >&2
      echo "usage: gen-notices.sh [--check]" >&2
      exit 2
      ;;
  esac
done

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

OUT="THIRD-PARTY-NOTICES.txt"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
# Always resolve into a scratch file first. In --check mode this candidate is
# only ever diffed against the committed $OUT, never copied over it — the
# generate path below is the sole place $OUT is written.
CANDIDATE="$TMP/THIRD-PARTY-NOTICES.candidate.txt"

if [[ "$CHECK" -eq 1 ]]; then
  echo "==> checking $OUT against the actual dependency graph (no changes will be written)"
else
  echo "==> generating $OUT"
fi

# ── Go modules ────────────────────────────────────────────────────────────────
GO_NOTICES="$TMP/go-notices.txt"
GO_STATUS="skipped (no go.mod)"
if [[ -f go.mod ]]; then
  DETECTOR="$(command -v go-licence-detector || true)"
  if [[ -z "$DETECTOR" ]]; then
    echo "==> installing go.elastic.co/go-licence-detector@v0.10.0"
    if GOBIN="$TMP/bin" go install go.elastic.co/go-licence-detector@v0.10.0 2>"$TMP/go-install.err"; then
      DETECTOR="$TMP/bin/go-licence-detector"
    else
      echo "    could not install go-licence-detector (no network?) — skipping Go section"
      cat "$TMP/go-install.err" >&2 || true
    fi
  fi

  if [[ -n "$DETECTOR" ]]; then
    echo "==> resolving Go module graph"
    OVERRIDES=()
    [[ -f scripts/notices/go-licence-overrides.json ]] && \
      OVERRIDES=(-overrides scripts/notices/go-licence-overrides.json)

    if go mod download 2>"$TMP/go-mod.err" \
      && go list -m -json all 2>>"$TMP/go-mod.err" | "$DETECTOR" \
           -includeIndirect \
           -rules scripts/notices/go-licence-rules.json \
           "${OVERRIDES[@]+"${OVERRIDES[@]}"}" \
           -noticeTemplate scripts/notices/go-modules.tmpl \
           -noticeOut "$GO_NOTICES" 2>>"$TMP/go-mod.err"
    then
      GO_STATUS="ok"
    else
      echo "    go module notice generation failed:" >&2
      cat "$TMP/go-mod.err" >&2 || true

      # An undetectable licence is a licensing defect, not an environment one.
      # Refuse rather than write a notices file that silently drops every Go
      # module — the resulting file would be shorter, valid-looking, and
      # missing attribution this binary is obliged to reproduce.
      if grep -qi "no licence file found\|no license file found" "$TMP/go-mod.err"; then
        cat >&2 <<'UNKNOWN'

    REFUSING to write THIRD-PARTY-NOTICES.txt.

    A Go module in the graph ships no licence file, so its attribution
    requirements are unknown. Omitting the whole Go section to get a green run
    would produce a file that looks complete and is not — and attribution is a
    condition of redistributing this code, not an optional extra.

    To resolve, either:
      * get the upstream module to ship a LICENSE file (the correct fix), or
      * record it in scripts/notices/go-licence-overrides.json WITH the
        evidence for the licence you are asserting. Do not guess: asserting a
        licence on someone else's behalf is worse than an incomplete file.

    The committed THIRD-PARTY-NOTICES.txt has been left untouched.
UNKNOWN
        exit "$EXIT_UNKNOWN_LICENCE"
      fi

      GO_STATUS="failed (see stderr above) — section omitted"
      : > "$GO_NOTICES"
    fi
  else
    : > "$GO_NOTICES"
  fi
else
  : > "$GO_NOTICES"
fi

# ── npm packages (web/) ───────────────────────────────────────────────────────
NPM_NOTICES="$TMP/npm-notices.txt"
NPM_STATUS="skipped (web/package.json not present yet)"
if [[ -f web/package.json ]]; then
  echo "==> resolving npm dependency graph (web/)"
  if [[ ! -d web/node_modules ]]; then
    ( cd web && npm ci --no-audit --no-fund 2>"$TMP/npm-install.err" ) || \
      ( cd web && npm install --no-audit --no-fund 2>>"$TMP/npm-install.err" )
  fi
  if npx --yes license-checker-rseidelsohn \
      --production --json --excludePrivatePackages --start web \
      2>"$TMP/npm-lc.err" | node scripts/notices/npm-notices.mjs > "$NPM_NOTICES" 2>>"$TMP/npm-lc.err"
  then
    NPM_STATUS="ok"
  else
    echo "    npm notice generation failed:" >&2
    cat "$TMP/npm-lc.err" >&2 || true
    NPM_STATUS="failed (see stderr above) — section omitted"
    : > "$NPM_NOTICES"
  fi
else
  : > "$NPM_NOTICES"
fi

# ── Compose ───────────────────────────────────────────────────────────────────
{
  cat <<HEADER
================================================================================
Cackle — THIRD-PARTY NOTICES
================================================================================

This file is generated by scripts/gen-notices.sh from the actual dependency
graph. Do not edit it by hand.

Cackle is distributed under the MIT licence (see LICENSE). It bundles the
third-party software listed below. Each entry gives the component name, its
version, the licence it is distributed under, and the full text of that
licence, as those licences require.

Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
Go modules section : ${GO_STATUS}
npm packages section: ${NPM_STATUS}

HEADER

  if [[ -s "$NPM_NOTICES" ]]; then
    echo "================================================================================"
    echo "npm packages (bundled into the Cackle web UI)"
    echo "================================================================================"
    echo
    cat "$NPM_NOTICES"
  fi

  echo "================================================================================"
  echo "Go standard library (compiled into the cackle binary)"
  echo "================================================================================"
  echo
  echo "--------------------------------------------------------------------------------"
  echo "Component : The Go Programming Language standard library"
  echo "Version   : $(go env GOVERSION 2>/dev/null || echo unknown)"
  echo "Licence   : BSD-3-Clause (plus the Go patent grant, reproduced below)"
  echo "--------------------------------------------------------------------------------"
  echo
  if GOROOT_DIR="$(go env GOROOT 2>/dev/null)" && [[ -f "$GOROOT_DIR/LICENSE" ]]; then
    cat "$GOROOT_DIR/LICENSE"
    echo
    [[ -f "$GOROOT_DIR/PATENTS" ]] && cat "$GOROOT_DIR/PATENTS"
    echo
  fi

  if [[ -s "$GO_NOTICES" ]]; then
    echo "================================================================================"
    echo "Go modules (compiled into the cackle binary)"
    echo "================================================================================"
    echo
    cat "$GO_NOTICES"
  fi
} > "$CANDIDATE"

if [[ "$CHECK" -eq 1 ]]; then
  # The "Generated:" timestamp is expected to differ on every single run —
  # it is not dependency drift, it is the clock. Redact it (in both sides of
  # the comparison) before diffing so --check reports real graph changes
  # only; a real `notices` run still writes the true timestamp verbatim.
  redact() { sed 's/^Generated: .*/Generated: <redacted>/'; }

  if [[ ! -f "$OUT" ]]; then
    echo "==> $OUT does not exist — run 'npm run notices' to create it" >&2
    exit 1
  fi

  if diff -u <(redact <"$OUT") <(redact <"$CANDIDATE") >"$TMP/notices.diff"; then
    echo "==> $OUT matches the actual dependency graph — up to date"
    exit 0
  else
    echo "==> $OUT is OUT OF DATE with the actual dependency graph:" >&2
    echo >&2
    cat "$TMP/notices.diff" >&2
    echo >&2
    echo "    Run: npm run notices   (or: ./scripts/gen-notices.sh), then commit the result." >&2
    exit 1
  fi
fi

cp "$CANDIDATE" "$OUT"

echo "==> wrote $OUT ($(wc -l < "$OUT" | tr -d ' ') lines)"
