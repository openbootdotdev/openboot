# OpenBoot

> One command. Your Mac is ready to code.
> **[openboot.dev](https://openboot.dev)**

<p align="center">
  <img src="demo.gif" alt="OpenBoot Demo" width="800" />
</p>

Setting up a new Mac still wastes hours. You manually install tools one by one, search for that dotfiles repo, configure macOS defaults, set up your shell... and somehow it's 3pm.

**OpenBoot** gives you a CLI and a [Web Dashboard](https://openboot.dev/dashboard) to handle all of it — whether you're setting up a fresh machine, capturing your current one, or standardizing your team's environment.

<p align="center">
  <a href="https://github.com/openbootdotdev/openboot/releases"><img src="https://img.shields.io/github/v/release/openbootdotdev/openboot" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/openbootdotdev/openboot" alt="License"></a>
  <a href="https://codecov.io/gh/openbootdotdev/openboot"><img src="https://codecov.io/gh/openbootdotdev/openboot/branch/main/graph/badge.svg" alt="codecov"></a>
</p>

## Two Paths, One Tool

### 🖥️ Fresh Mac? Install everything.

Run one command, pick your tools in the TUI, and you're done.

```bash
curl -fsSL openboot.dev/install.sh | bash
```

1. Choose a preset (`minimal`, `developer`, or `full`)
2. Customize your package selection in a searchable TUI
3. Sit back while everything installs

**Done.** Shell, dotfiles, macOS preferences — all configured.

### 📸 Already set up? Capture and share.

Snapshot your current Mac and turn it into a shareable config on [openboot.dev](https://openboot.dev).

```bash
curl -fsSL openboot.dev/install.sh | bash -s -- snapshot
```

Captures your Homebrew packages, macOS preferences, shell config, and git settings. Upload to openboot.dev to get a one-line install URL, or save locally with `--local`. [Learn more →](https://openboot.dev/docs/snapshot)

---

## Web Dashboard

The [openboot.dev dashboard](https://openboot.dev/dashboard) is where you manage and share your configs — no CLI knowledge required.

- ✨ **Visual Config Builder** — Create setups by clicking, not typing YAML
- 📦 **Import from Brewfile** — Drop your existing Brewfile and it maps everything automatically
- 🔗 **One-Line Install URLs** — Every config gets a shareable URL: `openboot.dev/yourname/my-setup`
- 🔍 **Package Search** — Browse and search thousands of Homebrew packages and casks
- 👥 **Team Configs** — Create standard environments your whole team installs with one command

Sign in with GitHub at [openboot.dev](https://openboot.dev) to get started.

## For Teams

Standardize your dev environment so every developer — new or existing — works with the same tools. [Full guide →](https://openboot.dev/docs/teams)

**How it works:**

1. **Create a team config** on the [dashboard](https://openboot.dev/dashboard) — or snapshot a reference machine and upload it
2. **Share one URL** in your README or onboarding docs:
   ```bash
   curl -fsSL openboot.dev/yourteam/frontend/install.sh | bash
   ```
3. **New developer joins** → runs the command → ready to code in minutes
4. **Stack changes?** Update the config in the dashboard — the URL stays the same

---

## Choose Your Preset

Start with a curated preset, then customize it in the TUI or on the [dashboard](https://openboot.dev/dashboard). [Compare presets →](https://openboot.dev/docs/presets)

| Preset | Best For | Includes |
|--------|----------|----------|
| **minimal** | CLI essentials | ripgrep, fd, bat, fzf, lazygit, gh, git-lfs, Warp, Raycast, Rectangle |
| **developer** | Full-stack devs | + Node, Go, Docker, lazydocker, pre-commit, VS Code, Chrome, OrbStack, TablePlus |
| **full** | Power users | + Python, Rust, kubectl, Terraform, cmake, Ollama, Cursor, Figma, ngrok |

Not sure? Pick **developer** and toggle what you don't need.

## What's Included

OpenBoot handles everything a traditional Mac setup requires:

- ✅ **Homebrew packages & GUI apps** — Docker, VS Code, Chrome, Warp, etc.
- ✅ **Dotfiles** — Clone your repo, deploy with GNU Stow, or skip
- ✅ **Shell setup** — Oh-My-Zsh with sensible aliases
- ✅ **macOS preferences** — Developer-friendly defaults (Dock, Finder, etc.)
- ✅ **Git identity** — Configure name/email during setup
- ✅ **Smart installs** — Skips already-installed tools, no wasted time

<details>
<summary><strong>🤔 Why not Brewfile / chezmoi / nix-darwin?</strong></summary>

| | OpenBoot | Brewfile | Strap | chezmoi | nix-darwin |
|---|:---:|:---:|:---:|:---:|:---:|
| Web dashboard | ✅ | — | — | — | — |
| Interactive TUI | ✅ | — | — | — | — |
| Team config sharing | ✅ | — | — | — | — |
| One-command setup | ✅ | — | ✅ | ✅ | — |
| Learning curve | Low | Low | Low | High | Very High |

OpenBoot combines the simplicity of Brewfile with the power of dotfiles managers, plus a web dashboard and team sharing built in.

</details>

---

## Advanced Usage

<details>
<summary><strong>🤖 CI / Automation</strong></summary>

```bash
OPENBOOT_GIT_NAME="Your Name" \
OPENBOOT_GIT_EMAIL="you@example.com" \
curl -fsSL openboot.dev/install.sh | bash -s -- --preset developer --silent
```

</details>

<details>
<summary><strong>⚙️ Commands</strong></summary>

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
<summary><strong>🎛️ CLI Options</strong></summary>

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
<summary><strong>🔑 Environment Variables</strong></summary>

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
Just macOS 12.0+ and an internet connection. OpenBoot installs Homebrew for you if needed.

**What if I already have some tools installed?**  
OpenBoot detects them and skips reinstalling. You only get what's new.

**Can I see what will be installed before running?**  
Yes. Add `--dry-run` to preview everything, or use the interactive TUI to toggle individual packages.

**Is my data tracked?**  
No. Zero telemetry, zero analytics. Fully open source (MIT license).

---

## Docs & Links

📖 Full documentation at **[openboot.dev/docs](https://openboot.dev/docs)** — [Quick Start](https://openboot.dev/docs/quick-start) · [Presets](https://openboot.dev/docs/presets) · [Snapshot](https://openboot.dev/docs/snapshot) · [Custom Configs](https://openboot.dev/docs/custom-configs) · [Teams](https://openboot.dev/docs/teams)

## Contributing

Found a bug or want to add a feature? [Open an issue](https://github.com/openbootdotdev/openboot/issues) or submit a PR.

<details>
<summary><strong>🛠️ Development Setup</strong></summary>

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
