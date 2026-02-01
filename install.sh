#!/bin/bash

# OpenBoot Main Installation Orchestrator
# Runs after boot.sh - implements 5-step user flow:
# 1. Git configuration (name/email)
# 2. Preset selection (Minimal/Standard/Full)
# 3. Package customization (optional)
# 4. Dotfiles selection
# 5. Confirmation and installation

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION="0.1.0"

# Source all lib modules
source "$SCRIPT_DIR/lib/core/detect.sh"
source "$SCRIPT_DIR/lib/core/error.sh"
source "$SCRIPT_DIR/lib/core/progress.sh"
source "$SCRIPT_DIR/lib/ui/gum.sh"
source "$SCRIPT_DIR/lib/packages.sh"
source "$SCRIPT_DIR/lib/brew.sh"

# Flags
PRESET=""
SILENT=false
DOTFILES_MODE=""
DRY_RUN=false
RESUME=false

# Selected packages (for customization)
SELECTED_CLI=()
SELECTED_CASK=()

# Show help message
show_help() {
    cat << EOF
OpenBoot Installer v$VERSION

Usage: ./install.sh [OPTIONS]

OPTIONS:
    --help          Show this help message
    --preset NAME   Set preset (minimal, standard, full)
    --silent        Non-interactive mode (requires env vars)
    --dotfiles MODE Set dotfiles mode (clone, link, skip)
    --dry-run       Show what would be installed without installing
    --resume        Resume from last incomplete step

ENVIRONMENT VARIABLES (for --silent mode):
    OPENBOOT_GIT_NAME   Git user name (required in silent mode)
    OPENBOOT_GIT_EMAIL  Git user email (required in silent mode)
    OPENBOOT_PRESET     Default preset if --preset not specified
    OPENBOOT_DOTFILES   Dotfiles repository URL

EXAMPLES:
    ./install.sh                          # Interactive installation
    ./install.sh --preset standard        # Skip preset selection
    ./install.sh --dry-run                # Preview installation
    ./install.sh --resume                 # Resume interrupted install
    
    # Silent mode
    OPENBOOT_GIT_NAME="John" OPENBOOT_GIT_EMAIL="john@example.com" \\
        ./install.sh --silent --preset minimal

EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help)
                show_help
                exit 0
                ;;
            --preset)
                if [[ -z "${2:-}" ]]; then
                    echo "Error: --preset requires a value" >&2
                    exit 1
                fi
                PRESET="$2"
                shift 2
                ;;
            --silent)
                SILENT=true
                shift
                ;;
            --dotfiles)
                if [[ -z "${2:-}" ]]; then
                    echo "Error: --dotfiles requires a value" >&2
                    exit 1
                fi
                DOTFILES_MODE="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --resume)
                RESUME=true
                shift
                ;;
            *)
                echo "Unknown option: $1" >&2
                echo "Use --help for usage information" >&2
                exit 1
                ;;
        esac
    done
}

# Validate silent mode requirements
validate_silent_mode() {
    if $SILENT; then
        if [[ -z "${OPENBOOT_GIT_NAME:-}" ]]; then
            echo "Error: OPENBOOT_GIT_NAME required in silent mode" >&2
            exit 1
        fi
        if [[ -z "${OPENBOOT_GIT_EMAIL:-}" ]]; then
            echo "Error: OPENBOOT_GIT_EMAIL required in silent mode" >&2
            exit 1
        fi
        # Use env var preset if not specified via flag
        if [[ -z "$PRESET" && -n "${OPENBOOT_PRESET:-}" ]]; then
            PRESET="$OPENBOOT_PRESET"
        fi
        # Default to minimal in silent mode if no preset
        if [[ -z "$PRESET" ]]; then
            PRESET="minimal"
        fi
    fi
}

# Step 1: Git configuration
step_git_config() {
    if $RESUME && progress_is_complete "git_config"; then
        echo "Git configuration already complete, skipping..."
        return 0
    fi
    
    progress_start "git_config"
    ui_header "Step 1: Git Configuration"
    echo ""
    
    local git_name
    local git_email
    
    git_name=$(ui_input "Your name:" "John Doe" "" "OPENBOOT_GIT_NAME")
    git_email=$(ui_input "Your email:" "john@example.com" "" "OPENBOOT_GIT_EMAIL")
    
    if [[ -z "$git_name" || -z "$git_email" ]]; then
        echo "Error: Git name and email are required" >&2
        return 1
    fi
    
    if ! $DRY_RUN; then
        git config --global user.name "$git_name"
        git config --global user.email "$git_email"
        echo "Git configured: $git_name <$git_email>"
    else
        echo "[DRY-RUN] Would configure git: $git_name <$git_email>"
    fi
    
    progress_complete "git_config"
    echo ""
}

# Step 2: Preset selection
step_preset_selection() {
    if $RESUME && progress_is_complete "preset_selection"; then
        echo "Preset selection already complete, skipping..."
        # Load saved preset from progress (simplified: just use minimal as fallback)
        if [[ -z "$PRESET" ]]; then
            PRESET="minimal"
        fi
        return 0
    fi
    
    progress_start "preset_selection"
    ui_header "Step 2: Preset Selection"
    echo ""
    
    if [[ -z "$PRESET" ]]; then
        echo "Choose a preset based on your needs:"
        echo "  Minimal  - Essential CLI tools + free apps (fastest)"
        echo "  Standard - Development tools (Node, Docker, VS Code)"
        echo "  Full     - Everything including office & communication apps"
        echo ""
        
        PRESET=$(ui_choose "Select preset:" "minimal" "standard" "full")
    fi
    
    # Validate preset
    if [[ ! "$PRESET" =~ ^(minimal|standard|full)$ ]]; then
        echo "Error: Invalid preset '$PRESET'. Must be: minimal, standard, or full" >&2
        return 1
    fi
    
    echo "Selected preset: $PRESET"
    
    # Load default packages for selected preset
    IFS=' ' read -r -a SELECTED_CLI <<< "$(get_packages "$PRESET" "cli")"
    IFS=' ' read -r -a SELECTED_CASK <<< "$(get_packages "$PRESET" "cask")"
    
    progress_complete "preset_selection"
    echo ""
}

# Step 3: Package customization
step_package_customization() {
    if $RESUME && progress_is_complete "package_customization"; then
        echo "Package customization already complete, skipping..."
        return 0
    fi
    
    progress_start "package_customization"
    ui_header "Step 3: Package Customization"
    echo ""
    
    echo "CLI packages to install (${#SELECTED_CLI[@]} packages):"
    printf "  %s\n" "${SELECTED_CLI[@]}"
    echo ""
    
    echo "GUI applications to install (${#SELECTED_CASK[@]} apps):"
    printf "  %s\n" "${SELECTED_CASK[@]}"
    echo ""
    
    # In interactive mode, ask if user wants to customize
    if ! $SILENT; then
        if ui_confirm "Do you want to customize packages?" false; then
            echo ""
            echo "Package customization is available in interactive mode."
            echo "For advanced customization, edit lib/packages.sh directly."
            echo ""
            # TODO: Implement multi-select customization with gum filter
            # For now, just use the preset packages
        fi
    fi
    
    progress_complete "package_customization"
    echo ""
}

# Step 4: Dotfiles selection
step_dotfiles_selection() {
    if $RESUME && progress_is_complete "dotfiles_selection"; then
        echo "Dotfiles selection already complete, skipping..."
        return 0
    fi
    
    progress_start "dotfiles_selection"
    ui_header "Step 4: Dotfiles Selection"
    echo ""
    
    if [[ -z "$DOTFILES_MODE" ]]; then
        echo "Dotfiles can configure your shell, editor, and tools."
        echo ""
        
        DOTFILES_MODE=$(ui_choose "Dotfiles setup:" "skip" "clone" "link")
    fi
    
    case "$DOTFILES_MODE" in
        skip)
            echo "Skipping dotfiles setup"
            ;;
        clone)
            echo "Dotfiles will be cloned (not yet implemented)"
            # TODO: Implement with lib/dotfiles.sh when ready
            ;;
        link)
            echo "Dotfiles will be linked (not yet implemented)"
            # TODO: Implement with lib/dotfiles.sh when ready
            ;;
        *)
            echo "Unknown dotfiles mode: $DOTFILES_MODE, skipping"
            DOTFILES_MODE="skip"
            ;;
    esac
    
    progress_complete "dotfiles_selection"
    echo ""
}

# Step 5: Confirmation and installation
step_confirmation_and_install() {
    if $RESUME && progress_is_complete "installation"; then
        echo "Installation already complete!"
        return 0
    fi
    
    progress_start "installation"
    ui_header "Step 5: Confirmation"
    echo ""
    
    echo "Installation Summary:"
    echo "  Preset:     $PRESET"
    echo "  CLI tools:  ${#SELECTED_CLI[@]} packages"
    echo "  GUI apps:   ${#SELECTED_CASK[@]} applications"
    echo "  Dotfiles:   $DOTFILES_MODE"
    echo ""
    
    if $DRY_RUN; then
        echo "[DRY-RUN] Would install the following packages:"
        echo ""
        echo "CLI packages:"
        printf "  brew install %s\n" "${SELECTED_CLI[@]}"
        echo ""
        echo "GUI applications:"
        printf "  brew install --cask %s\n" "${SELECTED_CASK[@]}"
        echo ""
        progress_complete "installation"
        return 0
    fi
    
    # Confirmation in interactive mode
    if ! $SILENT; then
        if ! ui_confirm "Proceed with installation?" true; then
            echo "Installation cancelled by user"
            exit 0
        fi
    fi
    
    echo ""
    ui_header "Installing packages..."
    echo ""
    
    # Install CLI packages
    if [[ ${#SELECTED_CLI[@]} -gt 0 ]]; then
        echo "Installing CLI packages..."
        brew_install_packages "${SELECTED_CLI[@]}"
        echo ""
    fi
    
    # Install cask applications
    if [[ ${#SELECTED_CASK[@]} -gt 0 ]]; then
        echo "Installing GUI applications..."
        brew_install_casks "${SELECTED_CASK[@]}"
        echo ""
    fi
    
    progress_complete "installation"
}

# Show completion message
show_completion() {
    echo ""
    ui_header "Installation Complete!"
    echo ""
    echo "OpenBoot has successfully configured your Mac."
    echo ""
    echo "What was installed:"
    echo "  - Git configured with your identity"
    echo "  - ${#SELECTED_CLI[@]} CLI packages"
    echo "  - ${#SELECTED_CASK[@]} GUI applications"
    echo ""
    echo "Next steps:"
    echo "  - Restart your terminal to apply shell changes"
    echo "  - Run 'brew doctor' to verify Homebrew health"
    echo ""
    echo "Log file: ~/.openboot/logs/"
    echo ""
}

# Main function
main() {
    parse_args "$@"
    validate_silent_mode
    
    # Initialize error handling and progress tracking
    error_init
    progress_init
    
    ui_header "OpenBoot Installer v$VERSION"
    echo ""
    
    if $DRY_RUN; then
        echo "[DRY-RUN MODE - No changes will be made]"
        echo ""
    fi
    
    if $RESUME; then
        echo "Resuming from last incomplete step..."
        echo ""
    fi
    
    # Execute 5-step flow
    step_git_config
    step_preset_selection
    step_package_customization
    step_dotfiles_selection
    step_confirmation_and_install
    
    show_completion
}

# Run main
main "$@"
