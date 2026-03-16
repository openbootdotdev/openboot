#!/bin/bash
set -euo pipefail

VERSION="${OPENBOOT_VERSION:-latest}"
REPO="openbootdotdev/openboot"
BINARY_NAME="openboot"
TAP_NAME="openbootdotdev/tap"
DRY_RUN="${OPENBOOT_DRY_RUN:-false}"

install_xcode_clt() {
    if xcode-select -p &>/dev/null; then
        return 0
    fi

    echo ""
    echo "⚠️  ACTION REQUIRED"
    echo ""
    echo "Xcode Command Line Tools need to be installed."
    echo "A dialog will appear - please click 'Install' and enter your password."
    echo ""
    read -p "Press Enter to launch installer..." -r
    echo ""

    xcode-select --install 2>/dev/null || true

    echo "Waiting for installation to complete..."
    until xcode-select -p &>/dev/null; do
        sleep 5
    done
    echo "✓ Xcode Command Line Tools installed!"
    echo ""
}

install_homebrew() {
    if command -v brew &>/dev/null; then
        return 0
    fi

    echo "Installing Homebrew..."
    echo ""

    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

    local arch
    arch=$(uname -m)
    case "$arch" in
        arm64)
            if [[ -x "/opt/homebrew/bin/brew" ]]; then
                export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
                export HOMEBREW_PREFIX="/opt/homebrew"
                export HOMEBREW_CELLAR="/opt/homebrew/Cellar"
                export HOMEBREW_REPOSITORY="/opt/homebrew"
            fi
            ;;
        x86_64)
            if [[ -x "/usr/local/bin/brew" ]]; then
                export PATH="/usr/local/bin:/usr/local/sbin:$PATH"
                export HOMEBREW_PREFIX="/usr/local"
                export HOMEBREW_CELLAR="/usr/local/Cellar"
                export HOMEBREW_REPOSITORY="/usr/local/Homebrew"
            fi
            ;;
        *)
            echo "Error: Unsupported architecture: $arch" >&2
            exit 1
            ;;
    esac

    echo ""
    echo "Homebrew installed!"
    echo ""
}

detect_os() {
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        darwin) echo "darwin" ;;
        *)      echo "Error: OpenBoot only supports macOS" >&2; exit 1 ;;
    esac
}

detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64)  echo "amd64" ;;
        arm64)   echo "arm64" ;;
        aarch64) echo "arm64" ;;
        *)       echo "unsupported: $arch" >&2; exit 1 ;;
    esac
}

main() {
    # When run via "curl | bash", stdin is the script content, not the terminal.
    # Reopen stdin from /dev/tty so interactive prompts (read, sudo, Homebrew) work.
    if [[ ! -t 0 ]] && [[ -e /dev/tty ]]; then
        exec < /dev/tty || true
    fi

    local snapshot_mode=false
    if [[ "${1:-}" == "snapshot" ]]; then
        snapshot_mode=true
    fi

    echo ""
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "🔍 DRY RUN MODE - No changes will be made"
        echo "========================================"
    elif [[ "$snapshot_mode" == true ]]; then
        echo "OpenBoot Snapshot"
        echo "================="
    else
        echo "OpenBoot Installer"
        echo "=================="
    fi
    echo ""

    local os arch
    os=$(detect_os)
    arch=$(detect_arch)

    echo "Detected: ${os}/${arch}"
    echo ""

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "Would perform:"
        echo "  1. Check/Install Xcode Command Line Tools"
        echo "  2. Check/Install Homebrew"
        echo "  3. Run: brew install ${TAP_NAME}/${BINARY_NAME}"
        echo "  4. Launch: ${BINARY_NAME}"
        echo ""
        echo "To actually install, run without OPENBOOT_DRY_RUN:"
        echo "  curl -fsSL https://openboot.dev/install.sh | bash"
        echo ""
        exit 0
    fi

    if [[ "$os" == "darwin" && "$snapshot_mode" == false ]]; then
        install_xcode_clt
        install_homebrew
    fi

    if brew list openboot &>/dev/null 2>&1; then
        echo "OpenBoot is already installed via Homebrew."
        echo ""
        read -p "Reinstall? (y/N) " -n 1 -r
        echo
        
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo "Reinstalling OpenBoot..."
            brew reinstall ${TAP_NAME}/openboot
            echo ""
            echo "✓ OpenBoot reinstalled!"
        else
            echo "Using existing installation."
        fi
    else
        echo "Installing OpenBoot via Homebrew..."
        echo ""
        
        brew install ${TAP_NAME}/openboot
        
        echo ""
        echo "✓ OpenBoot installed!"
    fi

    echo ""
    echo "Starting OpenBoot setup..."
    echo ""
    
    exec openboot "$@"
}

main "$@"
