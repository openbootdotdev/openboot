# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

OpenBoot is a macOS-only CLI tool (Go 1.24) that automates dev environment setup — Homebrew packages/casks, npm globals, shell config (Oh-My-Zsh), macOS preferences, and dotfiles. Built with Cobra (CLI) and Charmbracelet (bubbletea/lipgloss/huh for TUI).

## Commands

```bash
# Build
make build                         # Dev build (version=dev)
make build VERSION=0.24.0          # Specific version
make build-release VERSION=0.24.0  # Optimized + UPX compression

# Test
make test-unit                     # go test -v -timeout 5m ./...
make test-integration              # go test -v -tags=integration ./...
make test-e2e                      # go test -v -tags=e2e -short ./...
make test-all                      # All tests + coverage report

# Run a single test
go test -v -run TestFunctionName ./internal/package/...

# Lint
go vet ./...

# Clean
make clean
```

## Architecture

Entry point: `cmd/openboot/main.go` → `cli.Execute()`.

The installer orchestrates a 7-step wizard: check deps → Homebrew setup → git config → preset selection → package install → shell setup → macOS prefs + dotfiles.

### Structure

```
openboot/
├── cmd/openboot/         # main.go → cli.Execute()
├── internal/
│   ├── auth/             # OAuth-like login, token in ~/.openboot/auth.json (0600)
│   ├── brew/             # Homebrew ops, parallel install (4 workers), retry logic, uninstall
│   ├── cleaner/          # Diff current system vs desired state, remove extra packages
│   ├── cli/              # Cobra commands: root, snapshot, doctor, clean, update, version
│   ├── config/           # Embedded YAML (packages + presets), remote config fetch
│   │   └── data/         # packages.yaml (9 categories), presets.yaml (3 presets)
│   ├── dotfiles/         # Clone + stow/symlink with .openboot.bak backup
│   ├── installer/        # Main orchestrator: 7-step wizard + snapshot restore
│   ├── macos/            # `defaults write` preferences, app restart
│   ├── npm/              # Batch install with sequential fallback, uninstall
│   ├── search/           # Online search via openboot.dev API (8s timeout)
│   ├── shell/            # Oh-My-Zsh install, .zshrc config, snapshot restore
│   ├── snapshot/         # Capture/match/restore environment state
│   ├── system/           # RunCommand/RunCommandSilent, arch detection, git config
│   ├── ui/               # TUI components (bubbletea Model pattern, lipgloss styling)
│   └── updater/          # Auto-update: check GitHub → download → replace binary
├── test/
│   ├── integration/      # Build tag: //go:build integration
│   └── e2e/              # Build tag: //go:build e2e
├── testutil/             # Shared test helpers
├── scripts/install.sh    # curl|bash installer (detects arch, verifies checksums)
└── Makefile
```

### Dependency Graph

```
cli (root)
├── installer (orchestrator)
│   ├── brew → ui
│   ├── npm → ui
│   ├── config (no deps)
│   ├── dotfiles (no deps)
│   ├── macos (no deps)
│   ├── shell (no deps)
│   ├── system (no deps)
│   └── ui → config, search, snapshot, system
├── cleaner → brew, npm, snapshot, ui
├── updater → ui
├── auth → ui
└── snapshot → config, macos
```

### Where to Look

| Task | Location | Notes |
|------|----------|-------|
| Add CLI command | `internal/cli/` | Register in root.go init(), follow cobra pattern |
| Add package category | `internal/config/data/packages.yaml` | Rebuild after changing embedded YAML |
| Change install flow | `internal/installer/installer.go` | 7 steps: homebrew → git → preset → packages → shell → macos → dotfiles |
| Change clean/uninstall | `internal/cleaner/cleaner.go` | Diffs current vs desired, calls brew/npm Uninstall |
| Add TUI component | `internal/ui/` | Use bubbletea Model pattern, lipgloss styling |
| Change brew behavior | `internal/brew/brew.go` | Parallel workers, StickyProgress for output, Uninstall/UninstallCask |
| Add snapshot data | `internal/snapshot/capture.go` | Add to CaptureWithProgress steps |
| Change snapshot restore | `internal/installer/installer.go` | stepRestoreGit, stepRestoreShell + packages/shell/macos |
| Update self-update | `internal/updater/updater.go` | AutoUpgrade() called from root.go RunE |
| Modify presets | `internal/config/data/presets.yaml` | 3 presets: minimal, developer, full |

## Conventions

- **Error wrapping**: Always `fmt.Errorf("context: %w", err)` — never bare error returns
- **UI output**: Use `ui.Header/Success/Error/Info/Warn/Muted` — never raw `fmt` for user-facing text
- **Command execution**: `system.RunCommand()` (interactive) or `system.RunCommandSilent()` (capture output)
- **Embedded data**: `//go:embed data/*.yaml` with `embed.FS`, loaded in `init()`
- **Dry-run**: All destructive operations must check `cfg.DryRun` first
- **Paths**: Always `os.UserHomeDir()` — never hardcoded `~`
- **Concurrency**: `sync.WaitGroup` with bounded workers (max 4 for brew) — no unbounded goroutines
- **Version**: Default `"dev"` in `internal/cli/root.go`, injected via `-ldflags -X` at build time. Never edit manually.
- **Config storage**: `~/.openboot/` directory for auth, state, snapshots
- **Commits**: Conventional format (`feat:`, `fix:`, `docs:`, `refactor:`), one thing per commit
- **CLI changes**: Must maintain backward compatibility — old syntax continues to work
- **Testing**: Table-driven tests with `testify/assert` (non-fatal) and `testify/require` (fatal). Integration tests use `//go:build integration`, E2E uses `//go:build e2e`.
- **No `panic()`** except `log.Fatalf` in `init()` for fatal config errors
- **No ignored errors** (`_ = err`) in production code
- **No direct stdout** for styled text — always through `ui` package
- **Color palette**: Primary `#22c55e`, Secondary `#60a5fa`, Warning `#eab308`, Danger `#ef4444`, Subtle `#666666`

## Release Process

Tag-driven. CI handles everything. **Never edit root.go for version bumps.**

```bash
git tag v0.25.0
git push --tags
# CI builds binaries with version injected via ldflags, creates GitHub release
```

- Version is `"dev"` in source — overridden by `-ldflags -X` at build time
- Dev builds (`version=dev`) skip auto-update
- CI workflow: `.github/workflows/release.yml` extracts version from git tag
- **When to release**: Only for user-facing changes (features, bug fixes, package updates). Skip for docs, CI config, test-only changes.

### Writing Release Notes

CI creates a release with a generic install-only body. After CI completes, update with a proper changelog via `gh release edit`.

**Step 1: Gather commits since last release**

```bash
PREV_TAG=$(git tag --sort=-v:refname | sed -n '2p')
git log ${PREV_TAG}..HEAD --oneline
```

**Step 2: Write the changelog**

```markdown
## What's New
- **Feature name** — One sentence, user-facing benefit only (`openboot <command>`)

## Improvements
- **Area** — What changed and why users care

## Bug Fixes
- **What was broken** — What's fixed now

## Installation

\`\`\`bash
brew install openbootdotdev/tap/openboot
\`\`\`

## Binaries

| Platform | Architecture | Download |
|----------|--------------|----------|
| macOS | Apple Silicon (M1/M2/M3/M4) | `openboot-darwin-arm64` |
| macOS | Intel | `openboot-darwin-amd64` |
```

**Rules:**

- Omit empty sections (no "Bug Fixes" if there are none)
- Write for **users**, not developers. No internal refactors, no test-only changes
- **Bold name**: 2–4 words max, noun form. Not a sentence.
- **Description**: ONE sentence, ~10–15 words max. User benefit only — no implementation details.
- Include the CLI command at the end if it's a new/changed command
- Keep Installation and Binaries sections at the bottom (always)

**Do / Don't:**

```
✓ - **Post-install script** — Run custom shell commands after your environment is set up (`openboot -u <user>`)
✗ - **Post-install script** — Run custom shell commands after your environment is set up. Add a post_install array to your config on openboot.dev and each command runs sequentially in your home directory after packages, shell, dotfiles, and macOS preferences are applied.

✓ - **Custom config install** — Shell, dotfiles, and macOS setup now run correctly when installing from a remote config
✗ - **Custom config installs now run shell, dotfiles, and macOS setup** — When installing via openboot -u <user>, shell configuration (Oh-My-Zsh), dotfiles cloning, and macOS preferences were silently skipped. All three steps now run as expected.
```

**Step 3: Update the release on GitHub**

Use a `'EOF'` heredoc so the shell doesn't interpret backticks:

```bash
gh release edit v0.25.0 --repo openbootdotdev/openboot --notes "$(cat <<'EOF'
## What's New
- **Feature name** — One sentence description (`openboot <command>`)

## Installation

```bash
brew install openbootdotdev/tap/openboot
```

## Binaries

| Platform | Architecture | Download |
|----------|--------------|----------|
| macOS | Apple Silicon (M1/M2/M3/M4) | `openboot-darwin-arm64` |
| macOS | Intel | `openboot-darwin-amd64` |
EOF
)"
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `OPENBOOT_DRY_RUN` | Enable dry-run mode |
| `OPENBOOT_DISABLE_AUTOUPDATE` | Disable auto-update (`1`) |
| `OPENBOOT_GIT_NAME` / `OPENBOOT_GIT_EMAIL` | Git identity |
| `OPENBOOT_PRESET` | Default preset |
| `OPENBOOT_USER` | Config alias/username |
| `OPENBOOT_VERSION` | Version for `install.sh` |
| `OPENBOOT_INSTALL_DIR` | Custom install directory |

## Notes

- **macOS only**: No Linux/Windows support. darwin binaries only.
- **Auto-update**: Enabled by default. Config: `~/.openboot/config.json` `{"autoupdate": "true"|"notify"|"false"}`. Env: `OPENBOOT_DISABLE_AUTOUPDATE=1`.
- **Snapshot upload**: Requires auth token from openboot.dev OAuth flow.
- **install.sh**: Supports `OPENBOOT_DRY_RUN=true` and `OPENBOOT_INSTALL_DIR=path`.
