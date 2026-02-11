# OpenBoot

> Set up your Mac — or capture the one you already have.

[![Release](https://img.shields.io/github/v/release/openbootdotdev/openboot)](https://github.com/openbootdotdev/openboot/releases)
[![License](https://img.shields.io/github/license/openbootdotdev/openboot)](LICENSE)
[![codecov](https://codecov.io/gh/openbootdotdev/openboot/branch/main/graph/badge.svg)](https://codecov.io/gh/openbootdotdev/openboot)

<p align="center">
  <img src="demo.svg" alt="OpenBoot Demo" width="800" />
</p>

```bash
curl -fsSL openboot.dev/install | bash
```

OpenBoot bootstraps your entire macOS development environment in minutes — or snapshots the one you already have. Homebrew packages, GUI apps, dotfiles, Oh-My-Zsh, and macOS preferences — through an interactive TUI. No config files to write. No manual steps. Just one command.

## Why OpenBoot?

Setting up a new Mac still takes hours. You either run `brew install` 50 times, maintain a Brewfile, or wrestle with Nix. OpenBoot handles it all:

| | OpenBoot | Brewfile | Strap | chezmoi | nix-darwin |
|---|:---:|:---:|:---:|:---:|:---:|
| Homebrew packages | ✅ | ✅ | ✅ | — | ✅ |
| GUI apps (casks) | ✅ | ✅ | — | — | ✅ |
| Dotfiles | ✅ | — | — | ✅ | ✅ |
| Custom scripts | ✅ | — | — | ✅ | ✅ |
| Interactive TUI | ✅ | — | — | — | — |
| Web dashboard | ✅ | — | — | — | — |
| Team config sharing | ✅ | — | — | — | — |
| Skip already-installed | ✅ | — | ✅ | — | ✅ |
| One-command setup | ✅ | — | ✅ | ✅ | — |
| Learning curve | Low | Low | Low | High | Very High |

## Features

- **One-command setup** — `curl | bash` and you're done
- **Snapshot** — capture your existing Mac's setup and save it locally or share it as a config
- **Interactive TUI** — search and select from 50+ curated dev tools across 13 categories
- **3 presets** — minimal (CLI essentials), developer (ready-to-code), full (everything)
- **Smart install** — detects already-installed packages, only installs what's new
- **Parallel + sequential** — CLI tools install 4x in parallel, GUI apps install one at a time (for password prompts)
- **Dotfiles** — clone your repo and deploy configs via GNU Stow
- **Oh-My-Zsh** — installs with sensible aliases
- **macOS preferences** — developer-friendly defaults
- **Web dashboard** — create, share, and duplicate configs at [openboot.dev](https://openboot.dev)
- **Dry-run mode** — preview everything before installing
- **CI/automation** — silent mode with environment variables
- **No telemetry** — zero analytics, zero tracking

## Quick Start

### New Mac? Bootstrap it:

```bash
curl -fsSL openboot.dev/install | bash
```

OpenBoot guides you through:
1. Git identity configuration
2. Preset selection (minimal / developer / full)
3. Package customization (searchable, categorized)
4. Installation (parallel CLI, sequential GUI)
5. Shell setup (Oh-My-Zsh + aliases)
6. Dotfiles deployment (GNU Stow)
7. macOS preferences

### Already set up? Snapshot it:

```bash
curl -fsSL openboot.dev/install | bash -s -- snapshot
```

Captures your Homebrew packages, macOS preferences, shell config, and git settings. Save locally with `--local` or upload to share.

## Presets

| Preset | Focus | Includes |
|--------|-------|----------|
| **minimal** | CLI essentials | ripgrep, fd, bat, fzf, lazygit, gh, Warp, Raycast |
| **developer** | Ready-to-code | + Node, Go, Docker, VS Code, Chrome, OrbStack |
| **full** | Complete setup | + Python, Rust, kubectl, Terraform, Ollama, Cursor, Figma |

## Custom Configs

Create a config at [openboot.dev/dashboard](https://openboot.dev/dashboard) and share it with your team:

```bash
curl -fsSL openboot.dev/YOUR_USERNAME | bash
```

Import from an existing Brewfile, pick packages from the searchable catalog, or duplicate an existing config.

## CI / Automation

```bash
OPENBOOT_GIT_NAME="Your Name" \
OPENBOOT_GIT_EMAIL="you@example.com" \
curl -fsSL openboot.dev/install | bash -s -- --preset developer --silent
```

## Commands

```bash
openboot                 # Interactive setup
openboot doctor          # Check system health
openboot update          # Update Homebrew and packages
openboot update --dry-run  # Preview updates
openboot version         # Print version
```

## CLI Options

```
-p, --preset NAME   Set preset (minimal, developer, full)
-u, --user NAME     Use remote config from openboot.dev
-s, --silent        Non-interactive mode (requires env vars)
    --dry-run       Preview what would be installed
    --update        Update Homebrew and packages
    --rollback      Restore backed up files
    --resume        Resume incomplete installation
    --shell MODE    Shell setup: install, skip
    --macos MODE    macOS prefs: configure, skip
    --dotfiles MODE Dotfiles: clone, link, skip
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENBOOT_GIT_NAME` | Git user name (required in silent mode) |
| `OPENBOOT_GIT_EMAIL` | Git user email (required in silent mode) |
| `OPENBOOT_PRESET` | Default preset |
| `OPENBOOT_USER` | Remote config username |

## Requirements

- macOS 12.0 (Monterey) or later
- Apple Silicon or Intel
- Internet connection
- Admin privileges (for Homebrew)

## Development

```bash
git clone https://github.com/openbootdotdev/openboot.git
cd openboot
go build -o openboot ./cmd/openboot
./openboot --dry-run
```

## Related

- [openboot.dev](https://openboot.dev) — Website, dashboard & API
- [dotfiles](https://github.com/openbootdotdev/dotfiles) — Dotfiles template

## License

MIT
