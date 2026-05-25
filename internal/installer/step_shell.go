package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/shell"
)

// installOhMyZshFunc is a var so tests can stub out the real installer
// (which downloads and runs the upstream script).
var installOhMyZshFunc = shell.InstallOhMyZsh

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

	// If the cloned dotfiles reference Oh-My-Zsh but the shell step didn't
	// install it (e.g. remote config with oh_my_zsh:false), install OMZ now
	// before linking so the resulting .zshrc isn't broken. Skip when the shell
	// step already tried — it would just fail again with the same error.
	if !plan.DryRun && !plan.InstallOhMyZsh && !shell.IsOhMyZshInstalled() {
		if dfPath, err := dotfiles.DefaultPath(); err == nil && dotfiles.ReferencesOMZ(dfPath) {
			if err := installOhMyZshFunc(false); err != nil {
				r.Error(fmt.Sprintf("dotfiles require Oh-My-Zsh but installation failed: %v", err))
			} else {
				r.Success("Oh-My-Zsh installed (required by dotfiles)")
			}
		}
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
