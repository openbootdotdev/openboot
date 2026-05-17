# L4/L5 destructive e2e → local Tart VM

**Date:** 2026-05-17
**Status:** approved (brainstorming complete, awaiting plan)

## Problem

L4 (`make test-vm-*`) and L5 (`make test-destructive` / `make test-smoke`)
run today on GitHub Actions `macos-latest` (`.github/workflows/test.yml:144-176`,
`.github/workflows/release.yml:49-68`). The targets are **named** "vm" but
no VM exists — they invoke `go test` directly on the runner host
(`Makefile:54-65`). `testutil/machost.go:4-5` admits this in a comment
("no Tart VM, no SSH … intended for ephemeral CI runners") and preserves
the API surface of a previously-existing Tart helper that was ripped out
(`testutil/machost.go:94`).

This is wrong for two reasons:

1. **GHA `macos-latest` is not a clean Mac.** It's a shared runner with
   pre-installed brew, Xcode, user state, and quirks (no real screen
   recording prompt, no FileVault, no first-run defaults). Tests that
   exercise "openboot bootstrapping a fresh Mac" are not actually
   exercising that scenario.
2. **GHA macOS runners are expensive and slow** for what is essentially
   "run a script in a throwaway VM" — we already have Apple Silicon
   hardware locally that can do this faster and more honestly via Tart.

## Goals

- L4 + L5 collapse into a single **L4 VM e2e** tier that runs **locally
  inside a Tart VM**.
- The 12 existing `test/e2e/*.go` files compile and run **unchanged** —
  the migration is driver-layer plumbing, not a test rewrite.
- CI keeps L1, L2, lint, curl-bash-smoke, cli-compat (all GHA-friendly).
  L4 leaves CI entirely.
- No mandatory release gate on L4. Running it before tagging is
  documented expectation, not mechanical enforcement. Cutting a release
  without it is allowed.

## Non-goals

- **No self-hosted GHA runner.** Out of scope — keeps infra dependencies
  to zero.
- **No `MacHost` → remote-driver refactor.** `MacHost` keeps its current
  shape ("I am the throwaway host"); Tart provisions that host from
  outside.
- **No Intel Mac support for L4.** Tart requires Apple Silicon. Intel
  contributors run L1 only; L4 only fires before release on an Apple
  Silicon dev machine.
- **No registry/private-image publishing.** Each dev pulls the base
  image once from the public cirruslabs mirror.

## Architecture

```
┌─ Local macOS (Apple Silicon) ─────────────────────────┐
│                                                       │
│  $ make test-vm                                       │
│       │                                               │
│       ▼                                               │
│  scripts/vm/run.sh test-vm-inner                      │
│   1. tart clone <base> openboot-ephemeral-$$          │
│   2. tart run --no-graphics openboot-ephemeral-$$ &   │
│   3. wait_for_ssh (poll `tart ip`, 60s timeout)       │
│   4. rsync working tree → VM:/Users/admin/openboot    │
│   5. ssh exec: cd openboot && make test-vm-inner      │
│   6. trap EXIT: tart stop + tart delete               │
│                                                       │
└───────────────────────────────────────────────────────┘
```

Inside the VM, `make test-vm-inner` runs `go test -tags="e2e,vm"
./test/e2e/...` — exactly what L4 does today, just inside an actually
throwaway environment. `MacHost` sees a fresh macOS with no prior
openboot state, no shared runner pollution, and can mutate freely
because the VM is deleted at the end.

## Components

### scripts/vm/

New directory:

```
scripts/vm/
├── run.sh              # entry point, called by Makefile
├── lib.sh              # shared funcs: wait_for_ssh, ssh_exec, die
└── README.md           # base-image setup + troubleshooting
```

**`run.sh` contract:**

```bash
scripts/vm/run.sh <make-target-inside-vm>
```

Examples:
- `scripts/vm/run.sh test-vm-inner`
- `scripts/vm/run.sh "test-vm-inner-run TEST=TestVM_Journey_FirstTimeUser"`

**Pseudocode:**

```bash
set -euo pipefail
TARGET="$1"
BASE="${OPENBOOT_VM_BASE:-macos-tahoe-base}"
VM="openboot-ephemeral-$$"
KEEP="${OPENBOOT_VM_KEEP:-0}"

# Pre-flight
command -v tart >/dev/null || die "tart not installed — brew install cirruslabs/cli/tart"
tart list | awk '{print $2}' | grep -qx "$BASE" \
  || die "base image '$BASE' missing — see scripts/vm/README.md for one-time setup"

# Sweep any leaked ephemerals from prior crashed runs
tart list | awk '/openboot-ephemeral-/{print $2}' | while read -r leaked; do
  echo "cleaning leaked VM: $leaked"
  tart stop  "$leaked" 2>/dev/null || true
  tart delete "$leaked" 2>/dev/null || true
done

tart clone "$BASE" "$VM"
trap_cleanup() {
  if [[ "$KEEP" == "1" ]]; then
    echo "OPENBOOT_VM_KEEP=1 — leaving '$VM' running for debug; tart ssh $VM"
    return
  fi
  tart stop   "$VM" 2>/dev/null || true
  tart delete "$VM" 2>/dev/null || true
}
trap trap_cleanup EXIT

tart run --no-graphics "$VM" &
VM_IP=$(wait_for_ssh "$VM")   # 60s timeout, error includes tart logs

rsync -az --delete \
  --exclude='/.git/objects' \
  --exclude='/openboot' \
  --exclude='/coverage.*' \
  -e 'ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null' \
  ./ admin@"$VM_IP":/Users/admin/openboot/

ssh_exec "$VM_IP" \
  "cd /Users/admin/openboot && CI=true OPENBOOT_IN_VM=1 make $TARGET"
```

### Makefile

**Delete:**

- `test-vm-quick` — no place for a "lite VM check" inside a real VM
- `test-vm-full` — collapses with `test-vm`
- `test-vm-release` — same
- `test-vm-run` — replaced by new version that goes through `run.sh`
- `test-destructive` — L5 entry point, gone after the merge
- `test-smoke` / `test-smoke-prebuilt` — smoke is a subset under `vm` tag now

**Add / rewrite:**

```makefile
# Developer-facing: provisions a Tart VM and runs the full e2e suite
test-vm: build
	scripts/vm/run.sh test-vm-inner

# Developer-facing: single named test inside the VM
test-vm-run: build
	scripts/vm/run.sh "test-vm-inner-run TEST=$(TEST)"

# In-VM: not called directly by developers
test-vm-inner:
	go test -v -timeout 60m -tags="e2e,vm" ./test/e2e/...

test-vm-inner-run:
	go test -v -timeout 45m -tags="e2e,vm" -run $(TEST) ./test/e2e/...
```

The `Makefile:41-51` header comment block is rewritten to reflect the
new model.

### testutil/machost.go

**Comment-only edits except one logical change.** Top-of-file comment
loses the "no Tart VM, no SSH" language and the `CopyFile` "Preserved
for API compatibility" note (`testutil/machost.go:4-9, 94`).

**Single logical change** — `requireEphemeralHost` (`testutil/machost.go:113-119`)
adds an `OPENBOOT_IN_VM` branch:

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

`run.sh` sets `OPENBOOT_IN_VM=1` over SSH. `CI=true` stays as a fallback
so old call sites and ad-hoc runs still work.

### Build tags

Merge `e2e,destructive` → `e2e,vm`. Files affected:

- `test/e2e/real_install_test.go` — flip tag
- `test/e2e/smoke_test.go` — flip tag

After this change there is no test under `e2e && destructive` — that
tag combination is retired from the codebase.

## Data flow: a single run

```
host:                                       VM (admin@<vm-ip>):
                                            ┌─────────────────┐
$ make test-vm                              │ macOS Tahoe     │
  → scripts/vm/run.sh test-vm-inner         │ Go installed    │
  → tart clone macos-tahoe-base ephemeral   │ git, expect     │
  → tart run --no-graphics                  │ NO brew/openboot│
  → wait_for_ssh                            └─────────────────┘
  → rsync ./ → /Users/admin/openboot/   ─────→ /Users/admin/openboot
  → ssh: make test-vm-inner             ─────→ go test -tags="e2e,vm"
                                                ./test/e2e/...
                                                  ↓
                                                MacHost{}
                                                  ↓
                                                bash -c "openboot install ..."
                                                  ↓
                                                real brew install, real
                                                defaults write, real
                                                .zshrc edits — VM is the
                                                blast radius
  ← exit code propagates              ←──── exit
  → trap: tart stop && tart delete
```

## Error handling

| Failure | Behavior |
|---|---|
| `tart` not installed | `die` with brew install hint, before clone |
| Base image absent | `die` with link to `scripts/vm/README.md` setup section, before clone |
| `tart clone` fails | bubble up; nothing to clean (clone is atomic) |
| `tart run` fails | trap deletes the partial VM |
| `wait_for_ssh` times out (60s) | dump `tart logs` to stderr, exit non-zero, trap deletes VM |
| `rsync` fails | exit non-zero, trap deletes VM |
| Test failure inside VM | SSH propagates non-zero exit, trap deletes VM (unless `OPENBOOT_VM_KEEP=1`) |
| Run interrupted (Ctrl-C) | trap fires on EXIT, cleans VM |
| Hard kill (`kill -9 run.sh`) | trap **doesn't** fire — leaked VM is swept by next run's pre-flight sweep |
| Multiple parallel `make test-vm` | each gets unique `$$` name; no collision |

## CI / release flow

### Delete

- `.github/workflows/test.yml` — `macos-e2e` job (lines 144-160), `destructive` job (lines 162-176)
- `.github/workflows/release.yml` — the `Destructive tests` step inside `gate-tests` (lines 67-68); `smoke-test` job (lines 104-148)
- `.github/workflows/smoke-test.yml` — entire file (redundant with the smoke-test job in release.yml, both go local-only)

### Keep

- `release.yml`: `detect-release-type`, `gate-tests` (stripped to vet + L1 unit), `build`, `release`, `update-homebrew-tap`
- `test.yml`: everything except the two deleted jobs (unit, contract, curl-bash-smoke, cli-compat, lint)
- `auto-release.yml`: as-is for **patch fast lane only** — see below

### Change to auto-release.yml

The current workflow auto-tags whenever ≥5 feat/fix, ≥7 days + new commits, or any `fix:` (patch fast lane) accumulates on `main`.

**New behavior:**

- **Patch fast lane** (`fix:`-only since last tag) — keep auto-tagging. Patch changes are small enough that the release pipeline's L1 + smoke (gone) was already sufficient signal; skipping local VM e2e for these is acceptable.
- **Feat threshold** (≥5 feat/fix or ≥7 days with new commits including any `feat:`) — **stop auto-tagging.** Instead, open a GitHub issue tagged `release-ready` with a checklist:
  - [ ] run `make test-vm` locally
  - [ ] verify smoke output
  - [ ] `git tag -a vX.Y.Z -m "..."` and push

This is **not** a hard gate — the human can skip the checklist and tag anyway. It's a nudge to slow down on feat-bearing releases, matching the user's preference that L4 stays optional.

### No release gate

Explicitly: **no machine check that L4 ran before a tag.** Cutting a release without L4 is permitted. The expectation that L4 *should* be run before tagging lives in `CONTRIBUTING.md` and the auto-release issue template, not in CI.

## Documentation changes

- **`CONTRIBUTING.md`** — Test Layering table: L4 + L5 row merges to "L4 VM e2e (runs inside Tart VM via `make test-vm`)". New section "VM E2E setup" with one-time `tart pull` + `tart clone` commands using `ghcr.nju.edu.cn/cirruslabs/macos-tahoe-base:latest`. Document `OPENBOOT_VM_BASE` and `OPENBOOT_VM_KEEP` env vars.
- **`CLAUDE.md`** — "Commands" table: replace `make test-vm-release` row with `make test-vm`, add "requires Apple Silicon + Tart locally" note.
- **`AGENTS.md`** — "Tools you may NOT use" section already covers this; no change.
- **`docs/HARNESS.md`** — Table: L4/L5 rows merge. "What's intentionally NOT in the harness" gains: "No CI gate for VM e2e — Apple Silicon Tart VMs don't run on `macos-latest`, and the user has decided L4 stays optional. Running it before tagging is convention, not enforcement."
- **`Makefile`** — Comment block at lines 41-51 rewritten.
- **`testutil/machost.go`** — Top-of-file comment rewritten.

## Risks & open knobs

| Risk | Mitigation |
|---|---|
| Base image `:latest` tag drifts on remote, breaks reproducibility | `scripts/vm/README.md` pins a specific image digest; users update intentionally. `run.sh` does **not** re-pull. |
| `rsync` ships the `./openboot` build artifact (or `coverage.*`) into the VM | rsync `--exclude` list covers these. |
| `.git/objects` (~hundreds of MB) ships into VM unnecessarily | excluded via rsync. VM doesn't need full git history. |
| `wait_for_ssh` flakes on first boot | 60s timeout, error includes `tart logs`. `OPENBOOT_VM_KEEP=1` lets user attach for debug. |
| Hard kill of `run.sh` leaks VM | pre-flight sweep of `openboot-ephemeral-*` VMs on next run. |
| Apple Silicon contributors only | documented; Intel users run L1, don't run L4. |
| Tart VM startup adds ~30s per `make test-vm` invocation | acceptable for a pre-release check; not a daily dev-loop tool. |

## Implementation order

Each step is its own PR for easy revert:

1. **Add Tart driver layer.** New `scripts/vm/{run.sh, lib.sh, README.md}` + new Makefile targets `test-vm`, `test-vm-run`, `test-vm-inner`, `test-vm-inner-run`. **Old targets stay.** Verify `make test-vm` succeeds locally.
2. **Merge build tags.** Flip `e2e,destructive` → `e2e,vm` in `real_install_test.go`, `smoke_test.go`. Verify they still compile and run inside `make test-vm`.
3. **Update `testutil/machost.go`.** Add `OPENBOOT_IN_VM` branch, rewrite top-of-file comment.
4. **Delete CI destructive jobs.** Remove `macos-e2e`, `destructive` from `test.yml`; strip `Destructive tests` step + `smoke-test` job from `release.yml`; delete `smoke-test.yml` entirely.
5. **Delete old Makefile targets.** Remove `test-vm-quick`, `test-vm-full`, `test-vm-release`, `test-destructive`, `test-smoke`, `test-smoke-prebuilt`. Rewrite Makefile header comment.
6. **Update `auto-release.yml`.** Feat threshold opens issue instead of tagging; patch fast lane unchanged.
7. **Documentation sync.** `CONTRIBUTING.md`, `CLAUDE.md`, `HARNESS.md`.

Steps 1–3 are additive and can land before any CI changes. Steps 4–5 are the destructive cuts. Step 6 changes release cadence and should go last so the team has time to read about it.
