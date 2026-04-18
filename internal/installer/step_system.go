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

func applyMacOSPrefs(plan InstallPlan, r Reporter) error {
	if len(plan.MacOSPrefs) == 0 {
		return nil
	}

	r.Header("Step 7: macOS Preferences")
	fmt.Println()

	if plan.DryRun {
		r.Info(fmt.Sprintf("[DRY-RUN] Would apply %d macOS preferences", len(plan.MacOSPrefs)))
		fmt.Println()
		return nil
	}

	if err := macos.CreateScreenshotsDir(false); err != nil {
		r.Error(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
	}
	if err := macos.Configure(plan.MacOSPrefs, false); err != nil {
		r.Warn(fmt.Sprintf("Some macOS preferences could not be set: %v", err))
	}
	r.Success(fmt.Sprintf("macOS preferences configured (%d settings)", len(plan.MacOSPrefs)))
	macos.RestartAffectedApps(false)
	fmt.Println()
	return nil
}

func applyPostInstall(plan InstallPlan, r Reporter) error {
	if len(plan.PostInstall) == 0 {
		return nil
	}

	r.Header("Step 8: Post-Install Script")
	fmt.Println()

	if !plan.DryRun && (plan.Silent || !system.HasTTY()) && !plan.AllowPostInstall {
		r.Warn("Skipping post-install script in silent mode (use --allow-post-install to enable)")
		fmt.Println()
		return nil
	}

	script := strings.Join(plan.PostInstall, "\n")
	r.Info(fmt.Sprintf("Post-install script (%d lines):", len(plan.PostInstall)))
	fmt.Println()
	ui.PrintScriptPreview(script)
	fmt.Println()

	if !plan.DryRun && !plan.Silent && system.HasTTY() {
		run, err := ui.Confirm("Run post-install script?", true)
		if err != nil {
			return err
		}
		if !run {
			r.Muted("Skipping post-install script")
			fmt.Println()
			return nil
		}
	}

	if plan.DryRun {
		fmt.Println("[DRY-RUN] Would run the script above")
		fmt.Println()
		return nil
	}

	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	var errs []error
	for i, cmdStr := range plan.PostInstall {
		c := exec.Command("/bin/zsh", "-c", cmdStr)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Dir = home
		if err := c.Run(); err != nil {
			errs = append(errs, fmt.Errorf("post_install[%d]: %w", i, err))
		}
	}
	if len(errs) == 0 {
		r.Success("Post-install script complete")
	}
	fmt.Println()
	return errors.Join(errs...)
}

func stepMacOS(opts *config.InstallOptions, st *config.InstallState) error {
	if opts.Macos == "skip" {
		return nil
	}

	ui.Header("Step 7: macOS Preferences")
	fmt.Println()

	// --macos configure flag or non-interactive mode: apply all defaults directly.
	if opts.Macos == "configure" || opts.Silent || (opts.DryRun && !system.HasTTY()) {
		if err := macos.CreateScreenshotsDir(opts.DryRun); err != nil {
			ui.Error(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
		}
		if err := macos.Configure(macos.DefaultPreferences, opts.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Some macOS preferences could not be set: %v", err))
		}
		if !opts.DryRun {
			ui.Success("macOS preferences configured")
			macos.RestartAffectedApps(opts.DryRun)
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

	if err := macos.CreateScreenshotsDir(opts.DryRun); err != nil {
		ui.Error(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
	}

	if err := macos.Configure(selected, opts.DryRun); err != nil {
		ui.Warn(fmt.Sprintf("Some macOS preferences could not be set: %v", err))
	}

	if !opts.DryRun {
		ui.Success(fmt.Sprintf("macOS preferences configured (%d settings)", len(selected)))
		macos.RestartAffectedApps(opts.DryRun)
	}

	fmt.Println()
	return nil
}

func stepRestoreMacOS(opts *config.InstallOptions, st *config.InstallState) error {
	ui.Header("Restore: macOS Preferences")
	fmt.Println()

	if len(st.SnapshotMacOS) == 0 {
		ui.Muted("No macOS preferences in snapshot, skipping")
		fmt.Println()
		return nil
	}

	prefs := make([]macos.Preference, 0, len(st.SnapshotMacOS))
	for _, p := range st.SnapshotMacOS {
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

	if opts.DryRun {
		ui.Info(fmt.Sprintf("[DRY-RUN] Would restore %d macOS preferences from snapshot", len(prefs)))
		fmt.Println()
		return nil
	}

	if err := macos.Configure(prefs, opts.DryRun); err != nil {
		ui.Warn(fmt.Sprintf("Some macOS preferences could not be set: %v", err))
	}

	if err := macos.CreateScreenshotsDir(opts.DryRun); err != nil {
		ui.Warn(fmt.Sprintf("Failed to create Screenshots dir: %v", err))
	}

	macos.RestartAffectedApps(opts.DryRun)
	ui.Success(fmt.Sprintf("macOS preferences restored (%d settings)", len(prefs)))
	fmt.Println()
	return nil
}

func stepPostInstall(opts *config.InstallOptions, st *config.InstallState) error {
	if opts.PostInstall == "skip" {
		return nil
	}

	if st.RemoteConfig == nil || len(st.RemoteConfig.PostInstall) == 0 {
		return nil
	}

	ui.Header("Step 8: Post-Install Script")
	fmt.Println()

	commands := st.RemoteConfig.PostInstall

	if !opts.DryRun && (opts.Silent || !system.HasTTY()) {
		if !opts.AllowPostInstall {
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

	if !opts.DryRun && !opts.Silent && system.HasTTY() {
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
	if !opts.DryRun {
		var err error
		home, err = system.HomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
	}

	var errs []error
	if opts.DryRun {
		fmt.Println("[DRY-RUN] Would run the script above")
	} else {
		for i, cmdStr := range commands {
			c := exec.Command("/bin/zsh", "-c", cmdStr)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Dir = home
			if err := c.Run(); err != nil {
				errs = append(errs, fmt.Errorf("post_install[%d]: %w", i, err))
			}
		}
	}

	if len(errs) == 0 && !opts.DryRun {
		ui.Success("Post-install script complete")
	}
	fmt.Println()
	return errors.Join(errs...)
}
