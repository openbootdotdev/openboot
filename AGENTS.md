# PROJECT KNOWLEDGE BASE

**Generated:** 2026-02-11
**Commit:** 5fd1715
**Branch:** main

## OVERVIEW

Mac dev environment setup CLI. Go 1.24 + Cobra + Charmbracelet (bubbletea/lipgloss/huh) TUI.
Installs Homebrew packages, casks, npm globals, shell config, macOS preferences, dotfiles.

## STRUCTURE

```
openboot/
├── cmd/openboot/         # main.go → cli.Execute()
├── internal/
│   ├── auth/             # OAuth-like login, token in ~/.openboot/auth.json (0600)
│   ├── brew/             # Homebrew ops, parallel install (4 workers), retry logic
│   ├── cli/              # Cobra commands: root, snapshot, doctor, update, version
│   ├── config/           # Embedded YAML (packages + presets), remote config fetch
│   │   └── data/         # packages.yaml (9 categories), presets.yaml (3 presets)
│   ├── dotfiles/         # Clone + stow/symlink with .openboot.bak backup
│   ├── installer/        # Main orchestrator: 7-step wizard (693 lines)
│   ├── macos/            # `defaults write` preferences, app restart
│   ├── npm/              # Batch install with sequential fallback
│   ├── search/           # Online search via openboot.dev API (8s timeout)
│   ├── shell/            # Oh-My-Zsh install, .zshrc config
│   ├── snapshot/         # Capture/match/restore environment state (see subdir AGENTS.md)
│   ├── system/           # RunCommand/RunCommandSilent, arch detection, git config
│   ├── ui/               # TUI components (see subdir AGENTS.md)
│   └── updater/          # Auto-update: check GitHub → download → replace binary
├── test/
│   ├── integration/      # Build tag: //go:build integration
│   └── e2e/              # Build tag: //go:build e2e
├── testutil/             # Shared test helpers
├── scripts/install.sh    # curl|bash installer (detects arch, verifies checksums)
└── Makefile              # Build targets
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add CLI command | `internal/cli/` | Register in root.go init(), follow cobra pattern |
| Add package category | `internal/config/data/packages.yaml` | Rebuild after changing embedded YAML |
| Change install flow | `internal/installer/installer.go` | 7 steps: homebrew → git → preset → packages → shell → macos → dotfiles |
| Add TUI component | `internal/ui/` | Use bubbletea Model pattern, lipgloss styling |
| Change brew behavior | `internal/brew/brew.go` | Parallel workers, StickyProgress for output |
| Add snapshot data | `internal/snapshot/capture.go` | Add to CaptureWithProgress steps |
| Update self-update | `internal/updater/updater.go` | AutoUpgrade() called from root.go RunE |
| Modify presets | `internal/config/data/presets.yaml` | 3 presets: minimal, developer, full |

## DEPENDENCY GRAPH

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
├── updater → ui
├── auth → ui
└── snapshot → config, macos
```

## CONVENTIONS

- **Error wrapping**: `fmt.Errorf("context: %w", err)` — always wrap with context
- **UI output**: Use `ui.Header/Success/Error/Info/Warn/Muted` — never raw fmt for user-facing text
- **Command exec**: `system.RunCommand()` (interactive) or `system.RunCommandSilent()` (capture output)
- **Embedded data**: `//go:embed data/*.yaml` with `embed.FS`, loaded in `init()`
- **Testing**: Table-driven with testify/assert. Build tags for integration/e2e
- **Concurrency**: `sync.WaitGroup` with bounded workers (max 4 for brew)
- **Dry-run**: All destructive operations check `cfg.DryRun` first
- **Version string**: Default `"dev"` in `internal/cli/root.go` — injected via ldflags at build time, never edit manually
- **Config storage**: `~/.openboot/` directory for auth, state, snapshots

## ANTI-PATTERNS

- No `as any` equivalent — no type assertion abuse
- No ignored errors (`_ = err`) in production code
- No `panic()` except `log.Fatalf` in `init()` for fatal config errors
- No hardcoded `~` paths — always `os.UserHomeDir()`
- No unbounded goroutines — always WaitGroup + max workers
- No direct stdout for styled text — always through `ui` package

## COMMANDS

```bash
make build                    # Dev build (version=dev)
make build VERSION=0.19.0     # Build with specific version
make build-release VERSION=0.19.0  # Optimized + UPX with version
make test-unit                # go test -v ./...
make test-integration         # go test -v -tags=integration ./...
make test-e2e                 # go test -v -tags=e2e -short ./...
make test-all                 # All above + coverage
make clean                    # Remove binaries + coverage
go vet ./...                  # Lint check
```

## RELEASE PROCESS

Tag-driven. CI handles everything. **Never edit root.go for version bumps.**

```bash
git tag v0.19.0
git push --tags
# CI builds binaries with version injected via ldflags, creates GitHub release
```

- Version is `"dev"` in source — overridden by `-ldflags -X` at build time
- Dev builds (`version=dev`) skip auto-update
- CI workflow: `.github/workflows/release.yml` extracts version from git tag

**When to release**: Only for user-facing changes (features, bug fixes, package updates). Skip for docs, AGENTS.md, CI config, test-only changes.

## NOTES

- **macOS only**: No Linux/Windows support. darwin binaries only.
- **Auto-update**: Enabled by default. Config: `~/.openboot/config.json` `{"autoupdate": "true"|"notify"|"false"}`. Env: `OPENBOOT_DISABLE_AUTOUPDATE=1`.
- **Color palette**: Primary #22c55e (green), Secondary #60a5fa (blue), Warning #eab308, Danger #ef4444, Subtle #666666.
- **Snapshot upload**: Requires auth token from openboot.dev OAuth flow.
- **install.sh**: Supports `OPENBOOT_DRY_RUN=true` and `OPENBOOT_INSTALL_DIR=path`.
