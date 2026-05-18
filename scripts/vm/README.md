# Tart VM e2e

The `scripts/vm/run.sh` driver spins up an ephemeral Tart VM for the
destructive e2e suite (`make test-vm-parallel`). The 12 test files in `test/e2e/`
run inside it.

## One-time setup

Tart only runs on Apple Silicon Macs.

```bash
# 1. Install Tart (https://tart.run)
brew install cirruslabs/cli/tart

# 2. Pull the base image (downloads ~25GB)
#    Canonical upstream:
tart pull ghcr.io/cirruslabs/tahoe-base:latest
#    Faster mirror for users in China:
#    tart pull ghcr.nju.edu.cn/cirruslabs/tahoe-base:latest

# 3. Give it the local name run.sh expects
tart clone ghcr.io/cirruslabs/tahoe-base:latest tahoe-base
```

Verify with `tart list` — you should see `tahoe-base`.

## Running

```bash
make test-vm-parallel                                 # full suite (~14 min, 2 parallel VMs) — use before tagging
make test-vm                                          # serial fallback — single VM, useful for debugging
make test-vm-run TEST=TestVM_Journey_FirstTimeUser    # one test
```

## Environment variables

| Var | Default | Effect |
|---|---|---|
| `OPENBOOT_VM_BASE` | `tahoe-base` | Local Tart image to clone from |
| `OPENBOOT_VM_KEEP` | `0` | When `1`, do not destroy the VM at exit. Useful for debugging — attach with the SSH key path printed at exit. Remember to clean up manually. |

## Troubleshooting

- **`base image 'tahoe-base' not found locally`** — run the one-time
  setup above.
- **SSH not reachable within 60s** — Tart logs dumped to stderr.
  Try `OPENBOOT_VM_KEEP=1 make test-vm` (it'll still fail at boot, but the
  VM is left running so you can `tart ssh openboot-ephemeral-<pid>` and
  see what's up).
- **Leaked VMs after a hard kill** — just run `make test-vm` again; run.sh
  sweeps all `openboot-ephemeral-*` VMs on startup automatically. If you
  want to inspect a leaked VM before deleting it, stop and delete it manually:

  ```bash
  tart stop openboot-ephemeral-<pid>
  tart delete openboot-ephemeral-<pid>
  ```

- **Base image needs updating** — re-pull and re-clone:

  ```bash
  tart delete tahoe-base
  tart pull ghcr.io/cirruslabs/tahoe-base:latest
  tart clone ghcr.io/cirruslabs/tahoe-base:latest tahoe-base
  ```

## Why a base image (not vanilla)

`-base` includes Xcode CLI tools and Homebrew pre-installed. `run.sh`
installs Go on first boot via `mise` (which the base image ships). openboot
*does* install Homebrew, so "fresh Mac" purists would prefer
`macos-tahoe-vanilla`. We pick `-base` because:

1. Boot-to-test latency drops significantly vs. having to bootstrap brew
   and Go from scratch.
2. `MacHost.Run("brew install ...")` exercises the "already-installed
   brew" code path, which is the more common real-world scenario (users
   running openboot a second time, or after `brew install` ran for some
   other reason).
3. Tests that specifically need to exercise the "no brew yet" path can
   uninstall it as the first step.
