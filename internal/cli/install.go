package cli

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// installCfg is the single config instance shared by the root command (openboot)
// and the install subcommand (openboot install). Both bind their flags here so
// that `openboot -p developer` and `openboot install -p developer` are identical.
var installCfg = &config.Config{}

var installCmd = &cobra.Command{
	Use:   "install [source]",
	Short: "Set up your Mac dev environment",
	Long: `Install and configure your Mac development environment.

Source resolution (positional argument, in order):
  1. ./path, /path, or *.json  → local file
  2. user/slug                  → openboot.dev config
  3. preset name                → built-in preset (minimal, developer, full)
  4. other word                 → treated as an openboot.dev alias

With no arguments, resumes from your saved sync source (or runs the interactive
wizard if you have never synced before).

Explicit flags (--from, --user, -p) take precedence over the positional argument.`,
	Example: `  # Interactive setup (or resume last sync)
  openboot install

  # Quick setup with a built-in preset
  openboot install -p developer

  # Install from your cloud config
  openboot install -u githubusername

  # Install from a specific cloud config
  openboot install alice/dev-setup

  # Install from a local file or snapshot
  openboot install --from ./backup.json

  # Preview changes without installing
  openboot install --dry-run`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runInstallCmd,
}

func init() {
	installCmd.Flags().SortFlags = false

	installCmd.Flags().StringVarP(&installCfg.Preset, "preset", "p", "", "use a preset: minimal, developer, full")
	installCmd.Flags().StringVarP(&installCfg.User, "user", "u", "", "install from an alias or openboot.dev/username/slug config")
	installCmd.Flags().String("from", "", "install from a local config or snapshot JSON file")
	installCmd.Flags().BoolVarP(&installCfg.Silent, "silent", "s", false, "non-interactive mode (for CI/CD)")
	installCmd.Flags().BoolVar(&installCfg.DryRun, "dry-run", false, "preview changes without installing")
	installCmd.Flags().BoolVar(&installCfg.PackagesOnly, "packages-only", false, "install packages only, skip system config")

	installCmd.Flags().StringVar(&installCfg.Shell, "shell", "", "shell setup: install, skip")
	installCmd.Flags().StringVar(&installCfg.Macos, "macos", "", "macOS preferences: configure, skip")
	installCmd.Flags().StringVar(&installCfg.Dotfiles, "dotfiles", "", "dotfiles: clone, link, skip")
	installCmd.Flags().StringVar(&installCfg.PostInstall, "post-install", "", "post-install script: skip")

	installCmd.Flags().BoolVar(&installCfg.Update, "update", false, "update Homebrew before installing")
	installCmd.Flags().BoolVar(&installCfg.AllowPostInstall, "allow-post-install", false, "allow post-install scripts in silent mode")
}

// applyEnvOverrides applies environment variable overrides to cfg.
// Called at the start of runInstallCmd so env vars are respected on both
// the `openboot` root path and the `openboot install` subcommand path.
func applyEnvOverrides(cfg *config.Config) {
	if cfg.Silent {
		if name := os.Getenv("OPENBOOT_GIT_NAME"); name != "" {
			cfg.GitName = name
		}
		if email := os.Getenv("OPENBOOT_GIT_EMAIL"); email != "" {
			cfg.GitEmail = email
		}
	}
	if preset := os.Getenv("OPENBOOT_PRESET"); preset != "" && cfg.Preset == "" {
		cfg.Preset = preset
	}
	if user := os.Getenv("OPENBOOT_USER"); user != "" && cfg.User == "" {
		cfg.User = user
	}
}

func runInstallCmd(cmd *cobra.Command, args []string) error {
	applyEnvOverrides(installCfg)

	if installCfg.RemoteConfig == nil {
		src, err := resolveInstallSource(cmd, args)
		if err != nil {
			return err
		}

		if src.kind == sourceSyncSource {
			return runSyncInstall(src.syncSource)
		}

		if err := applyInstallSource(src); err != nil {
			return err
		}
	}

	err := installer.Run(installCfg)
	if errors.Is(err, installer.ErrUserCancelled) {
		return nil
	}
	if err == nil {
		saveSyncSourceIfRemote(installCfg)
	}
	return err
}

// ── Source resolution ─────────────────────────────────────────────────────────

type sourceKind int

const (
	sourceNone sourceKind = iota // no args, no sync source → interactive wizard
	sourceSyncSource
	sourceCloud
	sourceFile
	sourcePreset
)

type installSource struct {
	kind       sourceKind
	userSlug   string
	path       string
	syncSource *syncpkg.SyncSource
}

// resolveInstallSource inspects flags and args to determine where to install from.
// Precedence: --from > --user > -p > positional arg > saved sync source > interactive.
func resolveInstallSource(cmd *cobra.Command, args []string) (*installSource, error) {
	if fromFile, _ := cmd.Flags().GetString("from"); fromFile != "" {
		return &installSource{kind: sourceFile, path: fromFile}, nil
	}
	if installCfg.User != "" {
		return &installSource{kind: sourceCloud, userSlug: installCfg.User}, nil
	}
	if installCfg.Preset != "" {
		return &installSource{kind: sourcePreset}, nil
	}

	if len(args) > 0 {
		return resolvePositionalArg(args[0])
	}

	if source, _ := syncpkg.LoadSource(); source != nil {
		return &installSource{kind: sourceSyncSource, syncSource: source}, nil
	}

	return &installSource{kind: sourceNone}, nil
}

// resolvePositionalArg interprets a position argument by pattern:
//  1. file-like (./, /, ../, or ends in .json) → local file
//  2. user/slug format → cloud config
//  3. plain word → preset if matches built-in, else cloud alias
func resolvePositionalArg(arg string) (*installSource, error) {
	if looksLikeFilePath(arg) {
		return &installSource{kind: sourceFile, path: arg}, nil
	}
	if looksLikeUserSlug(arg) {
		return &installSource{kind: sourceCloud, userSlug: arg}, nil
	}
	if _, ok := config.GetPreset(arg); ok {
		installCfg.Preset = arg
		return &installSource{kind: sourcePreset}, nil
	}
	// Fall through: treat as a cloud alias — FetchRemoteConfig's alias
	// resolver will handle it or return a clear error.
	return &installSource{kind: sourceCloud, userSlug: arg}, nil
}

func looksLikeFilePath(s string) bool {
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "/") {
		return true
	}
	return strings.HasSuffix(s, ".json")
}

var slugPartRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func looksLikeUserSlug(s string) bool {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return false
	}
	return slugPartRe.MatchString(parts[0]) && slugPartRe.MatchString(parts[1])
}

// applyInstallSource loads the chosen source into cfg so installer.Run can use it.
// Not used for sourceSyncSource (that path has its own flow).
func applyInstallSource(src *installSource) error {
	switch src.kind {
	case sourceNone:
		return nil

	case sourceCloud:
		installCfg.User = src.userSlug
		var token string
		if stored, _ := auth.LoadToken(); stored != nil {
			token = stored.Token
		}
		rc, err := config.FetchRemoteConfig(src.userSlug, token)
		if err != nil {
			return fmt.Errorf("fetch remote config: %w", err)
		}
		installCfg.RemoteConfig = rc
		if installCfg.Preset == "" {
			installCfg.Preset = rc.Preset
		}
		return nil

	case sourceFile:
		rc, err := config.LoadRemoteConfigFromFile(src.path)
		if err != nil {
			return fmt.Errorf("load config from file: %w", err)
		}
		installCfg.RemoteConfig = rc
		if installCfg.Preset == "" {
			installCfg.Preset = rc.Preset
		}
		return nil

	case sourcePreset:
		// installCfg.Preset is already set (by flag or resolvePositionalArg).
		return nil

	case sourceSyncSource:
		// sourceSyncSource is handled by runSyncInstall, not this function.
		return nil
	}
	return nil
}

// ── Sync-source install flow ──────────────────────────────────────────────────

// runSyncInstall is the flow when `openboot install` is called without args
// and a sync source exists. It fetches the remote config, shows a diff, and
// applies only the additions (install is add-only).
func runSyncInstall(source *syncpkg.SyncSource) error {
	printSyncSourceHeader(source)

	var token string
	if stored, _ := auth.LoadToken(); stored != nil {
		token = stored.Token
	}
	rc, err := config.FetchRemoteConfig(source.UserSlug, token)
	if err != nil {
		return fmt.Errorf("fetch remote config: %w", err)
	}

	diff, err := syncpkg.ComputeDiff(rc)
	if err != nil {
		return fmt.Errorf("compute diff: %w", err)
	}

	label := sourceLabel(source)
	if label == "" {
		label = sourceLabelForConfig(rc)
	}

	// Only consider "missing" items — install never uninstalls.
	missingCount := diff.TotalMissing() + diff.TotalChanged()
	if missingCount == 0 {
		ui.Success(fmt.Sprintf("Already up to date with %s.", label))
		updateSyncedAt(source, "", rc)
		return nil
	}

	printInstallDiff(diff)

	if installCfg.DryRun {
		ui.Muted(fmt.Sprintf("Dry run: would apply %d change(s) from %s.", missingCount, label))
		return nil
	}

	if !installCfg.Silent {
		confirmed, err := ui.Confirm(fmt.Sprintf("Apply %d change(s) from %s?", missingCount, label), true)
		if err != nil {
			return fmt.Errorf("confirm: %w", err)
		}
		if !confirmed {
			ui.Info("Cancelled.")
			return nil
		}
	}

	fmt.Println()
	plan := buildInstallPlan(diff, rc)
	result, execErr := syncpkg.Execute(plan, false)

	fmt.Println()
	if result.Installed > 0 {
		ui.Success(fmt.Sprintf("Installed %d package(s)", result.Installed))
	}
	if result.Updated > 0 {
		ui.Success(fmt.Sprintf("Updated %d setting(s)", result.Updated))
	}
	for _, e := range result.Errors {
		ui.Error(fmt.Sprintf("Failed: %s", e))
	}

	if execErr == nil || result.Installed > 0 || result.Updated > 0 {
		updateSyncedAt(source, "", rc)
	}
	return execErr
}

// printSyncSourceHeader shows the "→ Syncing with X (last synced Y)" line at
// the top of a sync-source install. Warns in yellow if > 90 days stale.
func printSyncSourceHeader(source *syncpkg.SyncSource) {
	label := sourceLabel(source)
	fmt.Println()
	if source.SyncedAt.IsZero() {
		ui.Info(fmt.Sprintf("→ Syncing with %s", label))
	} else {
		d := time.Since(source.SyncedAt)
		rel := relativeTime(d)
		if d > 90*24*time.Hour {
			ui.Warn(fmt.Sprintf("→ Syncing with %s  last synced %s", label, rel))
		} else {
			ui.Info(fmt.Sprintf("→ Syncing with %s (last synced %s)", label, rel))
		}
	}
	fmt.Println()
}
