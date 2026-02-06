package installer

import (
	"fmt"
	"os"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/shell"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
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
	ui.Header("OpenBoot Installer v0.10.0")
	fmt.Println()

	if cfg.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	if cfg.RemoteConfig != nil {
		return runCustomInstall(cfg)
	}

	return runInteractiveInstall(cfg)
}

func runCustomInstall(cfg *config.Config) error {
	ui.Info(fmt.Sprintf("Custom config: @%s/%s", cfg.RemoteConfig.Username, cfg.RemoteConfig.Slug))

	if len(cfg.RemoteConfig.Taps) > 0 {
		ui.Info(fmt.Sprintf("Adding %d taps, installing %d packages...", len(cfg.RemoteConfig.Taps), len(cfg.RemoteConfig.Packages)))
	} else {
		ui.Info(fmt.Sprintf("Installing %d packages...", len(cfg.RemoteConfig.Packages)))
	}
	fmt.Println()

	if len(cfg.RemoteConfig.Taps) > 0 {
		if err := brew.InstallTaps(cfg.RemoteConfig.Taps, cfg.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Some taps failed: %v", err))
		}
		fmt.Println()
	}

	cfg.SelectedPkgs = make(map[string]bool)
	for _, pkg := range cfg.RemoteConfig.Packages {
		cfg.SelectedPkgs[pkg] = true
	}

	if err := stepInstallPackages(cfg); err != nil {
		return err
	}

	fmt.Println()
	ui.Success("Package installation complete!")
	ui.Muted("Dotfiles and shell setup will be handled by the install script.")
	fmt.Println()
	return nil
}

func runInteractiveInstall(cfg *config.Config) error {
	if err := stepGitConfig(cfg); err != nil {
		return err
	}

	if err := stepPresetSelection(cfg); err != nil {
		return err
	}

	if err := stepPackageCustomization(cfg); err != nil {
		return err
	}

	if err := stepInstallPackages(cfg); err != nil {
		return err
	}

	if err := stepShell(cfg); err != nil {
		ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
	}

	if err := stepDotfiles(cfg); err != nil {
		ui.Error(fmt.Sprintf("Dotfiles setup failed: %v", err))
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

func stepPackageCustomization(cfg *config.Config) error {
	ui.Header("Step 3: Package Selection")
	fmt.Println()

	if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
		cfg.SelectedPkgs = config.GetPackagesForPreset(cfg.Preset)
		total := len(cfg.SelectedPkgs)
		ui.Info(fmt.Sprintf("Using preset packages: %d selected", total))
		fmt.Println()
		return nil
	}

	ui.Info("Customize your packages (based on preset: " + cfg.Preset + ")")
	ui.Muted("Use Tab to switch categories, Space to toggle, Enter to confirm")
	fmt.Println()

	selected, confirmed, err := ui.RunSelector(cfg.Preset)
	if err != nil {
		return err
	}

	if !confirmed {
		ui.Muted("Installation cancelled.")
		os.Exit(0)
	}

	cfg.SelectedPkgs = selected

	if cfg.RemoteConfig != nil && len(cfg.RemoteConfig.Packages) > 0 {
		for _, pkg := range cfg.RemoteConfig.Packages {
			cfg.SelectedPkgs[pkg] = true
		}
	}

	count := 0
	for _, v := range selected {
		if v {
			count++
		}
	}
	ui.Success(fmt.Sprintf("Selected %d packages", count))
	fmt.Println()
	return nil
}

func stepInstallPackages(cfg *config.Config) error {
	ui.Header("Step 4: Installation")
	fmt.Println()

	var cliPkgs, caskPkgs []string

	if cfg.RemoteConfig != nil {
		for pkg := range cfg.SelectedPkgs {
			if config.IsCaskPackage(pkg) {
				caskPkgs = append(caskPkgs, pkg)
			} else {
				cliPkgs = append(cliPkgs, pkg)
			}
		}
	} else {
		for _, cat := range config.Categories {
			for _, pkg := range cat.Packages {
				if cfg.SelectedPkgs[pkg.Name] {
					if pkg.IsCask {
						caskPkgs = append(caskPkgs, pkg.Name)
					} else {
						cliPkgs = append(cliPkgs, pkg.Name)
					}
				}
			}
		}
	}

	total := len(cliPkgs) + len(caskPkgs)
	if total == 0 {
		ui.Muted("No packages selected")
		return nil
	}

	ui.Info(fmt.Sprintf("Installing %d packages (%d CLI, %d GUI)...", total, len(cliPkgs), len(caskPkgs)))
	fmt.Println()

	if err := brew.InstallWithProgress(cliPkgs, caskPkgs, cfg.DryRun); err != nil {
		ui.Error(fmt.Sprintf("Some packages failed: %v", err))
	}

	if !cfg.DryRun {
		ui.Success("Package installation complete")
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

	ui.Header("Step 7: macOS Preferences")
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
	var cliCount, caskCount int
	for _, cat := range config.Categories {
		for _, pkg := range cat.Packages {
			if cfg.SelectedPkgs[pkg.Name] {
				if pkg.IsCask {
					caskCount++
				} else {
					cliCount++
				}
			}
		}
	}

	fmt.Println()
	ui.Header("Installation Complete!")
	fmt.Println()

	ui.Success("OpenBoot has successfully configured your Mac.")
	fmt.Println()

	ui.Info("What was installed:")
	ui.Info("  - Git configured with your identity")
	ui.Info(fmt.Sprintf("  - %d CLI packages", cliCount))
	ui.Info(fmt.Sprintf("  - %d GUI applications", caskCount))
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
