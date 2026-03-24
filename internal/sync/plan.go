package sync

import (
	"errors"
	"fmt"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/npm"
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
	return n
}

// IsEmpty returns true if no actions are planned.
func (p *SyncPlan) IsEmpty() bool {
	return p.TotalActions() == 0
}

// Execute applies all planned changes. Errors are collected rather than
// stopping on the first failure, matching the pattern in cleaner.Execute.
func Execute(plan *SyncPlan, dryRun bool) (*SyncResult, error) {
	result := &SyncResult{}
	var errs []error

	// Install taps first (other packages may depend on them)
	if len(plan.InstallTaps) > 0 {
		if err := brew.InstallTaps(plan.InstallTaps, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("install taps: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("taps: %v", err))
		} else {
			result.Installed += len(plan.InstallTaps)
		}
	}

	// Install formulae
	if len(plan.InstallFormulae) > 0 {
		if err := brew.Install(plan.InstallFormulae, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("install formulae: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("formulae: %v", err))
		} else {
			result.Installed += len(plan.InstallFormulae)
		}
	}

	// Install casks
	if len(plan.InstallCasks) > 0 {
		if err := brew.InstallCask(plan.InstallCasks, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("install casks: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("casks: %v", err))
		} else {
			result.Installed += len(plan.InstallCasks)
		}
	}

	// Install npm
	if len(plan.InstallNpm) > 0 {
		if err := npm.Install(plan.InstallNpm, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("install npm: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("npm: %v", err))
		} else {
			result.Installed += len(plan.InstallNpm)
		}
	}

	// Uninstall taps
	if len(plan.UninstallTaps) > 0 {
		if err := brew.Untap(plan.UninstallTaps, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("untap: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("untap: %v", err))
		} else {
			result.Uninstalled += len(plan.UninstallTaps)
		}
	}

	// Uninstall formulae
	if len(plan.UninstallFormulae) > 0 {
		if err := brew.Uninstall(plan.UninstallFormulae, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("uninstall formulae: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("uninstall formulae: %v", err))
		} else {
			result.Uninstalled += len(plan.UninstallFormulae)
		}
	}

	// Uninstall casks
	if len(plan.UninstallCasks) > 0 {
		if err := brew.UninstallCask(plan.UninstallCasks, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("uninstall casks: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("uninstall casks: %v", err))
		} else {
			result.Uninstalled += len(plan.UninstallCasks)
		}
	}

	// Uninstall npm
	if len(plan.UninstallNpm) > 0 {
		if err := npm.Uninstall(plan.UninstallNpm, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("uninstall npm: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("uninstall npm: %v", err))
		} else {
			result.Uninstalled += len(plan.UninstallNpm)
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

	// Apply macOS preferences
	if len(plan.UpdateMacOSPrefs) > 0 {
		prefs := make([]macos.Preference, len(plan.UpdateMacOSPrefs))
		for i, rp := range plan.UpdateMacOSPrefs {
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
			}
		}
		if err := macos.Configure(prefs, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("update macos prefs: %w", err))
			result.Errors = append(result.Errors, fmt.Sprintf("macos: %v", err))
		} else {
			result.Updated += len(plan.UpdateMacOSPrefs)
		}
	}

	return result, errors.Join(errs...)
}
