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

Tests are split across six tiers. Which one runs where:

| Tier | What | How to run | When it runs |
|------|------|------------|--------------|
| **L1 Unit / Contract** | Pure-Go logic + command runners faked via `Runner` interface (no fork, no network) | `make test-unit` (~15s) | Every commit (pre-push hook); CI on push/PR |
| **L2 Integration** | Real `brew` / `git` / `npm` against temp dirs; real `httptest` servers | `make test-integration` (~75s) | CI on push/PR |
| **L3 Contract schema** | JSON schema validation against [openboot-contract](https://github.com/openbootdotdev/openboot-contract) | (runs in CI only) | CI on push/PR |
| **L4 E2E binary** | Compiled binary driven by scripts; `-tags=e2e` | `make test-e2e` | CI on release |
| **L5 E2E VM** | [Tart](https://github.com/cirruslabs/tart) macOS VMs (install Homebrew, run real flows) | `make test-vm-quick` (2 min) / `test-vm-release` (20 min) / `test-vm-full` (60 min) | Manual, before tagging a release |
| **L6 Destructive** | Actually installs real packages into a real system | `make test-destructive` / `test-smoke` | CI on release, plus manual `workflow_dispatch` |

Rules of thumb:

- **Local dev:** run nothing manually if hooks are installed. `make test-unit` on demand when you want a sanity check. Skip L2+ unless you're touching code that interacts with real brew/git/npm.
- **Before pushing:** `make test-unit` (the pre-push hook does this automatically).
- **Before tagging a release:** `make test-vm-release` locally (needs Tart).

## Git Hooks

`make install-hooks` symlinks two hooks from `scripts/hooks/` into `.git/hooks/`:

- **pre-commit** (<5s) — `go vet` + `go build`. Early-exits when no `.go` files are staged.
- **pre-push** (~15s) — `go test -race ./...` (L1 suite).

Skip once with git's standard bypass flag. Remove entirely with `make uninstall-hooks`.

## Writing Tests

**When adding code that calls external binaries** (brew, npm, git, etc): don't hard-code `exec.Command` — use the package's `Runner` interface. See `internal/brew/runner.go` for the pattern. Tests can then substitute a pure-Go fake via `brew.SetRunner(…)`.

**Don't hit the network** in L1. Use `httptest.NewServer` + `server.Close()` when testing connection errors, not DNS timeouts on bogus hostnames.

**Don't assume filesystem state.** Use `t.TempDir()` and `t.Setenv("HOME", t.TempDir())` when code reads `~/.something`.

Integration tests (L2) *may* touch the real filesystem but only inside temp dirs, and they must not depend on the developer's `~/.dotfiles`, `~/.zshrc`, or similar.

## Code Expectations

- Standard Go style (`go vet` must pass — the pre-commit hook enforces this)
- Add tests for new features
- Conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`)
- One logical change per PR
- CLI breaking changes need an entry in [CHANGELOG.md](CHANGELOG.md) + a major version bump

## Architecture

See [CLAUDE.md](CLAUDE.md) for how everything fits together and [docs/SPEC.md](docs/SPEC.md) for the v1.0 CLI surface.

## Questions

Open a [Discussion](https://github.com/openbootdotdev/openboot/discussions).
