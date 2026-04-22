# OpenBoot

> Setting up a new Mac shouldn't take two hours of your weekend.
>
> **[openboot.dev](https://openboot.dev)**

<p align="center">
  <a href="https://github.com/openbootdotdev/openboot/stargazers"><img src="https://img.shields.io/github/stars/openbootdotdev/openboot?style=social" alt="GitHub stars"></a>
  <a href="https://github.com/openbootdotdev/openboot/releases"><img src="https://img.shields.io/github/v/release/openbootdotdev/openboot" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/openbootdotdev/openboot" alt="License"></a>
  <a href="https://codecov.io/gh/openbootdotdev/openboot"><img src="https://codecov.io/gh/openbootdotdev/openboot/branch/main/graph/badge.svg" alt="codecov"></a>
  <img src="https://img.shields.io/badge/platform-macOS-lightgrey" alt="macOS">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8" alt="Go">
</p>

<p align="center">
  <img src="demo.gif" alt="OpenBoot Demo" width="800" />
</p>

You know the drill. New Mac, same two-hour ritual:

```bash
brew install git node go python rust docker kubectl terraform
brew install --cask visual-studio-code docker iterm2 chrome slack figma
npm install -g typescript eslint prettier
# dig through old laptop for .zshrc
# re-configure git identity
# tweak macOS settings one by one
# two hours later, still missing something
```

Here's the alternative:

```bash
curl -fsSL openboot.dev/install.sh | bash
```

Pick what you need in a terminal UI. Takes minutes. Or snapshot your current Mac and share it—your whole team gets the same setup with one command.

No tracking. No telemetry. Just works.

## Why OpenBoot?

Brewfiles are manual YAML editing. Nix has a brutal learning curve. Shell scripts break silently. Dotfile repos become unmaintainable after six months.

OpenBoot is the first tool that handles **everything** — packages, dotfiles, shell config, macOS preferences, git identity — in an interactive TUI you can actually navigate. No config files to learn. No YAML to write. Just pick what you need and go.

## Quick Start

```bash
curl -fsSL openboot.dev/install.sh | bash
```

Works on a fresh Mac — installs Homebrew, Xcode CLI tools, and everything else automatically.

<details>
<summary>Want to inspect the script first?</summary>

```bash
curl -fsSL openboot.dev/install.sh -o install.sh
cat install.sh
bash install.sh
```

</details>

<details>
<summary>Already have Homebrew?</summary>

```bash
brew install openbootdotdev/tap/openboot
openboot
```

</details>

## How It Compares

| | OpenBoot | Brewfile | chezmoi | nix-darwin |
|---|:---:|:---:|:---:|:---:|
| Interactive package picker | **TUI** | manual edit | — | — |
| Web dashboard | **[openboot.dev](https://openboot.dev)** | — | — | — |
| Shareable install URL | `openboot install myalias` | — | — | — |
| Snapshot & restore | full environment | — | dotfiles only | full (steep curve) |
| Learning curve | **Low** | Low | High | Very High |

## What It Does

- **Homebrew packages & apps** — Installs Docker, VS Code, Chrome, whatever you need
- **Dotfiles** — Clone your repo and symlink with GNU Stow, or skip it
- **Shell config** — Sets up Oh-My-Zsh with useful aliases
- **macOS settings** — Developer-friendly defaults for Dock, Finder, keyboard
- **Git setup** — Asks for your name and email, configures git
- **Smart about duplicates** — Detects what's already installed, skips it
- **Snapshot** — Capture everything and save/publish to share with another Mac

## Web Dashboard

[openboot.dev](https://openboot.dev) — if you'd rather click than type commands.

- **Visual builder** — Pick packages with checkboxes instead of editing YAML
- **Brewfile import** — Already have a Brewfile? Drop it in, it maps automatically
- **Shareable links** — Every config gets a URL: `openboot.dev/yourname/my-setup`
- **Team configs** — Share one link, everyone gets the same environment

## Presets

Three starting points. Pick one, adjust in the TUI. [Full list →](https://openboot.dev/docs/presets)

| Preset | What's In It |
|--------|--------------|
| **minimal** | CLI tools: ripgrep, fd, bat, fzf, lazygit, gh, Warp, Raycast, Rectangle |
| **developer** | Minimal + Node, Go, Docker, VS Code, Chrome, OrbStack, TablePlus |
| **full** | Developer + Python, Rust, kubectl, Terraform, Ollama, Cursor, Figma |

Most people start with **developer** and uncheck what they don't need.

## Snapshot

Already have a Mac set up the way you like? Save it.

```bash
openboot snapshot
```

This captures everything: Homebrew packages, macOS settings, shell config, git identity. Upload it to [openboot.dev](https://openboot.dev) for a shareable URL, or save it locally with `--local`.

When you restore a snapshot, you get everything back exactly as it was. [Docs →](https://openboot.dev/docs/snapshot)

## For Teams

New hire runs one command, gets the same environment as everyone else. [Guide →](https://openboot.dev/docs/teams)

```bash
curl -fsSL openboot.dev/yourteam/frontend | bash
```

Make your config on the [dashboard](https://openboot.dev/dashboard), put the one-liner in your onboarding docs. When your stack changes, update the config — the install command stays the same.

## Advanced Usage

<details>
<summary><strong>CI / Automation</strong></summary>

```bash
OPENBOOT_GIT_NAME="Your Name" \
OPENBOOT_GIT_EMAIL="you@example.com" \
curl -fsSL openboot.dev/install.sh | bash -s -- --preset developer --silent
```

</details>

<details>
<summary><strong>All Commands</strong></summary>

```bash
openboot                            # Resume last sync (or interactive if none)
openboot install                    # Same as above, explicit
openboot install alice/dev-setup    # Install from a cloud config
openboot install ./backup.json      # Install from a local file
openboot install -p developer       # Install a built-in preset
openboot install --dry-run          # Preview without installing

openboot snapshot                   # Capture (interactive menu in terminal)
openboot snapshot --local           # Save to ~/.openboot/snapshot.json
openboot snapshot --publish         # Upload to openboot.dev
openboot snapshot --import FILE     # Restore from a snapshot file

openboot login / logout             # openboot.dev auth
openboot version                    # Print version
```

Removed in v1.0: `pull`, `push`, `diff`, `clean`, `log`, `restore`, `init`, `setup-agent`, `doctor`, `update`. See [CHANGELOG.md](CHANGELOG.md) for migration.

</details>

<details>
<summary><strong>CLI Options</strong></summary>

```
-p, --preset NAME      Set preset (minimal, developer, full)
-u, --user NAME        Use alias or openboot.dev username/slug config
    --from FILE        Install from a local config or snapshot JSON file
-s, --silent           Non-interactive mode (requires env vars)
    --dry-run          Preview what would be installed
    --packages-only    Install packages only, skip system config
    --update           Update Homebrew before installing
    --shell MODE       Shell setup: install, skip
    --macos MODE       macOS prefs: configure, skip
    --dotfiles MODE    Dotfiles: clone, link, skip
    --post-install MODE  Post-install script: skip
    --allow-post-install Allow post-install scripts in silent mode
```

</details>

<details>
<summary><strong>Environment Variables</strong></summary>

| Variable | Description |
|----------|-------------|
| `OPENBOOT_GIT_NAME` | Git user name (required in silent mode) |
| `OPENBOOT_GIT_EMAIL` | Git user email (required in silent mode) |
| `OPENBOOT_PRESET` | Default preset |
| `OPENBOOT_USER` | Config alias or username/slug |

</details>

---

## FAQ

**Do I need anything installed first?**
macOS 12.0 or newer. Homebrew if you have it, but the installer will get it for you if not.

**What if I already have some of these tools?**
It checks what's installed and skips anything you already have.

**Is my data tracked?**
No. No telemetry, no analytics. Code is open source, check for yourself.

---

## Docs

📖 **[openboot.dev/docs](https://openboot.dev/docs)** — [Quick Start](https://openboot.dev/docs/quick-start) · [Presets](https://openboot.dev/docs/presets) · [Snapshot](https://openboot.dev/docs/snapshot) · [Custom Configs](https://openboot.dev/docs/custom-configs) · [Teams](https://openboot.dev/docs/teams)

## Contributing

Bug reports and feature requests: [open an issue](https://github.com/openbootdotdev/openboot/issues). Pull requests welcome.

<details>
<summary><strong>Development Setup</strong></summary>

```bash
git clone https://github.com/openbootdotdev/openboot.git
cd openboot
go build -o openboot ./cmd/openboot
./openboot install --dry-run
```

</details>

---

**[openboot.dev](https://openboot.dev)** · [Dashboard](https://openboot.dev/dashboard) · [Docs](https://openboot.dev/docs) · [Dotfiles template](https://github.com/openbootdotdev/dotfiles)

**License:** MIT
