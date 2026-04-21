package installer

import (
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
		return fmt.Errorf("configure macOS preferences: %w", err)
	}
	r.Success(fmt.Sprintf("macOS preferences configured (%d settings)", len(plan.MacOSPrefs)))
	if err := macos.RestartAffectedApps(false); err != nil {
		r.Warn(fmt.Sprintf("Could not restart affected apps: %v", err))
	}
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

	c := exec.Command("/bin/zsh", "-c", script) //nolint:gosec // post-install scripts require explicit user opt-in (--allow-post-install flag)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = home
	if err := c.Run(); err != nil {
		fmt.Println()
		return fmt.Errorf("post-install: %w", err)
	}
	r.Success("Post-install script complete")
	fmt.Println()
	return nil
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
			if err := macos.RestartAffectedApps(opts.DryRun); err != nil {
				ui.Warn(fmt.Sprintf("Could not restart affected apps: %v", err))
			}
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
		if err := macos.RestartAffectedApps(opts.DryRun); err != nil {
			ui.Warn(fmt.Sprintf("Could not restart affected apps: %v", err))
		}
	}

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

	if opts.DryRun {
		fmt.Println("[DRY-RUN] Would run the script above")
		fmt.Println()
		return nil
	}

	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	c := exec.Command("/bin/zsh", "-c", script) //nolint:gosec // post-install scripts require explicit user opt-in (--allow-post-install flag)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = home
	if err := c.Run(); err != nil {
		fmt.Println()
		return fmt.Errorf("post-install: %w", err)
	}
	ui.Success("Post-install script complete")
	fmt.Println()
	return nil
}
