# CLAUDE.md

## Project

OpenBoot is a **macOS-only** Go 1.24 CLI that automates dev-environment setup: Homebrew packages/casks, npm globals, Oh-My-Zsh, macOS `defaults`, and dotfiles. Built on **Cobra** (CLI) + **Charmbracelet** (bubbletea / lipgloss / huh for TUI).

Entry point: `cmd/openboot/main.go` → `internal/cli.Execute()`.
Core flow: `openboot install` runs a 7-step wizard in `internal/installer/installer.go`.

For full contribution guide (test layering L1–L6, Runner interface, hook setup) see @CONTRIBUTING.md.
For protocol / data model / P1–P7 invariants see @docs/SPEC.md.

## Commands

```bash
# Build — version injected via -ldflags, default is "dev"
make build
make build-release VERSION=0.25.0    # optimized + UPX

# Test — full tier table in CONTRIBUTING.md
make test-unit                       # L1 (~15s) — pre-push hook
make test-integration                # L2 (~75s) — real brew/git/npm in temp dirs
make test-e2e                        # L4 compiled binary
make test-vm-release                 # L5 destructive macOS (~20m) — before tagging
make test-destructive                # L6 — actually installs
make test-coverage                   # coverage.out + coverage.html

# Single test
go test -v -run TestFoo ./internal/<pkg>/...

# Hooks (opt-in): pre-commit = vet+build, pre-push = L1
make install-hooks

go vet ./...
make clean
```

## Layout

```
cmd/openboot/        # main.go → cli.Execute()
internal/
  auth/              # OAuth-like login, token in ~/.openboot/auth.json (0600)
  brew/              # Homebrew ops, sequential install with retry, uninstall
  cli/               # Cobra cmds: install, snapshot, login, logout, version
  config/            # Package catalog + presets + remote fetch (embed fallback in data/)
  diff/              # Pure-logic system-vs-config comparison
  dotfiles/          # Clone + stow with .openboot.bak backup
  httputil/          # HTTP Do() with rate-limit + Retry-After
  installer/         # 7-step wizard orchestrator + snapshot restore
  macos/             # defaults write + app restart
  npm/               # Batch install with sequential fallback
  permissions/       # macOS screen-recording probe
  search/            # openboot.dev API client (8s timeout)
  shell/             # Oh-My-Zsh install + .zshrc + snapshot restore
  snapshot/          # Capture / match / restore env state
  state/             # Reminder state in ~/.openboot/state.json
  sync/              # Compute diff + execute plan for remote config
  system/            # RunCommand / RunCommandSilent, arch, git config
  ui/                # bubbletea Model pattern, lipgloss styling
  updater/           # Auto-update: check GitHub → download → replace
test/{integration,e2e}/   # //go:build integration | e2e (+ vm, destructive, smoke)
testutil/            # shared helpers + MacHost (destructive E2E on real macOS)
scripts/
  install.sh         # curl|bash installer
  hooks/             # pre-commit, pre-push (install via `make install-hooks`)
```

## Where to Look

| Task | Location | Notes |
|------|----------|-------|
| Add CLI command | `internal/cli/` | Register in `root.go init()`, follow cobra pattern |
| Change install flow | `internal/installer/installer.go` | 7-step wizard orchestrator |
| Change sync behavior | `internal/sync/diff.go`, `internal/sync/plan.go` | Diff → confirm → execute |
| Add package category | `openboot.dev/src/lib/package-metadata.ts` | Server is source of truth; CLI fetches `/api/packages` and caches 24h in `~/.openboot/packages-cache.json`. `data/packages.yaml` is fallback only. |
| Modify presets | `internal/config/data/presets.yaml` | 3 presets: minimal, developer, full |
| Change brew behavior | `internal/brew/brew.go` + `brew_install.go` | Parallel workers, StickyProgress, Uninstall/UninstallCask |
| Add snapshot data | `internal/snapshot/capture.go` | Extend `CaptureWithProgress` steps |
| Update self-update | `internal/updater/updater.go` | `AutoUpgrade()` called from `root.go` RunE |
| Change publish flow | `internal/cli/snapshot_publish.go` (`publishSnapshot`) | Slug resolution, see P7 in `docs/SPEC.md` |
| Source resolution (install) | `internal/cli/install.go` (`resolvePositionalArg`) | file / user-slug / preset / alias detection |
| HTTP with retry | `internal/httputil/ratelimit.go` | Use `httputil.Do()` — handles 429 + Retry-After |
| Test tier / when to run | `CONTRIBUTING.md` "Test Layering" | L1–L6 table |
| Release process | `.github/workflows/` | Tag-driven, release-notes template |

## Project-specific conventions

These cannot be inferred from code alone — everything else is enforced by `go vet` / review.

- **Error wrapping**: `fmt.Errorf("context: %w", err)` — never bare returns.
- **UI output**: always through `ui.*` helpers; raw `fmt.Println` is a bug in user-facing paths.
- **Subprocess**: `system.RunCommand` (interactive) / `system.RunCommandSilent` (captured). Do not call `exec.Command` directly from feature code — add to `system/` if a wrapper is missing.
- **Destructive ops**: check `cfg.DryRun` first. Always.
- **Paths**: `os.UserHomeDir()` — never hardcode `~` or `/Users/...`.
- **State**: everything user-local goes under `~/.openboot/` (auth, cache, snapshots, state).
- **Concurrency**: bounded `sync.WaitGroup` — brew install is sequential with retry; `GetInstalledPackages` uses 2 goroutines for formula+cask list. No unbounded goroutines.
- **Embedded data**: `//go:embed data/*.yaml` loaded in `init()`.
- **Tests**: table-driven, `testify/require` for fatal, `testify/assert` for non-fatal. L1 uses the `Runner` interface to fake subprocess calls — no real network, no real fork.
- **Commits**: Conventional (`feat:` / `fix:` / `docs:` / `refactor:` / `test:` / `chore:` / `ci:`), one thing per commit.

## Env vars

| Var | Purpose |
|-----|---------|
| `OPENBOOT_DISABLE_AUTOUPDATE=1` | Skip auto-update check |
| `OPENBOOT_GIT_NAME` / `OPENBOOT_GIT_EMAIL` | Git identity override (silent mode) |
| `OPENBOOT_PRESET` | Default preset |
| `OPENBOOT_USER` | Config alias/username |
| `OPENBOOT_API_URL` | Override API base URL (testing; https or http://localhost only) |
| `OPENBOOT_DOTFILES` | Override dotfiles repo URL |
| `OPENBOOT_DRY_RUN` | Dry-run mode for `scripts/install.sh` (not the CLI) |
| `OPENBOOT_VERSION` | Pin version in `scripts/install.sh` |
