package installer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/permissions"
	"github.com/openbootdotdev/openboot/internal/state"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

var ErrUserCancelled = errors.New("user cancelled")

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
			ui.Info("You can set it up via dotfiles or manually after installation")
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

	printPackageList("CLI tools", cfg.RemoteConfig.Packages)
	printPackageList("Apps", cfg.RemoteConfig.Casks)
	printPackageList("npm", cfg.RemoteConfig.Npm)
	fmt.Println()

	if !cfg.Silent && !cfg.DryRun {
		proceed, err := ui.Confirm("Install these packages?", true)
		if err != nil {
			return err
		}
		if !proceed {
			return ErrUserCancelled
		}
		fmt.Println()
	}

	if len(cfg.RemoteConfig.Taps) > 0 {
		if err := brew.InstallTaps(cfg.RemoteConfig.Taps, cfg.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Some taps failed: %v", err))
		}
		fmt.Println()
	}

	cfg.SelectedPkgs = make(map[string]bool)
	for _, pkg := range cfg.RemoteConfig.Packages {
		cfg.SelectedPkgs[pkg.Name] = true
	}

	if err := stepInstallPackages(cfg); err != nil {
		return err
	}

	var softErrs []error

	if len(cfg.RemoteConfig.Npm) > 0 {
		if err := stepInstallNpmWithRetry(cfg); err != nil {
			ui.Error(fmt.Sprintf("npm package installation failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("npm: %w", err))
		}
	}

	if cfg.RemoteConfig.DotfilesRepo != "" {
		cfg.DotfilesURL = cfg.RemoteConfig.DotfilesRepo
	}

	if err := stepShell(cfg); err != nil {
		ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
	}

	if err := stepDotfiles(cfg); err != nil {
		ui.Error(fmt.Sprintf("Dotfiles setup failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("dotfiles: %w", err))
	}

	// If dotfiles were applied, .zshrc is managed by dotfiles.
	// If no dotfiles, stepShell already handled brew shellenv.

	if len(cfg.RemoteConfig.MacOSPrefs) > 0 {
		cfg.SnapshotMacOS = cfg.RemoteConfig.MacOSPrefs
		if err := stepRestoreMacOS(cfg); err != nil {
			ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
		}
	} else {
		if err := stepMacOS(cfg); err != nil {
			ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
		}
	}

	if err := stepPostInstall(cfg); err != nil {
		ui.Error(fmt.Sprintf("Post-install script failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("post-install: %w", err))
	}

	fmt.Println()
	ui.Header("Installation Complete!")
	fmt.Println()
	ui.Success("OpenBoot has successfully configured your Mac.")
	fmt.Println()

	if len(softErrs) > 0 {
		fmt.Println()
		ui.Warn(fmt.Sprintf("%d setup step(s) had errors — run 'openboot doctor' to diagnose.", len(softErrs)))
		return errors.Join(softErrs...)
	}
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

	if err := stepShell(cfg); err != nil {
		ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
	}

	if err := stepRestoreMacOS(cfg); err != nil {
		ui.Error(fmt.Sprintf("macOS restore failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
	}

	if cfg.SnapshotDotfiles != "" {
		if err := stepDotfiles(cfg); err != nil {
			ui.Error(fmt.Sprintf("Dotfiles restore failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("dotfiles: %w", err))
		}
	}

	showCompletion(cfg)

	if len(softErrs) > 0 {
		fmt.Println()
		ui.Warn(fmt.Sprintf("%d restore step(s) had errors — run 'openboot doctor' to diagnose.", len(softErrs)))
		return errors.Join(softErrs...)
	}
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
	if !cfg.PackagesOnly {
		ui.Info("  - Git configured with your identity")
	}
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

func printPackageList(label string, pkgs config.PackageEntryList) {
	if len(pkgs) == 0 {
		return
	}
	hasDesc := false
	for _, pkg := range pkgs {
		if pkg.Desc != "" {
			hasDesc = true
			break
		}
	}
	if !hasDesc {
		ui.Muted(fmt.Sprintf("  %s: %s", label, strings.Join(pkgs.Names(), ", ")))
		return
	}
	ui.Muted(fmt.Sprintf("  %s:", label))
	for _, pkg := range pkgs {
		if pkg.Desc != "" {
			ui.Muted(fmt.Sprintf("    %s — %s", pkg.Name, pkg.Desc))
		} else {
			ui.Muted(fmt.Sprintf("    %s", pkg.Name))
		}
	}
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
		if err := state.SaveState(statePath, reminderState); err != nil {
			ui.Warn(fmt.Sprintf("Failed to save install state: %v", err))
		}
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

	if err := state.SaveState(statePath, reminderState); err != nil {
		ui.Warn(fmt.Sprintf("Failed to save install state: %v", err))
	}
	fmt.Println()
}
