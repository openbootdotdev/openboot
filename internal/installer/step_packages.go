package installer

import (
	"fmt"
	"strings"
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

func categorizeSelectedPackages(cfg *config.Config) categorizedPackages {
	var result categorizedPackages

	if cfg.RemoteConfig != nil {
		caskSet := make(map[string]bool)
		for _, c := range cfg.RemoteConfig.Casks {
			caskSet[c.Name] = true
		}
		npmSet := make(map[string]bool)
		for _, n := range cfg.RemoteConfig.Npm {
			npmSet[n.Name] = true
		}
		for pkg := range cfg.SelectedPkgs {
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
	for _, cat := range config.Categories {
		for _, pkg := range cat.Packages {
			if cfg.SelectedPkgs[pkg.Name] {
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
	for _, pkg := range cfg.OnlinePkgs {
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

func stepPresetSelection(cfg *config.Config) error {
	ui.Header("Step 2: Preset Selection")
	fmt.Println()

	if cfg.Preset == "" {
		if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
			cfg.Preset = "minimal"
		} else {
			var err error
			cfg.Preset, err = ui.SelectPreset()
			if err != nil {
				return err
			}
		}
	}

	// Handle "scratch" as special case - use minimal but show full catalog
	if cfg.Preset == "scratch" {
		ui.Success("Selected: scratch (choose from full catalog)")
		ui.Muted("You'll be able to search and select individual packages")
		fmt.Println()
		return nil
	}

	preset, ok := config.GetPreset(cfg.Preset)
	if !ok {
		return fmt.Errorf("invalid preset: %s", cfg.Preset)
	}

	ui.Success(fmt.Sprintf("Selected preset: %s", preset.Name))
	ui.Info(fmt.Sprintf("CLI packages: %d", len(preset.CLI)))
	ui.Info(fmt.Sprintf("GUI applications: %d", len(preset.Cask)))
	if len(preset.Npm) > 0 {
		ui.Info(fmt.Sprintf("npm packages: %d", len(preset.Npm)))
	}

	fmt.Println()
	return nil
}

func stepPackageCustomization(cfg *config.Config) error {
	ui.Header("Step 3: Package Selection")
	fmt.Println()

	if cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
		cfg.SelectedPkgs = config.GetPackagesForPreset(cfg.Preset)
		total := len(cfg.SelectedPkgs)
		ui.Info(fmt.Sprintf("Using preset packages: %d selected", total))
		fmt.Println()
		return nil
	}

	ui.Info("Customize your packages (based on preset: " + cfg.Preset + ")")
	ui.Muted("Use Tab to switch categories, Space to toggle, Enter to confirm")
	fmt.Println()

	selected, onlinePkgs, confirmed, err := ui.RunSelector(cfg.Preset)
	if err != nil {
		return err
	}

	if !confirmed {
		ui.Muted("Installation cancelled.")
		return ErrUserCancelled
	}

	cfg.SelectedPkgs = selected
	cfg.OnlinePkgs = onlinePkgs

	if cfg.RemoteConfig != nil && len(cfg.RemoteConfig.Packages) > 0 {
		for _, pkg := range cfg.RemoteConfig.Packages {
			cfg.SelectedPkgs[pkg.Name] = true
		}
	}

	count := 0
	for _, v := range selected {
		if v {
			count++
		}
	}
	ui.Success(fmt.Sprintf("Selected %d packages", count))
	fmt.Println()
	return nil
}

func stepInstallPackages(cfg *config.Config) error {
	ui.Header("Step 4: Installation")
	fmt.Println()

	pkgs := categorizeSelectedPackages(cfg)
	cliPkgs := pkgs.cli
	caskPkgs := pkgs.cask

	total := len(cliPkgs) + len(caskPkgs)
	if total == 0 {
		ui.Muted("No packages selected")
		return nil
	}

	state, stateErr := loadState()
	if stateErr != nil {
		ui.Warn(fmt.Sprintf("Failed to load install state: %v", stateErr))
	}

	var newCli []string
	var newCask []string

	if !cfg.DryRun {
		actualFormulae, actualCasks, checkErr := brew.GetInstalledPackages()
		if checkErr != nil {
			ui.Warn(fmt.Sprintf("Failed to check installed packages: %v", checkErr))
		} else {
			removed := state.reconcileBrewWithSystem(actualFormulae, actualCasks)
			if removed > 0 {
				if err := state.save(); err != nil {
					ui.Warn(fmt.Sprintf("Failed to update install state: %v", err))
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
			ui.Muted(fmt.Sprintf("Skipping %d packages from previous install", stateSkipped))
		}

		cliPkgs = newCli
		caskPkgs = newCask
	}

	if len(cliPkgs)+len(caskPkgs) == 0 {
		ui.Success("All packages already installed!")
		fmt.Println()
		return nil
	}

	ui.Info(fmt.Sprintf("Installing %d packages (%d CLI, %d GUI)...", len(cliPkgs)+len(caskPkgs), len(cliPkgs), len(caskPkgs)))
	fmt.Println()

	installedCli, installedCask, brewErr := brew.InstallWithProgress(cliPkgs, caskPkgs, cfg.DryRun)
	if brewErr != nil {
		ui.Error(fmt.Sprintf("Some packages failed: %v", brewErr))
	}

	if !cfg.DryRun {
		for _, pkg := range installedCli {
			if err := state.markFormula(pkg); err != nil {
				ui.Warn(fmt.Sprintf("Failed to track installed package %s: %v", pkg, err))
			}
		}
		for _, pkg := range installedCask {
			if err := state.markCask(pkg); err != nil {
				ui.Warn(fmt.Sprintf("Failed to track installed package %s: %v", pkg, err))
			}
		}
		ui.Success("Package installation complete")
	}
	fmt.Println()
	return nil
}

func stepInstallNpm(cfg *config.Config) error {
	var npmPkgs []string

	if cfg.RemoteConfig != nil {
		npmPkgs = cfg.RemoteConfig.Npm.Names()
	} else {
		pkgs := categorizeSelectedPackages(cfg)
		npmPkgs = pkgs.npm
	}

	if len(npmPkgs) == 0 {
		return nil
	}

	state, stateErr := loadState()
	if stateErr != nil {
		ui.Warn(fmt.Sprintf("Failed to load install state: %v", stateErr))
	}

	var newNpm []string
	if !cfg.DryRun {
		actualNpm, npmCheckErr := npm.GetInstalledPackages()
		if npmCheckErr != nil {
			ui.Warn(fmt.Sprintf("Failed to check installed npm packages: %v", npmCheckErr))
		} else {
			removed := state.reconcileNpmWithSystem(actualNpm)
			if removed > 0 {
				if err := state.save(); err != nil {
					ui.Warn(fmt.Sprintf("Failed to update install state: %v", err))
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
			ui.Muted(fmt.Sprintf("Skipping %d npm packages from previous install", stateSkipped))
		}

		npmPkgs = newNpm
	}

	if len(npmPkgs) == 0 {
		ui.Success("All npm packages already installed!")
		return nil
	}

	fmt.Println()
	ui.Header("NPM Global Packages")
	fmt.Println()
	ui.Info(fmt.Sprintf("Installing %d npm packages...", len(npmPkgs)))
	fmt.Println()

	err := npm.Install(npmPkgs, cfg.DryRun)

	if !cfg.DryRun && err == nil {
		for _, pkg := range npmPkgs {
			if err := state.markNpm(pkg); err != nil {
				ui.Warn(fmt.Sprintf("Failed to track installed package %s: %v", pkg, err))
			}
		}
	}

	return err
}

func stepInstallNpmWithRetry(cfg *config.Config) error {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := stepInstallNpm(cfg)
		if err == nil {
			return nil
		}

		if attempt == maxAttempts {
			ui.Error(fmt.Sprintf("npm package installation failed after %d attempts: %v", maxAttempts, err))
			return fmt.Errorf("npm installation failed after %d attempts: %w", maxAttempts, err)
		}

		if cfg.Silent || !system.HasTTY() {
			ui.Warn(fmt.Sprintf("npm installation failed (attempt %d/%d), retrying...", attempt, maxAttempts))
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		fmt.Println()
		fmt.Printf("  Retry npm installation? [Y/n] ")
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "n" || response == "no" {
			ui.Muted("Skipping npm package retry")
			return err
		}
	}
	return nil
}
