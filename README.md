# OpenBoot

> One command. Your Mac is ready to code.

<p align="center">
  <img src="demo.gif" alt="OpenBoot Demo" width="800" />
</p>

```bash
curl -fsSL openboot.dev/install | bash
```

**70+ dev tools. Interactive TUI. Parallel installs. Zero config files.**

[![Release](https://img.shields.io/github/v/release/openbootdotdev/openboot)](https://github.com/openbootdotdev/openboot/releases)
[![License](https://img.shields.io/github/license/openbootdotdev/openboot)](LICENSE)
[![codecov](https://codecov.io/gh/openbootdotdev/openboot/branch/main/graph/badge.svg)](https://codecov.io/gh/openbootdotdev/openboot)

- **Interactive TUI** — search and select from 70+ curated dev tools across 13 categories
- **Snapshot** — capture your existing Mac's setup, share it, or restore it on a new machine
- **Parallel installs** — CLI tools install 4× in parallel; GUI apps install sequentially for password prompts
- **Dotfiles & shell** — clone your dotfiles repo, deploy via GNU Stow, install Oh-My-Zsh with aliases
- **Web dashboard** — create, share, and duplicate configs at [openboot.dev](https://openboot.dev)
- **No telemetry** — zero analytics, zero tracking, fully open source

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

### Team onboarding? Share a config:

Create a config at [openboot.dev/dashboard](https://openboot.dev/dashboard), then have your team run:

```bash
curl -fsSL openboot.dev/YOUR_USERNAME | bash
```

Import from an existing Brewfile, pick packages from the catalog, or duplicate an existing config.

## Presets

| Preset | Focus | Includes |
|--------|-------|----------|
| **minimal** | CLI essentials | ripgrep, fd, bat, fzf, lazygit, gh, Warp, Raycast |
| **developer** | Ready-to-code | + Node, Go, Docker, VS Code, Chrome, OrbStack |
| **full** | Complete setup | + Python, Rust, kubectl, Terraform, Ollama, Cursor, Figma |

## Why OpenBoot?

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

<details>
<summary>CLI Options</summary>

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

</details>

<details>
<summary>Environment Variables</summary>

| Variable | Description |
|----------|-------------|
| `OPENBOOT_GIT_NAME` | Git user name (required in silent mode) |
| `OPENBOOT_GIT_EMAIL` | Git user email (required in silent mode) |
| `OPENBOOT_PRESET` | Default preset |
| `OPENBOOT_USER` | Remote config username |

</details>

## Requirements

- macOS 12.0+ (Monterey or later)
- Internet connection

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
