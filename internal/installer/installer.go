package installer

import (
	"fmt"
	"os"

	"github.com/fullstackjam/openboot/internal/brew"
	"github.com/fullstackjam/openboot/internal/config"
	"github.com/fullstackjam/openboot/internal/dotfiles"
	"github.com/fullstackjam/openboot/internal/macos"
	"github.com/fullstackjam/openboot/internal/shell"
	"github.com/fullstackjam/openboot/internal/system"
	"github.com/fullstackjam/openboot/internal/ui"
)

func Run(cfg *config.Config) error {
	if cfg.Update {
		return runUpdate(cfg)
	}

	if cfg.Rollback {
		return runRollback(cfg)
	}

	return runInstall(cfg)
}

func runInstall(cfg *config.Config) error {
	fmt.Println()
	ui.Header("OpenBoot Installer v0.2.0")
	fmt.Println()

	if cfg.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	if err := stepGitConfig(cfg); err != nil {
		return err
	}

	if err := stepPresetSelection(cfg); err != nil {
		return err
	}

	if err := stepConfirmAndInstall(cfg); err != nil {
		return err
	}

	if err := stepDotfiles(cfg); err != nil {
		ui.Error(fmt.Sprintf("Dotfiles setup failed: %v", err))
	}

	if err := stepShell(cfg); err != nil {
		ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
	}

	if err := stepMacOS(cfg); err != nil {
		ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
	}

	showCompletion(cfg)
	return nil
}

func stepGitConfig(cfg *config.Config) error {
	ui.Header("Step 1: Git Configuration")
	fmt.Println()

	var name, email string

	// In dry-run mode without TTY, use placeholder values
	if cfg.DryRun && !system.HasTTY() {
		name = cfg.GitName
		email = cfg.GitEmail
		if name == "" {
			name = "Your Name"
		}
		if email == "" {
			email = "you@example.com"
		}
	} else if cfg.Silent {
		name = cfg.GitName
		email = cfg.GitEmail
		if name == "" || email == "" {
			return fmt.Errorf("OPENBOOT_GIT_NAME and OPENBOOT_GIT_EMAIL required in silent mode")
		}
	} else {
		var err error
		name, email, err = ui.InputGitConfig()
		if err != nil {
			return err
		}
	}

	if name == "" || email == "" {
		return fmt.Errorf("git name and email are required")
	}

	if cfg.DryRun {
		fmt.Printf("[DRY-RUN] Would configure git: %s <%s>\n", name, email)
	} else {
		if err := system.ConfigureGit(name, email); err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Git configured: %s <%s>", name, email))
	}

	fmt.Println()
	return nil
}

func stepPresetSelection(cfg *config.Config) error {
	ui.Header("Step 2: Preset Selection")
	fmt.Println()

	if cfg.Preset == "" {
		if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
			cfg.Preset = "minimal"
		} else {
			var err error
			cfg.Preset, err = ui.SelectPreset()
			if err != nil {
				return err
			}
		}
	}

	preset, ok := config.GetPreset(cfg.Preset)
	if !ok {
		return fmt.Errorf("invalid preset: %s", cfg.Preset)
	}

	ui.Success(fmt.Sprintf("Selected preset: %s", preset.Name))
	ui.Info(fmt.Sprintf("CLI packages: %d", len(preset.CLI)))
	ui.Info(fmt.Sprintf("GUI applications: %d", len(preset.Cask)))

	fmt.Println()
	return nil
}

func stepConfirmAndInstall(cfg *config.Config) error {
	preset, _ := config.GetPreset(cfg.Preset)

	ui.Header("Step 3: Installation")
	fmt.Println()

	if !cfg.Silent && !cfg.DryRun {
		proceed, err := ui.Confirm("Proceed with installation?", true)
		if err != nil {
			return err
		}
		if !proceed {
			ui.Muted("Installation cancelled.")
			os.Exit(0)
		}
	}

	fmt.Println()

	if err := brew.Install(preset.CLI, cfg.DryRun); err != nil {
		ui.Error(fmt.Sprintf("Failed to install CLI packages: %v", err))
	}

	fmt.Println()

	if err := brew.InstallCask(preset.Cask, cfg.DryRun); err != nil {
		ui.Error(fmt.Sprintf("Failed to install GUI applications: %v", err))
	}

	return nil
}

func stepDotfiles(cfg *config.Config) error {
	if cfg.Dotfiles == "skip" {
		return nil
	}

	ui.Header("Step 4: Dotfiles")
	fmt.Println()

	dotfilesURL := dotfiles.GetDotfilesURL()

	if cfg.Dotfiles == "" && dotfilesURL == "" {
		if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
			ui.Muted("Skipping dotfiles (no URL provided)")
			fmt.Println()
			return nil
		}

		setup, err := ui.Confirm("Do you have a dotfiles repository to set up?", false)
		if err != nil {
			return err
		}
		if !setup {
			ui.Muted("Skipping dotfiles setup")
			fmt.Println()
			return nil
		}

		dotfilesURL, err = ui.Input("Dotfiles repository URL", "https://github.com/username/dotfiles")
		if err != nil {
			return err
		}
	}

	if dotfilesURL != "" {
		if err := dotfiles.Clone(dotfilesURL, cfg.DryRun); err != nil {
			return err
		}
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

func stepShell(cfg *config.Config) error {
	if cfg.Shell == "skip" {
		return nil
	}

	ui.Header("Step 5: Shell Configuration")
	fmt.Println()

	if cfg.Shell == "" {
		if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
			cfg.Shell = "install"
		} else {
			install, err := ui.Confirm("Install Oh-My-Zsh and configure shell?", true)
			if err != nil {
				return err
			}
			if !install {
				ui.Muted("Skipping shell configuration")
				fmt.Println()
				return nil
			}
			cfg.Shell = "install"
		}
	}

	if cfg.Shell == "install" {
		if shell.IsOhMyZshInstalled() {
			ui.Muted("Oh-My-Zsh already installed")
		} else {
			if err := shell.InstallOhMyZsh(cfg.DryRun); err != nil {
				return fmt.Errorf("failed to install Oh-My-Zsh: %w", err)
			}
			if !cfg.DryRun {
				ui.Success("Oh-My-Zsh installed")
			}
		}

		if err := shell.ConfigureZshrc(cfg.DryRun); err != nil {
			return fmt.Errorf("failed to configure .zshrc: %w", err)
		}
		if !cfg.DryRun {
			ui.Success("Shell aliases configured")
		}
	}

	fmt.Println()
	return nil
}

func stepMacOS(cfg *config.Config) error {
	if cfg.Macos == "skip" {
		return nil
	}

	ui.Header("Step 6: macOS Preferences")
	fmt.Println()

	if cfg.Macos == "" {
		if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
			cfg.Macos = "configure"
		} else {
			configure, err := ui.Confirm("Apply developer-friendly macOS preferences?", true)
			if err != nil {
				return err
			}
			if !configure {
				ui.Muted("Skipping macOS preferences")
				fmt.Println()
				return nil
			}
			cfg.Macos = "configure"
		}
	}

	if cfg.Macos == "configure" {
		if err := macos.CreateScreenshotsDir(cfg.DryRun); err != nil {
			ui.Error(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
		}

		if err := macos.Configure(macos.DefaultPreferences, cfg.DryRun); err != nil {
			return err
		}

		if !cfg.DryRun {
			ui.Success("macOS preferences configured")
			macos.RestartAffectedApps(cfg.DryRun)
		}
	}

	fmt.Println()
	return nil
}

func showCompletion(cfg *config.Config) {
	preset, _ := config.GetPreset(cfg.Preset)

	fmt.Println()
	ui.Header("Installation Complete!")
	fmt.Println()

	ui.Success("OpenBoot has successfully configured your Mac.")
	fmt.Println()

	ui.Info("What was installed:")
	ui.Info(fmt.Sprintf("  - Git configured with your identity"))
	ui.Info(fmt.Sprintf("  - %d CLI packages", len(preset.CLI)))
	ui.Info(fmt.Sprintf("  - %d GUI applications", len(preset.Cask)))
	fmt.Println()

	ui.Info("Next steps:")
	ui.Info("  - Restart your terminal to apply changes")
	ui.Info("  - Run 'brew doctor' to verify Homebrew health")
	fmt.Println()
}

func runUpdate(cfg *config.Config) error {
	ui.Header("OpenBoot Update")
	fmt.Println()

	if err := brew.Update(cfg.DryRun); err != nil {
		return err
	}

	if !cfg.DryRun {
		brew.Cleanup()
	}

	fmt.Println()
	ui.Header("Update Complete!")
	return nil
}

func runRollback(cfg *config.Config) error {
	ui.Header("OpenBoot Rollback")
	fmt.Println()
	ui.Muted("Rollback functionality coming soon...")
	return nil
}
