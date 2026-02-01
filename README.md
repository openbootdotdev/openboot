# OpenBoot

> One-line macOS development environment setup

## Quick Start

```bash
curl -fsSL openboot.dev/install | bash
```

## Prerequisites

- macOS 12.0 (Monterey) or later
- Internet connection
- Admin privileges (for Homebrew)

## Usage

### Interactive Mode
Simply run the quick start command. OpenBoot will guide you through:
1. Git identity configuration
2. Preset selection (Minimal, Standard, Full)
3. Package customization
4. Dotfiles setup

```bash
curl -fsSL openboot.dev/install | bash
```

### Non-Interactive Mode (CI/Automation)
Use environment variables and the `--silent` flag to run OpenBoot without user input.

```bash
OPENBOOT_GIT_NAME="Your Name" \
OPENBOOT_GIT_EMAIL="you@example.com" \
curl -fsSL openboot.dev/install | bash -s -- --preset minimal --silent
```

## Presets

| Preset | CLI Tools | GUI Apps | Time |
|--------|-----------|----------|------|
| **Minimal** | curl, wget, jq, tree, htop, watch, gh, stow, ssh-copy-id, rsync | Warp, Maccy, Scroll Reverser | ~5 min |
| **Standard** | + node, tmux | + VS Code, Chrome, OrbStack, Postman, Typora | ~15 min |
| **Full** | + kubectl, helm, argocd, awscli, wireguard-tools, wrk, telnet, zola | + Feishu, WeChat, Telegram, Notion, MS Office, MS Edge, Netease Music, BetterDisplay, BalenaEtcher, Clash Verge | ~30 min |

## Options

- `--help`: Show help message
- `--preset NAME`: Set preset (`minimal`, `standard`, `full`)
- `--silent`: Non-interactive mode (requires env vars)
- `--dotfiles MODE`: Set dotfiles mode (`clone`, `link`, `skip`)
- `--dry-run`: Show what would be installed without installing
- `--resume`: Resume from last incomplete step

## Environment Variables

- `OPENBOOT_GIT_NAME`: Git user name (required in silent mode)
- `OPENBOOT_GIT_EMAIL`: Git user email (required in silent mode)
- `OPENBOOT_PRESET`: Default preset if `--preset` not specified
- `OPENBOOT_DOTFILES`: Dotfiles repository URL

## Troubleshooting

### Installation fails with "interactive terminal" error
If you are running in a non-interactive environment (like a script or CI), ensure you use the `--silent` flag and provide the required environment variables.

### Homebrew installation fails
OpenBoot requires Homebrew. If Homebrew installation fails, ensure you have an active internet connection and admin privileges. You can try installing Homebrew manually first:
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

## License

MIT
