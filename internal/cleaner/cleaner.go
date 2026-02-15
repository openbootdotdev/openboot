package cleaner

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// CleanResult holds the diff between current system and desired state.
type CleanResult struct {
	ExtraFormulae []string
	ExtraCasks    []string
	ExtraNpm      []string
}

func (r *CleanResult) TotalExtra() int {
	return len(r.ExtraFormulae) + len(r.ExtraCasks) + len(r.ExtraNpm)
}

func DiffFromSnapshot(snap *snapshot.Snapshot) (*CleanResult, error) {
	desiredFormulae := toSet(snap.Packages.Formulae)
	desiredCasks := toSet(snap.Packages.Casks)
	desiredNpm := toSet(snap.Packages.Npm)

	return diff(desiredFormulae, desiredCasks, desiredNpm)
}

func DiffFromLists(formulae, casks, npmPkgs []string) (*CleanResult, error) {
	return diff(toSet(formulae), toSet(casks), toSet(npmPkgs))
}

func diff(desiredFormulae, desiredCasks, desiredNpm map[string]bool) (*CleanResult, error) {
	result := &CleanResult{}

	// Get currently installed brew packages
	installedFormulae, installedCasks, err := brew.GetInstalledPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to get installed brew packages: %w", err)
	}

	// Find extra formulae
	for pkg := range installedFormulae {
		if !desiredFormulae[pkg] {
			result.ExtraFormulae = append(result.ExtraFormulae, pkg)
		}
	}

	// Find extra casks
	for pkg := range installedCasks {
		if !desiredCasks[pkg] {
			result.ExtraCasks = append(result.ExtraCasks, pkg)
		}
	}

	// Find extra npm packages
	if npm.IsAvailable() {
		installedNpm, err := npm.GetInstalledPackages()
		if err != nil {
			ui.Warn(fmt.Sprintf("Failed to check npm packages: %v", err))
		} else {
			for pkg := range installedNpm {
				if !desiredNpm[pkg] {
					result.ExtraNpm = append(result.ExtraNpm, pkg)
				}
			}
		}
	}

	return result, nil
}

// Execute removes the extra packages identified in a CleanResult.
func Execute(result *CleanResult, dryRun bool) error {
	var errs []error

	if len(result.ExtraFormulae) > 0 {
		fmt.Println()
		ui.Header("Removing extra formulae")
		fmt.Println()
		if err := brew.Uninstall(result.ExtraFormulae, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("formulae cleanup: %w", err))
		}
	}

	if len(result.ExtraCasks) > 0 {
		fmt.Println()
		ui.Header("Removing extra casks")
		fmt.Println()
		if err := brew.UninstallCask(result.ExtraCasks, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("cask cleanup: %w", err))
		}
	}

	if len(result.ExtraNpm) > 0 {
		fmt.Println()
		ui.Header("Removing extra npm packages")
		fmt.Println()
		if err := npm.Uninstall(result.ExtraNpm, dryRun); err != nil {
			errs = append(errs, fmt.Errorf("npm cleanup: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d cleanup steps had failures", len(errs))
	}
	return nil
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
