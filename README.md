# OpenBoot

> One command. Your Mac is ready to code.
> **[openboot.dev](https://openboot.dev)**

<p align="center">
  <a href="https://github.com/openbootdotdev/openboot/releases"><img src="https://img.shields.io/github/v/release/openbootdotdev/openboot" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/openbootdotdev/openboot" alt="License"></a>
  <a href="https://codecov.io/gh/openbootdotdev/openboot"><img src="https://codecov.io/gh/openbootdotdev/openboot/branch/main/graph/badge.svg" alt="codecov"></a>
  <img src="https://img.shields.io/badge/platform-macOS-lightgrey" alt="macOS">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8" alt="Go">
</p>

Setting up a new Mac takes hours. Homebrew packages, dotfiles, shell config, macOS preferences — you've done it before, and it never gets faster.

OpenBoot handles all of it. Pick your tools in a TUI, or snapshot your current setup and share it as a one-line install URL.

Zero telemetry. Fully open source. MIT licensed.

<p align="center">
  <img src="demo.gif" alt="OpenBoot Demo" width="800" />
</p>

## Quick Start

```bash
brew install openbootdotdev/tap/openboot
openboot
```

<details>
<summary>Alternative: one-line installer</summary>

```bash
curl -fsSL openboot.dev/install.sh | bash
```

</details>

## What It Does

- **Homebrew packages & GUI apps** — Docker, VS Code, Chrome, Warp, and more
- **Dotfiles** — Clone your repo, deploy with GNU Stow, or skip
- **Shell setup** — Oh-My-Zsh with sensible aliases
- **macOS preferences** — Developer-friendly defaults (Dock, Finder, keyboard)
- **Git identity** — Configure name and email during setup
- **Smart installs** — Detects already-installed tools, skips them

## Web Dashboard

[openboot.dev](https://openboot.dev) — manage and share configs visually, no CLI required.

- **Visual Config Builder** — Create setups by clicking, not typing YAML
- **Import from Brewfile** — Drop your Brewfile, everything maps automatically
- **Shareable URLs** — Every config gets a link: `openboot.dev/yourname/my-setup`
- **Team Configs** — One command to standardize your whole team's environment

## Presets

Start with a curated preset, customize in the TUI. [Compare presets →](https://openboot.dev/docs/presets)

| Preset | Best For | Includes |
|--------|----------|----------|
| **minimal** | CLI essentials | ripgrep, fd, bat, fzf, lazygit, gh, Warp, Raycast, Rectangle |
| **developer** | Full-stack devs | + Node, Go, Docker, VS Code, Chrome, OrbStack, TablePlus |
| **full** | Power users | + Python, Rust, kubectl, Terraform, Ollama, Cursor, Figma |

Not sure? Pick **developer** and toggle what you don't need.

## Snapshot

Already set up? Capture your environment and share it.

```bash
openboot snapshot
```

Captures Homebrew packages, macOS preferences, shell config, and git settings. Upload to [openboot.dev](https://openboot.dev) for a shareable install URL, or save locally with `--local`. [Learn more →](https://openboot.dev/docs/snapshot)

## For Teams

New developer joins → runs one command → ready to code. [Full guide →](https://openboot.dev/docs/teams)

```bash
brew install openbootdotdev/tap/openboot
openboot --user yourteam/frontend
```

Create configs on the [dashboard](https://openboot.dev/dashboard), share the install command in your onboarding docs. Stack changes? Update the config — the command stays the same.

## How It Compares

| | OpenBoot | Brewfile | Strap | chezmoi | nix-darwin |
|---|:---:|:---:|:---:|:---:|:---:|
| Web dashboard | ✅ | — | — | — | — |
| Interactive TUI | ✅ | — | — | — | — |
| Team config sharing | ✅ | — | — | — | — |
| One-command setup | ✅ | — | ✅ | ✅ | — |
| Learning curve | Low | Low | Low | High | Very High |

---

## Advanced Usage

<details>
<summary><strong>CI / Automation</strong></summary>

```bash
brew install openbootdotdev/tap/openboot
OPENBOOT_GIT_NAME="Your Name" \
OPENBOOT_GIT_EMAIL="you@example.com" \
openboot --preset developer --silent
```

Or with the one-line installer:

```bash
OPENBOOT_GIT_NAME="Your Name" \
OPENBOOT_GIT_EMAIL="you@example.com" \
curl -fsSL openboot.dev/install.sh | bash -s -- --preset developer --silent
```

</details>

<details>
<summary><strong>All Commands</strong></summary>

```bash
openboot                 # Interactive setup
openboot snapshot        # Capture your current setup
openboot doctor          # Check system health
openboot update          # Update Homebrew and packages
openboot update --dry-run  # Preview updates
openboot version         # Print version
```

</details>

<details>
<summary><strong>CLI Options</strong></summary>

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
<summary><strong>Environment Variables</strong></summary>

| Variable | Description |
|----------|-------------|
| `OPENBOOT_GIT_NAME` | Git user name (required in silent mode) |
| `OPENBOOT_GIT_EMAIL` | Git user email (required in silent mode) |
| `OPENBOOT_PRESET` | Default preset |
| `OPENBOOT_USER` | Remote config username |

</details>

---

## FAQ

**Do I need anything installed first?**
Just macOS 12.0+ and Homebrew. The one-line installer handles Homebrew for you if needed.

**What if I already have some tools?**
OpenBoot detects them and skips reinstalling.

**Is my data tracked?**
No. Zero telemetry, zero analytics. Fully open source.

---

## Docs

📖 **[openboot.dev/docs](https://openboot.dev/docs)** — [Quick Start](https://openboot.dev/docs/quick-start) · [Presets](https://openboot.dev/docs/presets) · [Snapshot](https://openboot.dev/docs/snapshot) · [Custom Configs](https://openboot.dev/docs/custom-configs) · [Teams](https://openboot.dev/docs/teams)

## Contributing

Found a bug or want a feature? [Open an issue](https://github.com/openbootdotdev/openboot/issues) or submit a PR.

<details>
<summary><strong>Development Setup</strong></summary>

```bash
git clone https://github.com/openbootdotdev/openboot.git
cd openboot
go build -o openboot ./cmd/openboot
./openboot --dry-run
```

</details>

---

**[openboot.dev](https://openboot.dev)** · [Dashboard](https://openboot.dev/dashboard) · [Docs](https://openboot.dev/docs) · [Dotfiles template](https://github.com/openbootdotdev/dotfiles)

**License:** MIT
