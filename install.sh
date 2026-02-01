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
source "$SCRIPT_DIR/lib/core/rollback.sh"
source "$SCRIPT_DIR/lib/ui/gum.sh"
source "$SCRIPT_DIR/lib/packages.sh"
source "$SCRIPT_DIR/lib/brew.sh"
source "$SCRIPT_DIR/lib/shell.sh"

# Flags
PRESET=""
SILENT=false
DOTFILES_MODE=""
DRY_RUN=false
RESUME=false
ROLLBACK=false
SHELL_FRAMEWORK=""
UPDATE=false

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
    --preset NAME   Set preset (minimal, standard, full, devops, frontend, data, mobile, ai)
    --silent        Non-interactive mode (requires env vars)
    --dotfiles MODE Set dotfiles mode (clone, link, skip)
    --shell MODE    Set shell framework (install, skip)
    --dry-run       Show what would be installed without installing
    --resume        Resume from last incomplete step
    --rollback      Restore backed up files to their original state
    --update        Update Homebrew and upgrade all installed packages

ENVIRONMENT VARIABLES (for --silent mode):
    OPENBOOT_GIT_NAME         Git user name (required in silent mode)
    OPENBOOT_GIT_EMAIL        Git user email (required in silent mode)
    OPENBOOT_PRESET           Default preset if --preset not specified
    OPENBOOT_DOTFILES         Dotfiles repository URL
    OPENBOOT_SHELL_FRAMEWORK  Shell framework (install, skip)

EXAMPLES:
    ./install.sh                          # Interactive installation
    ./install.sh --preset standard        # Skip preset selection
    ./install.sh --dry-run                # Preview installation
    ./install.sh --resume                 # Resume interrupted install
    ./install.sh --rollback               # Restore original files
    
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
            --shell)
                if [[ -z "${2:-}" ]]; then
                    echo "Error: --shell requires a value" >&2
                    exit 1
                fi
                SHELL_FRAMEWORK="$2"
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
            --rollback)
                ROLLBACK=true
                shift
                ;;
            --update)
                UPDATE=true
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
        echo ""
        echo "  minimal   $(get_preset_description minimal)"
        echo "  standard  $(get_preset_description standard)"
        echo "  full      $(get_preset_description full)"
        echo "  devops    $(get_preset_description devops)"
        echo "  frontend  $(get_preset_description frontend)"
        echo "  data      $(get_preset_description data)"
        echo "  mobile    $(get_preset_description mobile)"
        echo "  ai        $(get_preset_description ai)"
        echo ""
        
        PRESET=$(ui_choose "Select preset:" "minimal" "standard" "full" "devops" "frontend" "data" "mobile" "ai")
    fi
    
    # Validate preset
    if [[ ! "$PRESET" =~ ^(minimal|standard|full|devops|frontend|data|mobile|ai)$ ]]; then
        echo "Error: Invalid preset '$PRESET'. Must be: minimal, standard, full, devops, frontend, data, mobile, or ai" >&2
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

step_shell_framework() {
    if $RESUME && progress_is_complete "shell_framework"; then
        echo "Shell framework setup already complete, skipping..."
        return 0
    fi
    
    progress_start "shell_framework"
    ui_header "Step 5: Shell Framework (Optional)"
    echo ""
    
    if [[ -n "$SHELL_FRAMEWORK" ]]; then
        if shell_is_omz_installed && [[ "$SHELL_FRAMEWORK" == "install" ]]; then
            echo "Oh-My-Zsh is already installed"
            SHELL_FRAMEWORK="skip"
        else
            echo "Shell framework: $SHELL_FRAMEWORK"
        fi
    elif shell_is_omz_installed; then
        echo "Oh-My-Zsh is already installed"
        SHELL_FRAMEWORK="skip"
    elif $SILENT; then
        SHELL_FRAMEWORK="${OPENBOOT_SHELL_FRAMEWORK:-skip}"
        echo "Shell framework: $SHELL_FRAMEWORK"
    else
        echo "Oh-My-Zsh enhances your terminal with themes and plugins."
        echo ""
        SHELL_FRAMEWORK=$(ui_choose "Install Oh-My-Zsh?" "skip" "install")
    fi
    
    progress_complete "shell_framework"
    echo ""
}

step_confirmation_and_install() {
    if $RESUME && progress_is_complete "installation"; then
        echo "Installation already complete!"
        return 0
    fi
    
    progress_start "installation"
    ui_header "Step 6: Confirmation"
    echo ""
    
    echo "Installation Summary:"
    echo "  Preset:       $PRESET"
    echo "  CLI tools:    ${#SELECTED_CLI[@]} packages"
    echo "  GUI apps:     ${#SELECTED_CASK[@]} applications"
    echo "  Dotfiles:     $DOTFILES_MODE"
    echo "  Oh-My-Zsh:    $SHELL_FRAMEWORK"
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
        if [[ "$SHELL_FRAMEWORK" == "install" ]]; then
            echo "Shell framework:"
            echo "  Oh-My-Zsh + plugins (zsh-autosuggestions, zsh-syntax-highlighting, etc.)"
            echo ""
        fi
        progress_complete "installation"
        return 0
    fi
    
    if ! $SILENT; then
        if ! ui_confirm "Proceed with installation?" true; then
            echo "Installation cancelled by user"
            exit 0
        fi
    fi
    
    echo ""
    ui_header "Installing packages..."
    echo ""
    
    if [[ ${#SELECTED_CLI[@]} -gt 0 ]]; then
        echo "Installing CLI packages..."
        brew_install_packages "${SELECTED_CLI[@]}"
        echo ""
    fi
    
    if [[ ${#SELECTED_CASK[@]} -gt 0 ]]; then
        echo "Installing GUI applications..."
        brew_install_casks "${SELECTED_CASK[@]}"
        echo ""
    fi
    
    if [[ "$SHELL_FRAMEWORK" == "install" ]]; then
        echo "Setting up Oh-My-Zsh..."
        shell_setup_omz
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
    [[ "$SHELL_FRAMEWORK" == "install" ]] && echo "  - Oh-My-Zsh with plugins"
    echo ""
    echo "Next steps:"
    echo "  - Restart your terminal to apply shell changes"
    echo "  - Run 'brew doctor' to verify Homebrew health"
    echo ""
    echo "Log file: ~/.openboot/logs/"
    echo ""
}

run_update() {
    ui_header "OpenBoot Update"
    echo ""
    
    if ! brew_ensure_installed; then
        echo "Error: Homebrew not found. Cannot update."
        exit 1
    fi
    
    echo "Updating Homebrew..."
    brew update
    echo ""
    
    echo "Outdated packages:"
    local outdated
    outdated=$(brew outdated 2>/dev/null)
    if [[ -z "$outdated" ]]; then
        echo "  All packages are up to date!"
    else
        echo "$outdated" | sed 's/^/  /'
        echo ""
        
        if $SILENT; then
            echo "Upgrading all packages..."
            brew upgrade
        else
            if ui_confirm "Upgrade all packages?" true; then
                echo ""
                echo "Upgrading packages..."
                brew upgrade
            fi
        fi
    fi
    
    echo ""
    echo "Outdated casks:"
    local outdated_casks
    outdated_casks=$(brew outdated --cask 2>/dev/null)
    if [[ -z "$outdated_casks" ]]; then
        echo "  All casks are up to date!"
    else
        echo "$outdated_casks" | sed 's/^/  /'
        echo ""
        
        if $SILENT; then
            echo "Upgrading all casks..."
            brew upgrade --cask
        else
            if ui_confirm "Upgrade all casks?" true; then
                echo ""
                echo "Upgrading casks..."
                brew upgrade --cask
            fi
        fi
    fi
    
    echo ""
    echo "Cleaning up old versions..."
    brew cleanup
    
    echo ""
    ui_header "Update Complete!"
}

main() {
    parse_args "$@"
    
    if $ROLLBACK; then
        ui_header "OpenBoot Rollback"
        echo ""
        rollback_status
        echo ""
        rollback_interactive
        exit $?
    fi
    
    if $UPDATE; then
        run_update
        exit $?
    fi
    
    validate_silent_mode
    
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
    
    step_git_config
    step_preset_selection
    step_package_customization
    step_dotfiles_selection
    step_shell_framework
    step_confirmation_and_install
    
    show_completion
}

# Run main
main "$@"
