package installer

import (
	"context"
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
	return RunContext(context.Background(), cfg)
}

func RunContext(ctx context.Context, cfg *config.Config) error {
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()

	var err error
	if opts.Update {
		err = runUpdate(opts, st)
	} else {
		err = runInstallContext(ctx, opts, st)
	}

	cfg.ApplyState(st)
	return err // already wrapped by runInstall/runUpdate
}

func runInstall(opts *config.InstallOptions, st *config.InstallState) error {
	return runInstallContext(context.Background(), opts, st)
}

func runInstallContext(ctx context.Context, opts *config.InstallOptions, st *config.InstallState) error {
	slog.Info("install_started",
		"version", opts.Version,
		"preset", opts.Preset,
		"user", opts.User,
		"dry_run", opts.DryRun,
		"silent", opts.Silent,
	)
	ui.Println()
	ui.Header(fmt.Sprintf("OpenBoot Installer v%s", opts.Version))
	ui.Println()

	if opts.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		ui.Println()
	}

	if err := checkDependencies(opts, st); err != nil {
		return fmt.Errorf("check dependencies: %w", err)
	}

	plan, err := Plan(opts, st)
	if err != nil {
		return fmt.Errorf("plan install: %w", err)
	}

	// Write resolved selections back to st so callers that hold a reference to
	// st (e.g. Run → cfg.ApplyState) can observe the final selected packages.
	if plan.SelectedPkgs != nil {
		st.SelectedPkgs = plan.SelectedPkgs
	}
	if plan.OnlinePkgs != nil {
		st.OnlinePkgs = plan.OnlinePkgs
	}

	return ApplyContext(ctx, plan, ConsoleReporter{})
}

// PlanForConfig runs the pre-flight dependency check and resolves the install
// plan for cfg — the linear RunContext flow up to (but not including) Apply. It
// exists so the wizard pipeline can do the interactive prep on a normal
// terminal, then stream the apply itself via ApplyContext with its own Reporter.
func PlanForConfig(cfg *config.Config) (InstallPlan, error) {
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	if err := checkDependencies(opts, st); err != nil {
		return InstallPlan{}, fmt.Errorf("check dependencies: %w", err)
	}
	plan, err := Plan(opts, st)
	if err != nil {
		return InstallPlan{}, fmt.Errorf("plan install: %w", err)
	}
	return plan, nil
}

// Apply executes a resolved InstallPlan, reporting progress via r.
// All user interaction has already happened in Plan(); this function only performs actions.
func Apply(plan InstallPlan, r Reporter) error {
	return ApplyContext(context.Background(), plan, r)
}

func ApplyContext(ctx context.Context, plan InstallPlan, r Reporter) error {
	if !plan.PackagesOnly && !plan.SkipGit {
		if err := applyGitConfig(plan, r); err != nil {
			return fmt.Errorf("apply git config: %w", err)
		}
	}

	var softErrs []error

	if err := applyPackages(ctx, plan, r); err != nil {
		softErrs = append(softErrs, fmt.Errorf("brew: %w", err))
	}

	if err := applyNpm(ctx, plan, r); err != nil {
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
	ui.Println()
	if errCount > 0 {
		r.Header("Installation finished with errors")
	} else {
		r.Header("Installation Complete!")
	}
	ui.Println()
	if errCount > 0 {
		r.Warn(fmt.Sprintf("%d step(s) had errors — check the output above for details.", errCount))
	} else {
		r.Success("OpenBoot has successfully configured your Mac.")
	}
	ui.Println()

	r.Info("What was installed:")
	if !plan.PackagesOnly {
		r.Info("  - Git configured with your identity")
	}
	r.Info(fmt.Sprintf("  - %d CLI packages", len(plan.Formulae)))
	r.Info(fmt.Sprintf("  - %d GUI applications", len(plan.Casks)))
	if len(plan.Npm) > 0 {
		r.Info(fmt.Sprintf("  - %d npm global packages", len(plan.Npm)))
	}
	ui.Println()

	showScreenRecordingReminderFromPlan(plan)

	r.Info("Next steps:")
	r.Info("  - Restart your terminal to apply changes")
	r.Info("  - Run 'brew doctor' to verify Homebrew health")
	ui.Println()
}

// ShowScreenRecordingReminderAfterTUI re-runs the screen-recording permission
// reminder for a plan applied by the full-screen wizard. The wizard forces
// plan.Silent=true to keep prompts out of the alt-screen, which also
// suppresses this reminder; the CLI calls this after the TUI exits, back on a
// normal terminal.
func ShowScreenRecordingReminderAfterTUI(plan InstallPlan) {
	plan.Silent = false
	showScreenRecordingReminderFromPlan(plan)
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
		ui.Println()
	}

	gitName, gitEmail := system.GetExistingGitConfig()
	if gitName == "" || gitEmail == "" {
		if !opts.PackagesOnly {
			hasIssues = true
			ui.Warn("Git user information is not configured")
			ui.Info("You can set it up via dotfiles or manually after installation")
			ui.Println()
		}
	}

	if hasIssues && !opts.Silent {
		cont, err := ui.Confirm("Continue with installation?", true)
		if err != nil {
			return fmt.Errorf("confirm continue: %w", err)
		}
		if !cont {
			return fmt.Errorf("installation cancelled")
		}
		ui.Println()
	}

	return nil
}

func RunFromSnapshot(cfg *config.Config) error {
	return RunFromSnapshotContext(context.Background(), cfg)
}

func RunFromSnapshotContext(ctx context.Context, cfg *config.Config) error {
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()

	ui.Println()
	ui.Header("OpenBoot — Restore from Snapshot")
	ui.Println()

	if opts.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		ui.Println()
	}

	plan := PlanFromSnapshot(opts, st)
	err := ApplyContext(ctx, plan, ConsoleReporter{})
	cfg.ApplyState(st)
	return err
}

func runUpdate(opts *config.InstallOptions, st *config.InstallState) error {
	ui.Header("OpenBoot Update")
	ui.Println()

	if err := brew.Update(opts.DryRun); err != nil {
		return fmt.Errorf("brew update: %w", err)
	}

	if !opts.DryRun {
		brew.Cleanup() //nolint:errcheck,gosec // best-effort cleanup; failure is non-critical
	}

	ui.Println()
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

	ui.Println()
	ui.Header("Screen Recording Permission")
	ui.Println()
	ui.Info(fmt.Sprintf("You installed: %s", strings.Join(matchingPkgs, ", ")))
	ui.Info("These apps need Screen Recording permission for screen sharing.")
	ui.Println()

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
	ui.Println()
}
