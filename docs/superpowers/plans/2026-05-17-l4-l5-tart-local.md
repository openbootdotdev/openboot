# L4/L5 Destructive E2E → Local Tart VM Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move destructive e2e (current L4 + L5) off GitHub Actions `macos-latest` and into a local Tart VM, driven by `scripts/vm/run.sh`. Collapse to one tier. No CI gate — running it before release is convention, not enforcement.

**Architecture:** A bash entrypoint (`scripts/vm/run.sh`) clones a pre-existing local Tart base image into a `$$`-named ephemeral VM, boots it, rsyncs the working tree in, SSH-execs `make test-vm-inner` inside it, then tears down via an EXIT trap. The 12 `test/e2e/*.go` files run unchanged — `testutil/MacHost` already assumes "I'm on a throwaway macOS host." The Tart VM is now that host.

**Tech Stack:** Bash + `tart` CLI (cirruslabs) + `rsync` + `ssh`. Go test runner inside the VM. Tart base image: `ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest`, pulled locally once and named `macos-tahoe-base`.

**Spec:** `docs/superpowers/specs/2026-05-17-l4-l5-tart-local-design.md`

---

## File Structure

**New files:**
- `scripts/vm/run.sh` — entrypoint, called by Makefile, ~80 lines
- `scripts/vm/lib.sh` — shared functions (`die`, `wait_for_ssh`, `ssh_exec`, `sweep_leaked_vms`), ~50 lines
- `scripts/vm/README.md` — base-image setup + env vars + troubleshooting, ~60 lines

**Modified files:**
- `Makefile` — add `test-vm`, `test-vm-run`, `test-vm-inner`, `test-vm-inner-run`; later delete `test-vm-quick`, `test-vm-release`, `test-vm-full`, `test-vm-run` (old), `test-destructive`, `test-smoke`, `test-smoke-prebuilt`; rewrite header comment block at lines 41-51
- `testutil/machost.go` — comment rewrite + `requireEphemeralHost` gains `OPENBOOT_IN_VM` branch
- `test/e2e/real_install_test.go` — flip `//go:build e2e && destructive` → `//go:build e2e && vm`
- `test/e2e/smoke_test.go` — flip `//go:build e2e && destructive && smoke` → `//go:build e2e && vm`
- `.github/workflows/test.yml` — delete `macos-e2e` job (lines 142-158), `destructive` job (lines 160-174), `run_destructive` workflow_dispatch input (lines 15-20)
- `.github/workflows/release.yml` — delete `Destructive tests` step (lines 67-68), delete `smoke-test` job (lines 104-148), remove `smoke-test` from `release` job's `needs` (line 151)
- `.github/workflows/smoke-test.yml` — delete entire file
- `.github/workflows/auto-release.yml` — split feat-threshold from patch fast lane: open issue on feat, keep auto-tag on patch
- `CONTRIBUTING.md` — Test Layering table, new "VM E2E setup" section
- `CLAUDE.md` — Commands table row
- `docs/HARNESS.md` — table rows merge, new "What's intentionally NOT in the harness" entry

---

## Task 1: Add Tart driver layer (scripts/vm/) and new Makefile targets

This task is additive. Old `test-vm-*` / `test-destructive` / `test-smoke` Makefile targets stay until Task 5. New targets coexist with them.

**Files:**
- Create: `scripts/vm/run.sh`
- Create: `scripts/vm/lib.sh`
- Create: `scripts/vm/README.md`
- Modify: `Makefile` (add four targets to `.PHONY` line, add four target stanzas at end of file)

### Step 1.1: Create scripts/vm/lib.sh

- [ ] Write the file:

```bash
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

# ssh_exec <ip> <command> — run a shell command in the VM as admin.
# Uses a fixed -o pair so first-run host-key prompts don't hang the script.
ssh_exec() {
  local ip="$1"
  local cmd="$2"
  ssh \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
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
```

- [ ] Make it executable: `chmod +x scripts/vm/lib.sh`

### Step 1.2: Create scripts/vm/run.sh

- [ ] Write the file:

```bash
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
VM="openboot-ephemeral-$$"

# Pre-flight
command -v tart   >/dev/null 2>&1 || die "tart not installed — brew install cirruslabs/cli/tart"
command -v rsync  >/dev/null 2>&1 || die "rsync not installed"
command -v ssh    >/dev/null 2>&1 || die "ssh not installed"

tart list --format=json 2>/dev/null \
  | python3 -c "import json,sys; sys.exit(0 if any(v['Name']=='$BASE' for v in json.load(sys.stdin)) else 1)" \
  || die "base image '$BASE' not found locally — see scripts/vm/README.md for one-time setup"

sweep_leaked_vms

# Clone + register cleanup
tart clone "$BASE" "$VM"

cleanup() {
  if [ "$KEEP" = "1" ]; then
    printf 'scripts/vm: OPENBOOT_VM_KEEP=1 — leaving "%s" running for debug\n' "$VM" >&2
    printf 'scripts/vm: tart ssh %s  # to attach\n' "$VM" >&2
    printf 'scripts/vm: tart stop %s && tart delete %s  # to clean up\n' "$VM" "$VM" >&2
    return
  fi
  tart stop   "$VM" 2>/dev/null || true
  tart delete "$VM" 2>/dev/null || true
}
trap cleanup EXIT

# Boot
tart run --no-graphics "$VM" >/dev/null 2>&1 &
VM_IP=$(wait_for_ssh "$VM")
printf 'scripts/vm: VM "%s" at %s ready\n' "$VM" "$VM_IP" >&2

# Sync source
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
rsync -az --delete \
  --exclude='/.git/objects' \
  --exclude='/openboot' \
  --exclude='/coverage.out' \
  --exclude='/coverage.html' \
  -e 'ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR' \
  "$REPO_ROOT/" "admin@${VM_IP}:/Users/admin/openboot/"

# Execute target inside VM
ssh_exec "$VM_IP" "cd /Users/admin/openboot && CI=true OPENBOOT_IN_VM=1 make ${TARGET}"
```

- [ ] Make it executable: `chmod +x scripts/vm/run.sh`

### Step 1.3: Create scripts/vm/README.md

- [ ] Write the file:

````markdown
# Tart VM e2e

The `scripts/vm/run.sh` driver spins up an ephemeral Tart VM for the
destructive e2e suite (`make test-vm`). The 12 test files in `test/e2e/`
run inside it.

## One-time setup

Tart only runs on Apple Silicon Macs.

```bash
# 1. Install Tart (https://tart.run)
brew install cirruslabs/cli/tart

# 2. Pull the base image (downloads ~25GB; use whichever mirror is fastest)
tart pull ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest

# 3. Give it the local name run.sh expects
tart clone ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest macos-tahoe-base
```

Verify with `tart list` — you should see `macos-tahoe-base`.

## Running

```bash
make test-vm                                          # full suite (~30 min)
make test-vm-run TEST=TestVM_Journey_FirstTimeUser    # one test
```

## Environment variables

| Var | Default | Effect |
|---|---|---|
| `OPENBOOT_VM_BASE` | `macos-tahoe-base` | Local Tart image to clone from |
| `OPENBOOT_VM_KEEP` | `0` | When `1`, do not destroy the VM at exit. Useful for debugging — attach with `tart ssh openboot-ephemeral-<pid>`. Remember to clean up manually. |

## Troubleshooting

- **`base image 'macos-tahoe-base' not found locally`** — run the one-time
  setup above.
- **SSH not reachable within 60s** — Tart logs dumped to stderr.
  Try `OPENBOOT_VM_KEEP=1 make test-vm` (it'll still fail at boot, but the
  VM is left running so you can `tart ssh openboot-ephemeral-<pid>` and
  see what's up).
- **Leaked VMs after a hard kill** — run.sh sweeps `openboot-ephemeral-*`
  on next start, or remove them manually:

  ```bash
  tart list | awk '/openboot-ephemeral-/{print $2}' | xargs -I{} sh -c 'tart stop {} ; tart delete {}'
  ```

- **Base image needs updating** — re-pull and re-clone:

  ```bash
  tart delete macos-tahoe-base
  tart pull ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest
  tart clone ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest macos-tahoe-base
  ```

## Why a base image (not vanilla)

`-base` includes Xcode CLI tools, Homebrew, and Go pre-installed. openboot
*does* install Homebrew, so "fresh Mac" purists would prefer
`macos-tahoe-vanilla`. We pick `-base` because:

1. Boot-to-test latency drops from ~5 min (install brew + Go) to ~30s.
2. `MacHost.Run("brew install ...")` exercises the "already-installed
   brew" code path, which is the more common real-world scenario (users
   running openboot a second time, or after `brew install` ran for some
   other reason).
3. Tests that specifically need to exercise the "no brew yet" path can
   uninstall it as the first step.
````

### Step 1.4: Add new Makefile targets

- [ ] Read the current `.PHONY` line:

```bash
sed -n '1,4p' Makefile
```

Expected: shows the `.PHONY` line listing existing targets.

- [ ] Append the four new targets to the `.PHONY` line and append new target stanzas at end of file. Edit `Makefile`:

Old (`Makefile:1-3`):
```makefile
.PHONY: test-unit test-e2e test-destructive test-smoke test-smoke-prebuilt test-coverage test-all \
       test-vm test-vm-run test-vm-quick test-vm-release test-vm-full \
       install-hooks uninstall-hooks
```

New:
```makefile
.PHONY: test-unit test-e2e test-destructive test-smoke test-smoke-prebuilt test-coverage test-all \
       test-vm test-vm-run test-vm-quick test-vm-release test-vm-full \
       test-vm-inner test-vm-inner-run \
       install-hooks uninstall-hooks
```

(Note: `test-vm` and `test-vm-run` are already in `.PHONY` because the old aliases use those names. After Task 5 those old aliases are removed and the new targets — which use the same names — take their place. For Task 1 we coexist: rename the existing `test-vm` alias to `test-vm-OLD-DELETE-ME` temporarily so the new `test-vm` can compile alongside.)

Concretely, in `Makefile:67-68` find:
```makefile
# Aliases
test-vm: test-vm-release
```

Replace with:
```makefile
# Old alias — kept temporarily, deleted in Task 5. Renamed so the new
# Tart-driven `test-vm` below can coexist during the migration.
test-vm-OLD-DELETE-ME: test-vm-release
```

Then append at end of file (after the existing `uninstall-hooks` stanza):

```makefile

# =============================================================================
# Tart VM e2e — new entrypoints (the old test-vm-* targets above are deprecated
# and removed in the next phase). See scripts/vm/README.md for setup.
# =============================================================================

# Developer-facing: provisions a Tart VM and runs the full e2e suite inside.
test-vm: build
	scripts/vm/run.sh test-vm-inner

# Developer-facing: runs one named test inside a Tart VM.
test-vm-run: build
	scripts/vm/run.sh "test-vm-inner-run TEST=$(TEST)"

# In-VM: invoked over SSH by run.sh — not called by developers directly.
test-vm-inner:
	go test -v -timeout 60m -tags="e2e,vm" ./test/e2e/...

test-vm-inner-run:
	go test -v -timeout 45m -tags="e2e,vm" -run $(TEST) ./test/e2e/...
```

Note: the existing `test-vm:` target on line 68 (the old alias) must be renamed first or the new `test-vm:` will conflict.

### Step 1.5: Smoke-test the driver — happy path

- [ ] Confirm Tart is installed and the base image exists:

```bash
command -v tart && tart list | grep macos-tahoe-base
```

Expected: tart found, `macos-tahoe-base` listed. If missing, do the one-time setup from `scripts/vm/README.md` before continuing.

- [ ] Run a fast in-VM target to validate plumbing without doing real installs:

```bash
scripts/vm/run.sh "go version"
```

Wait — `run.sh` expects a make target, not an arbitrary command. So instead, temporarily add a no-op target to Makefile and run it. Skip this and use the actual target:

```bash
# This invokes `make test-vm-inner-run TEST=TestVM_Infra` inside the VM —
# TestVM_Infra is the cheapest test in the suite (host-arch check only).
make test-vm-run TEST=TestVM_Infra
```

Expected: VM clones in ~1s, boots in ~30s, rsync transfers in a few seconds, test runs and passes, VM is deleted at the end. Total ~1-2 min.

- [ ] Verify the VM was cleaned up:

```bash
tart list | grep openboot-ephemeral
```

Expected: no output.

### Step 1.6: Smoke-test the driver — failure paths

- [ ] Test the `OPENBOOT_VM_KEEP=1` debug knob:

```bash
OPENBOOT_VM_KEEP=1 make test-vm-run TEST=TestVM_Infra
```

Expected: test passes, message at end says "leaving 'openboot-ephemeral-<pid>' running for debug". Confirm:

```bash
tart list | grep openboot-ephemeral
```

Expected: one ephemeral VM listed. Clean it up:

```bash
VM=$(tart list | awk '/openboot-ephemeral-/{print $2}' | head -1)
tart stop "$VM" && tart delete "$VM"
```

- [ ] Test the leaked-VM sweep:

```bash
# Simulate a leaked VM
tart clone macos-tahoe-base openboot-ephemeral-99999
# Run again — sweep should remove it before clone
make test-vm-run TEST=TestVM_Infra
```

Expected: message "cleaning leaked VM: openboot-ephemeral-99999" before clone. Test passes. After:

```bash
tart list | grep openboot-ephemeral
```

Expected: no output.

### Step 1.7: Commit

- [ ] Stage and commit:

```bash
git add scripts/vm/ Makefile
git commit -m "feat(test): add Tart VM driver for destructive e2e

scripts/vm/run.sh provisions an ephemeral Tart VM, rsyncs the working
tree in, runs an in-VM make target over SSH, and tears down on EXIT.
New Makefile targets test-vm / test-vm-run / test-vm-inner /
test-vm-inner-run plumb this through.

Old destructive targets (test-vm-quick/release/full, test-destructive,
test-smoke) stay for now — removed in a follow-up after build tags
collapse. See docs/superpowers/specs/2026-05-17-l4-l5-tart-local-design.md."
```

---

## Task 2: Collapse build tags (destructive → vm)

Two test files carry the `destructive` tag. After this task, `e2e,destructive` is retired — everything destructive runs under `e2e,vm` inside the Tart VM.

**Files:**
- Modify: `test/e2e/real_install_test.go:1`
- Modify: `test/e2e/smoke_test.go:1`

### Step 2.1: Flip real_install_test.go tag

- [ ] Edit `test/e2e/real_install_test.go` line 1:

Old:
```go
//go:build e2e && destructive
```

New:
```go
//go:build e2e && vm
```

### Step 2.2: Flip smoke_test.go tag

- [ ] Edit `test/e2e/smoke_test.go` line 1:

Old:
```go
//go:build e2e && destructive && smoke
```

New:
```go
//go:build e2e && vm
```

### Step 2.3: Verify compilation under e2e,vm

- [ ] From repo root, vet the e2e package under the new tag combo:

```bash
go vet -tags="e2e,vm" ./test/e2e/...
```

Expected: no output (or only warnings unrelated to this change). If errors, the file uses an import gated by the old tag — investigate.

- [ ] List which tests now match `e2e,vm` to confirm the merge:

```bash
go test -tags="e2e,vm" -list '.*' ./test/e2e/... 2>&1 | grep -E '^Test' | wc -l
```

Expected: a number greater than before the change (since the two formerly-destructive files now contribute tests under `e2e,vm`).

### Step 2.4: Run inside VM to confirm the merged set still works

- [ ] Run a representative test from each formerly-destructive file inside the VM:

```bash
# A test that came from real_install_test.go:
make test-vm-run TEST=TestRealInstall_BasicFlow
```

(If `TestRealInstall_BasicFlow` doesn't exist, pick another test name from `real_install_test.go` — `grep -E '^func Test' test/e2e/real_install_test.go`.)

Expected: test runs to completion (pass or fail on its merits — what matters is that it executes, proving the tag flip worked).

### Step 2.5: Commit

- [ ] Stage and commit:

```bash
git add test/e2e/real_install_test.go test/e2e/smoke_test.go
git commit -m "test(e2e): collapse destructive tag into vm tag

After Task 1 introduced the Tart VM driver, every destructive test
runs inside an ephemeral VM — there is no longer a meaningful
'destructive vs vm' distinction. Merge into a single e2e,vm tag.

The e2e,destructive build tag is retired and unused after this commit."
```

---

## Task 3: Update testutil/machost.go

`requireEphemeralHost` adds an `OPENBOOT_IN_VM` branch so tests pass the gate when invoked by `scripts/vm/run.sh` (which sets that var). Comment header is rewritten to reflect the new reality.

**Files:**
- Modify: `testutil/machost.go:1-10` (top-of-file comment)
- Modify: `testutil/machost.go:93-95` (CopyFile comment)
- Modify: `testutil/machost.go:113-119` (requireEphemeralHost body)

### Step 3.1: Rewrite top-of-file comment

- [ ] Edit `testutil/machost.go` lines 1-10 (the `//go:build` line + comment block):

Old:
```go
//go:build e2e && vm

// MacHost runs destructive openboot E2E tests directly against the current
// macOS host — no Tart VM, no SSH. It's intended for ephemeral CI runners
// (GitHub Actions macos-latest) that can be thrown away after the run.
//
// A host refuses to activate unless CI=true or OPENBOOT_E2E_DESTRUCTIVE=1
// is set, so `go test -tags="e2e,vm"` on a developer machine is a no-op
// rather than a foot-gun.

package testutil
```

New:
```go
//go:build e2e && vm

// MacHost runs destructive openboot E2E tests directly against the
// current macOS host — typically an ephemeral Tart VM provisioned by
// scripts/vm/run.sh. The driver SSHs in, rsyncs the source, and invokes
// `make test-vm-inner`; MacHost then executes commands against the VM
// it's already running inside.
//
// A host refuses to activate unless OPENBOOT_IN_VM=1 (set by run.sh) or
// the legacy CI=true / OPENBOOT_E2E_DESTRUCTIVE=1 envs are set — so
// `go test -tags="e2e,vm"` on a bare developer machine is a no-op
// rather than a foot-gun.

package testutil
```

### Step 3.2: Rewrite CopyFile comment

- [ ] Edit `testutil/machost.go` lines 93-94:

Old:
```go
// CopyFile copies a local file to a destination path on the same host.
// Preserved for API compatibility with the old Tart-based helper.
```

New:
```go
// CopyFile copies a local file to a destination path on the same host.
```

### Step 3.3: Add OPENBOOT_IN_VM branch to requireEphemeralHost

- [ ] Edit `testutil/machost.go` lines 113-119:

Old:
```go
func requireEphemeralHost(t *testing.T) {
	t.Helper()
	if os.Getenv("CI") == "true" || os.Getenv("OPENBOOT_E2E_DESTRUCTIVE") == "1" {
		return
	}
	t.Skip("destructive macOS E2E tests require CI=true or OPENBOOT_E2E_DESTRUCTIVE=1")
}
```

New:
```go
func requireEphemeralHost(t *testing.T) {
	t.Helper()
	if os.Getenv("OPENBOOT_IN_VM") == "1" {
		return
	}
	if os.Getenv("CI") == "true" || os.Getenv("OPENBOOT_E2E_DESTRUCTIVE") == "1" {
		return
	}
	t.Skip("destructive macOS E2E tests require running inside scripts/vm/run.sh (or set OPENBOOT_E2E_DESTRUCTIVE=1)")
}
```

### Step 3.4: Verify the file compiles under e2e,vm

- [ ] Vet the file:

```bash
go vet -tags="e2e,vm" ./testutil/...
```

Expected: no output.

### Step 3.5: Run a test inside the VM to confirm the gate still opens

- [ ] Run a single test that uses `MacHost`:

```bash
make test-vm-run TEST=TestVM_Infra
```

Expected: test enters `MacHost` (not skipped) and runs. The previous task already verified this — repeating after the machost.go edit confirms `OPENBOOT_IN_VM=1` (set by run.sh) is recognized.

- [ ] Confirm the gate still **blocks** on a bare developer machine. From the repo root **on the host (not inside the VM)**:

```bash
go test -tags="e2e,vm" -run TestVM_Infra -count=1 ./test/e2e/...
```

Expected: test reports `SKIP: destructive macOS E2E tests require running inside scripts/vm/run.sh (or set OPENBOOT_E2E_DESTRUCTIVE=1)`. This proves the gate works.

### Step 3.6: Commit

- [ ] Stage and commit:

```bash
git add testutil/machost.go
git commit -m "test(machost): recognize OPENBOOT_IN_VM as ephemeral-host signal

scripts/vm/run.sh sets OPENBOOT_IN_VM=1 over SSH when it invokes the
in-VM make target. requireEphemeralHost now accepts that as a more
precise signal than CI=true (which leaks in from any GHA runner, not
just throwaway ones). CI=true and OPENBOOT_E2E_DESTRUCTIVE=1 stay as
fallbacks for ad-hoc/legacy use.

Comment block at top of file rewritten to drop the obsolete 'no Tart
VM, no SSH' description."
```

---

## Task 4: Delete CI destructive jobs

Remove `macos-e2e` + `destructive` from `test.yml`, the destructive step + smoke job from `release.yml`, and the standalone `smoke-test.yml`. No machine gate replaces them — running L4 is a documented expectation, not enforced.

**Files:**
- Modify: `.github/workflows/test.yml` (delete lines 15-20 input, 142-174 jobs)
- Modify: `.github/workflows/release.yml` (delete lines 67-68 step, 104-148 smoke-test job, edit line 151 `needs`)
- Delete: `.github/workflows/smoke-test.yml`

### Step 4.1: Edit test.yml — delete the `run_destructive` workflow_dispatch input

- [ ] Edit `.github/workflows/test.yml` lines 14-20:

Old:
```yaml
  workflow_dispatch:
    inputs:
      run_destructive:
        description: 'Run destructive (real install) tests'
        required: false
        type: boolean
        default: false
```

New:
```yaml
  workflow_dispatch:
```

### Step 4.2: Edit test.yml — delete the `macos-e2e` and `destructive` jobs

- [ ] Edit `.github/workflows/test.yml` lines 142-175 (delete the `macos-e2e` and `destructive` job blocks entirely; the `cli-compat` job at line 176 stays).

Concretely, delete this entire block:

```yaml
  macos-e2e:
    name: macos e2e (L4)
    runs-on: macos-latest
    # Only on release tags or manual dispatch — these are slow and destructive.
    if: startsWith(github.ref, 'refs/tags/v') || github.event_name == 'workflow_dispatch'
    timeout-minutes: 45
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: "go.mod"

      - name: Run macOS E2E (release tier)
        run: make test-vm-release

  destructive:
    name: destructive (L5)
    runs-on: macos-latest
    if: ${{ inputs.run_destructive == true }}
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: "go.mod"

      - name: Run destructive tests
        run: make test-destructive

```

The `cli-compat` job stays.

### Step 4.3: Verify test.yml is valid YAML

- [ ] Parse the file:

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))" && echo OK
```

Expected: `OK`.

- [ ] Verify job names remaining:

```bash
python3 -c "import yaml; d=yaml.safe_load(open('.github/workflows/test.yml')); print(list(d['jobs'].keys()))"
```

Expected output: `['lint', 'unit', 'contract', 'curl-bash-smoke', 'cli-compat']` (no `macos-e2e`, no `destructive`).

### Step 4.4: Edit release.yml — remove the destructive step from gate-tests

- [ ] Edit `.github/workflows/release.yml` lines 64-68. Delete the `Destructive tests` step (lines 67-68 in current file):

Old:
```yaml
      - name: Unit tests
        run: make test-unit

      - name: Destructive tests
        run: make test-destructive
```

New:
```yaml
      - name: Unit tests
        run: make test-unit
```

### Step 4.5: Edit release.yml — delete the smoke-test job

- [ ] Edit `.github/workflows/release.yml` lines 104-148. Delete the entire `smoke-test` job block:

```yaml
  smoke-test:
    needs: [detect-release-type, build]
    runs-on: macos-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'

      - name: Download arm64 artifact
        uses: actions/download-artifact@v4
        with:
          name: openboot-darwin-arm64
          path: artifacts

      - name: Prepare release binary for testing
        run: |
          chmod +x artifacts/openboot-darwin-arm64
          cp artifacts/openboot-darwin-arm64 ./openboot

      - name: Verify binary version string
        run: |
          VERSION_OUTPUT=$(./openboot version 2>&1 || true)
          echo "Binary version output: ${VERSION_OUTPUT}"
          EXPECTED="${{ needs.detect-release-type.outputs.version_clean }}"
          if [[ "${VERSION_OUTPUT}" != *"${EXPECTED}"* ]]; then
            echo "ERROR: Binary version '${VERSION_OUTPUT}' does not contain expected '${EXPECTED}'"
            exit 1
          fi

      - name: Run smoke tests against release binary
        run: make test-smoke-prebuilt

      - name: Upload snapshot artifact
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: smoke-test-results
          path: |
            /tmp/openboot-smoke-*.json
          retention-days: 7
          if-no-files-found: ignore

```

### Step 4.6: Edit release.yml — fix the `release` job's `needs`

- [ ] Edit `.github/workflows/release.yml` line 151 (the `release` job's `needs` clause now references the deleted `smoke-test`):

Old:
```yaml
  release:
    needs: [detect-release-type, build, smoke-test]
```

New:
```yaml
  release:
    needs: [detect-release-type, build]
```

### Step 4.7: Verify release.yml is valid YAML and job graph is intact

- [ ] Parse and print the dependency graph:

```bash
python3 -c "
import yaml
d = yaml.safe_load(open('.github/workflows/release.yml'))
for job, cfg in d['jobs'].items():
    needs = cfg.get('needs', [])
    print(f'  {job} ← {needs}')
"
```

Expected output (no `smoke-test`):
```
  detect-release-type ← []
  gate-tests ← detect-release-type
  build ← ['detect-release-type', 'gate-tests']
  release ← ['detect-release-type', 'build']
  update-homebrew-tap ← ['detect-release-type', 'release']
```

### Step 4.8: Delete smoke-test.yml

- [ ] Delete the file:

```bash
git rm .github/workflows/smoke-test.yml
```

### Step 4.9: Commit

- [ ] Stage and commit:

```bash
git add .github/workflows/test.yml .github/workflows/release.yml
git commit -m "ci: drop destructive e2e jobs from GHA macos-latest

Removed from test.yml:
  - macos-e2e job (L4)
  - destructive job (L5)
  - the run_destructive workflow_dispatch input

Removed from release.yml:
  - the 'Destructive tests' step in gate-tests
  - the smoke-test job and its dependent edge from release.needs

Removed entirely:
  - .github/workflows/smoke-test.yml (redundant with release.yml's
    smoke-test, which also goes away here)

Destructive e2e now runs only locally via scripts/vm/run.sh (added in
the previous commits). No CI gate replaces this — running 'make test-vm'
before tagging is a documented expectation, not enforced.

See docs/superpowers/specs/2026-05-17-l4-l5-tart-local-design.md."
```

---

## Task 5: Delete the old Makefile destructive targets and the dead alias

After Tasks 1-4, nothing references the old targets. Remove them and rewrite the header comment block.

**Files:**
- Modify: `Makefile` (delete lines 21-29, 41-72; rewrite header at 41-51; update `.PHONY`)

### Step 5.1: Delete old `test-destructive`, `test-smoke`, `test-smoke-prebuilt` targets

- [ ] Delete from `Makefile` lines 21-29:

```makefile
test-destructive: build
	go test -v -timeout 15m -tags="e2e,destructive" ./...

test-smoke: build
	go test -v -timeout 20m -tags="e2e,destructive,smoke" -run TestSmoke ./...

# test-smoke-prebuilt: like test-smoke but skips build (uses pre-built binary in PATH or ./openboot)
test-smoke-prebuilt:
	go test -v -timeout 20m -tags="e2e,destructive,smoke" -run TestSmoke ./...
```

### Step 5.2: Delete old `test-vm-*` block and rewrite the header comment

- [ ] Replace `Makefile` lines 41-72 (the entire "Destructive macOS E2E tests — three levels" block plus the dead `test-vm-OLD-DELETE-ME` alias) with:

Old (the block to be deleted, currently approximately lines 41-72):
```makefile
# =============================================================================
# Destructive macOS E2E tests — three levels
# =============================================================================
#
# These tests install real packages and modify ~/.zshrc / macOS defaults on
# the host they run on. They are intended for ephemeral macOS CI runners
# (GitHub Actions macos-latest) or a throwaway VM.
#
# On a developer machine `go test -tags="e2e,vm"` will skip unless you set
# OPENBOOT_E2E_DESTRUCTIVE=1 (see testutil/machost.go). Don't set that
# unless you mean it.

# L1: Quick sanity (~1min) — host/arch checks only, no package installs
test-vm-quick: build
	go test -v -timeout 5m -tags="e2e,vm" -run "TestVM_Infra" ./test/e2e/...

# L2: Release validation (~20min) — core user journeys
test-vm-release: build
	go test -v -timeout 30m -tags="e2e,vm" \
	  -run "TestVM_Infra|TestVM_Journey_DryRunIsCompletelySafe|TestVM_Journey_FirstTimeUser|TestVM_Journey_FullSetupConfiguresEverything|TestE2E_DryRunMinimal|TestE2E_SnapshotCapture" \
	  ./test/e2e/...

# L3: Full validation (~60min) — everything under -tags="e2e,vm"
test-vm-full: build
	go test -v -timeout 90m -tags="e2e,vm" ./test/e2e/...

# Old alias — kept temporarily, deleted in Task 5. Renamed so the new
# Tart-driven `test-vm` below can coexist during the migration.
test-vm-OLD-DELETE-ME: test-vm-release

# Single test by name (e.g. make test-vm-run TEST=TestVM_Journey_DryRunIsCompletelySafe)
test-vm-run: build
	go test -v -timeout 45m -tags="e2e,vm" -run $(TEST) ./test/e2e/...
```

New (replace with this comment block; the actual targets `test-vm`, `test-vm-run`, `test-vm-inner`, `test-vm-inner-run` are already at the end of file from Task 1):
```makefile
# =============================================================================
# Tart VM e2e — destructive tests run inside a throwaway Tart VM provisioned
# by scripts/vm/run.sh. The 12 files in test/e2e/ run via the `e2e,vm` build
# tag; the VM driver SSHs in and invokes `make test-vm-inner`.
#
# Requires Apple Silicon + Tart installed locally. See scripts/vm/README.md
# for one-time setup. The relevant targets are defined at the end of this
# file: test-vm, test-vm-run, test-vm-inner, test-vm-inner-run.
# =============================================================================
```

But note: the actual `test-vm` / `test-vm-run` / `test-vm-inner` / `test-vm-inner-run` targets are at the end of the file (added in Task 1). So this header block has no targets in it — it's just documentation.

Wait, we should move the four real targets up from the bottom of the file (added at the end in Task 1) into this section, so the file flows logically. Edit `Makefile` end-of-file block (the part added in Task 1):

Delete from end of file:
```makefile

# =============================================================================
# Tart VM e2e — new entrypoints (the old test-vm-* targets above are deprecated
# and removed in the next phase). See scripts/vm/README.md for setup.
# =============================================================================

# Developer-facing: provisions a Tart VM and runs the full e2e suite inside.
test-vm: build
	scripts/vm/run.sh test-vm-inner

# Developer-facing: runs one named test inside a Tart VM.
test-vm-run: build
	scripts/vm/run.sh "test-vm-inner-run TEST=$(TEST)"

# In-VM: invoked over SSH by run.sh — not called by developers directly.
test-vm-inner:
	go test -v -timeout 60m -tags="e2e,vm" ./test/e2e/...

test-vm-inner-run:
	go test -v -timeout 45m -tags="e2e,vm" -run $(TEST) ./test/e2e/...
```

Add directly after the new header comment block:
```makefile

# Developer-facing: provisions a Tart VM and runs the full e2e suite inside.
test-vm: build
	scripts/vm/run.sh test-vm-inner

# Developer-facing: runs one named test inside a Tart VM.
test-vm-run: build
	scripts/vm/run.sh "test-vm-inner-run TEST=$(TEST)"

# In-VM: invoked over SSH by run.sh — not called by developers directly.
test-vm-inner:
	go test -v -timeout 60m -tags="e2e,vm" ./test/e2e/...

test-vm-inner-run:
	go test -v -timeout 45m -tags="e2e,vm" -run $(TEST) ./test/e2e/...
```

### Step 5.3: Update the .PHONY line

- [ ] Edit `Makefile` lines 1-3:

Old:
```makefile
.PHONY: test-unit test-e2e test-destructive test-smoke test-smoke-prebuilt test-coverage test-all \
       test-vm test-vm-run test-vm-quick test-vm-release test-vm-full \
       test-vm-inner test-vm-inner-run \
       install-hooks uninstall-hooks
```

New:
```makefile
.PHONY: test-unit test-e2e test-coverage test-all \
       test-vm test-vm-run test-vm-inner test-vm-inner-run \
       install-hooks uninstall-hooks
```

### Step 5.4: Verify Makefile parses and the targets resolve

- [ ] Dry-run the new entrypoint:

```bash
make -n test-vm 2>&1 | head
```

Expected: shows `go build ...` (the `build` dependency) then `scripts/vm/run.sh test-vm-inner`.

- [ ] Confirm old targets are gone:

```bash
make test-destructive 2>&1 | head -1
```

Expected: `make: *** No rule to make target 'test-destructive'.  Stop.`

```bash
make test-vm-release 2>&1 | head -1
```

Expected: `make: *** No rule to make target 'test-vm-release'.  Stop.`

### Step 5.5: Run the full VM suite end-to-end as a final smoke

- [ ] If time permits (~30 min on Apple Silicon):

```bash
make test-vm
```

Expected: VM provisions, all e2e tests run, VM is destroyed. Failures here are bugs in tests, not the migration — but they prove the new path works.

If you don't want to wait 30 min, run a focused subset:

```bash
make test-vm-run TEST="TestVM_Journey_DryRunIsCompletelySafe|TestVM_Infra"
```

### Step 5.6: Commit

- [ ] Stage and commit:

```bash
git add Makefile
git commit -m "build(make): remove old destructive test targets

Deleted:
  - test-destructive
  - test-smoke / test-smoke-prebuilt
  - test-vm-quick / test-vm-release / test-vm-full
  - the temporary test-vm-OLD-DELETE-ME alias

The new test-vm / test-vm-run / test-vm-inner / test-vm-inner-run
targets (added two commits back) are now the only entrypoints.
Header comment block rewritten."
```

---

## Task 6: Update auto-release.yml — open issue on feat threshold, keep auto-tag on patch fast lane

The current sensor auto-tags whenever any threshold fires. New behavior: only the patch fast lane (`fix:` only) keeps auto-tagging; feat-bearing thresholds open a `release-ready` issue instead, prompting human review.

**Files:**
- Modify: `.github/workflows/auto-release.yml` (the `evaluate` job's `Decide` and `Tag and dispatch release` steps; add a new `Open release-ready issue` step)

### Step 6.1: Add `bump` to decision output and split downstream behavior

The `Decide` step already computes `bump` (`minor` vs `patch`) at lines 99-106. It's currently written to `GITHUB_OUTPUT`. We'll branch the next step on it.

- [ ] Edit `.github/workflows/auto-release.yml` — change the `Tag and dispatch release` step (lines 122-135) to only fire on patch bumps, and add a sibling step that opens an issue on minor bumps.

Old (lines 122-135):
```yaml
      - name: Tag and dispatch release
        if: steps.decide.outputs.release == 'true' && steps.decide.outputs.dry_run != 'true'
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          NEW_TAG: ${{ steps.decide.outputs.new_tag }}
          REASON: ${{ steps.decide.outputs.reason }}
        run: |
          set -euo pipefail
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git tag -a "$NEW_TAG" -m "Auto-release: $REASON"
          git push origin "$NEW_TAG"
          gh workflow run release.yml -f version="$NEW_TAG"
          echo "::notice::tagged $NEW_TAG and dispatched release.yml — $REASON"
```

New:
```yaml
      - name: Tag and dispatch release (patch fast lane)
        if: |
          steps.decide.outputs.release == 'true'
          && steps.decide.outputs.dry_run != 'true'
          && steps.decide.outputs.bump == 'patch'
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          NEW_TAG: ${{ steps.decide.outputs.new_tag }}
          REASON: ${{ steps.decide.outputs.reason }}
        run: |
          set -euo pipefail
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git tag -a "$NEW_TAG" -m "Auto-release: $REASON"
          git push origin "$NEW_TAG"
          gh workflow run release.yml -f version="$NEW_TAG"
          echo "::notice::tagged $NEW_TAG and dispatched release.yml — $REASON"

      - name: Open release-ready issue (feat threshold)
        if: |
          steps.decide.outputs.release == 'true'
          && steps.decide.outputs.dry_run != 'true'
          && steps.decide.outputs.bump == 'minor'
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          NEW_TAG: ${{ steps.decide.outputs.new_tag }}
          REASON: ${{ steps.decide.outputs.reason }}
        run: |
          set -euo pipefail

          # If an open release-ready issue already exists for this tag, skip
          # (we don't want to spam the queue every push to main).
          existing=$(gh issue list \
            --label release-ready \
            --state open \
            --search "in:title ${NEW_TAG}" \
            --json number --jq '.[0].number' || true)
          if [ -n "$existing" ]; then
            echo "::notice::release-ready issue #${existing} already open for ${NEW_TAG} — skipping"
            exit 0
          fi

          body=$(cat <<EOF
          Auto-release sensor wants to cut **${NEW_TAG}** (minor bump).

          **Why:** ${REASON}

          Because this release includes \`feat:\` changes, the sensor did
          **not** auto-tag. Run the destructive e2e suite locally, then
          cut the release manually:

          - [ ] \`make test-vm\` passes (Apple Silicon + Tart required — see scripts/vm/README.md)
          - [ ] sanity-check the curl|bash smoke and cli-compat results in the most recent test.yml run on main
          - [ ] \`git tag -a ${NEW_TAG} -m "..."\` and \`git push origin ${NEW_TAG}\`
          - [ ] close this issue

          Skipping \`make test-vm\` is allowed (it is not a hard gate),
          but \`feat:\` changes carry more risk than \`fix:\` patches.
          EOF
          )

          gh issue create \
            --title "Release ready: ${NEW_TAG}" \
            --label release-ready \
            --body "$body"
          echo "::notice::opened release-ready issue for ${NEW_TAG} — ${REASON}"
```

### Step 6.2: Update the header comment in auto-release.yml

- [ ] Edit `.github/workflows/auto-release.yml` lines 1-19 (the comment block at the top):

Old:
```yaml
name: Auto Release

# Sensor: when unreleased commits on main trip a threshold, automatically tag
# the next version and dispatch release.yml. Closes the "agent forgot to cut
# a release" loop — see docs/HARNESS.md.
#
# Triggers (any one fires):
#   - >=5 feat/fix commits since last stable tag
#   - >=7 days since last stable tag AND new commits exist
#   - any fix: present (patch fast lane)
#
# Version bump:
#   - any feat: present -> minor (X.Y+1.0)
#   - else              -> patch (X.Y.Z+1)
#
# Why we dispatch release.yml instead of relying on the tag-push trigger:
# GitHub deliberately suppresses workflow events fired by GITHUB_TOKEN, so a
# tag pushed from here would NOT start release.yml. We push the tag and then
# explicitly `gh workflow run release.yml -f version=<tag>`.
```

New:
```yaml
name: Auto Release

# Sensor: when unreleased commits on main trip a threshold, either auto-tag
# (patch fast lane) or open a release-ready issue (feat threshold).
# See docs/HARNESS.md.
#
# Triggers (any one fires):
#   - >=5 feat/fix commits since last stable tag
#   - >=7 days since last stable tag AND new commits exist
#   - any fix: present (patch fast lane)
#
# Version bump:
#   - any feat: present -> minor (X.Y+1.0) — opens issue, does NOT tag
#   - else              -> patch (X.Y.Z+1) — auto-tags + dispatches release.yml
#
# Why the split: feat: changes carry more risk than fix: patches and benefit
# from a human running the local Tart VM e2e suite before tag. The issue is
# a nudge, not a hard gate — a human can tag without running it.
#
# Why we dispatch release.yml on patch instead of relying on the tag-push
# trigger: GitHub deliberately suppresses workflow events fired by
# GITHUB_TOKEN, so a tag pushed from here would NOT start release.yml.
# We push the tag and then explicitly
# `gh workflow run release.yml -f version=<tag>`.
```

### Step 6.3: Add `issues: write` to permissions

- [ ] Edit `.github/workflows/auto-release.yml` lines 35-37:

Old:
```yaml
permissions:
  contents: write
  actions: write
```

New:
```yaml
permissions:
  contents: write
  actions: write
  issues: write
```

### Step 6.4: Verify auto-release.yml parses

- [ ] Parse:

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/auto-release.yml'))" && echo OK
```

Expected: `OK`.

- [ ] Dry-run the sensor logic from the workflow UI:

```bash
gh workflow run auto-release.yml -f dry_run=true
```

Then watch the latest run:

```bash
gh run watch
```

Expected: the run ends with either "no threshold tripped — skip" (if main has nothing new) or "DRY_RUN — would tag vX.Y.Z (<reason>)" (if it does). No tag is pushed, no issue is opened.

### Step 6.5: Commit

- [ ] Stage and commit:

```bash
git add .github/workflows/auto-release.yml
git commit -m "ci(auto-release): split feat threshold from patch fast lane

Patch (fix:-only) bumps continue to auto-tag and dispatch release.yml.
Minor bumps (feat: present) now open a 'release-ready' labeled issue
with a checklist instead of auto-tagging — the human is expected to
run make test-vm locally and then tag manually.

Skipping test-vm is allowed; the issue is a nudge, not a hard gate.
Rationale: feat: changes carry more risk and benefit from the local
Tart VM e2e suite added in earlier commits. fix: patches keep going
through the existing fast lane.

Adds 'issues: write' to the workflow permissions. Header comment
block rewritten."
```

---

## Task 7: Documentation sync

Update `CONTRIBUTING.md`, `CLAUDE.md`, and `docs/HARNESS.md` to reflect the new layering. `AGENTS.md` already covers the "do not run destructive tests outside an ephemeral VM" rule, so no change there.

**Files:**
- Modify: `CONTRIBUTING.md` (Test Layering table + new VM setup section)
- Modify: `CLAUDE.md` (Commands block, possibly Where to Look table)
- Modify: `docs/HARNESS.md` (table rows for L4/L5, "What's intentionally NOT in the harness")

### Step 7.1: Edit CONTRIBUTING.md — collapse L4 + L5 in the Test Layering table

- [ ] Read the current table at `CONTRIBUTING.md:31-42` to confirm the exact rows, then edit. Old:

```markdown
| Tier | What | How to run | When it runs |
|------|------|------------|--------------|
| **L1 Unit + Integration + Contract** | Pure-Go logic with faked `Runner` *plus* real `brew` / `git` / `npm` against temp dirs and real `httptest` servers | `make test-unit` (~75s) | Every push (pre-push hook); CI on push/PR |
| **L2 Contract schema** | JSON schema validation against [openboot-contract](https://github.com/openbootdotdev/openboot-contract) | (runs in CI only) | CI on push/PR |
| **L3 E2E binary** | Compiled binary driven by scripts; `-tags=e2e` | `make test-e2e` | CI on release |
| **L4 Destructive macOS** | Runs against a real macOS host (installs packages, modifies `~/.zshrc`, writes `defaults`) | `make test-vm-quick` / `test-vm-release` / `test-vm-full` — requires `CI=true` or `OPENBOOT_E2E_DESTRUCTIVE=1` | GH Actions `macos-latest` on release tags + manual dispatch |
| **L5 Destructive** | Actually installs real packages into a real system | `make test-destructive` / `test-smoke` | CI on release, plus manual `workflow_dispatch` |
```

New:
```markdown
| Tier | What | How to run | When it runs |
|------|------|------------|--------------|
| **L1 Unit + Integration + Contract** | Pure-Go logic with faked `Runner` *plus* real `brew` / `git` / `npm` against temp dirs and real `httptest` servers | `make test-unit` (~75s) | Every push (pre-push hook); CI on push/PR |
| **L2 Contract schema** | JSON schema validation against [openboot-contract](https://github.com/openbootdotdev/openboot-contract) | (runs in CI only) | CI on push/PR |
| **L3 E2E binary** | Compiled binary driven by scripts; `-tags=e2e` | `make test-e2e` | CI on release |
| **L4 VM e2e** | Full destructive suite (`-tags="e2e,vm"`) runs inside an ephemeral Tart VM provisioned by `scripts/vm/run.sh`. Installs real packages, modifies `~/.zshrc`, writes `defaults` — all contained to the throwaway VM. | `make test-vm` (~30 min, Apple Silicon + Tart required) | **Local only** — convention is to run before tagging a release. No CI gate. |
```

### Step 7.2: Edit CONTRIBUTING.md — replace "Rules of thumb" L4-specific lines

- [ ] In `CONTRIBUTING.md:43-47`:

Old:
```markdown
Rules of thumb:

- **Local dev:** run nothing manually if hooks are installed. `make test-unit` on demand when you want a sanity check. Skip L2+ unless you're cutting a release.
- **Before pushing:** `make test-unit` (the pre-push hook does this automatically). Requires `brew` / `git` / `npm` on PATH — they are queried read-only against temp dirs, no real installs.
- **Before tagging a release:** trigger the `macos-e2e` job via GitHub Actions (manual dispatch or tag push). To run locally on a throwaway macOS machine: `OPENBOOT_E2E_DESTRUCTIVE=1 make test-vm-release`.
```

New:
```markdown
Rules of thumb:

- **Local dev:** run nothing manually if hooks are installed. `make test-unit` on demand when you want a sanity check. Skip L2+ unless you're cutting a release.
- **Before pushing:** `make test-unit` (the pre-push hook does this automatically). Requires `brew` / `git` / `npm` on PATH — they are queried read-only against temp dirs, no real installs.
- **Before tagging a release (convention, not enforced):** `make test-vm` on an Apple Silicon Mac with Tart installed. See [VM E2E setup](#vm-e2e-setup) below. `auto-release.yml` opens a `release-ready` issue on `feat:` thresholds to nudge you here.
```

### Step 7.3: Edit CONTRIBUTING.md — add VM E2E setup section

- [ ] After the existing `## Test Layering` section (which ends before `## Git Hooks` at line 49), insert a new section:

```markdown
## VM E2E setup

Destructive tests (L4) run inside an ephemeral Tart VM. One-time setup
on an Apple Silicon Mac:

```bash
brew install cirruslabs/cli/tart
tart pull ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest
tart clone ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest macos-tahoe-base
```

Then:

```bash
make test-vm                                         # full suite (~30 min)
make test-vm-run TEST=TestVM_Journey_FirstTimeUser   # one test
OPENBOOT_VM_KEEP=1 make test-vm                      # don't destroy VM at exit (debug)
```

See `scripts/vm/README.md` for full environment-variable docs and
troubleshooting.

```

### Step 7.4: Edit CLAUDE.md — Commands table

- [ ] Edit `CLAUDE.md:31-46` (the Commands code block). Old:

```bash
# Test — full tier table in CONTRIBUTING.md
make test-unit                       # L1 (~75s) — unit + integration + contract; pre-push hook
make test-e2e                        # L3 compiled binary
make test-vm-release                 # L4 destructive macOS (~20m) — before tagging
make test-destructive                # L5 — actually installs
make test-coverage                   # coverage.out + coverage.html
```

New:
```bash
# Test — full tier table in CONTRIBUTING.md
make test-unit                       # L1 (~75s) — unit + integration + contract; pre-push hook
make test-e2e                        # L3 compiled binary
make test-vm                         # L4 (~30m) — destructive e2e in a local Tart VM; before tagging
make test-coverage                   # coverage.out + coverage.html
```

### Step 7.5: Edit CLAUDE.md — Project section if needed

- [ ] Read `CLAUDE.md` around `## Project` (top of file) to check if any L5 references exist:

```bash
grep -n 'L5\|test-destructive\|test-smoke\|test-vm-release\|test-vm-quick\|test-vm-full' CLAUDE.md
```

If any matches, edit them out (L5 row disappears since it merged into L4). Expected: 0 matches after Step 7.4.

### Step 7.6: Edit docs/HARNESS.md — merge L4 and L5 table rows

- [ ] Edit `docs/HARNESS.md` around the "Where each control lives" table. Find the existing rows:

Old:
```markdown
| Behav. | L3 e2e binary | release | `make test-e2e` |
| Behav. | L4 macOS e2e (`vm`) | release tags, manual dispatch | `make test-vm-release` |
| Behav. | L5 destructive (real installs) | release tags, manual dispatch | `make test-destructive` |
```

New:
```markdown
| Behav. | L3 e2e binary | release | `make test-e2e` |
| Behav. | L4 VM e2e (`vm`) — runs full destructive suite in a local Tart VM | local only (convention is pre-release; no CI gate) | `make test-vm` (driver: `scripts/vm/run.sh`) |
```

### Step 7.7: Edit docs/HARNESS.md — note the deleted CI jobs

- [ ] Find the row for the GHA smoke job and remove it (it was deleted in Task 4):

Old:
```markdown
| Behav. | curl\|bash smoke (install.sh + mock server) | every PR | `.github/workflows/test.yml` `curl-bash-smoke` job |
```

This row should stay — `curl-bash-smoke` was NOT deleted. Only `macos-e2e` and `destructive` were deleted, but those don't appear in the HARNESS.md table (it didn't list them as separate harness controls; they were just L4/L5 entries which we already updated).

Double-check no other obsolete entries:

```bash
grep -n 'test-destructive\|test-smoke\|test-vm-release\|test-vm-quick\|test-vm-full\|smoke-test.yml' docs/HARNESS.md
```

If matches found, decide case-by-case whether they refer to deleted code (delete the line) or are historical context (leave).

### Step 7.8: Edit docs/HARNESS.md — add the new "intentionally NOT" entry

- [ ] In `docs/HARNESS.md` under `## What's intentionally NOT in the harness`, append a new bullet at the end of the bulleted list:

```markdown
- **No CI gate for VM e2e.** Apple Silicon Tart VMs don't run on
  GitHub-hosted `macos-latest` runners (no nested virt, wrong arch
  guarantees), and we declined to set up a self-hosted runner. L4 is
  local-only. Running `make test-vm` before tagging is convention,
  encoded as a `release-ready` issue opened by `auto-release.yml`
  on `feat:` thresholds — not a hard gate. A human can release without
  it.
```

### Step 7.9: Edit docs/HARNESS.md — update the auto-release table row

- [ ] Find the auto-release row in the "Where each control lives" table:

Old:
```markdown
| Behav. | Auto-release sensor — tag + dispatch `release.yml` when unreleased commits trip a threshold (`>=5 feat/fix`, `>=7 days + new`, or any `fix:` for patch fast lane) | push to `main` | `.github/workflows/auto-release.yml` |
```

New:
```markdown
| Behav. | Auto-release sensor — patch fast lane (`fix:`-only) auto-tags + dispatches `release.yml`; feat threshold opens a `release-ready` issue with a `make test-vm` checklist instead | push to `main` | `.github/workflows/auto-release.yml` |
```

### Step 7.10: Edit docs/HARNESS.md — update the "steering loop" table

- [ ] Find this row in the "## The steering loop" table:

Old:
```markdown
| "Fixes and features piled up on `main` because nobody told an agent to cut a release." | Already handled: `.github/workflows/auto-release.yml` tags on threshold. Tune the thresholds there rather than re-introducing manual release cadence. |
```

New:
```markdown
| "Fixes and features piled up on `main` because nobody told an agent to cut a release." | Already handled: `.github/workflows/auto-release.yml` auto-tags patches and opens a `release-ready` issue for feats. Tune thresholds there. |
```

### Step 7.11: Verify final state

- [ ] Look for any remaining references to deleted things:

```bash
grep -rnE 'test-destructive|test-smoke|test-vm-quick|test-vm-release|test-vm-full|smoke-test\.yml|OPENBOOT_E2E_DESTRUCTIVE=1.+test-vm' \
  --include='*.md' --include='*.yml' --include='Makefile' . \
  | grep -v 'docs/superpowers/'
```

Expected: very few or no matches outside `docs/superpowers/` (which contains the spec — references there are historical and stay). Any match outside should be reviewed; either the migration missed a spot or the match is legitimately about something else.

### Step 7.12: Commit

- [ ] Stage and commit:

```bash
git add CONTRIBUTING.md CLAUDE.md docs/HARNESS.md
git commit -m "docs: reflect L4/L5 merge into local Tart VM

CONTRIBUTING.md: L4 and L5 rows collapse to a single L4 VM e2e row
('runs inside Tart VM, local only, no CI gate'). New 'VM E2E setup'
section walks through tart pull / tart clone. Rules of thumb updated.

CLAUDE.md: Commands block drops test-vm-release / test-destructive,
adds test-vm with a 'requires Apple Silicon + Tart' note.

docs/HARNESS.md: table rows for L4/L5 merge; auto-release row
reflects patch-vs-feat split; new 'intentionally NOT' entry explains
why there is no CI gate for VM e2e."
```

---

## Self-Review

**1. Spec coverage check:**

- Architecture diagram → Task 1 (`scripts/vm/run.sh` implements the boxes)
- `scripts/vm/` files → Task 1 Steps 1.1-1.3
- New Makefile targets → Task 1 Step 1.4; old targets deleted → Task 5
- `testutil/machost.go` `OPENBOOT_IN_VM` branch → Task 3 Step 3.3
- Comment rewrites → Task 3 Steps 3.1-3.2 + Task 5 Step 5.2 (Makefile header)
- Build tag merge → Task 2
- Data flow / error handling — covered by Task 1's preflight + trap + sweep + Step 1.6 verifications
- CI deletes → Task 4 (all jobs/steps/files explicitly enumerated)
- auto-release split → Task 6
- Documentation sync → Task 7 (CONTRIBUTING, CLAUDE, HARNESS)
- Risks/mitigations — base image `:latest` drift doc → Task 1 Step 1.3 README; rsync excludes → Task 1 Step 1.2; sweep → Task 1 Step 1.6 verification; `OPENBOOT_VM_KEEP` debug → Task 1 Step 1.2 + Step 1.6 verification

**2. Placeholder scan:** Scanned for "TBD", "TODO", "implement later", vague error-handling. None present — every step contains exact code or exact commands with expected output.

**3. Type/signature consistency:** `lib.sh` defines `die`, `wait_for_ssh`, `ssh_exec`, `sweep_leaked_vms` — `run.sh` calls exactly those four. `OPENBOOT_VM_BASE` and `OPENBOOT_VM_KEEP` are spelled identically across run.sh, README.md, CONTRIBUTING.md, and HARNESS.md. The four new Makefile targets (`test-vm`, `test-vm-run`, `test-vm-inner`, `test-vm-inner-run`) are spelled consistently in Task 1, Task 5, Task 7. The build tag `e2e && vm` matches between `real_install_test.go`, `smoke_test.go`, `machost.go`, and the in-VM `go test -tags="e2e,vm"`.
