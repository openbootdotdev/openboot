package installer

import (
	"errors"
	"fmt"
	"log/slog"
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
	slog.Info("install_started",
		"version", opts.Version,
		"preset", opts.Preset,
		"user", opts.User,
		"dry_run", opts.DryRun,
		"silent", opts.Silent,
	)
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

	plan, err := Plan(opts, st)
	if err != nil {
		return err
	}

	// Write resolved selections back to st so callers that hold a reference to
	// st (e.g. Run → cfg.ApplyState) can observe the final selected packages.
	if plan.SelectedPkgs != nil {
		st.SelectedPkgs = plan.SelectedPkgs
	}
	if plan.OnlinePkgs != nil {
		st.OnlinePkgs = plan.OnlinePkgs
	}

	// Remote-config installs: show what will be installed and confirm before proceeding.
	if plan.RemoteConfig != nil && !opts.Silent && !opts.DryRun {
		ui.Info(fmt.Sprintf("Custom config: @%s/%s", plan.RemoteConfig.Username, plan.RemoteConfig.Slug))
		fmt.Println()
		printPackageList("CLI tools", plan.RemoteConfig.Packages)
		printPackageList("Apps", plan.RemoteConfig.Casks)
		printPackageList("npm", plan.RemoteConfig.Npm)
		fmt.Println()
		proceed, err := ui.Confirm("Install these packages?", true)
		if err != nil {
			return err
		}
		if !proceed {
			return ErrUserCancelled
		}
		fmt.Println()
	}

	return Apply(plan, ConsoleReporter{})
}

// Apply executes a resolved InstallPlan, reporting progress via r.
// All user interaction has already happened in Plan(); this function only performs actions.
func Apply(plan InstallPlan, r Reporter) error {
	if !plan.PackagesOnly && !plan.SkipGit {
		if err := applyGitConfig(plan, r); err != nil {
			return err
		}
	}

	if err := applyPackages(plan, r); err != nil {
		return err
	}

	var softErrs []error

	if err := applyNpm(plan, r); err != nil {
		r.Error(fmt.Sprintf("npm package installation failed: %v", err))
		softErrs = append(softErrs, fmt.Errorf("npm: %w", err))
	}

	if !plan.PackagesOnly {
		if err := applyShell(plan, r); err != nil {
			r.Error(fmt.Sprintf("Shell setup failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("shell: %w", err))
		}
		if err := applyDotfiles(plan, r); err != nil {
			r.Error(fmt.Sprintf("Dotfiles setup failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("dotfiles: %w", err))
		}
		if err := applyMacOSPrefs(plan, r); err != nil {
			r.Error(fmt.Sprintf("macOS configuration failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("macos: %w", err))
		}
		if err := applyPostInstall(plan, r); err != nil {
			r.Error(fmt.Sprintf("Post-install script failed: %v", err))
			softErrs = append(softErrs, fmt.Errorf("post-install: %w", err))
		}
	}

	showCompletionFromPlan(plan, r, len(softErrs))

	if len(softErrs) > 0 {
		slog.Info("install_completed", "soft_errors", len(softErrs))
		return errors.Join(softErrs...)
	}
	slog.Info("install_completed", "soft_errors", 0)
	return nil
}

func showCompletionFromPlan(plan InstallPlan, r Reporter, errCount int) {
	fmt.Println()
	if errCount > 0 {
		r.Header("Installation finished with errors")
	} else {
		r.Header("Installation Complete!")
	}
	fmt.Println()
	if errCount > 0 {
		r.Warn(fmt.Sprintf("%d step(s) had errors — check the output above for details.", errCount))
	} else {
		r.Success("OpenBoot has successfully configured your Mac.")
	}
	fmt.Println()

	r.Info("What was installed:")
	if !plan.PackagesOnly {
		r.Info("  - Git configured with your identity")
	}
	r.Info(fmt.Sprintf("  - %d CLI packages", len(plan.Formulae)))
	r.Info(fmt.Sprintf("  - %d GUI applications", len(plan.Casks)))
	if len(plan.Npm) > 0 {
		r.Info(fmt.Sprintf("  - %d npm global packages", len(plan.Npm)))
	}
	fmt.Println()

	showScreenRecordingReminderFromPlan(plan)

	r.Info("Next steps:")
	r.Info("  - Restart your terminal to apply changes")
	r.Info("  - Run 'brew doctor' to verify Homebrew health")
	fmt.Println()
}

func showScreenRecordingReminderFromPlan(plan InstallPlan) {
	if plan.DryRun || plan.Silent {
		return
	}
	// reuse existing showScreenRecordingReminder logic by constructing opts/st
	opts := &config.InstallOptions{DryRun: plan.DryRun, Silent: plan.Silent}
	st := &config.InstallState{SelectedPkgs: plan.SelectedPkgs, OnlinePkgs: plan.OnlinePkgs}
	showScreenRecordingReminder(opts, st)
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

	plan := PlanFromSnapshot(opts, st)
	err := Apply(plan, ConsoleReporter{})
	cfg.ApplyState(st)
	return err
}

func runUpdate(opts *config.InstallOptions, st *config.InstallState) error {
	ui.Header("OpenBoot Update")
	fmt.Println()

	if err := brew.Update(opts.DryRun); err != nil {
		return err
	}

	if !opts.DryRun {
		brew.Cleanup() //nolint:errcheck,gosec // best-effort cleanup; failure is non-critical
	}

	fmt.Println()
	ui.Header("Update Complete!")
	return nil
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
