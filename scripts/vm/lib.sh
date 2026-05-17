#!/usr/bin/env bash
# Shared helpers for scripts/vm/run.sh.

die() {
  printf 'scripts/vm: %s\n' "$*" >&2
  exit 1
}

# wait_for_ssh <vm-name> — poll `tart ip` until SSH on port 22 is reachable.
# Echoes the VM's IP to stdout on success; dies on 60s timeout.
wait_for_ssh() {
  local vm="$1"
  local deadline=$(( $(date +%s) + 60 ))
  local ip=""
  while [ "$(date +%s)" -lt "$deadline" ]; do
    ip=$(tart ip "$vm" 2>/dev/null || true)
    if [ -n "$ip" ]; then
      if nc -z -G 2 "$ip" 22 2>/dev/null; then
        printf '%s' "$ip"
        return 0
      fi
    fi
    sleep 1
  done
  printf 'scripts/vm: SSH not reachable on %s within 60s\n' "$vm" >&2
  printf '----- tart logs %s -----\n' "$vm" >&2
  tart logs "$vm" 2>&1 | tail -50 >&2 || true
  exit 1
}

# ssh_exec <ip> <key-file> <command> — run a shell command in the VM as admin.
# Uses a fixed -o pair so first-run host-key prompts don't hang the script.
# IdentitiesOnly=yes prevents local SSH agents from flooding MaxAuthTries.
ssh_exec() {
  local ip="$1"
  local key="$2"
  local cmd="$3"
  ssh \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -o IdentitiesOnly=yes \
    -i "$key" \
    "admin@${ip}" \
    "$cmd"
}

# sweep_leaked_vms — delete any openboot-ephemeral-* VMs left over from a
# previous run that was killed before its EXIT trap could fire.
sweep_leaked_vms() {
  local leaked
  while IFS= read -r leaked; do
    [ -z "$leaked" ] && continue
    printf 'scripts/vm: cleaning leaked VM: %s\n' "$leaked" >&2
    tart stop   "$leaked" 2>/dev/null || true
    tart delete "$leaked" 2>/dev/null || true
  done < <(tart list --format=json 2>/dev/null \
             | python3 -c 'import json,sys; [print(v["Name"]) for v in json.load(sys.stdin) if v["Name"].startswith("openboot-ephemeral-")]' 2>/dev/null || true)
}
