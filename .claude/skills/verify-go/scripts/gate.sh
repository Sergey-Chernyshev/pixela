#!/usr/bin/env bash
# Deterministic Go gate for Pixela — the cheap, certain checks that must be green before agent review
# (and exactly what CI runs). Runs from apps/api. Reports every failure; exits non-zero if any fail.
#
# Usage:  bash .claude/skills/verify-go/scripts/gate.sh [packages]   (default: ./...)
set -uo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not in a git repo"; exit 2; }
API_DIR="$ROOT/apps/api"
[ -d "$API_DIR" ] || { echo "apps/api not found at $API_DIR"; exit 2; }
cd "$API_DIR" || exit 2
PKGS="${1:-./...}"

fail=0
run() { # name, command...
  local name="$1"; shift
  printf '\n=== %s ===\n' "$name"
  if "$@"; then printf '✓ %s OK\n' "$name"; else printf '✗ %s FAILED\n' "$name"; fail=1; fi
}

# gofmt: list mis-formatted files (empty = pass)
printf '\n=== gofmt ===\n'
unformatted="$(gofmt -l . 2>/dev/null)"
if [ -z "$unformatted" ]; then printf '✓ gofmt OK\n'; else printf '✗ gofmt — needs formatting:\n%s\n' "$unformatted"; fail=1; fi

run "go vet"   go vet "$PKGS"

if command -v golangci-lint >/dev/null 2>&1; then
  run "golangci-lint" golangci-lint run
else
  printf '\n=== golangci-lint ===\n✗ golangci-lint NOT INSTALLED — install the pinned version (task tools:install) and re-run\n'
  fail=1
fi

run "go build" go build "$PKGS"
run "go test -race" go test -race "$PKGS"

printf '\n========================================\n'
if [ "$fail" -eq 0 ]; then printf 'GATE: GREEN ✓\n'; else printf 'GATE: RED ✗  — fix the above before agent review\n'; fi
exit "$fail"
