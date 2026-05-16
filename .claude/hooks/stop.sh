#!/bin/bash
#
# Stop hook — fires when Claude finishes its turn.
#
# If any .go files in the working tree differ from HEAD, run the cheap
# end-of-turn sensors:
#   - go vet ./...      (catches obvious correctness errors)
#   - archtest          (catches new architecture-fitness violations)
#
# Skips the full L1 race test — that's pre-push's job. The point here is
# fast: <3s on a warm cache.
#
# Exit code semantics:
#   0  → allow stop
#   2  → prevent stop; stderr is fed back to Claude so it can fix
#
# Skip via OPENBOOT_SKIP_STOP_HOOK=1 if iterating.

set -uo pipefail

[ "${OPENBOOT_SKIP_STOP_HOOK:-}" = "1" ] && exit 0

cd "${CLAUDE_PROJECT_DIR:-$(pwd)}"

# Cheap gate: only fire if any tracked .go file is dirty or any untracked .go
# file exists. Skip otherwise — most turns don't touch Go.
dirty_go=$(git status --porcelain 2>/dev/null | grep -E '\.go$' | head -1 || true)
[ -z "$dirty_go" ] && exit 0

echo "[openboot stop] running end-of-turn sensors..." >&2

if ! vet_out=$(go vet ./... 2>&1); then
  printf '[openboot stop] go vet ./... failed:\n%s\n' "$vet_out" >&2
  exit 2
fi

# archtest catches new violations of project invariants (exec.Command outside
# system/, raw http outside httputil, os.Getenv("HOME"), etc.).
if ! arch_out=$(go test -count=1 ./internal/archtest/... 2>&1); then
  printf '[openboot stop] archtest failed:\n%s\n' "$arch_out" >&2
  exit 2
fi

exit 0
