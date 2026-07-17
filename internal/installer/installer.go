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

// ApplyReviewedPlan applies a plan the install wizard already resolved and
// the user already reviewed. It is runInstallContext's tail — banner, apply,
// completion — for the case where planning happened in the TUI instead.
//
// It runs on the normal terminal, by design: the wizard exits before this is
// called, so package results scroll into the user's scrollback and the
// completion summary stays on screen afterwards. Applying inside the
// alt-screen instead meant the whole run vanished on exit, and plan.Silent had
// to be forced on to keep prompts (npm retry, screen-recording) from painting
// over the TUI. Here the plan's own Silent value stands and those prompts work
// where they belong.
func ApplyReviewedPlan(ctx context.Context, plan InstallPlan) error {
	slog.Info("install_started",
		"version", plan.Version,
		"dry_run", plan.DryRun,
		"silent", plan.Silent,
	)
	ui.Println()
	ui.Header(fmt.Sprintf("OpenBoot Installer v%s", plan.Version))
	ui.Println()
	return ApplyContext(ctx, plan, ConsoleReporter{})
}

// Apply executes a resolved InstallPlan, reporting progress via r.
// All user interaction has already happened in Plan(); this function only performs actions.
func Apply(plan InstallPlan, r Reporter) error {
	return ApplyContext(context.Background(), plan, r)
}

func ApplyContext(ctx context.Context, plan InstallPlan, r Reporter) error {
	steps := plannedSteps(plan)
	var softErrs []error

	for i, s := range steps {
		// ctrl+c cancels ctx. brew/npm honour it (exec.CommandContext), but the
		// config steps take no ctx, so without this gate an aborted install would
		// keep symlinking dotfiles and rewriting macOS defaults after the user
		// asked to stop. Bail between steps so an abort skips not-yet-started work.
		if err := ctx.Err(); err != nil {
			return abortWith(softErrs, err)
		}

		r.Header(sectionTitle(i, len(steps), s.name))

		err := s.run(ctx, plan, r)
		if err == nil {
			continue
		}
		// Git is the one hard failure: everything after it authors commits or
		// writes config on its behalf, so a broken identity is not something to
		// carry on past. The rest are soft — a failed cask shouldn't cost you
		// your dotfiles.
		if s.name == "Git identity" {
			return fmt.Errorf("apply git config: %w", err)
		}
		// Report here, once. Steps must not also announce their own failure:
		// when both did, the same error printed twice in a row under its own
		// section, which reads like two separate things went wrong.
		r.Error(fmt.Sprintf("%s failed: %v", s.name, err))
		softErrs = append(softErrs, fmt.Errorf("%s: %w", strings.ToLower(s.name), err))
	}

	showCompletionFromPlan(plan, r, len(softErrs))

	if len(softErrs) > 0 {
		slog.Info("install_completed", "soft_errors", len(softErrs))
		return errors.Join(softErrs...)
	}
	slog.Info("install_completed", "soft_errors", 0)
	return nil
}

// applyStep is one titled section of the apply.
//
// The section title used to be printed by the step function itself, with a
// number written into the string: "Step 1: Git Configuration", "Step 4:
// Installation", "Step 6: Dotfiles". Steps are conditional, so those numbers
// could only ever be wrong — a real run printed 1, 4, 6, 7, with two
// unnumbered sections wedged between. The numbering has to be derived from the
// plan, because only the plan knows which steps exist.
//
// cond therefore mirrors each step function's own entry guard: a step that
// won't run must not consume a number. The step functions keep their guards —
// they're the ones that must not act — and this list keeps the titles honest.
type applyStep struct {
	name string
	cond bool
	run  func(context.Context, InstallPlan, Reporter) error
}

// noCtx adapts a step that takes no context to the uniform signature.
func noCtx(f func(InstallPlan, Reporter) error) func(context.Context, InstallPlan, Reporter) error {
	return func(_ context.Context, p InstallPlan, r Reporter) error { return f(p, r) }
}

// plannedSteps returns, in execution order, the sections this plan will run.
func plannedSteps(plan InstallPlan) []applyStep {
	sys := !plan.PackagesOnly
	all := []applyStep{
		{"Git identity", sys && !plan.SkipGit, noCtx(applyGitConfig)},
		{"Packages", len(plan.Formulae)+len(plan.Casks)+len(plan.Taps) > 0, applyPackages},
		{"npm globals", len(plan.Npm) > 0, applyNpm},
		{"Shell", sys && plan.InstallOhMyZsh, noCtx(applyShell)},
		{"Dotfiles", sys && plan.DotfilesURL != "", noCtx(applyDotfiles)},
		{"macOS preferences", sys && (len(plan.MacOSPrefs) > 0 || plan.DockApps != nil || plan.LoginItems != nil), noCtx(applyMacOSPrefs)},
		{"Post-install script", sys && len(plan.PostInstall) > 0, noCtx(applyPostInstall)},
	}
	out := make([]applyStep, 0, len(all))
	for _, s := range all {
		if s.cond {
			out = append(out, s)
		}
	}
	return out
}

// sectionTitle renders the header for step i of n.
func sectionTitle(i, n int, name string) string {
	return fmt.Sprintf("[%d/%d] %s", i+1, n, name)
}

// abortWith joins the soft errors accumulated so far with the context
// cancellation cause, for when ApplyContext bails out early on a cancelled
// context (ctrl+c). Returning here stops the remaining steps so an aborted
// install doesn't keep mutating the system.
func abortWith(softErrs []error, cause error) error {
	return errors.Join(append(softErrs, cause)...)
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
