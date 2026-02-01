#!/bin/bash

# OpenBoot Homebrew Management Module
# Provides functions for installing and managing packages via Homebrew
# Handles both CLI packages and GUI applications (casks)

# Source detection module for architecture and Homebrew prefix detection
source "$(dirname "${BASH_SOURCE[0]}")/core/detect.sh"

# ============================================================================
# brew_ensure_installed - Ensure Homebrew is installed
# ============================================================================
# Checks if Homebrew is already installed. This is redundant with boot.sh
# but provided for safety and modularity.
# Returns: 0 if installed, 1 if not
brew_ensure_installed() {
    if detect_existing_homebrew; then
        return 0
    else
        echo "Error: Homebrew is not installed. Please run boot.sh first." >&2
        return 1
    fi
}

# ============================================================================
# brew_is_installed - Check if a package is installed
# ============================================================================
# Checks if a given package (formula) is installed via Homebrew.
# Args:
#   $1: package name (e.g., "curl", "node")
# Returns: 0 if installed, 1 if not
brew_is_installed() {
    local package="$1"
    
    if [[ -z "$package" ]]; then
        echo "Error: package name required" >&2
        return 1
    fi
    
    brew list --formula 2>/dev/null | grep -q "^${package}$"
}

# ============================================================================
# brew_cask_is_installed - Check if a cask (GUI app) is installed
# ============================================================================
# Checks if a given cask (GUI application) is installed via Homebrew.
# Args:
#   $1: cask name (e.g., "visual-studio-code", "google-chrome")
# Returns: 0 if installed, 1 if not
brew_cask_is_installed() {
    local cask="$1"
    
    if [[ -z "$cask" ]]; then
        echo "Error: cask name required" >&2
        return 1
    fi
    
    brew list --cask 2>/dev/null | grep -q "^${cask}$"
}

# ============================================================================
# brew_install_packages - Install CLI packages via Homebrew
# ============================================================================
# Installs one or more CLI packages. Skips packages already installed (idempotent).
# Args:
#   $@: package names (e.g., "curl" "wget" "jq")
# Returns: 0 on success, 1 on failure
brew_install_packages() {
    local packages=("$@")
    local failed=0
    
    if [[ ${#packages[@]} -eq 0 ]]; then
        echo "Error: at least one package name required" >&2
        return 1
    fi
    
    # Ensure Homebrew is available
    if ! brew_ensure_installed; then
        return 1
    fi
    
    for pkg in "${packages[@]}"; do
        if brew_is_installed "$pkg"; then
            echo "✓ $pkg already installed, skipping"
        else
            echo "→ Installing $pkg..."
            if brew install "$pkg"; then
                echo "✓ $pkg installed successfully"
            else
                echo "✗ Failed to install $pkg" >&2
                failed=1
            fi
        fi
    done
    
    return $failed
}

# ============================================================================
# brew_install_casks - Install GUI applications via Homebrew Cask
# ============================================================================
# Installs one or more GUI applications (casks). Skips casks already installed (idempotent).
# Args:
#   $@: cask names (e.g., "visual-studio-code" "google-chrome")
# Returns: 0 on success, 1 on failure
brew_install_casks() {
    local casks=("$@")
    local failed=0
    
    if [[ ${#casks[@]} -eq 0 ]]; then
        echo "Error: at least one cask name required" >&2
        return 1
    fi
    
    # Ensure Homebrew is available
    if ! brew_ensure_installed; then
        return 1
    fi
    
    for cask in "${casks[@]}"; do
        if brew_cask_is_installed "$cask"; then
            echo "✓ $cask already installed, skipping"
        else
            echo "→ Installing $cask..."
            if brew install --cask "$cask"; then
                echo "✓ $cask installed successfully"
            else
                echo "✗ Failed to install $cask" >&2
                failed=1
            fi
        fi
    done
    
    return $failed
}

# ============================================================================
# brew_update - Update Homebrew and all installed packages
# ============================================================================
# Updates Homebrew itself and optionally upgrades all installed packages.
# Args:
#   $1: optional "upgrade" to also upgrade packages (default: update only)
# Returns: 0 on success, 1 on failure
brew_update() {
    local upgrade_packages="${1:-}"
    
    # Ensure Homebrew is available
    if ! brew_ensure_installed; then
        return 1
    fi
    
    echo "→ Updating Homebrew..."
    if ! brew update; then
        echo "✗ Failed to update Homebrew" >&2
        return 1
    fi
    echo "✓ Homebrew updated"
    
    if [[ "$upgrade_packages" == "upgrade" ]]; then
        echo "→ Upgrading installed packages..."
        if ! brew upgrade; then
            echo "✗ Failed to upgrade packages" >&2
            return 1
        fi
        echo "✓ Packages upgraded"
    fi
    
    return 0
}
