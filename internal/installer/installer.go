package installer

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/permissions"
	"github.com/openbootdotdev/openboot/internal/shell"
	"github.com/openbootdotdev/openboot/internal/state"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

var ErrUserCancelled = fmt.Errorf("user cancelled")

const (
	estimatedSecondsPerFormula = 15
	estimatedSecondsPerCask    = 30
	estimatedSecondsPerNpm     = 5
)

func Run(cfg *config.Config) error {
	if cfg.Update {
		return runUpdate(cfg)
	}

	return runInstall(cfg)
}

func runInstall(cfg *config.Config) error {
	fmt.Println()
	ui.Header(fmt.Sprintf("OpenBoot Installer v%s", cfg.Version))
	fmt.Println()

	if cfg.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	if err := checkDependencies(cfg); err != nil {
		return err
	}

	if cfg.RemoteConfig != nil {
		return runCustomInstall(cfg)
	}

	return runInteractiveInstall(cfg)
}

func checkDependencies(cfg *config.Config) error {
	if cfg.DryRun {
		return nil
	}

	hasIssues := false

	if !brew.IsInstalled() {
		hasIssues = true
		ui.Warn("Homebrew is not installed")
		ui.Info("Homebrew is required to install packages")
		ui.Muted("Install with: /bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"")
		fmt.Println()
	}

	gitName, gitEmail := system.GetExistingGitConfig()
	if gitName == "" || gitEmail == "" {
		if !cfg.PackagesOnly {
			hasIssues = true
			ui.Warn("Git user information is not configured")
			ui.Info("You'll be prompted to configure it during installation")
			fmt.Println()
		}
	}

	if hasIssues && !cfg.Silent {
		cont, err := ui.Confirm("Continue with installation?", true)
		if err != nil {
			return err
		}
		if !cont {
			return fmt.Errorf("installation cancelled")
		}
		fmt.Println()
	}

	return nil
}

func runCustomInstall(cfg *config.Config) error {
	ui.Info(fmt.Sprintf("Custom config: @%s/%s", cfg.RemoteConfig.Username, cfg.RemoteConfig.Slug))

	if len(cfg.RemoteConfig.Taps) > 0 {
		ui.Info(fmt.Sprintf("Adding %d taps, installing %d packages...", len(cfg.RemoteConfig.Taps), len(cfg.RemoteConfig.Packages)))
	} else {
		ui.Info(fmt.Sprintf("Installing %d packages...", len(cfg.RemoteConfig.Packages)))
	}
	fmt.Println()

	formulaeCount := len(cfg.RemoteConfig.Packages)
	caskCount := len(cfg.RemoteConfig.Casks)
	npmCount := len(cfg.RemoteConfig.Npm)
	totalPackages := formulaeCount + caskCount + npmCount

	minutes := estimateInstallMinutes(formulaeCount, caskCount, npmCount)
	ui.Info(fmt.Sprintf("Estimated install time: ~%d min for %d packages", minutes, totalPackages))
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

	if len(cfg.RemoteConfig.Npm) > 0 {
		if err := stepInstallNpmWithRetry(cfg); err != nil {
			ui.Error(fmt.Sprintf("npm package installation failed: %v", err))
		}
	}

	fmt.Println()
	ui.Muted("Dotfiles and shell setup will be handled by the install script.")
	fmt.Println()
	return nil
}

func runInteractiveInstall(cfg *config.Config) error {
	if !cfg.PackagesOnly {
		if err := stepGitConfig(cfg); err != nil {
			return err
		}
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

	var softErrs []error

	if err := stepInstallNpmWithRetry(cfg); err != nil {
		ui.Error(fmt.Sprintf("npm package installation failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("npm: %w", err))
	}

	if !cfg.PackagesOnly {
		if err := stepShell(cfg); err != nil {
			ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
		}

		if err := stepDotfiles(cfg); err != nil {
			ui.Error(fmt.Sprintf("Dotfiles setup failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("dotfiles: %w", err))
		}

		if err := stepMacOS(cfg); err != nil {
			ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
		}
	}

	showCompletion(cfg)

	if len(softErrs) > 0 {
		fmt.Println()
		ui.Warn(fmt.Sprintf("%d setup step(s) had errors — run 'openboot doctor' to diagnose.", len(softErrs)))
		return errors.Join(softErrs...)
	}
	return nil
}

func stepGitConfig(cfg *config.Config) error {
	ui.Header("Step 1: Git Configuration")
	fmt.Println()

	// Smart detection: skip if already configured
	existingName, existingEmail := system.GetExistingGitConfig()

	if existingName != "" && existingEmail != "" {
		ui.Success(fmt.Sprintf("✓ Already configured: %s <%s>", existingName, existingEmail))
		fmt.Println()
		return nil
	}

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

	// Handle "scratch" as special case - use minimal but show full catalog
	if cfg.Preset == "scratch" {
		ui.Success("Selected: scratch (choose from full catalog)")
		ui.Muted("You'll be able to search and select individual packages")
		fmt.Println()
		return nil
	}

	preset, ok := config.GetPreset(cfg.Preset)
	if !ok {
		return fmt.Errorf("invalid preset: %s", cfg.Preset)
	}

	ui.Success(fmt.Sprintf("Selected preset: %s", preset.Name))
	ui.Info(fmt.Sprintf("CLI packages: %d", len(preset.CLI)))
	ui.Info(fmt.Sprintf("GUI applications: %d", len(preset.Cask)))
	if len(preset.Npm) > 0 {
		ui.Info(fmt.Sprintf("npm packages: %d", len(preset.Npm)))
	}

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

	selected, onlinePkgs, confirmed, err := ui.RunSelector(cfg.Preset)
	if err != nil {
		return err
	}

	if !confirmed {
		ui.Muted("Installation cancelled.")
		return ErrUserCancelled
	}

	cfg.SelectedPkgs = selected
	cfg.OnlinePkgs = onlinePkgs

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

	pkgs := categorizeSelectedPackages(cfg)
	cliPkgs := pkgs.cli
	caskPkgs := pkgs.cask

	total := len(cliPkgs) + len(caskPkgs)
	if total == 0 {
		ui.Muted("No packages selected")
		return nil
	}

	state, _ := loadState()

	var newCli []string
	var newCask []string

	if !cfg.DryRun {
		for _, pkg := range cliPkgs {
			if !state.isFormulaInstalled(pkg) {
				newCli = append(newCli, pkg)
			}
		}
		for _, pkg := range caskPkgs {
			if !state.isCaskInstalled(pkg) {
				newCask = append(newCask, pkg)
			}
		}

		stateSkipped := (len(cliPkgs) - len(newCli)) + (len(caskPkgs) - len(newCask))
		if stateSkipped > 0 {
			ui.Muted(fmt.Sprintf("Skipping %d packages from previous install", stateSkipped))
		}

		cliPkgs = newCli
		caskPkgs = newCask
	}

	if len(cliPkgs)+len(caskPkgs) == 0 {
		ui.Success("All packages already installed!")
		fmt.Println()
		return nil
	}

	ui.Info(fmt.Sprintf("Installing %d packages (%d CLI, %d GUI)...", len(cliPkgs)+len(caskPkgs), len(cliPkgs), len(caskPkgs)))
	fmt.Println()

	installedCli, installedCask, brewErr := brew.InstallWithProgress(cliPkgs, caskPkgs, cfg.DryRun)
	if brewErr != nil {
		ui.Error(fmt.Sprintf("Some packages failed: %v", brewErr))
	}

	if !cfg.DryRun {
		for _, pkg := range installedCli {
			state.markFormula(pkg)
		}
		for _, pkg := range installedCask {
			state.markCask(pkg)
		}
		ui.Success("Package installation complete")
	}
	fmt.Println()
	return nil
}

func stepInstallNpm(cfg *config.Config) error {
	var npmPkgs []string

	if cfg.RemoteConfig != nil {
		npmPkgs = cfg.RemoteConfig.Npm
	} else {
		pkgs := categorizeSelectedPackages(cfg)
		npmPkgs = pkgs.npm
	}

	if len(npmPkgs) == 0 {
		return nil
	}

	state, _ := loadState()

	var newNpm []string
	if !cfg.DryRun {
		for _, pkg := range npmPkgs {
			if !state.isNpmInstalled(pkg) {
				newNpm = append(newNpm, pkg)
			}
		}

		stateSkipped := len(npmPkgs) - len(newNpm)
		if stateSkipped > 0 {
			ui.Muted(fmt.Sprintf("Skipping %d npm packages from previous install", stateSkipped))
		}

		npmPkgs = newNpm
	}

	if len(npmPkgs) == 0 {
		ui.Success("All npm packages already installed!")
		return nil
	}

	fmt.Println()
	ui.Header("NPM Global Packages")
	fmt.Println()
	ui.Info(fmt.Sprintf("Installing %d npm packages...", len(npmPkgs)))
	fmt.Println()

	err := npm.Install(npmPkgs, cfg.DryRun)

	if !cfg.DryRun && err == nil {
		for _, pkg := range npmPkgs {
			state.markNpm(pkg)
		}
	}

	return err
}

func stepInstallNpmWithRetry(cfg *config.Config) error {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := stepInstallNpm(cfg)
		if err == nil {
			return nil
		}

		if attempt == maxAttempts {
			ui.Error(fmt.Sprintf("npm package installation failed after %d attempts: %v", maxAttempts, err))
			return fmt.Errorf("npm installation failed after %d attempts: %w", maxAttempts, err)
		}

		if cfg.Silent || !system.HasTTY() {
			ui.Warn(fmt.Sprintf("npm installation failed (attempt %d/%d), retrying...", attempt, maxAttempts))
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		fmt.Println()
		fmt.Printf("  Retry npm installation? [Y/n] ")
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "n" || response == "no" {
			ui.Muted("Skipping npm package retry")
			return err
		}
	}
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

	// Smart detection: skip if Oh-My-Zsh is already installed
	if shell.IsOhMyZshInstalled() && cfg.Shell == "" {
		ui.Success("✓ Oh-My-Zsh already installed")
		fmt.Println()
		return nil
	}

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
	var cliCount, caskCount, npmCount int
	for _, cat := range config.Categories {
		for _, pkg := range cat.Packages {
			if cfg.SelectedPkgs[pkg.Name] {
				if pkg.IsNpm {
					npmCount++
				} else if pkg.IsCask {
					caskCount++
				} else {
					cliCount++
				}
			}
		}
	}
	for _, pkg := range cfg.OnlinePkgs {
		if pkg.IsNpm {
			npmCount++
		} else if pkg.IsCask {
			caskCount++
		} else {
			cliCount++
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
	if npmCount > 0 {
		ui.Info(fmt.Sprintf("  - %d npm global packages", npmCount))
	}
	fmt.Println()

	showScreenRecordingReminder(cfg)

	ui.Info("Next steps:")
	ui.Info("  - Restart your terminal to apply changes")
	ui.Info("  - Run 'brew doctor' to verify Homebrew health")
	fmt.Println()
}

func RunFromSnapshot(cfg *config.Config) error {
	fmt.Println()
	ui.Header("OpenBoot — Restore from Snapshot")
	fmt.Println()

	if cfg.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	if len(cfg.SnapshotTaps) > 0 {
		ui.Info(fmt.Sprintf("Adding %d taps...", len(cfg.SnapshotTaps)))
		fmt.Println()
		if err := brew.InstallTaps(cfg.SnapshotTaps, cfg.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Some taps failed: %v", err))
		}
		fmt.Println()
	}

	if err := stepInstallPackages(cfg); err != nil {
		return err
	}

	if err := stepInstallNpmWithRetry(cfg); err != nil {
		ui.Error(fmt.Sprintf("npm package installation failed: %v", err))
	}

	var softErrs []error

	if cfg.SnapshotGit != nil {
		if err := stepRestoreGit(cfg); err != nil {
			ui.Error(fmt.Sprintf("Git restore failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("git restore: %w", err))
		}
	}

	if cfg.SnapshotShell != nil && cfg.SnapshotShell.OhMyZsh {
		if err := stepRestoreShell(cfg); err != nil {
			ui.Error(fmt.Sprintf("Shell restore failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("shell restore: %w", err))
		}
	} else {
		if err := stepShell(cfg); err != nil {
			ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
		}
	}

	if err := stepMacOS(cfg); err != nil {
		ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
	}

	showCompletion(cfg)

	if len(softErrs) > 0 {
		fmt.Println()
		ui.Warn(fmt.Sprintf("%d restore step(s) had errors — run 'openboot doctor' to diagnose.", len(softErrs)))
		return errors.Join(softErrs...)
	}
	return nil
}

func stepRestoreGit(cfg *config.Config) error {
	ui.Header("Restore: Git Configuration")
	fmt.Println()

	git := cfg.SnapshotGit
	if git.UserName == "" && git.UserEmail == "" {
		ui.Muted("No git config in snapshot, skipping")
		fmt.Println()
		return nil
	}

	existingName, existingEmail := system.GetExistingGitConfig()

	if existingName != "" && existingEmail != "" {
		ui.Success(fmt.Sprintf("✓ Already configured: %s <%s>", existingName, existingEmail))
		fmt.Println()
		return nil
	}

	if cfg.DryRun {
		if existingName == "" && git.UserName != "" {
			fmt.Printf("[DRY-RUN] Would set git user.name = %s\n", git.UserName)
		}
		if existingEmail == "" && git.UserEmail != "" {
			fmt.Printf("[DRY-RUN] Would set git user.email = %s\n", git.UserEmail)
		}
		fmt.Println()
		return nil
	}

	nameToSet := existingName
	emailToSet := existingEmail
	if existingName == "" && git.UserName != "" {
		nameToSet = git.UserName
	}
	if existingEmail == "" && git.UserEmail != "" {
		emailToSet = git.UserEmail
	}

	if nameToSet != existingName || emailToSet != existingEmail {
		if err := system.ConfigureGit(nameToSet, emailToSet); err != nil {
			return fmt.Errorf("failed to restore git config: %w", err)
		}
	}

	ui.Success(fmt.Sprintf("Git restored: %s <%s>", nameToSet, emailToSet))
	fmt.Println()
	return nil
}

func stepRestoreShell(cfg *config.Config) error {
	ui.Header("Restore: Shell Configuration")
	fmt.Println()

	shellCfg := cfg.SnapshotShell

	if cfg.DryRun {
		fmt.Println("[DRY-RUN] Would restore shell config from snapshot")
	}

	ui.Info(fmt.Sprintf("Theme: %s, Plugins: %v", shellCfg.Theme, shellCfg.Plugins))
	fmt.Println()

	if err := shell.RestoreFromSnapshot(shellCfg.OhMyZsh, shellCfg.Theme, shellCfg.Plugins, cfg.DryRun); err != nil {
		return err
	}

	if err := shell.ConfigureZshrc(cfg.DryRun); err != nil {
		return fmt.Errorf("failed to configure .zshrc: %w", err)
	}

	if !cfg.DryRun {
		ui.Success("Shell configuration restored")
	}
	fmt.Println()
	return nil
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

func estimateInstallMinutes(formulaeCount, caskCount, npmCount int) int {
	totalSeconds := formulaeCount*estimatedSecondsPerFormula +
		caskCount*estimatedSecondsPerCask +
		npmCount*estimatedSecondsPerNpm
	minutes := totalSeconds / 60
	if minutes < 1 {
		minutes = 1
	}
	return minutes
}

type categorizedPackages struct {
	cli  []string
	cask []string
	npm  []string
}

func categorizeSelectedPackages(cfg *config.Config) categorizedPackages {
	var result categorizedPackages

	if cfg.RemoteConfig != nil {
		caskSet := make(map[string]bool)
		for _, c := range cfg.RemoteConfig.Casks {
			caskSet[c] = true
		}
		npmSet := make(map[string]bool)
		for _, n := range cfg.RemoteConfig.Npm {
			npmSet[n] = true
		}
		for pkg := range cfg.SelectedPkgs {
			if npmSet[pkg] || config.IsNpmPackage(pkg) {
				result.npm = append(result.npm, pkg)
			} else if caskSet[pkg] || config.IsCaskPackage(pkg) {
				result.cask = append(result.cask, pkg)
			} else {
				result.cli = append(result.cli, pkg)
			}
		}
		return result
	}

	seen := make(map[string]bool)
	for _, cat := range config.Categories {
		for _, pkg := range cat.Packages {
			if cfg.SelectedPkgs[pkg.Name] {
				seen[pkg.Name] = true
				if pkg.IsNpm {
					result.npm = append(result.npm, pkg.Name)
				} else if pkg.IsCask {
					result.cask = append(result.cask, pkg.Name)
				} else {
					result.cli = append(result.cli, pkg.Name)
				}
			}
		}
	}
	for _, pkg := range cfg.OnlinePkgs {
		if seen[pkg.Name] {
			continue
		}
		if pkg.IsNpm {
			result.npm = append(result.npm, pkg.Name)
		} else if pkg.IsCask {
			result.cask = append(result.cask, pkg.Name)
		} else {
			result.cli = append(result.cli, pkg.Name)
		}
	}
	return result
}

func findMatchingPackages(cfg *config.Config, triggerPkgs []string) []string {
	triggerSet := make(map[string]bool, len(triggerPkgs))
	for _, p := range triggerPkgs {
		triggerSet[p] = true
	}

	var matched []string
	for pkg := range cfg.SelectedPkgs {
		if triggerSet[pkg] {
			matched = append(matched, pkg)
		}
	}
	for _, pkg := range cfg.OnlinePkgs {
		if triggerSet[pkg.Name] {
			matched = append(matched, pkg.Name)
		}
	}
	return matched
}

func showScreenRecordingReminder(cfg *config.Config) {
	if cfg.DryRun || cfg.Silent {
		return
	}

	statePath := state.DefaultStatePath()
	reminderState, err := state.LoadState(statePath)
	if err != nil {
		return
	}

	if !state.ShouldShowReminder(reminderState) {
		return
	}

	if permissions.HasScreenRecordingPermission() {
		return
	}

	triggerPkgs := config.GetScreenRecordingPackages()
	matchingPkgs := findMatchingPackages(cfg, triggerPkgs)
	if len(matchingPkgs) == 0 {
		return
	}

	fmt.Println()
	ui.Header("Screen Recording Permission")
	fmt.Println()
	ui.Info(fmt.Sprintf("You installed: %s", strings.Join(matchingPkgs, ", ")))
	ui.Info("These apps need Screen Recording permission for screen sharing.")
	fmt.Println()

	choice, err := ui.SelectOption("What would you like to do?", []string{
		"Open System Settings",
		"Remind me next time",
		"Don't remind again",
	})
	if err != nil {
		state.MarkSkipped(reminderState)
		_ = state.SaveState(statePath, reminderState)
		return
	}

	switch choice {
	case "Open System Settings":
		if err := permissions.OpenScreenRecordingSettings(); err != nil {
			ui.Warn("Could not open System Settings")
		}
		state.MarkSkipped(reminderState)
	case "Remind me next time":
		state.MarkSkipped(reminderState)
	case "Don't remind again":
		state.MarkDismissed(reminderState)
	}

	_ = state.SaveState(statePath, reminderState)
	fmt.Println()
}
