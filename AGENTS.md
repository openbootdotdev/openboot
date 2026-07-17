# AGENTS.md

Canonical, tool-neutral instructions for coding agents working on this repo.
Codex reads this file directly; Claude Code imports it from `CLAUDE.md`.
If you're a human, you probably want [README.md](README.md) or
[CONTRIBUTING.md](CONTRIBUTING.md).

## Agent compatibility

- Keep shared repository guidance in this file. Do not duplicate it in a
  tool-specific instruction file.
- `CLAUDE.md` is a thin Claude Code adapter: it imports this file with
  `@AGENTS.md` and may contain only Claude-specific additions.
- Shared skills use the open Agent Skills format and live in
  `.agents/skills/<name>/SKILL.md`. Claude Code discovers the same skills
  through relative symlinks under `.claude/skills/`.
- Other tools that implement `AGENTS.md` or Agent Skills can consume the same
  canonical paths without another copy of the instructions.
- Tool-specific integration stays tool-specific. Claude Code hooks and
  permissions remain in `.claude/settings.json` and `.claude/hooks/`.

When adding a shared skill, create it under `.agents/skills/`, then expose it
to Claude Code without copying its contents:

```bash
mkdir -p .claude/skills/<name>
ln -s ../../../.agents/skills/<name>/SKILL.md .claude/skills/<name>/SKILL.md
```

The `agent-config` architecture test verifies the import and skill adapters.

## Read first

- **[CONTRIBUTING.md](CONTRIBUTING.md)** — test layering L1-L4, Runner
  interface, and hook setup.
- **[docs/HARNESS.md](docs/HARNESS.md)** — the steering meta-doc: when a
  class of issue recurs, which file to edit to prevent it next time.
- **[internal/archtest/README.md](internal/archtest/README.md)** — architecture
  fitness functions and baseline workflow.

## Project

OpenBoot is a **macOS-only** Go 1.25 CLI that automates dev-environment setup: Homebrew packages/casks, npm globals, Oh-My-Zsh, macOS `defaults`, and dotfiles. Built on **Cobra** (CLI) + **Charmbracelet** (bubbletea / lipgloss / huh for TUI).

Entry point: `cmd/openboot/main.go` -> `internal/cli.Execute()`.
Core flow: `openboot install` orchestrates plan -> apply in `internal/installer/installer.go`.

**The wizard plans; the apply is always linear.** On a TTY the planning phase runs as a full-screen TUI in `internal/ui/tui/wizard/` (boot probe -> select -> git -> review), then the wizard *exits* and hands its `InstallPlan` to `installer.ApplyReviewedPlan`, which applies it on the normal terminal with `ConsoleReporter` + `ui.StickyProgress`. This split is deliberate: a TUI is right for browsing a 100+ package catalog, and wrong for the apply — an alt-screen install discards its own output when it exits, so twenty minutes of package results and failures vanish. Streamed into the scrollback they stay where the user can scroll back, copy an error, and pipe it. Don't move the apply back inside the alt-screen.

Entry points: bare `install` -> `wizard.Run`; `-p <preset>` -> same, loadout preselected; slug/`-u`/`--from`/alias -> `wizard.RunForConfig` (config mode: the config's own packages on the select screen, preselected). Sync-source installs keep their linear diff pre-flight and apply linearly. `--silent`, `--dry-run`, `--update`, `--pick`, and non-TTY runs never enter the wizard.

## Working in parallel

This repo is often worked on from multiple concurrent terminals / agent sessions. Default to `git worktree add ../openboot-<topic> -b <branch>` rather than juggling branches in a single checkout — two sessions racing on the same working tree corrupts state (mid-edit files, half-staged diffs, hooks firing against the wrong branch). One worktree per concurrent task; remove it with `git worktree remove` when the PR merges.

## Commands

```bash
# Build — version injected via -ldflags, default is "dev"
make build
make build-release VERSION=0.25.0    # optimized + UPX

# Test — full tier table in CONTRIBUTING.md
make test-unit                       # L1 (~75s) — unit + integration + contract; pre-push hook
make test-e2e                        # L3 compiled binary
                                     # L4 — destructive e2e runs in CI only (vm-e2e-spike.yml on macos-14)
make test-coverage                   # coverage.out + coverage.html

# Single test
go test -v -run TestFoo ./internal/<pkg>/...

# Hooks (opt-in): pre-commit = vet+build, pre-push = L1
make install-hooks

go vet ./...
make clean
```

## Layout

```text
cmd/openboot/        # main.go -> cli.Execute()
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
  updater/           # Auto-update: check GitHub -> download -> replace
  doctor/            # Diagnostic checks for openboot doctor command
  logging/           # Structured rotating file log under ~/.openboot/logs/
test/{integration,e2e}/   # integration runs as part of L1; e2e gated by build tags (e2e, vm)
testutil/            # shared helpers + MacHost (destructive E2E on real macOS)
scripts/
  install.sh         # curl|bash installer
  hooks/             # pre-commit, pre-push (install via `make install-hooks`)
```

## Where to look

| Task | Location | Notes |
|------|----------|-------|
| Add CLI command | `internal/cli/` | Register in `root.go init()`, follow cobra pattern |
| Change install flow | `internal/installer/installer.go` | plan -> apply orchestrator; `PlanFromSelection` builds a plan from TUI picks |
| Change interactive install TUI | `internal/ui/tui/wizard/` | Redesign v5: boot/select/install screens; live install streams `internal/progress` events (brew/npm `SetProgressSink`) |
| Change sync behavior | `internal/sync/diff.go`, `internal/sync/plan.go` | Diff -> confirm -> execute |
| Add package category | `openboot.dev/src/lib/package-metadata.ts` | Server is source of truth; CLI fetches `/api/packages` and caches 24h in `~/.openboot/packages-cache.json`. `data/packages.yaml` is fallback only. |
| Modify presets | `internal/config/data/presets.yaml` | 3 presets: minimal, developer, full |
| Change brew behavior | `internal/brew/brew.go` + `brew_install.go` | Parallel workers, StickyProgress, Uninstall/UninstallCask |
| Cask download progress | `internal/brew/cache.go` + `internal/brew/sizecheck.go` | HEAD pre-fetch -> poll `brew --cache` for bytes; consumed by `installCasksWithProgress` |
| Add snapshot data | `internal/snapshot/capture.go` | Extend `CaptureWithProgress` steps |
| Update self-update | `internal/updater/updater.go` | `AutoUpgrade()` called from `root.go` RunE |
| Change publish flow | `internal/cli/snapshot_publish.go` (`publishSnapshot`) | Slug resolution |
| Source resolution (install) | `internal/cli/install.go` (`resolvePositionalArg`) | file / user-slug / preset / alias detection |
| HTTP with retry | `internal/httputil/ratelimit.go` | Use `httputil.Do()` — handles 429 + Retry-After (archtest: `no-raw-http`) |
| Test tier / when to run | `CONTRIBUTING.md` "Test Layering" | L1-L4 table |
| Release process | `.github/workflows/` | Tag-driven, release-notes template |

## Project-specific conventions

These cannot be inferred from code alone — everything else is enforced by `go vet` / review.
Bolded rules are enforced mechanically by `internal/archtest` (fitness functions in L1).

- **Error wrapping**: `fmt.Errorf("context: %w", err)` — never bare returns.
- **UI output** *(archtest: `fmtprint`)*: always through `ui.*` helpers; raw `fmt.Println` is a bug in user-facing paths.
- **Subprocess** *(archtest: `no-direct-exec`)*: `system.RunCommand` (interactive) / `system.RunCommandSilent` (captured). Do not call `exec.Command` directly from feature code — add to `system/` if a wrapper is missing.
- **Destructive ops** *(archtest: `dryrun`)*: check `cfg.DryRun` first. Always.
- **Paths** *(archtest: `no-os-getenv-home`)*: `os.UserHomeDir()` — never hardcode `~` or `/Users/...`, never `os.Getenv("HOME")`.
- **State**: everything user-local goes under `~/.openboot/` (auth, cache, snapshots, state).
- **Concurrency**: bounded `sync.WaitGroup` — brew install is sequential with retry; `GetInstalledPackages` uses 2 goroutines for formula+cask list. No unbounded goroutines.
- **Embedded data**: `//go:embed data/*.yaml` loaded in `init()`.
- **Tests**: table-driven, `testify/require` for fatal, `testify/assert` for non-fatal. L1 uses the `Runner` interface to fake subprocess calls — no real network, no real fork.
- **Commits**: Conventional (`feat:` / `fix:` / `docs:` / `refactor:` / `test:` / `chore:` / `ci:`), one thing per commit.

## When archtest fails

The failure message tells you exactly what to do:

1. If you added a violation by accident, fix the code by using the allowed
   wrapper or moving the call into the appropriate package.
2. If the violation is intentional, update the baseline:

   ```bash
   ARCHTEST_UPDATE_BASELINE=1 go test ./internal/archtest/...
   git add internal/archtest/baseline/
   ```

The baseline diff is the audit trail. Explain why the new call site is
justified in the commit message; do not silence the rule.

## Run before committing

```bash
go vet ./...                 # cheap, ~1s
go test ./internal/...       # L1, ~15s, includes archtest
```

Both are wired into `scripts/hooks/pre-commit` (vet only) and
`scripts/hooks/pre-push` (full L1). Install once with `make install-hooks`.

## Actions that require confirmation

- `git push --force` against `main` or release tags.
- `git commit --amend` on commits already pushed.
- `git reset --hard` when it would discard uncommitted work.
- Triggering L4 e2e tests outside CI; they install real packages on the host.
- Modifying the user's `~/.zshrc`, Homebrew installation, or macOS `defaults`.

Repository reads and edits, `make test-unit`, and `go vet` are safe without
extra confirmation.

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
