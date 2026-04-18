package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/shell"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

func applyShell(plan InstallPlan, r Reporter) error {
	if plan.InstallOhMyZsh {
		r.Header("Shell Configuration")
		fmt.Println()

		if plan.ShellTheme != "" || len(plan.ShellPlugins) > 0 {
			// Restore mode: install OMZ if missing, then write theme/plugins.
			if err := shell.RestoreFromSnapshot(true, plan.ShellTheme, plan.ShellPlugins, plan.DryRun); err != nil {
				return fmt.Errorf("restore shell config: %w", err)
			}
			if !plan.DryRun {
				r.Success(fmt.Sprintf("Shell restored (theme: %s, %d plugins)", plan.ShellTheme, len(plan.ShellPlugins)))
			}
		} else {
			// Fresh install mode: just install OMZ.
			if shell.IsOhMyZshInstalled() {
				r.Muted("Oh-My-Zsh already installed")
			} else {
				if err := shell.InstallOhMyZsh(plan.DryRun); err != nil {
					return fmt.Errorf("install oh-my-zsh: %w", err)
				}
				if !plan.DryRun {
					r.Success("Oh-My-Zsh installed")
				}
			}
		}
		fmt.Println()
	}

	// Ensure brew shellenv in .zshrc only when user has no dotfiles managing it.
	if plan.DotfilesURL == "" || plan.DotfilesURL == dotfiles.DefaultDotfilesURL {
		if err := shell.EnsureBrewShellenv(plan.DryRun); err != nil {
			return fmt.Errorf("ensure brew shellenv: %w", err)
		}
	}
	return nil
}

func applyDotfiles(plan InstallPlan, r Reporter) error {
	if plan.DotfilesURL == "" {
		return nil // explicitly skipped via --dotfiles skip
	}

	r.Header("Step 6: Dotfiles")
	fmt.Println()

	if plan.DotfilesURL == dotfiles.DefaultDotfilesURL {
		r.Info(fmt.Sprintf("Using OpenBoot default dotfiles (%s)", plan.DotfilesURL))
	}

	if err := dotfiles.Clone(plan.DotfilesURL, plan.DryRun); err != nil {
		return err
	}
	if err := dotfiles.Link(plan.DryRun); err != nil {
		return err
	}

	if !plan.DryRun {
		r.Success("Dotfiles configured")
	}
	fmt.Println()
	return nil
}

// hasDotfiles reports whether dotfiles will be applied in this install.
// Checks remote config, env var, opts flag, and local ~/.dotfiles existence.
func hasDotfiles(opts *config.InstallOptions, st *config.InstallState) bool {
	if opts.Dotfiles == "skip" {
		return false
	}
	if opts.DotfilesURL != "" {
		return true
	}
	if dotfiles.GetDotfilesURL() != "" {
		return true
	}
	return false
}

func stepShell(opts *config.InstallOptions, st *config.InstallState) error {
	if opts.Shell == "skip" {
		return nil
	}

	ui.Header("Shell Configuration")
	fmt.Println()

	// Install Oh-My-Zsh if not present — dotfiles .zshrc may depend on it
	if shell.IsOhMyZshInstalled() {
		ui.Success("Oh-My-Zsh already installed")
	} else if opts.Shell == "" {
		if opts.Silent || (opts.DryRun && !system.HasTTY()) {
			opts.Shell = "install"
		} else {
			install, err := ui.Confirm("Install Oh-My-Zsh?", true)
			if err != nil {
				return err
			}
			if install {
				opts.Shell = "install"
			} else {
				ui.Muted("Skipping Oh-My-Zsh")
			}
		}
	}

	if opts.Shell == "install" {
		if shell.IsOhMyZshInstalled() {
			ui.Muted("Oh-My-Zsh already installed")
		} else {
			if err := shell.InstallOhMyZsh(opts.DryRun); err != nil {
				return fmt.Errorf("install oh-my-zsh: %w", err)
			}
			if !opts.DryRun {
				ui.Success("Oh-My-Zsh installed")
			}
		}
	}

	// Only modify .zshrc if user has no dotfiles — dotfiles manage .zshrc themselves.
	if !hasDotfiles(opts, st) {
		if err := shell.EnsureBrewShellenv(opts.DryRun); err != nil {
			return fmt.Errorf("ensure brew shellenv: %w", err)
		}
	}

	fmt.Println()
	return nil
}

func stepDotfiles(opts *config.InstallOptions, st *config.InstallState) error {
	if opts.Dotfiles == "skip" {
		return nil
	}

	ui.Header("Step 6: Dotfiles")
	fmt.Println()

	var dotfilesURL string

	if opts.Dotfiles == "" {
		// Resolve from env var first, then remote config.
		dotfilesURL = dotfiles.GetDotfilesURL()
		if dotfilesURL == "" {
			dotfilesURL = opts.DotfilesURL
		}

		// Only prompt interactively if no URL is already configured.
		if dotfilesURL == "" && !opts.Silent && (!opts.DryRun || system.HasTTY()) {
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
			dotfilesURL = opts.DotfilesURL
		}
	}

	// Fall back to the OpenBoot default dotfiles template.
	if dotfilesURL == "" {
		dotfilesURL = dotfiles.DefaultDotfilesURL
		ui.Info(fmt.Sprintf("Using OpenBoot default dotfiles (%s)", dotfilesURL))
	}

	if err := dotfiles.Clone(dotfilesURL, opts.DryRun); err != nil {
		return err
	}

	if opts.Dotfiles == "link" || opts.Dotfiles == "" {
		if err := dotfiles.Link(opts.DryRun); err != nil {
			return err
		}
	}

	if !opts.DryRun {
		ui.Success("Dotfiles configured")
	}
	fmt.Println()
	return nil
}
