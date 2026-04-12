package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull remote config and apply all changes to your system",
	Long: `Fetch the latest remote config and apply all changes to your local system.

Like 'git pull', this fetches from the remote and applies changes locally.
Run 'openboot push' to upload your current system state to openboot.dev.

The sync source is automatically saved when you run 'openboot install <config>'.
If no source is saved, falls back to your logged-in openboot.dev config.
You can override it with --source.`,
	Example: `  # Pull latest config changes
  openboot pull

  # Preview changes without applying
  openboot pull --dry-run

  # Pull from a specific config
  openboot pull --source alice/my-setup`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// pull applies all changes automatically (like git pull).
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if !dryRun {
			_ = cmd.Flags().Set("yes", "true")
		}
		return runSync(cmd)
	},
}

func init() {
	pullCmd.Flags().String("source", "", "override remote config source (alias or username/slug)")
	pullCmd.Flags().Bool("dry-run", false, "preview changes without applying")
	pullCmd.Flags().Bool("install-only", false, "only install missing packages, skip removal prompts")
	pullCmd.Flags().BoolP("yes", "y", false, "auto-confirm all prompts (non-interactive)")
	rootCmd.AddCommand(pullCmd)
}

func runSync(cmd *cobra.Command) error {
	sourceOverride, _ := cmd.Flags().GetString("source")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	installOnly, _ := cmd.Flags().GetBool("install-only")
	yes, _ := cmd.Flags().GetBool("yes")

	// 1. Load sync source
	var source *syncpkg.SyncSource
	if sourceOverride != "" {
		source = &syncpkg.SyncSource{UserSlug: sourceOverride}
	} else {
		var err error
		source, err = syncpkg.LoadSource()
		if err != nil {
			return fmt.Errorf("load sync source: %w", err)
		}
		if source == nil {
			// Fall back to the logged-in user's config
			if stored, authErr := auth.LoadToken(); authErr == nil && stored != nil && stored.Username != "" {
				source = &syncpkg.SyncSource{UserSlug: stored.Username}
			} else {
				return fmt.Errorf("no sync source found — run 'openboot install <config>' first, or use --source")
			}
		}
	}

	// 2. Load auth token
	var token string
	if stored, err := auth.LoadToken(); err == nil && stored != nil {
		token = stored.Token
	}

	// 3. Fetch remote config
	fmt.Println()
	ui.Header("OpenBoot Pull")
	fmt.Println()

	sourceLabel := source.UserSlug
	if source.Username != "" && source.Slug != "" {
		sourceLabel = fmt.Sprintf("@%s/%s", source.Username, source.Slug)
	}
	ui.Info(fmt.Sprintf("Syncing with: %s", sourceLabel))

	if !source.SyncedAt.IsZero() {
		ui.Muted(fmt.Sprintf("  Last synced: %s", source.SyncedAt.Format("2006-01-02 15:04")))
	}
	fmt.Println()

	rc, err := config.FetchRemoteConfig(source.UserSlug, token)
	if err != nil {
		return fmt.Errorf("fetch remote config: %w", err)
	}

	// 4. Compute diff
	diff, err := syncpkg.ComputeDiff(rc)
	if err != nil {
		return fmt.Errorf("compute diff: %w", err)
	}

	// 5. Check if in sync
	if !diff.HasChanges() {
		ui.Success("Already up to date.")
		updateSyncedAt(source, sourceOverride, rc)
		return nil
	}

	// 6. Display diff summary
	printSyncDiff(diff)

	// 7. Build plan
	plan, err := buildSyncPlan(diff, rc, dryRun, installOnly, yes)
	if err != nil {
		return err
	}

	if plan.IsEmpty() {
		ui.Info("No changes selected.")
		return nil
	}

	// 8. Confirm (skip in dry-run and --yes modes)
	if !dryRun && !yes {
		confirmed, err := ui.Confirm(fmt.Sprintf("Apply %d changes?", plan.TotalActions()), true)
		if err != nil {
			return fmt.Errorf("confirm: %w", err)
		}
		if !confirmed {
			ui.Info("Pull cancelled.")
			return nil
		}
	}

	// 9. Execute
	fmt.Println()
	result, execErr := syncpkg.Execute(plan, dryRun)

	// 10. Show results
	fmt.Println()
	if result.Installed > 0 {
		ui.Success(fmt.Sprintf("Installed %d package(s)", result.Installed))
	}
	if result.Uninstalled > 0 {
		ui.Success(fmt.Sprintf("Removed %d package(s)", result.Uninstalled))
	}
	if result.Updated > 0 {
		ui.Success(fmt.Sprintf("Updated %d setting(s)", result.Updated))
	}
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			ui.Error(fmt.Sprintf("Failed: %s", e))
		}
	}

	// 11. Update sync source
	if execErr == nil || result.Installed > 0 || result.Updated > 0 {
		updateSyncedAt(source, sourceOverride, rc)
	}

	return execErr
}

func updateSyncedAt(source *syncpkg.SyncSource, override string, rc *config.RemoteConfig) {
	now := time.Now()
	userSlug := source.UserSlug
	if override != "" {
		userSlug = override
	}
	installedAt := source.InstalledAt
	if installedAt.IsZero() {
		installedAt = now
	}
	updated := &syncpkg.SyncSource{
		UserSlug:    userSlug,
		Username:    rc.Username,
		Slug:        rc.Slug,
		SyncedAt:    now,
		InstalledAt: installedAt,
	}
	if err := syncpkg.SaveSource(updated); err != nil {
		ui.Warn(fmt.Sprintf("Failed to update sync source: %v", err))
	}
}

func printSyncDiff(d *syncpkg.SyncDiff) {
	// Package changes
	hasPkgChanges := len(d.MissingFormulae) > 0 || len(d.MissingCasks) > 0 ||
		len(d.MissingNpm) > 0 || len(d.MissingTaps) > 0 ||
		len(d.ExtraFormulae) > 0 || len(d.ExtraCasks) > 0 ||
		len(d.ExtraNpm) > 0 || len(d.ExtraTaps) > 0

	if hasPkgChanges {
		fmt.Printf("  %s\n", ui.Green("Package Changes"))
		printMissingExtra("Formulae", d.MissingFormulae, d.ExtraFormulae)
		printMissingExtra("Casks", d.MissingCasks, d.ExtraCasks)
		printMissingExtra("NPM", d.MissingNpm, d.ExtraNpm)
		printMissingExtra("Taps", d.MissingTaps, d.ExtraTaps)
		fmt.Println()
	}

	// macOS changes
	if len(d.MacOSChanged) > 0 {
		fmt.Printf("  %s\n", ui.Green("macOS Changes"))
		for _, p := range d.MacOSChanged {
			desc := p.Desc
			if desc == "" {
				desc = fmt.Sprintf("%s.%s", p.Domain, p.Key)
			}
			fmt.Printf("    %s: %s %s %s\n", desc, p.LocalValue, ui.Yellow("→"), p.RemoteValue)
		}
		fmt.Println()
	}

	// Shell changes
	if d.Shell != nil {
		fmt.Printf("  %s\n", ui.Green("Shell Changes"))
		if d.Shell.ThemeChanged {
			localTheme := d.Shell.LocalTheme
			if localTheme == "" {
				localTheme = "(none)"
			}
			fmt.Printf("    Theme: %s %s %s\n", localTheme, ui.Yellow("→"), d.Shell.RemoteTheme)
		}
		if d.Shell.PluginsChanged {
			localPlugins := strings.Join(d.Shell.LocalPlugins, ", ")
			if localPlugins == "" {
				localPlugins = "(none)"
			}
			fmt.Printf("    Plugins: %s %s %s\n", localPlugins, ui.Yellow("→"), strings.Join(d.Shell.RemotePlugins, ", "))
		}
		fmt.Println()
	}

	// Dotfiles changes
	if d.DotfilesChanged {
		fmt.Printf("  %s\n", ui.Green("Dotfiles"))
		fmt.Printf("    Repo changed: %s %s %s\n", d.LocalDotfiles, ui.Yellow("→"), d.RemoteDotfiles)
		fmt.Println()
	}
}

func printMissingExtra(category string, missing, extra []string) {
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	if len(missing) > 0 {
		fmt.Printf("    %s to install (%d): %s\n", category, len(missing), strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		fmt.Printf("    %s extra (%d): %s\n", category, len(extra), strings.Join(extra, ", "))
	}
}

func buildSyncPlan(d *syncpkg.SyncDiff, rc *config.RemoteConfig, dryRun bool, installOnly bool, yes bool) (*syncpkg.SyncPlan, error) {
	// Dry-run or --yes: include all missing items without interactive prompts
	if dryRun || yes {
		return buildDryRunPlan(d), nil
	}

	plan := &syncpkg.SyncPlan{}

	// Missing packages — prompt to install (pre-checked)
	missingPkgs := buildMissingOptions(d)
	if len(missingPkgs) > 0 {
		selected, err := multiSelectPkgs("Select packages to install:", missingPkgs, true)
		if err != nil {
			return nil, fmt.Errorf("select packages: %w", err)
		}
		if selected != nil {
			missing := categorizeMissing(selected, d)
			plan.InstallFormulae = missing.Formulae
			plan.InstallCasks = missing.Casks
			plan.InstallNpm = missing.Npm
			plan.InstallTaps = missing.Taps
		}
	}

	// Extra packages — prompt to remove (unchecked) unless install-only
	if !installOnly {
		extraPkgs := buildExtraOptions(d)
		if len(extraPkgs) > 0 {
			selected, err := multiSelectPkgs("Select extra packages to remove:", extraPkgs, false)
			if err != nil {
				return nil, fmt.Errorf("select removals: %w", err)
			}
			if selected != nil {
				extra := categorizeExtra(selected, d)
				plan.UninstallFormulae = extra.Formulae
				plan.UninstallCasks = extra.Casks
				plan.UninstallNpm = extra.Npm
				plan.UninstallTaps = extra.Taps
			}
		}
	}

	// macOS changes
	if len(d.MacOSChanged) > 0 {
		apply, err := ui.Confirm(fmt.Sprintf("Apply %d macOS preference change(s)?", len(d.MacOSChanged)), true)
		if err != nil {
			return nil, fmt.Errorf("confirm macos: %w", err)
		}
		if apply {
			for _, p := range d.MacOSChanged {
				plan.UpdateMacOSPrefs = append(plan.UpdateMacOSPrefs, config.RemoteMacOSPref{
					Domain: p.Domain,
					Key:    p.Key,
					Type:   p.Type,
					Value:  p.RemoteValue,
					Desc:   p.Desc,
				})
			}
		}
	}

	// Shell config
	if d.Shell != nil {
		apply, err := ui.Confirm("Update shell config (theme and plugins)?", true)
		if err != nil {
			return nil, fmt.Errorf("confirm shell: %w", err)
		}
		if apply {
			plan.UpdateShell = true
			plan.ShellOhMyZsh = true
			plan.ShellTheme = d.Shell.RemoteTheme
			plan.ShellPlugins = d.Shell.RemotePlugins
		}
	}

	// Dotfiles
	if d.DotfilesChanged {
		apply, err := ui.Confirm(
			fmt.Sprintf("Update dotfiles repo: %s → %s?", d.LocalDotfiles, d.RemoteDotfiles), true)
		if err != nil {
			return nil, fmt.Errorf("confirm dotfiles: %w", err)
		}
		if apply {
			plan.UpdateDotfiles = d.RemoteDotfiles
		}
	}

	return plan, nil
}

// buildDryRunPlan creates a plan with all missing items included (no interactive prompts).
func buildDryRunPlan(d *syncpkg.SyncDiff) *syncpkg.SyncPlan {
	plan := &syncpkg.SyncPlan{
		InstallFormulae: d.MissingFormulae,
		InstallCasks:    d.MissingCasks,
		InstallNpm:      d.MissingNpm,
		InstallTaps:     d.MissingTaps,
	}

	if d.Shell != nil {
		plan.UpdateShell = true
		plan.ShellOhMyZsh = true
		plan.ShellTheme = d.Shell.RemoteTheme
		plan.ShellPlugins = d.Shell.RemotePlugins
	}

	if d.DotfilesChanged {
		plan.UpdateDotfiles = d.RemoteDotfiles
	}

	for _, p := range d.MacOSChanged {
		plan.UpdateMacOSPrefs = append(plan.UpdateMacOSPrefs, config.RemoteMacOSPref{
			Domain: p.Domain,
			Key:    p.Key,
			Type:   p.Type,
			Value:  p.RemoteValue,
			Desc:   p.Desc,
		})
	}

	return plan
}

type pkgCategory string

const (
	categoryFormulae pkgCategory = "formulae"
	categoryCasks    pkgCategory = "casks"
	categoryNpm      pkgCategory = "npm"
	categoryTaps     pkgCategory = "taps"
)

type pkgOption struct {
	Label    string
	Category pkgCategory
}

func buildMissingOptions(d *syncpkg.SyncDiff) []pkgOption {
	var opts []pkgOption
	for _, p := range d.MissingFormulae {
		opts = append(opts, pkgOption{Label: p, Category: categoryFormulae})
	}
	for _, p := range d.MissingCasks {
		opts = append(opts, pkgOption{Label: p + " (cask)", Category: categoryCasks})
	}
	for _, p := range d.MissingNpm {
		opts = append(opts, pkgOption{Label: p + " (npm)", Category: categoryNpm})
	}
	for _, p := range d.MissingTaps {
		opts = append(opts, pkgOption{Label: p + " (tap)", Category: categoryTaps})
	}
	return opts
}

func buildExtraOptions(d *syncpkg.SyncDiff) []pkgOption {
	var opts []pkgOption
	for _, p := range d.ExtraFormulae {
		opts = append(opts, pkgOption{Label: p, Category: categoryFormulae})
	}
	for _, p := range d.ExtraCasks {
		opts = append(opts, pkgOption{Label: p + " (cask)", Category: categoryCasks})
	}
	for _, p := range d.ExtraNpm {
		opts = append(opts, pkgOption{Label: p + " (npm)", Category: categoryNpm})
	}
	for _, p := range d.ExtraTaps {
		opts = append(opts, pkgOption{Label: p + " (tap)", Category: categoryTaps})
	}
	return opts
}

func multiSelectPkgs(title string, opts []pkgOption, preChecked bool) ([]pkgOption, error) {
	options := make([]huh.Option[int], len(opts))
	for i, opt := range opts {
		options[i] = huh.NewOption(opt.Label, i)
	}

	var selected []int
	if preChecked {
		selected = make([]int, len(opts))
		for i := range opts {
			selected[i] = i
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title(title).
				Options(options...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, nil
		}
		return nil, err
	}

	result := make([]pkgOption, 0, len(selected))
	for _, idx := range selected {
		result = append(result, opts[idx])
	}
	return result, nil
}

// categorizedPkgs holds packages grouped by type, returned by categorize functions.
type categorizedPkgs struct {
	Formulae []string
	Casks    []string
	Npm      []string
	Taps     []string
}

func categorizeMissing(selected []pkgOption, d *syncpkg.SyncDiff) categorizedPkgs {
	formulaeSet := syncpkg.ToSet(d.MissingFormulae)
	casksSet := syncpkg.ToSet(d.MissingCasks)
	npmSet := syncpkg.ToSet(d.MissingNpm)
	tapsSet := syncpkg.ToSet(d.MissingTaps)

	var result categorizedPkgs
	for _, opt := range selected {
		switch opt.Category {
		case categoryFormulae:
			name := opt.Label
			if formulaeSet[name] {
				result.Formulae = append(result.Formulae, name)
			}
		case categoryCasks:
			name := strings.TrimSuffix(opt.Label, " (cask)")
			if casksSet[name] {
				result.Casks = append(result.Casks, name)
			}
		case categoryNpm:
			name := strings.TrimSuffix(opt.Label, " (npm)")
			if npmSet[name] {
				result.Npm = append(result.Npm, name)
			}
		case categoryTaps:
			name := strings.TrimSuffix(opt.Label, " (tap)")
			if tapsSet[name] {
				result.Taps = append(result.Taps, name)
			}
		}
	}
	return result
}

func categorizeExtra(selected []pkgOption, d *syncpkg.SyncDiff) categorizedPkgs {
	formulaeSet := syncpkg.ToSet(d.ExtraFormulae)
	casksSet := syncpkg.ToSet(d.ExtraCasks)
	npmSet := syncpkg.ToSet(d.ExtraNpm)
	tapsSet := syncpkg.ToSet(d.ExtraTaps)

	var result categorizedPkgs
	for _, opt := range selected {
		switch opt.Category {
		case categoryFormulae:
			name := opt.Label
			if formulaeSet[name] {
				result.Formulae = append(result.Formulae, name)
			}
		case categoryCasks:
			name := strings.TrimSuffix(opt.Label, " (cask)")
			if casksSet[name] {
				result.Casks = append(result.Casks, name)
			}
		case categoryNpm:
			name := strings.TrimSuffix(opt.Label, " (npm)")
			if npmSet[name] {
				result.Npm = append(result.Npm, name)
			}
		case categoryTaps:
			name := strings.TrimSuffix(opt.Label, " (tap)")
			if tapsSet[name] {
				result.Taps = append(result.Taps, name)
			}
		}
	}
	return result
}
