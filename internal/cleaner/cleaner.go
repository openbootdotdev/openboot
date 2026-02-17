package cleaner

import (
	"errors"
	"fmt"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
)

type CleanResult struct {
	ExtraFormulae []string
	ExtraCasks    []string
	ExtraNpm      []string

	RemovedFormulae []string
	RemovedCasks    []string
	RemovedNpm      []string

	FailedFormulae []string
	FailedCasks    []string
	FailedNpm      []string
}

func (r *CleanResult) TotalExtra() int {
	return len(r.ExtraFormulae) + len(r.ExtraCasks) + len(r.ExtraNpm)
}

func (r *CleanResult) TotalRemoved() int {
	return len(r.RemovedFormulae) + len(r.RemovedCasks) + len(r.RemovedNpm)
}

func (r *CleanResult) TotalFailed() int {
	return len(r.FailedFormulae) + len(r.FailedCasks) + len(r.FailedNpm)
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

	installedFormulae, installedCasks, err := brew.GetInstalledPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to get installed brew packages: %w", err)
	}

	for pkg := range installedFormulae {
		if !desiredFormulae[pkg] {
			result.ExtraFormulae = append(result.ExtraFormulae, pkg)
		}
	}

	for pkg := range installedCasks {
		if !desiredCasks[pkg] {
			result.ExtraCasks = append(result.ExtraCasks, pkg)
		}
	}

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

type uninstallOneFn func(pkg string, dryRun bool) error

type uninstallOp struct {
	label        string
	pkgs         []string
	uninstallOne uninstallOneFn
	removed      *[]string
	failed       *[]string
}

func Execute(result *CleanResult, dryRun bool) error {
	ops := []uninstallOp{
		{
			label:        "Removing extra formulae",
			pkgs:         result.ExtraFormulae,
			uninstallOne: func(pkg string, dry bool) error { return brew.Uninstall([]string{pkg}, dry) },
			removed:      &result.RemovedFormulae,
			failed:       &result.FailedFormulae,
		},
		{
			label:        "Removing extra casks",
			pkgs:         result.ExtraCasks,
			uninstallOne: func(pkg string, dry bool) error { return brew.UninstallCask([]string{pkg}, dry) },
			removed:      &result.RemovedCasks,
			failed:       &result.FailedCasks,
		},
		{
			label:        "Removing extra npm packages",
			pkgs:         result.ExtraNpm,
			uninstallOne: func(pkg string, dry bool) error { return npm.Uninstall([]string{pkg}, dry) },
			removed:      &result.RemovedNpm,
			failed:       &result.FailedNpm,
		},
	}

	var errs []error
	for _, op := range ops {
		if len(op.pkgs) == 0 {
			continue
		}
		fmt.Println()
		ui.Header(op.label)
		fmt.Println()
		for _, pkg := range op.pkgs {
			if err := op.uninstallOne(pkg, dryRun); err != nil {
				*op.failed = append(*op.failed, pkg)
				errs = append(errs, fmt.Errorf("%s: %w", pkg, err))
			} else {
				*op.removed = append(*op.removed, pkg)
			}
		}
	}

	return errors.Join(errs...)
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
