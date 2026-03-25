package installer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

func stepMacOS(cfg *config.Config) error {
	if cfg.Macos == "skip" {
		return nil
	}

	ui.Header("Step 7: macOS Preferences")
	fmt.Println()

	// --macos configure flag or non-interactive mode: apply all defaults directly.
	if cfg.Macos == "configure" || cfg.Silent || (cfg.DryRun && !system.HasTTY()) {
		if err := macos.CreateScreenshotsDir(cfg.DryRun); err != nil {
			ui.Error(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
		}
		if err := macos.Configure(macos.DefaultPreferences, cfg.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Some macOS preferences could not be set: %v", err))
		}
		if !cfg.DryRun {
			ui.Success("macOS preferences configured")
			macos.RestartAffectedApps(cfg.DryRun)
		}
		fmt.Println()
		return nil
	}

	ui.Info("Choose which macOS preferences to apply")
	ui.Muted("Use Tab to switch categories, Space to toggle, Enter to confirm")
	fmt.Println()

	selected, confirmed, err := ui.RunMacOSSelector()
	if err != nil {
		return fmt.Errorf("macOS selector: %w", err)
	}

	if !confirmed || len(selected) == 0 {
		ui.Muted("Skipping macOS preferences")
		fmt.Println()
		return nil
	}

	if err := macos.CreateScreenshotsDir(cfg.DryRun); err != nil {
		ui.Error(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
	}

	if err := macos.Configure(selected, cfg.DryRun); err != nil {
		ui.Warn(fmt.Sprintf("Some macOS preferences could not be set: %v", err))
	}

	if !cfg.DryRun {
		ui.Success(fmt.Sprintf("macOS preferences configured (%d settings)", len(selected)))
		macos.RestartAffectedApps(cfg.DryRun)
	}

	fmt.Println()
	return nil
}

func stepRestoreMacOS(cfg *config.Config) error {
	ui.Header("Restore: macOS Preferences")
	fmt.Println()

	if len(cfg.SnapshotMacOS) == 0 {
		ui.Muted("No macOS preferences in snapshot, skipping")
		fmt.Println()
		return nil
	}

	prefs := make([]macos.Preference, 0, len(cfg.SnapshotMacOS))
	for _, p := range cfg.SnapshotMacOS {
		prefType := p.Type
		if prefType == "" {
			prefType = macos.InferPreferenceType(p.Value)
		}
		prefs = append(prefs, macos.Preference{
			Domain: p.Domain,
			Key:    p.Key,
			Type:   prefType,
			Value:  p.Value,
			Desc:   p.Desc,
		})
	}

	if cfg.DryRun {
		ui.Info(fmt.Sprintf("[DRY-RUN] Would restore %d macOS preferences from snapshot", len(prefs)))
		fmt.Println()
		return nil
	}

	if err := macos.Configure(prefs, cfg.DryRun); err != nil {
		ui.Warn(fmt.Sprintf("Some macOS preferences could not be set: %v", err))
	}

	if err := macos.CreateScreenshotsDir(cfg.DryRun); err != nil {
		ui.Warn(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
	}

	macos.RestartAffectedApps(cfg.DryRun)
	ui.Success(fmt.Sprintf("macOS preferences restored (%d settings)", len(prefs)))
	fmt.Println()
	return nil
}

func stepPostInstall(cfg *config.Config) error {
	if cfg.PostInstall == "skip" {
		return nil
	}

	if cfg.RemoteConfig == nil || len(cfg.RemoteConfig.PostInstall) == 0 {
		return nil
	}

	ui.Header("Step 8: Post-Install Script")
	fmt.Println()

	commands := cfg.RemoteConfig.PostInstall

	if !cfg.DryRun && (cfg.Silent || !system.HasTTY()) {
		if !cfg.AllowPostInstall {
			ui.Warn("Skipping post-install script in silent mode (use --allow-post-install to enable)")
			fmt.Println()
			return nil
		}
	}

	// Show script preview for all modes that proceed (interactive, dry-run, silent+allowed)
	script := strings.Join(commands, "\n")
	lineCount := len(commands)
	ui.Info(fmt.Sprintf("Post-install script (%d lines):", lineCount))
	fmt.Println()
	ui.PrintScriptPreview(script)
	fmt.Println()

	if !cfg.DryRun && !cfg.Silent && system.HasTTY() {
		run, err := ui.Confirm("Run post-install script?", true)
		if err != nil {
			return err
		}
		if !run {
			ui.Muted("Skipping post-install script")
			fmt.Println()
			return nil
		}
	}

	var home string
	if !cfg.DryRun {
		var err error
		home, err = system.HomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
	}

	var errs []error
	if cfg.DryRun {
		fmt.Println("[DRY-RUN] Would run the script above")
	} else {
		cmd := exec.Command("/bin/zsh", "-c", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = home
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Errorf("post-install script: %w", err))
		}
	}

	if len(errs) == 0 && !cfg.DryRun {
		ui.Success("Post-install script complete")
	}
	fmt.Println()
	return errors.Join(errs...)
}
