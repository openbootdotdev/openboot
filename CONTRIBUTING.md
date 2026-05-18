# Contributing

Contributions are welcome. @fullstackjam maintains the project and reviews all PRs — he has final say on what gets merged, but good ideas land fast.

## Quick Start

```bash
git clone https://github.com/YOUR_USERNAME/openboot.git
cd openboot
make install-hooks      # opt-in: pre-commit + pre-push checks
git checkout -b fix-something

# Fast feedback loop (runs on every push via git hook, ~15s)
make test-unit

# Commit + push
git commit -m "fix: the thing"
git push origin fix-something
```

Then open a PR — use the template, it's short.

## Good First Contributions

- Add a package to `internal/config/data/packages.yaml`
- Fix a typo or improve an error message
- Add a missing test

See [issues labeled `good first issue`](https://github.com/openbootdotdev/openboot/issues?q=is%3Aopen+label%3A%22good+first+issue%22) for tracked tasks.

## Test Layering

Tests are split across four tiers. Which one runs where:

| Tier | What | How to run | When it runs |
|------|------|------------|--------------|
| **L1 Unit + Integration + Contract** | Pure-Go logic with faked `Runner` *plus* real `brew` / `git` / `npm` against temp dirs and real `httptest` servers | `make test-unit` (~75s) | Every push (pre-push hook); CI on push/PR |
| **L2 Contract schema** | JSON schema validation against [openboot-contract](https://github.com/openbootdotdev/openboot-contract) | (runs in CI only) | CI on push/PR |
| **L3 E2E binary** | Compiled binary driven by scripts; `-tags=e2e` | `make test-e2e` | CI on release |
| **L4 VM e2e** | Full destructive suite (`-tags="e2e,vm"`) runs inside an ephemeral Tart VM provisioned by `scripts/vm/run.sh`. Installs real packages, modifies `~/.zshrc`, writes `defaults` — all contained to the throwaway VM. | `make test-vm-parallel` (~14 min, Apple Silicon + Tart required); `make test-vm` is the serial single-VM fallback | **Local only** — convention is to run before tagging a release. No CI gate. |

Rules of thumb:

- **Local dev:** run nothing manually if hooks are installed. `make test-unit` on demand when you want a sanity check. Skip L2+ unless you're cutting a release.
- **Before pushing:** `make test-unit` (the pre-push hook does this automatically). Requires `brew` / `git` / `npm` on PATH — they are queried read-only against temp dirs, no real installs.
- **Before tagging a release (convention, not enforced):** `make test-vm-parallel` on an Apple Silicon Mac with Tart installed. See [VM E2E setup](#vm-e2e-setup) below. `auto-release.yml` opens a `release-ready` issue on `feat:` thresholds to nudge you here.

## VM E2E setup

Destructive tests (L4) run inside an ephemeral Tart VM. One-time setup
on an Apple Silicon Mac:

```bash
brew install cirruslabs/cli/tart
tart pull ghcr.nju.edu.cn/cirruslabs/tahoe-base:latest
tart clone ghcr.nju.edu.cn/cirruslabs/tahoe-base:latest tahoe-base
```

Then:

```bash
make test-vm-parallel                                # full suite in parallel (~14 min)
make test-vm                                         # full suite serial fallback (~30 min)
make test-vm-run TEST=TestVM_Journey_FirstTimeUser   # one test
OPENBOOT_VM_KEEP=1 make test-vm-parallel             # don't destroy VMs at exit (debug)
```

See `scripts/vm/README.md` for full environment-variable docs and
troubleshooting.

## Git Hooks

`make install-hooks` symlinks two hooks from `scripts/hooks/` into `.git/hooks/`:

- **pre-commit** (<5s) — `go vet` + `go build`. Early-exits when no `.go` files are staged.
- **pre-push** (~75s) — `go test -race ./...` (L1 suite: unit + integration + contract).

Skip once with git's standard bypass flag. Remove entirely with `make uninstall-hooks`.

## Writing Tests

**When adding code that calls external binaries** (brew, npm, git, etc): don't hard-code `exec.Command` — use the package's `Runner` interface. See `internal/brew/runner.go` for the pattern. Tests can then substitute a pure-Go fake via `brew.SetRunner(…)`.

**Don't hit the network** in L1. Use `httptest.NewServer` + `server.Close()` when testing connection errors, not DNS timeouts on bogus hostnames.

**Don't assume filesystem state.** Use `t.TempDir()` and `t.Setenv("HOME", t.TempDir())` when code reads `~/.something`.

Integration tests live alongside L1 in `test/integration/` and *may* touch the real filesystem but only inside temp dirs. They must not depend on the developer's `~/.dotfiles`, `~/.zshrc`, or similar, and must only read (never install/uninstall) real `brew` / `npm` / `git` state.

## Code Expectations

- Standard Go style (`go vet` must pass — the pre-commit hook enforces this)
- Add tests for new features
- Conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`)
- One logical change per PR
- CLI breaking changes need an entry in [CHANGELOG.md](CHANGELOG.md) + a major version bump

## Architecture

See [CLAUDE.md](CLAUDE.md) for how everything fits together.

## Questions

Open a [Discussion](https://github.com/openbootdotdev/openboot/discussions).
