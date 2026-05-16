#!/bin/bash
set -uo pipefail

cd "${CLAUDE_PROJECT_DIR:-$(pwd)}"

# --- Stale-branch sensor (always runs, cheap, ~1s with --quiet fetch) ---
# After `ship-pr` flow / auto-merge with --delete-branch, the remote branch
# is gone but the local branch lingers. Detect and suggest cleanup so the
# next session doesn't start on an orphan branch.
if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
  current=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)
  if [ -n "$current" ] && [ "$current" != "main" ]; then
    # Prune local refs for deleted remote branches. Network call, but cheap
    # against an already-cloned repo; silenced so we don't spam the session.
    git fetch --prune --quiet origin 2>/dev/null || true
    track=$(git rev-parse --symbolic-full-name --abbrev-ref "${current}@{upstream}" 2>/dev/null || true)
    if [ -n "$track" ] && ! git show-ref --verify --quiet "refs/remotes/${track}"; then
      echo "[openboot session-start] Branch '$current' tracks '$track' — gone on remote (likely merged)." >&2
      echo "  Cleanup: git checkout main && git pull && git branch -d $current" >&2
    fi
  fi
fi

# --- Cache warming (Claude Code on the web only) ---
# Local users don't need this — their caches stay warm across sessions.
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

set -e

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
