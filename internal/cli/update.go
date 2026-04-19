package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/openbootdotdev/openboot/internal/updater"
)

// update subcommand flags. Package-level so tests can reset them if needed.
var (
	updateVersion     string
	updateRollback    bool
	updateListBackups bool
	updateDryRun      bool
)

// Test seams — real implementations by default; tests replace via t.Cleanup.
var (
	updateIsHomebrewInstall  = updater.IsHomebrewInstall
	updateGetLatestVersion   = updater.GetLatestVersion
	updateDownloadAndReplace = updater.DownloadAndReplace
	updateRollbackFn         = updater.Rollback
	updateListBackupsFn      = updater.ListBackups
	updateGetBackupDir       = updater.GetBackupDir
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update OpenBoot, pin to a specific version, or roll back",
	Long: `Manage the OpenBoot binary itself.

Examples:
  openboot update                  # upgrade to the latest release
  openboot update --version 0.24.1 # pin to an exact version (must be X.Y.Z)
  openboot update --rollback       # restore the most recent pre-upgrade backup
  openboot update --list-backups   # print pre-upgrade backups under ~/.openboot/backup

Backups are written to ~/.openboot/backup/ before each direct-download
upgrade. Only the 5 most recent backups are retained.

If OpenBoot was installed via Homebrew, --version and --rollback are rejected;
use 'brew upgrade openboot' or 'brew' commands instead.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runUpdateCmd,
}

func init() {
	updateCmd.Flags().SortFlags = false
	updateCmd.Flags().StringVar(&updateVersion, "version", "", "pin to an exact version (e.g. 0.25.0)")
	updateCmd.Flags().BoolVar(&updateRollback, "rollback", false, "restore the most recent backup")
	updateCmd.Flags().BoolVar(&updateListBackups, "list-backups", false, "list backups under ~/.openboot/backup")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "print what would be done without making changes")
}

func runUpdateCmd(_ *cobra.Command, _ []string) error {
	// Mutually-exclusive modes: only one of --rollback / --list-backups /
	// --version may be set (version-less upgrade is the default).
	set := 0
	if updateRollback {
		set++
	}
	if updateListBackups {
		set++
	}
	if updateVersion != "" {
		set++
	}
	if set > 1 {
		return fmt.Errorf("--version, --rollback, and --list-backups are mutually exclusive")
	}

	switch {
	case updateListBackups:
		return runListBackups()
	case updateRollback:
		return runRollback()
	case updateVersion != "":
		return runPinnedUpgrade(updateVersion)
	default:
		return runLatestUpgrade()
	}
}

func runListBackups() error {
	dir, err := updateGetBackupDir()
	if err != nil {
		return fmt.Errorf("resolve backup dir: %w", err)
	}
	names, err := updateListBackupsFn()
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	if len(names) == 0 {
		ui.Info(fmt.Sprintf("No backups in %s", dir))
		return nil
	}
	ui.Header(fmt.Sprintf("Backups in %s", dir))
	for _, n := range names {
		fmt.Println("  " + n)
	}
	return nil
}

func runRollback() error {
	if updateIsHomebrewInstall() {
		ui.Warn("OpenBoot is managed by Homebrew — rollback is not supported.")
		ui.Muted("Use 'brew' commands to change versions.")
		return fmt.Errorf("rollback refused: Homebrew-managed install")
	}
	if updateDryRun {
		names, err := updateListBackupsFn()
		if err != nil {
			return fmt.Errorf("list backups: %w", err)
		}
		if len(names) == 0 {
			ui.Info("Dry-run: no backups available to roll back to.")
			return nil
		}
		ui.Info(fmt.Sprintf("Dry-run: would restore backup %s", names[0]))
		return nil
	}
	if err := updateRollbackFn(); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	ui.Success("Rollback complete.")
	return nil
}

func runPinnedUpgrade(v string) error {
	if err := updater.ValidateSemver(v); err != nil {
		return err
	}
	if updateIsHomebrewInstall() {
		ui.Warn("OpenBoot is managed by Homebrew — --version is not supported.")
		ui.Muted("Run 'brew upgrade openboot' to update via Homebrew.")
		return fmt.Errorf("version pin refused: Homebrew-managed install")
	}
	if updateDryRun {
		ui.Info(fmt.Sprintf("Dry-run: would download and install OpenBoot v%s", updater.TrimVersionPrefix(v)))
		return nil
	}
	ui.Info(fmt.Sprintf("Installing OpenBoot v%s...", updater.TrimVersionPrefix(v)))

	if err := updateDownloadAndReplace(v, version); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	ui.Success(fmt.Sprintf("Installed OpenBoot v%s.", updater.TrimVersionPrefix(v)))
	return nil
}

func runLatestUpgrade() error {
	if updateIsHomebrewInstall() {
		ui.Warn("OpenBoot is managed by Homebrew.")
		ui.Muted("Run 'brew upgrade openboot' to update.")
		return fmt.Errorf("use 'brew upgrade openboot'")
	}
	if updateDryRun {
		ui.Info("Dry-run: would check GitHub for the latest release and upgrade.")
		return nil
	}
	latest, err := updateGetLatestVersion()
	if err != nil {
		return fmt.Errorf("look up latest version: %w", err)
	}
	ui.Info(fmt.Sprintf("Installing OpenBoot %s...", latest))

	if err := updateDownloadAndReplace(latest, version); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	ui.Success(fmt.Sprintf("Installed OpenBoot %s.", latest))
	return nil
}
