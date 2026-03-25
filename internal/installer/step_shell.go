package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/shell"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// hasDotfiles reports whether dotfiles will be applied in this install.
// Checks remote config, env var, cfg flag, and local ~/.dotfiles existence.
func hasDotfiles(cfg *config.Config) bool {
	if cfg.Dotfiles == "skip" {
		return false
	}
	if cfg.DotfilesURL != "" {
		return true
	}
	if dotfiles.GetDotfilesURL() != "" {
		return true
	}
	return false
}

func stepShell(cfg *config.Config) error {
	if cfg.Shell == "skip" {
		return nil
	}

	ui.Header("Shell Configuration")
	fmt.Println()

	// Install Oh-My-Zsh if not present — dotfiles .zshrc may depend on it
	if shell.IsOhMyZshInstalled() {
		ui.Success("Oh-My-Zsh already installed")
	} else if cfg.Shell == "" {
		if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
			cfg.Shell = "install"
		} else {
			install, err := ui.Confirm("Install Oh-My-Zsh?", true)
			if err != nil {
				return err
			}
			if install {
				cfg.Shell = "install"
			} else {
				ui.Muted("Skipping Oh-My-Zsh")
			}
		}
	}

	if cfg.Shell == "install" {
		if shell.IsOhMyZshInstalled() {
			ui.Muted("Oh-My-Zsh already installed")
		} else {
			if err := shell.InstallOhMyZsh(cfg.DryRun); err != nil {
				return fmt.Errorf("install oh-my-zsh: %w", err)
			}
			if !cfg.DryRun {
				ui.Success("Oh-My-Zsh installed")
			}
		}
	}

	// Only modify .zshrc if user has no dotfiles — dotfiles manage .zshrc themselves.
	if !hasDotfiles(cfg) {
		if err := shell.EnsureBrewShellenv(cfg.DryRun); err != nil {
			return fmt.Errorf("ensure brew shellenv: %w", err)
		}
	}

	fmt.Println()
	return nil
}

func stepDotfiles(cfg *config.Config) error {
	if cfg.Dotfiles == "skip" {
		return nil
	}

	ui.Header("Step 6: Dotfiles")
	fmt.Println()

	var dotfilesURL string

	if cfg.Dotfiles == "" {
		// Resolve from env var first, then remote config.
		dotfilesURL = dotfiles.GetDotfilesURL()
		if dotfilesURL == "" {
			dotfilesURL = cfg.DotfilesURL
		}

		// Only prompt interactively if no URL is already configured.
		if dotfilesURL == "" && !cfg.Silent && !(cfg.DryRun && !system.HasTTY()) {
			setup, err := ui.Confirm("Do you have your own dotfiles repository?", false)
			if err != nil {
				return err
			}
			if setup {
				dotfilesURL, err = ui.Input("Dotfiles repository URL (https:// only)", "https://github.com/username/dotfiles")
				if err != nil {
					return err
				}
				if dotfilesURL != "" {
					if vErr := config.ValidateDotfilesURL(dotfilesURL); vErr != nil {
						return fmt.Errorf("invalid dotfiles URL: %w", vErr)
					}
				}
			}
		}
	} else {
		dotfilesURL = dotfiles.GetDotfilesURL()
		if dotfilesURL == "" {
			dotfilesURL = cfg.DotfilesURL
		}
	}

	// Fall back to the OpenBoot default dotfiles template.
	if dotfilesURL == "" {
		dotfilesURL = dotfiles.DefaultDotfilesURL
		ui.Info(fmt.Sprintf("Using OpenBoot default dotfiles (%s)", dotfilesURL))
	}

	if err := dotfiles.Clone(dotfilesURL, cfg.DryRun); err != nil {
		return err
	}

	if cfg.Dotfiles == "link" || cfg.Dotfiles == "" {
		if err := dotfiles.Link(cfg.DryRun); err != nil {
			return err
		}
	}

	if !cfg.DryRun {
		ui.Success("Dotfiles configured")
	}
	fmt.Println()
	return nil
}
