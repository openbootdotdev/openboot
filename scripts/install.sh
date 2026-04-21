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

    # brew may already be installed but missing from PATH (fresh shell without
    # ~/.zprofile sourced). Detect at known prefixes before reinstalling — a
    # second install triggers `chown -R /opt/homebrew` which fails on SIP-protected
    # signed .app bundles in Caskroom (e.g. wetype).
    local brew_bin=""
    if [[ -x "/opt/homebrew/bin/brew" ]]; then
        brew_bin="/opt/homebrew/bin/brew"
    elif [[ -x "/usr/local/bin/brew" ]]; then
        brew_bin="/usr/local/bin/brew"
    fi

    if [[ -n "$brew_bin" ]]; then
        eval "$("$brew_bin" shellenv)"
        return 0
    fi

    echo "Installing Homebrew..."
    echo ""

    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

    local arch
    arch=$(uname -m)
    case "$arch" in
        arm64)
            brew_bin="/opt/homebrew/bin/brew"
            ;;
        x86_64)
            brew_bin="/usr/local/bin/brew"
            ;;
        *)
            echo "Error: Unsupported architecture: $arch" >&2
            exit 1
            ;;
    esac

    if [[ -x "$brew_bin" ]]; then
        eval "$("$brew_bin" shellenv)"
        # Persist PATH for future shells. Homebrew's installer only prints
        # instructions; without this, the next `curl | bash` sees no brew on PATH
        # and tries to reinstall.
        local zprofile="${HOME}/.zprofile"
        local shellenv_line="eval \"\$(${brew_bin} shellenv)\""
        if [[ ! -f "$zprofile" ]] || ! grep -qF "$shellenv_line" "$zprofile" 2>/dev/null; then
            printf '\n%s\n' "$shellenv_line" >> "$zprofile"
            echo "Added Homebrew to $zprofile"
        fi
    fi

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
