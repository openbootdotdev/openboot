package installer

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/shell"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// installOhMyZshFunc is a var so tests can stub out the real installer
// (which downloads and runs the upstream script).
var installOhMyZshFunc = shell.InstallOhMyZsh

func applyShell(plan InstallPlan, r Reporter) error {
	if plan.InstallOhMyZsh {
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
		ui.Println()
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

	if plan.DotfilesURL == dotfiles.DefaultDotfilesURL {
		r.Info(fmt.Sprintf("Using OpenBoot default dotfiles (%s)", plan.DotfilesURL))
	}

	if err := dotfiles.Clone(plan.DotfilesURL, plan.DryRun); err != nil {
		return fmt.Errorf("clone dotfiles: %w", err)
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
		return fmt.Errorf("link dotfiles: %w", err)
	}

	// Dotfiles commonly ship their own .zshrc whose plugins=() list references
	// external oh-my-zsh plugins (zsh-autosuggestions, fast-syntax-highlighting,
	// ...). When the remote config carries no shell block, that list never flows
	// through the shell step, so those plugins were never cloned and zsh logs
	// "plugin '...' not found" at every startup. Clone them off the linked
	// .zshrc now (no-op if OMZ is absent or no external plugins are referenced).
	if err := shell.CloneExternalPluginsFromZshrc(plan.DryRun); err != nil {
		return fmt.Errorf("clone plugins referenced by dotfiles: %w", err)
	}

	if !plan.DryRun {
		r.Success("Dotfiles configured")
	}
	ui.Println()
	return nil
}
