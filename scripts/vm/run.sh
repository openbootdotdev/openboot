#!/usr/bin/env bash
# Run an in-VM make target inside an ephemeral Tart VM.
#
# Usage:
#   scripts/vm/run.sh <make-target>
#
# Env vars:
#   OPENBOOT_VM_BASE — local Tart image name to clone (default: macos-tahoe-base)
#   OPENBOOT_VM_KEEP — if "1", do not destroy the VM at exit (for debugging)
#
# Exit codes match the in-VM make target.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

[ $# -ge 1 ] || die "usage: $0 <make-target>"
TARGET="$1"

BASE="${OPENBOOT_VM_BASE:-macos-tahoe-base}"
KEEP="${OPENBOOT_VM_KEEP:-0}"
VM_TEST="${OPENBOOT_VM_TEST:-}"
VM="openboot-ephemeral-$$"

# Pre-flight — all checks before we create any disk state.
command -v tart   >/dev/null 2>&1 || die "tart not installed — brew install cirruslabs/cli/tart"
command -v rsync  >/dev/null 2>&1 || die "rsync not installed"
command -v ssh    >/dev/null 2>&1 || die "ssh not installed"

tart list --format=json 2>/dev/null \
  | python3 -c "import json,sys; sys.exit(0 if any(v['Name']=='$BASE' for v in json.load(sys.stdin)) else 1)" \
  || die "base image '$BASE' not found locally — see scripts/vm/README.md for one-time setup"

sweep_leaked_vms

# Ephemeral SSH key — injected into the VM via tart exec after boot.
# Created AFTER pre-flight so a failed check doesn't leave a private key on disk.
# Avoids interference from local SSH agents (1Password, gpg-agent, etc.)
# that exhaust the server's MaxAuthTries before a real key is tried.
SSH_KEY_DIR="$(mktemp -d)"
SSH_KEY="${SSH_KEY_DIR}/id_ed25519"
ssh-keygen -t ed25519 -f "$SSH_KEY" -N "" -q

cleanup() {
  rm -rf "$SSH_KEY_DIR" 2>/dev/null || true
  if [ -n "$VM" ] && [ "$KEEP" = "1" ]; then
    printf 'scripts/vm: OPENBOOT_VM_KEEP=1 — leaving "%s" running for debug\n' "$VM" >&2
    printf 'scripts/vm: tart ssh %s    # attach (admin/admin)\n' "$VM" >&2
    printf 'scripts/vm: tart stop %s && tart delete %s    # clean up when done\n' "$VM" "$VM" >&2
    return
  fi
  [ -n "$VM" ] && tart stop   "$VM" 2>/dev/null || true
  [ -n "$VM" ] && tart delete "$VM" 2>/dev/null || true
}
trap cleanup EXIT

# Clone VM (trap is now registered — cleanup will fire on any subsequent die())
tart clone "$BASE" "$VM"

# Boot
tart run --no-graphics "$VM" >/dev/null 2>&1 &
VM_IP=$(wait_for_ssh "$VM")
printf 'scripts/vm: VM "%s" at %s ready\n' "$VM" "$VM_IP" >&2

# Inject ephemeral SSH key via tart exec (works without prior SSH auth setup)
PUBKEY="$(cat "${SSH_KEY}.pub")"
tart exec "$VM" mkdir -p /Users/admin/.ssh
tart exec "$VM" sh -c "printf '%s\n' '$PUBKEY' >> /Users/admin/.ssh/authorized_keys"
tart exec "$VM" chmod 700 /Users/admin/.ssh
tart exec "$VM" chmod 600 /Users/admin/.ssh/authorized_keys

# Ensure Go is available via mise (the macos-tahoe-base image ships mise but
# not Go pre-activated in the default shell PATH). Both commands are idempotent.
tart exec "$VM" sh -c '/opt/homebrew/bin/mise install go@latest && /opt/homebrew/bin/mise use -g go@latest'

# Sync source
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
rsync -az --delete \
  --exclude='/.git/objects' \
  --exclude='/coverage.out' \
  --exclude='/coverage.html' \
  -e "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes -i $SSH_KEY" \
  "$REPO_ROOT/" "admin@${VM_IP}:/Users/admin/openboot/"

# Execute target inside VM — use mise exec to put Go on PATH.
# When OPENBOOT_VM_TEST is set, run only the matching tests; single-quote the
# value so that regexp metacharacters (| [ ]) survive the remote shell intact.
if [ -n "$VM_TEST" ]; then
  MAKE_CMD="test-vm-inner-run TEST='${VM_TEST}'"
else
  MAKE_CMD="${TARGET}"
fi
ssh_exec "$VM_IP" "$SSH_KEY" \
  "cd /Users/admin/openboot && CI=true OPENBOOT_IN_VM=1 /opt/homebrew/bin/mise exec go -- make ${MAKE_CMD}"
