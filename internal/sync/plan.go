package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/shell"
)

// SyncPlan describes the concrete actions to apply after the user selects
// which diff items to act on.
type SyncPlan struct {
	// Packages to install
	InstallFormulae []string
	InstallCasks    []string
	InstallNpm      []string
	InstallTaps     []string

	// Packages to uninstall
	UninstallFormulae []string
	UninstallCasks    []string
	UninstallNpm      []string
	UninstallTaps     []string

	// Dotfiles
	UpdateDotfiles string // new repo URL (empty = no change)

	// macOS
	UpdateMacOSPrefs []config.RemoteMacOSPref

	// Shell
	UpdateShell  bool
	ShellOhMyZsh bool
	ShellTheme   string
	ShellPlugins []string
}

// SyncResult summarizes what was applied.
type SyncResult struct {
	Installed   int
	Uninstalled int
	Updated     int
	Errors      []string
}

// TotalActions returns the number of planned actions.
func (p *SyncPlan) TotalActions() int {
	n := len(p.InstallFormulae) + len(p.InstallCasks) + len(p.InstallNpm) + len(p.InstallTaps) +
		len(p.UninstallFormulae) + len(p.UninstallCasks) + len(p.UninstallNpm) + len(p.UninstallTaps) +
		len(p.UpdateMacOSPrefs)
	if p.UpdateDotfiles != "" {
		n++
	}
	if p.UpdateShell {
		n++
	}
	return n
}

// IsEmpty returns true if no actions are planned.
func (p *SyncPlan) IsEmpty() bool {
	return p.TotalActions() == 0
}

// stepResult tracks success/failure of a single sync step.
type stepResult struct {
	count int
	err   error
	label string
}

// executeSyncStep runs fn when items is non-empty, collecting the result.
func executeSyncStep(items []string, label string, fn func() error) stepResult {
	if len(items) == 0 {
		return stepResult{}
	}
	if err := fn(); err != nil {
		return stepResult{label: label, err: err}
	}
	return stepResult{count: len(items)}
}

// Execute applies all planned changes. Errors are collected rather than
// stopping on the first failure so the caller gets a full summary of what ran.
func Execute(plan *SyncPlan, dryRun bool) (*SyncResult, error) {
	result := &SyncResult{}
	var errs []error

	installSteps := []stepResult{
		// Install taps first (other packages may depend on them)
		executeSyncStep(plan.InstallTaps, "taps", func() error {
			return brew.InstallTaps(plan.InstallTaps, dryRun)
		}),
	}
	// Run formulae+casks through one shared StickyProgress bar — same flow
	// the wizard path uses. Skipping this and calling brew.Install /
	// brew.InstallCask separately would lose the byte-level progress bar.
	if len(plan.InstallFormulae) > 0 || len(plan.InstallCasks) > 0 {
		_, _, err := brew.InstallWithProgress(context.Background(), plan.InstallFormulae, plan.InstallCasks, dryRun)
		step := stepResult{label: "brew", err: err}
		if err == nil {
			step.count = len(plan.InstallFormulae) + len(plan.InstallCasks)
		}
		installSteps = append(installSteps, step)
	}
	installSteps = append(installSteps,
		executeSyncStep(plan.InstallNpm, "npm", func() error {
			return npm.Install(plan.InstallNpm, dryRun)
		}),
	)
	for _, s := range installSteps {
		if s.err != nil {
			errs = append(errs, fmt.Errorf("install %s: %w", s.label, s.err))
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", s.label, s.err))
		} else {
			result.Installed += s.count
		}
	}

	uninstallSteps := []stepResult{
		executeSyncStep(plan.UninstallTaps, "untap", func() error {
			return brew.Untap(plan.UninstallTaps, dryRun)
		}),
		executeSyncStep(plan.UninstallFormulae, "uninstall formulae", func() error {
			return brew.Uninstall(plan.UninstallFormulae, dryRun)
		}),
		executeSyncStep(plan.UninstallCasks, "uninstall casks", func() error {
			return brew.UninstallCask(plan.UninstallCasks, dryRun)
		}),
		executeSyncStep(plan.UninstallNpm, "uninstall npm", func() error {
			return npm.Uninstall(plan.UninstallNpm, dryRun)
		}),
	}
	for _, s := range uninstallSteps {
		if s.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.label, s.err))
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", s.label, s.err))
		} else {
			result.Uninstalled += s.count
		}
	}

	// Update dotfiles
	if plan.UpdateDotfiles != "" {
		if err := dotfiles.Clone(plan.UpdateDotfiles, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("update dotfiles: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("dotfiles: %v", err))
		} else if err := dotfiles.Link(dryRun); err != nil {
			errs = append(errs, fmt.Errorf("link dotfiles: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("dotfiles link: %v", err))
		} else {
			result.Updated++
		}
	}

	// Update shell config
	if plan.UpdateShell {
		if err := shell.RestoreFromSnapshot(plan.ShellOhMyZsh, plan.ShellTheme, plan.ShellPlugins, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("update shell: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("shell: %v", err))
		} else {
			result.Updated++
		}
	}

	// Apply macOS preferences
	if len(plan.UpdateMacOSPrefs) > 0 {
		if err := applyMacOSPrefs(plan.UpdateMacOSPrefs, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("update macos prefs: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("macos: %v", err))
		} else {
			result.Updated += len(plan.UpdateMacOSPrefs)
		}
	}

	return result, errors.Join(errs...)
}

// applyMacOSPrefs converts remote pref descriptors to macos.Preference values
// and applies them via macos.Configure.
func applyMacOSPrefs(remotePrefs []config.RemoteMacOSPref, dryRun bool) error {
	prefs := make([]macos.Preference, len(remotePrefs))
	for i, rp := range remotePrefs {
		prefType := rp.Type
		if prefType == "" {
			prefType = macos.InferPreferenceType(rp.Value)
		}
		prefs[i] = macos.Preference{
			Domain: rp.Domain,
			Key:    rp.Key,
			Type:   prefType,
			Value:  rp.Value,
			Desc:   rp.Desc,
			Host:   rp.Host,
		}
	}
	return macos.Configure(prefs, dryRun)
}
