package initializer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/ui"
)

type Config struct {
	ProjectDir    string
	ProjectConfig *config.ProjectConfig
	DryRun        bool
	Silent        bool
	Update        bool
	Version       string
}

func Run(cfg *Config) error {
	fmt.Println()
	ui.Header(fmt.Sprintf("OpenBoot Initializer v%s", cfg.Version))
	fmt.Println()

	if cfg.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	ui.Info(fmt.Sprintf("Project directory: %s", cfg.ProjectDir))
	fmt.Println()

	if err := checkDependencies(cfg); err != nil {
		return err
	}

	if cfg.Update && !cfg.DryRun {
		ui.Info("Updating Homebrew...")
		if err := brew.Update(cfg.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Failed to update Homebrew: %v", err))
		}
		fmt.Println()
	}

	if cfg.ProjectConfig.HasPackages() {
		if err := stepInstallPackages(cfg); err != nil {
			return err
		}
	}

	if cfg.ProjectConfig.HasEnv() {
		stepShowEnvVars(cfg)
	}

	if cfg.ProjectConfig.HasInit() {
		if err := stepRunInit(cfg); err != nil {
			return err
		}
	}

	if cfg.ProjectConfig.HasVerify() {
		if err := stepRunVerify(cfg); err != nil {
			return err
		}
	}

	fmt.Println()
	ui.Success("✓ Environment ready")
	fmt.Println()

	return nil
}

func checkDependencies(cfg *Config) error {
	if cfg.DryRun {
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

	if hasIssues && !cfg.Silent {
		cont, err := ui.Confirm("Continue with initialization?", true)
		if err != nil {
			return err
		}
		if !cont {
			return fmt.Errorf("initialization cancelled")
		}
		fmt.Println()
	}

	return nil
}

func stepInstallPackages(cfg *Config) error {
	pc := cfg.ProjectConfig

	if pc.Brew == nil || (len(pc.Brew.Packages) == 0 && len(pc.Brew.Casks) == 0 && len(pc.Brew.Taps) == 0) {
		if len(pc.Npm) == 0 {
			return nil
		}
	}

	ui.Header("Installing Packages")
	fmt.Println()

	if pc.Brew != nil {
		if len(pc.Brew.Taps) > 0 {
			ui.Info(fmt.Sprintf("Adding %d Homebrew taps...", len(pc.Brew.Taps)))
			if !cfg.DryRun {
				if err := brew.InstallTaps(pc.Brew.Taps, cfg.DryRun); err != nil {
					ui.Warn(fmt.Sprintf("Some taps failed: %v", err))
				}
			} else {
				for _, tap := range pc.Brew.Taps {
					ui.Muted(fmt.Sprintf("  • %s", tap))
				}
			}
			fmt.Println()
		}

		if len(pc.Brew.Packages) > 0 {
			ui.Info(fmt.Sprintf("Installing %d Homebrew packages...", len(pc.Brew.Packages)))
			if !cfg.DryRun {
				if err := brew.Install(pc.Brew.Packages, cfg.DryRun); err != nil {
					return fmt.Errorf("install brew packages: %w", err)
				}
			} else {
				for _, pkg := range pc.Brew.Packages {
					ui.Muted(fmt.Sprintf("  • %s", pkg))
				}
			}
			fmt.Println()
		}

		if len(pc.Brew.Casks) > 0 {
			ui.Info(fmt.Sprintf("Installing %d Homebrew casks...", len(pc.Brew.Casks)))
			if !cfg.DryRun {
				if err := brew.InstallCask(pc.Brew.Casks, cfg.DryRun); err != nil {
					return fmt.Errorf("install brew casks: %w", err)
				}
			} else {
				for _, cask := range pc.Brew.Casks {
					ui.Muted(fmt.Sprintf("  • %s", cask))
				}
			}
			fmt.Println()
		}
	}

	if len(pc.Npm) > 0 {
		ui.Info(fmt.Sprintf("Installing %d npm packages...", len(pc.Npm)))
		if !cfg.DryRun {
			if err := npm.Install(pc.Npm, cfg.DryRun); err != nil {
				return fmt.Errorf("install npm packages: %w", err)
			}
		} else {
			for _, pkg := range pc.Npm {
				ui.Muted(fmt.Sprintf("  • %s", pkg))
			}
		}
		fmt.Println()
	}

	return nil
}

func stepShowEnvVars(cfg *Config) {
	ui.Header("Environment Variables")
	fmt.Println()

	for key, value := range cfg.ProjectConfig.Env {
		ui.Muted(fmt.Sprintf("  export %s=\"%s\"", key, value))
	}
	fmt.Println()
	ui.Muted("Add these to your shell profile")
	fmt.Println()
}

func stepRunInit(cfg *Config) error {
	ui.Header("Running Initialization Scripts")
	fmt.Println()

	for i, script := range cfg.ProjectConfig.Init {
		ui.Info(fmt.Sprintf("[%d/%d] Running: %s", i+1, len(cfg.ProjectConfig.Init), script))

		if cfg.DryRun {
			ui.Muted(fmt.Sprintf("  [dry-run] Would execute: %s", script))
			continue
		}

		if err := runScript(script, cfg.ProjectDir); err != nil {
			return fmt.Errorf("init script failed: %s: %w", script, err)
		}
	}

	fmt.Println()

	return nil
}

func stepRunVerify(cfg *Config) error {
	ui.Header("Verifying Environment")
	fmt.Println()

	hasFailures := false

	for _, script := range cfg.ProjectConfig.Verify {
		ui.Info(fmt.Sprintf("Checking: %s", script))

		if cfg.DryRun {
			ui.Muted(fmt.Sprintf("  [dry-run] Would execute: %s", script))
			continue
		}

		if err := runScript(script, cfg.ProjectDir); err != nil {
			ui.Error(fmt.Sprintf("  ✗ Failed: %s", script))
			hasFailures = true
		} else {
			ui.Success(fmt.Sprintf("  ✓ Passed: %s", script))
		}
	}

	fmt.Println()

	if hasFailures {
		return fmt.Errorf("verification failed")
	}

	return nil
}

func runScript(script string, workdir string) error {
	parts := strings.Fields(script)
	if len(parts) == 0 {
		return fmt.Errorf("empty script")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workdir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
