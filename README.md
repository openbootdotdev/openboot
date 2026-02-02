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
2. Preset selection
3. Package customization
4. Dotfiles setup
5. Oh-My-Zsh installation (optional)

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

| Preset | Focus | Key CLI Tools | Key GUI Apps |
|--------|-------|---------------|--------------|
| **minimal** | Essential tools | ripgrep, fd, bat, fzf, lazygit, gh | Warp, Raycast, Maccy |
| **standard** | General dev | + Node, Go, Rust, Docker, tmux, neovim | + VS Code, OrbStack, Chrome |
| **full** | Everything | + kubectl, terraform, Python, ffmpeg | + Office, Slack, Obsidian |
| **devops** | Infrastructure | kubectl, helm, k9s, terraform, argocd | VS Code, OrbStack, Lens |
| **frontend** | Web development | Node, pnpm, Bun, Deno, Playwright | VS Code, Figma, Arc, Cursor |
| **data** | Data science | Python, R, Julia, DuckDB, PostgreSQL | VS Code, DBeaver, RStudio |
| **mobile** | iOS & Android | CocoaPods, Fastlane, Gradle, scrcpy | Android Studio, Figma |
| **ai** | AI/ML development | Ollama, llm, Python, uv, DuckDB | Cursor, LM Studio, ChatGPT |

## Options

- `--help`: Show help message
- `--preset NAME`: Set preset (minimal, standard, full, devops, frontend, data, mobile, ai)
- `--silent`: Non-interactive mode (requires env vars)
- `--shell MODE`: Install shell framework (install, skip)
- `--dotfiles MODE`: Set dotfiles mode (clone, link, skip)
- `--dry-run`: Show what would be installed without installing
- `--resume`: Resume from last incomplete step
- `--rollback`: Restore backed up files to their original state
- `--update`: Update Homebrew and upgrade all packages

## Environment Variables

- `OPENBOOT_GIT_NAME`: Git user name (required in silent mode)
- `OPENBOOT_GIT_EMAIL`: Git user email (required in silent mode)
- `OPENBOOT_PRESET`: Default preset if `--preset` not specified
- `OPENBOOT_DOTFILES`: Dotfiles repository URL

## Testing

### Dry Run (Safe Preview)
See what would be installed without making any changes:

```bash
# Test from remote
curl -fsSL openboot.dev/install | bash -s -- --dry-run

# Test locally
./boot.sh --dry-run
./install.sh --preset minimal --dry-run
```

### Local Testing
Clone and run locally to test changes:

```bash
git clone https://github.com/openbootdotdev/openboot.git
cd openboot

# Test boot.sh (prerequisites only)
./boot.sh --dry-run

# Test full install with specific preset
./install.sh --preset minimal --dry-run
./install.sh --preset devops --dry-run

# Test silent mode
OPENBOOT_GIT_NAME="Test" OPENBOOT_GIT_EMAIL="test@test.com" \
  ./install.sh --preset minimal --silent --dry-run
```

### VM Testing (Full Installation)
For testing actual installations without affecting your system:

**Option 1: macOS VM (UTM/Parallels)**
```bash
# Create a clean macOS VM
# Run the full installation
curl -fsSL openboot.dev/install | bash
```

**Option 2: Fresh User Account**
```bash
# Create a new macOS user for testing
# Log into that user and run the installer
```

### Validation Checklist

After installation, verify:

```bash
# Check Homebrew
brew doctor

# Check installed packages
brew list
brew list --cask

# Check CLI tools
which rg fd bat fzf gh

# Check shell
echo $SHELL
```

## Rollback

If something goes wrong, OpenBoot automatically backs up your original files before making changes. To restore:

```bash
./install.sh --rollback
```

Backups are stored in `~/.openboot/backup/` with timestamps.

## Troubleshooting

### Installation fails with "interactive terminal" error
If you are running in a non-interactive environment (like a script or CI), ensure you use the `--silent` flag and provide the required environment variables.

### Homebrew installation fails
OpenBoot requires Homebrew. If Homebrew installation fails, ensure you have an active internet connection and admin privileges. You can try installing Homebrew manually first:
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

### Package installation fails
Some casks may fail if the app is already installed or requires a specific macOS version. OpenBoot continues with remaining packages. Check `~/.openboot/logs/` for details.

## License

MIT
