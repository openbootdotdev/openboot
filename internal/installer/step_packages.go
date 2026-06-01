package installer

import (
	"context"
	"fmt"
	"time"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

type categorizedPackages struct {
	cli  []string
	cask []string
	npm  []string
}

const (
	brewInstallBaseTimeout       = 30 * time.Minute
	brewInstallTimeoutPerPackage = 20 * time.Minute
	brewInstallMaxTimeout        = 6 * time.Hour
	npmInstallBaseTimeout        = 10 * time.Minute
	npmInstallTimeoutPerPackage  = 10 * time.Minute
	npmInstallMaxTimeout         = 2 * time.Hour
)

func categorizeSelectedPackages(opts *config.InstallOptions, st *config.InstallState) categorizedPackages {
	var result categorizedPackages

	if st.RemoteConfig != nil {
		caskSet := make(map[string]bool)
		for _, c := range st.RemoteConfig.Casks {
			caskSet[c.Name] = true
		}
		npmSet := make(map[string]bool)
		for _, n := range st.RemoteConfig.Npm {
			npmSet[n.Name] = true
		}
		for pkg := range st.SelectedPkgs {
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
	for _, cat := range config.GetCategories() {
		for _, pkg := range cat.Packages {
			if st.SelectedPkgs[pkg.Name] {
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
	for _, pkg := range st.OnlinePkgs {
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

func applyPackages(ctx context.Context, plan InstallPlan, r Reporter) error { //nolint:gocyclo // orchestrates multiple package categories; splitting would obscure the install sequence
	r.Header("Step 4: Installation")
	ui.Println()

	if len(plan.Taps) > 0 {
		if err := brew.InstallTaps(plan.Taps, plan.DryRun); err != nil {
			r.Warn(fmt.Sprintf("Some taps failed: %v", err))
		}
		ui.Println()
	}

	cliPkgs := plan.Formulae
	caskPkgs := plan.Casks
	total := len(cliPkgs) + len(caskPkgs)
	if total == 0 {
		r.Muted("No packages selected")
		return nil
	}

	state, stateErr := loadState()
	if stateErr != nil {
		r.Warn(fmt.Sprintf("Failed to load install state: %v", stateErr))
	}

	var newCli, newCask []string
	if !plan.DryRun {
		actualFormulae, actualCasks, checkErr := brew.GetInstalledPackages()
		if checkErr != nil {
			r.Warn(fmt.Sprintf("Failed to check installed packages: %v", checkErr))
		} else {
			removed := state.reconcileBrewWithSystem(actualFormulae, actualCasks)
			if removed > 0 {
				if err := state.save(); err != nil {
					r.Warn(fmt.Sprintf("Failed to update install state: %v", err))
				}
			}
		}

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
			r.Muted(fmt.Sprintf("Skipping %d packages from previous install", stateSkipped))
		}
		cliPkgs = newCli
		caskPkgs = newCask
	}

	if len(cliPkgs)+len(caskPkgs) == 0 {
		r.Success("All packages already installed!")
		ui.Println()
		return nil
	}

	r.Info(fmt.Sprintf("Installing %d packages (%d CLI, %d GUI)...", len(cliPkgs)+len(caskPkgs), len(cliPkgs), len(caskPkgs)))
	ui.Println()

	brewCtx, cancel := packageInstallContext(ctx, len(cliPkgs)+len(caskPkgs))
	defer cancel()
	installedCli, installedCask, brewErr := brew.InstallWithProgress(brewCtx, cliPkgs, caskPkgs, plan.DryRun)
	if brewErr != nil {
		r.Error(fmt.Sprintf("Some packages failed: %v", brewErr))
	}

	if !plan.DryRun {
		for _, pkg := range installedCli {
			if err := state.markFormula(pkg); err != nil {
				r.Warn(fmt.Sprintf("Failed to track installed package %s: %v", pkg, err))
			}
		}
		for _, pkg := range installedCask {
			if err := state.markCask(pkg); err != nil {
				r.Warn(fmt.Sprintf("Failed to track installed package %s: %v", pkg, err))
			}
		}
		if brewErr == nil {
			r.Success("Package installation complete")
		}
	}
	ui.Println()
	return brewErr
}

func packageInstallContext(parent context.Context, totalPackages int) (context.Context, context.CancelFunc) {
	timeout := brewInstallBaseTimeout + time.Duration(totalPackages)*brewInstallTimeoutPerPackage
	if timeout > brewInstallMaxTimeout {
		timeout = brewInstallMaxTimeout
	}
	return context.WithTimeout(parent, timeout)
}

func applyNpm(ctx context.Context, plan InstallPlan, r Reporter) error { //nolint:gocyclo // handles npm batch + sequential fallback with per-package error tracking
	npmPkgs := plan.Npm
	if len(npmPkgs) == 0 {
		return nil
	}

	state, stateErr := loadState()
	if stateErr != nil {
		r.Warn(fmt.Sprintf("Failed to load install state: %v", stateErr))
	}

	var newNpm []string
	if !plan.DryRun {
		actualNpm, npmCheckErr := npm.GetInstalledPackagesContext(ctx)
		if npmCheckErr != nil {
			r.Warn(fmt.Sprintf("Failed to check installed npm packages: %v", npmCheckErr))
		} else {
			removed := state.reconcileNpmWithSystem(actualNpm)
			if removed > 0 {
				if err := state.save(); err != nil {
					r.Warn(fmt.Sprintf("Failed to update install state: %v", err))
				}
			}
		}

		for _, pkg := range npmPkgs {
			if !state.isNpmInstalled(pkg) {
				newNpm = append(newNpm, pkg)
			}
		}
		stateSkipped := len(npmPkgs) - len(newNpm)
		if stateSkipped > 0 {
			r.Muted(fmt.Sprintf("Skipping %d npm packages from previous install", stateSkipped))
		}
		npmPkgs = newNpm
	}

	if len(npmPkgs) == 0 {
		r.Success("All npm packages already installed!")
		return nil
	}

	ui.Println()
	r.Header("NPM Global Packages")
	ui.Println()
	r.Info(fmt.Sprintf("Installing %d npm packages...", len(npmPkgs)))
	ui.Println()

	maxAttempts := 3
	var lastErr error
	npmCtx, cancel := npmInstallContext(ctx, len(npmPkgs))
	defer cancel()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = npm.InstallContext(npmCtx, npmPkgs, plan.DryRun)
		if lastErr == nil {
			break
		}
		if attempt == maxAttempts {
			r.Error(fmt.Sprintf("npm package installation failed after %d attempts: %v", maxAttempts, lastErr))
			return fmt.Errorf("npm installation failed after %d attempts: %w", maxAttempts, lastErr)
		}
		if plan.Silent || !system.HasTTY() {
			r.Warn(fmt.Sprintf("npm installation failed (attempt %d/%d), retrying...", attempt, maxAttempts))
			timer := time.NewTimer(time.Duration(attempt) * 2 * time.Second)
			select {
			case <-npmCtx.Done():
				timer.Stop()
				return npmCtx.Err()
			case <-timer.C:
			}
			continue
		}
		ui.Println()
		retry, err := ui.Confirm("Retry npm installation?", true)
		if err != nil || !retry {
			r.Muted("Skipping npm package retry")
			return lastErr
		}
	}

	if !plan.DryRun && lastErr == nil {
		for _, pkg := range npmPkgs {
			if err := state.markNpm(pkg); err != nil {
				r.Warn(fmt.Sprintf("Failed to track installed package %s: %v", pkg, err))
			}
		}
	}
	return lastErr
}

func npmInstallContext(parent context.Context, totalPackages int) (context.Context, context.CancelFunc) {
	timeout := npmInstallBaseTimeout + time.Duration(totalPackages)*npmInstallTimeoutPerPackage
	if timeout > npmInstallMaxTimeout {
		timeout = npmInstallMaxTimeout
	}
	return context.WithTimeout(parent, timeout)
}
