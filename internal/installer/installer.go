package installer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/permissions"
	installstate "github.com/openbootdotdev/openboot/internal/state"
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
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()

	var err error
	if opts.Update {
		err = runUpdate(opts, st)
	} else {
		err = runInstall(opts, st)
	}

	cfg.ApplyState(st)
	return err
}

func runInstall(opts *config.InstallOptions, st *config.InstallState) error {
	fmt.Println()
	ui.Header(fmt.Sprintf("OpenBoot Installer v%s", opts.Version))
	fmt.Println()

	if opts.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	if err := checkDependencies(opts, st); err != nil {
		return err
	}

	if st.RemoteConfig != nil {
		return runCustomInstall(opts, st)
	}

	return runInteractiveInstall(opts, st)
}

func checkDependencies(opts *config.InstallOptions, st *config.InstallState) error {
	if opts.DryRun {
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
		if !opts.PackagesOnly {
			hasIssues = true
			ui.Warn("Git user information is not configured")
			ui.Info("You can set it up via dotfiles or manually after installation")
			fmt.Println()
		}
	}

	if hasIssues && !opts.Silent {
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

func runCustomInstall(opts *config.InstallOptions, st *config.InstallState) error {
	ui.Info(fmt.Sprintf("Custom config: @%s/%s", st.RemoteConfig.Username, st.RemoteConfig.Slug))

	if len(st.RemoteConfig.Taps) > 0 {
		ui.Info(fmt.Sprintf("Adding %d taps, installing %d packages...", len(st.RemoteConfig.Taps), len(st.RemoteConfig.Packages)))
	} else {
		ui.Info(fmt.Sprintf("Installing %d packages...", len(st.RemoteConfig.Packages)))
	}
	fmt.Println()

	formulaeCount := len(st.RemoteConfig.Packages)
	caskCount := len(st.RemoteConfig.Casks)
	npmCount := len(st.RemoteConfig.Npm)
	totalPackages := formulaeCount + caskCount + npmCount

	minutes := estimateInstallMinutes(formulaeCount, caskCount, npmCount)
	ui.Info(fmt.Sprintf("Estimated install time: ~%d min for %d packages", minutes, totalPackages))
	fmt.Println()

	printPackageList("CLI tools", st.RemoteConfig.Packages)
	printPackageList("Apps", st.RemoteConfig.Casks)
	printPackageList("npm", st.RemoteConfig.Npm)
	fmt.Println()

	if !opts.Silent && !opts.DryRun {
		proceed, err := ui.Confirm("Install these packages?", true)
		if err != nil {
			return err
		}
		if !proceed {
			return ErrUserCancelled
		}
		fmt.Println()
	}

	if len(st.RemoteConfig.Taps) > 0 {
		if err := brew.InstallTaps(st.RemoteConfig.Taps, opts.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Some taps failed: %v", err))
		}
		fmt.Println()
	}

	st.SelectedPkgs = make(map[string]bool)
	for _, pkg := range st.RemoteConfig.Packages {
		st.SelectedPkgs[pkg.Name] = true
	}

	if err := stepInstallPackages(opts, st); err != nil {
		return err
	}

	var softErrs []error

	if len(st.RemoteConfig.Npm) > 0 {
		if err := stepInstallNpmWithRetry(opts, st); err != nil {
			ui.Error(fmt.Sprintf("npm package installation failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("npm: %w", err))
		}
	}

	if st.RemoteConfig.DotfilesRepo != "" {
		opts.DotfilesURL = st.RemoteConfig.DotfilesRepo
	}

	if err := stepShell(opts, st); err != nil {
		ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
	}

	if err := stepDotfiles(opts, st); err != nil {
		ui.Error(fmt.Sprintf("Dotfiles setup failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("dotfiles: %w", err))
	}

	// If dotfiles were applied, .zshrc is managed by dotfiles.
	// If no dotfiles, stepShell already handled brew shellenv.

	if len(st.RemoteConfig.MacOSPrefs) > 0 {
		st.SnapshotMacOS = st.RemoteConfig.MacOSPrefs
		if err := stepRestoreMacOS(opts, st); err != nil {
			ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
		}
	} else {
		if err := stepMacOS(opts, st); err != nil {
			ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
		}
	}

	if err := stepPostInstall(opts, st); err != nil {
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

func runInteractiveInstall(opts *config.InstallOptions, st *config.InstallState) error {
	if !opts.PackagesOnly {
		if err := stepGitConfig(opts, st); err != nil {
			return err
		}
	}

	if err := stepPresetSelection(opts, st); err != nil {
		return err
	}

	if err := stepPackageCustomization(opts, st); err != nil {
		return err
	}

	if err := stepInstallPackages(opts, st); err != nil {
		return err
	}

	var softErrs []error

	if err := stepInstallNpmWithRetry(opts, st); err != nil {
		ui.Error(fmt.Sprintf("npm package installation failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("npm: %w", err))
	}

	if !opts.PackagesOnly {
		if err := stepShell(opts, st); err != nil {
			ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
		}

		if err := stepDotfiles(opts, st); err != nil {
			ui.Error(fmt.Sprintf("Dotfiles setup failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("dotfiles: %w", err))
		}

		if err := stepMacOS(opts, st); err != nil {
			ui.Error(fmt.Sprintf("macOS configuration failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
		}
	}

	showCompletion(opts, st)

	if len(softErrs) > 0 {
		fmt.Println()
		ui.Warn(fmt.Sprintf("%d setup step(s) had errors — run 'openboot doctor' to diagnose.", len(softErrs)))
		return errors.Join(softErrs...)
	}
	return nil
}

func RunFromSnapshot(cfg *config.Config) error {
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()

	fmt.Println()
	ui.Header("OpenBoot — Restore from Snapshot")
	fmt.Println()

	if opts.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	if len(st.SnapshotTaps) > 0 {
		ui.Info(fmt.Sprintf("Adding %d taps...", len(st.SnapshotTaps)))
		fmt.Println()
		if err := brew.InstallTaps(st.SnapshotTaps, opts.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Some taps failed: %v", err))
		}
		fmt.Println()
	}

	if err := stepInstallPackages(opts, st); err != nil {
		cfg.ApplyState(st)
		return err
	}

	if err := stepInstallNpmWithRetry(opts, st); err != nil {
		ui.Error(fmt.Sprintf("npm package installation failed: %v", err))
	}

	var softErrs []error

	if st.SnapshotGit != nil {
		if err := stepRestoreGit(opts, st); err != nil {
			ui.Error(fmt.Sprintf("Git restore failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("git restore: %w", err))
		}
	}

	if err := stepShell(opts, st); err != nil {
		ui.Error(fmt.Sprintf("Shell setup failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
	}

	if err := stepRestoreMacOS(opts, st); err != nil {
		ui.Error(fmt.Sprintf("macOS restore failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
	}

	if st.SnapshotDotfiles != "" {
		if err := stepDotfiles(opts, st); err != nil {
			ui.Error(fmt.Sprintf("Dotfiles restore failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("dotfiles: %w", err))
		}
	}

	showCompletion(opts, st)

	cfg.ApplyState(st)

	if len(softErrs) > 0 {
		fmt.Println()
		ui.Warn(fmt.Sprintf("%d restore step(s) had errors — run 'openboot doctor' to diagnose.", len(softErrs)))
		return errors.Join(softErrs...)
	}
	return nil
}

func runUpdate(opts *config.InstallOptions, st *config.InstallState) error {
	ui.Header("OpenBoot Update")
	fmt.Println()

	if err := brew.Update(opts.DryRun); err != nil {
		return err
	}

	if !opts.DryRun {
		brew.Cleanup()
	}

	fmt.Println()
	ui.Header("Update Complete!")
	return nil
}

func showCompletion(opts *config.InstallOptions, st *config.InstallState) {
	var cliCount, caskCount, npmCount int
	for _, cat := range config.Categories {
		for _, pkg := range cat.Packages {
			if st.SelectedPkgs[pkg.Name] {
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
	for _, pkg := range st.OnlinePkgs {
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
	if !opts.PackagesOnly {
		ui.Info("  - Git configured with your identity")
	}
	ui.Info(fmt.Sprintf("  - %d CLI packages", cliCount))
	ui.Info(fmt.Sprintf("  - %d GUI applications", caskCount))
	if npmCount > 0 {
		ui.Info(fmt.Sprintf("  - %d npm global packages", npmCount))
	}
	fmt.Println()

	showScreenRecordingReminder(opts, st)

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

func findMatchingPackages(opts *config.InstallOptions, st *config.InstallState, triggerPkgs []string) []string {
	triggerSet := make(map[string]bool, len(triggerPkgs))
	for _, p := range triggerPkgs {
		triggerSet[p] = true
	}

	var matched []string
	for pkg := range st.SelectedPkgs {
		if triggerSet[pkg] {
			matched = append(matched, pkg)
		}
	}
	for _, pkg := range st.OnlinePkgs {
		if triggerSet[pkg.Name] {
			matched = append(matched, pkg.Name)
		}
	}
	return matched
}

func showScreenRecordingReminder(opts *config.InstallOptions, st *config.InstallState) {
	if opts.DryRun || opts.Silent {
		return
	}

	statePath := installstate.DefaultStatePath()
	reminderState, err := installstate.LoadState(statePath)
	if err != nil {
		return
	}

	if !installstate.ShouldShowReminder(reminderState) {
		return
	}

	if permissions.HasScreenRecordingPermission() {
		return
	}

	triggerPkgs := config.GetScreenRecordingPackages()
	matchingPkgs := findMatchingPackages(opts, st, triggerPkgs)
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
		installstate.MarkSkipped(reminderState)
		if err := installstate.SaveState(statePath, reminderState); err != nil {
			ui.Warn(fmt.Sprintf("Failed to save install state: %v", err))
		}
		return
	}

	switch choice {
	case "Open System Settings":
		if err := permissions.OpenScreenRecordingSettings(); err != nil {
			ui.Warn("Could not open System Settings")
		}
		installstate.MarkSkipped(reminderState)
	case "Remind me next time":
		installstate.MarkSkipped(reminderState)
	case "Don't remind again":
		installstate.MarkDismissed(reminderState)
	}

	if err := installstate.SaveState(statePath, reminderState); err != nil {
		ui.Warn(fmt.Sprintf("Failed to save install state: %v", err))
	}
	fmt.Println()
}
