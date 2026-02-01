#!/bin/bash

# OpenBoot Package Definitions Module
# Defines preset package lists for different installation levels
# Presets: minimal, standard, full, devops, frontend, data, mobile, ai

# ============================================================================
# MINIMAL PRESET - Essential CLI tools and utilities
# Modern replacements for classic tools + essential utilities
# ============================================================================

MINIMAL_CLI=(
  # Essential networking & data
  "curl"
  "wget"
  "jq"
  "yq"
  
  # Modern CLI replacements (faster, better UX)
  "ripgrep"        # rg - faster grep
  "fd"             # faster find
  "bat"            # cat with syntax highlighting
  "eza"            # modern ls
  "fzf"            # fuzzy finder
  "zoxide"         # smarter cd
  
  # System monitoring & utilities
  "htop"
  "btop"           # prettier htop alternative
  "tree"
  "watch"
  "tldr"           # simplified man pages
  
  # Git & development essentials
  "gh"             # GitHub CLI
  "git-delta"      # better git diff
  "lazygit"        # terminal UI for git
  "stow"           # dotfiles manager
  
  # SSH & sync
  "ssh-copy-id"
  "rsync"
)

MINIMAL_CASK=(
  "warp"             # Modern terminal
  "raycast"          # Spotlight replacement (free tier)
  "maccy"            # Clipboard manager (free)
  "scroll-reverser"  # Mouse/trackpad scroll fix (free)
  "stats"            # System monitor in menubar (free)
)

# ============================================================================
# STANDARD PRESET - General development (includes minimal)
# For developers working across multiple languages/frameworks
# ============================================================================

STANDARD_CLI=(
  "${MINIMAL_CLI[@]}"
  
  # Languages & runtimes
  "node"
  "go"
  "rustup"         # Rust toolchain installer
  
  # Development tools
  "tmux"
  "neovim"
  "httpie"         # Better curl for APIs
  "jless"          # JSON viewer
  
  # Containers
  "docker"         # Docker CLI (works with OrbStack)
  "docker-compose"
  
  # Database clients
  "redis"
  "sqlite"
)

STANDARD_CASK=(
  "${MINIMAL_CASK[@]}"
  
  # Development
  "visual-studio-code"
  "orbstack"         # Docker/Linux VM (faster than Docker Desktop)
  
  # Browsers
  "google-chrome"
  "arc"              # Modern browser
  
  # API & debugging
  "postman"
  "proxyman"         # HTTP debugging proxy
  
  # Productivity
  "notion"
  "typora"           # Markdown editor
)

# ============================================================================
# FULL PRESET - Comprehensive suite (includes standard)
# Everything a power user might need
# ============================================================================

FULL_CLI=(
  "${STANDARD_CLI[@]}"
  
  # Cloud & Infrastructure
  "kubectl"
  "helm"
  "argocd"
  "awscli"
  "terraform"
  
  # Networking & debugging
  "wireguard-tools"
  "mtr"              # Better traceroute
  "nmap"
  "wrk"              # HTTP benchmarking
  "telnet"
  
  # Additional languages
  "python"
  "pipx"
  "uv"               # Fast Python package manager
  
  # Misc utilities
  "zola"             # Static site generator
  "ffmpeg"
  "imagemagick"
  "pandoc"
)

FULL_CASK=(
  "${STANDARD_CASK[@]}"
  
  # Communication
  "feishu"
  "wechat"
  "telegram"
  "discord"
  "slack"
  
  # Office & productivity
  "microsoft-office"
  "obsidian"
  
  # Browsers
  "microsoft-edge"
  "firefox"
  
  # Media & utilities
  "neteasemusic"
  "iina"             # Video player
  "keka"             # Archive utility
  
  # System utilities
  "betterdisplay"
  "balenaetcher"
  "clash-verge-rev"
  "aldente"          # Battery management
)

# ============================================================================
# DEVOPS PRESET - Kubernetes, cloud, infrastructure
# For SRE, DevOps, Platform engineers
# ============================================================================

DEVOPS_CLI=(
  "${MINIMAL_CLI[@]}"
  
  # Kubernetes core
  "kubectl"
  "helm"
  "kustomize"
  "k9s"              # Kubernetes TUI
  "kubectx"          # Context/namespace switching
  "stern"            # Multi-pod log tailing
  
  # GitOps & CD
  "argocd"
  "flux"
  
  # Cloud providers
  "awscli"
  "azure-cli"
  "google-cloud-sdk"
  
  # Infrastructure as Code
  "terraform"
  "pulumi"
  "ansible"
  
  # Secrets & security
  "vault"
  "sops"
  "age"
  
  # Service mesh & networking
  "istioctl"
  "cilium-cli"
  
  # Containers
  "docker"
  "docker-compose"
  "crane"            # Container image tool
  "dive"             # Docker image explorer
  
  # Monitoring & debugging
  "k6"               # Load testing
  "trivy"            # Security scanner
)

DEVOPS_CASK=(
  "${MINIMAL_CASK[@]}"
  "visual-studio-code"
  "orbstack"
  "lens"              # Kubernetes IDE
  "aws-vault"         # AWS credentials manager
)

# ============================================================================
# FRONTEND PRESET - Web development focused
# For frontend, full-stack web developers
# ============================================================================

FRONTEND_CLI=(
  "${MINIMAL_CLI[@]}"
  
  # JavaScript ecosystem
  "node"
  "pnpm"             # Fast, disk-efficient package manager
  "yarn"
  "bun"              # Fast JS runtime & toolkit
  "fnm"              # Fast Node version manager
  "deno"             # Alternative JS/TS runtime
  
  # Build & bundling
  "vite"
  
  # Development tools
  "tmux"
  "neovim"
  "httpie"
  
  # Testing
  "playwright"
)

FRONTEND_CASK=(
  "${MINIMAL_CASK[@]}"
  
  # Editors
  "visual-studio-code"
  "cursor"           # AI-powered editor
  
  # Browsers (for testing)
  "google-chrome"
  "firefox"
  "arc"
  "microsoft-edge"
  
  # Design & prototyping
  "figma"
  "sketch"
  
  # Development
  "postman"
  "proxyman"
  "imageoptim"       # Image compression
)

# ============================================================================
# DATA PRESET - Data science and analytics
# For data scientists, analysts, ML engineers
# ============================================================================

DATA_CLI=(
  "${MINIMAL_CLI[@]}"
  
  # Python ecosystem
  "python"
  "pipx"
  "uv"               # Fast Python installer
  "pyenv"            # Python version manager
  
  # Data tools
  "duckdb"           # Fast analytical database
  "postgresql"
  "sqlite"
  "mysql"
  
  # Data processing
  "csvkit"           # CSV utilities
  "miller"           # Like awk for CSV/JSON
  "xsv"              # Fast CSV toolkit
  
  # Other languages
  "r"
  "julia"
  
  # Visualization & notebooks
  "jupyterlab"
)

DATA_CASK=(
  "${MINIMAL_CASK[@]}"
  "visual-studio-code"
  "dbeaver-community"     # Universal database client
  "db-browser-for-sqlite"
  "tableplus"             # Modern database client
  "rstudio"               # R IDE
)

# ============================================================================
# MOBILE PRESET - iOS and Android development
# For mobile app developers
# ============================================================================

MOBILE_CLI=(
  "${MINIMAL_CLI[@]}"
  
  # iOS development
  "cocoapods"
  "fastlane"
  "swiftlint"
  "xcode-build-server"
  
  # React Native / Cross-platform
  "node"
  "yarn"
  "watchman"
  
  # Android
  "openjdk@17"
  "gradle"
  
  # Utilities
  "scrcpy"           # Android screen mirroring
)

MOBILE_CASK=(
  "${MINIMAL_CASK[@]}"
  "visual-studio-code"
  "android-studio"
  "sf-symbols"       # Apple SF Symbols browser
  "zeplin"           # Design handoff
  "figma"
  "proxyman"         # HTTP debugging
)

# ============================================================================
# AI PRESET - AI/ML development and experimentation
# For AI researchers, ML engineers, AI app developers
# ============================================================================

AI_CLI=(
  "${MINIMAL_CLI[@]}"
  
  # Python ecosystem (ML foundation)
  "python"
  "pipx"
  "uv"
  "pyenv"
  
  # Local LLM tools
  "ollama"           # Run LLMs locally
  "llm"              # CLI for LLMs
  
  # Data & ML utilities
  "duckdb"
  "sqlite"
  
  # Development tools
  "node"             # For AI apps (LangChain.js, etc.)
  "tmux"
  "neovim"
  
  # GPU & performance (if applicable)
  "htop"
  "btop"
)

AI_CASK=(
  "${MINIMAL_CASK[@]}"
  "visual-studio-code"
  "cursor"              # AI-native editor
  "lm-studio"           # Local LLM GUI
  "chatgpt"             # OpenAI desktop app
  "jan"                 # Local AI assistant
)

# ============================================================================
# Helper Functions
# ============================================================================

get_packages() {
    local preset="$1"
    local type="$2"
    
    if [[ ! "$preset" =~ ^(minimal|standard|full|devops|frontend|data|mobile|ai)$ ]]; then
        echo "Error: Invalid preset '$preset'" >&2
        return 1
    fi
    
    if [[ ! "$type" =~ ^(cli|cask)$ ]]; then
        echo "Error: Invalid type '$type'. Must be: cli or cask" >&2
        return 1
    fi
    
    local array_name
    array_name=$(echo "${preset}_${type}" | tr '[:lower:]' '[:upper:]')
    
    eval "echo \"\${${array_name}[@]}\""
}

get_all_presets() {
    echo "minimal standard full devops frontend data mobile ai"
}

# Get preset description for UI
get_preset_description() {
    local preset="$1"
    case "$preset" in
        minimal)  echo "Essential CLI tools + modern replacements (fastest)" ;;
        standard) echo "General development (Node, Go, Rust, Docker)" ;;
        full)     echo "Everything including office & communication apps" ;;
        devops)   echo "Kubernetes, Terraform, cloud CLIs, GitOps" ;;
        frontend) echo "Web development (Node, pnpm, Bun, browsers, Figma)" ;;
        data)     echo "Data science (Python, R, Julia, DuckDB, databases)" ;;
        mobile)   echo "iOS & Android development (Xcode tools, Android Studio)" ;;
        ai)       echo "AI/ML development (Ollama, LLM tools, Cursor)" ;;
        *)        echo "Unknown preset" ;;
    esac
}
