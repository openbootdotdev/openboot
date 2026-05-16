#!/bin/bash
set -euo pipefail

# Only run in Claude Code on the web; skip on local machines.
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-$(pwd)}"

echo "[openboot session-start] Go version: $(go version)"

# Pre-fetch and verify Go module deps so tests/builds are fast and offline-safe.
echo "[openboot session-start] Downloading Go modules..."
go mod download

# Warm the build cache so the first `go build` / `go test` is fast.
echo "[openboot session-start] Warming build cache (go build ./...)..."
go build ./... >/dev/null

# Warm the test binary cache without executing tests.
echo "[openboot session-start] Warming test cache (go test -count=0 ./...)..."
go test -count=0 ./... >/dev/null

echo "[openboot session-start] Done."
