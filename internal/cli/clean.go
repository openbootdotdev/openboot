package cli

import (
	"fmt"
	"strings"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/cleaner"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove packages not in your config or snapshot",
	Long: `Compare your system against a config or snapshot and remove extra packages.

Sources (checked in order):
  1. --from <file>         Compare against a snapshot file
  2. --user <username>     Compare against your openboot.dev config
  3. Local snapshot         Compare against ~/.openboot/snapshot.json

Examples:
  openboot clean                              Clean against local snapshot
  openboot clean --user myname                Clean against cloud config
  openboot clean --from my-setup.json         Clean against a snapshot file
  openboot clean --dry-run                    Preview what would be removed`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runClean(cmd)
	},
}

func init() {
	cleanCmd.Flags().String("from", "", "snapshot file to compare against")
	cleanCmd.Flags().String("user", "", "openboot.dev username/slug to compare against")
	cleanCmd.Flags().Bool("dry-run", false, "preview changes without removing anything")
}

func runClean(cmd *cobra.Command) error {
	fromFile, _ := cmd.Flags().GetString("from")
	user, _ := cmd.Flags().GetString("user")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	fmt.Println()
	ui.Header("OpenBoot Clean")
	fmt.Println()

	if dryRun {
		ui.Muted("[DRY-RUN MODE — No packages will be removed]")
		fmt.Println()
	}

	var result *cleaner.CleanResult
	var err error

	switch {
	case fromFile != "":
		result, err = cleanFromFile(fromFile)
	case user != "":
		result, err = cleanFromRemote(user)
	default:
		result, err = cleanFromLocalSnapshot()
	}

	if err != nil {
		return err
	}

	if result.TotalExtra() == 0 {
		ui.Success("Your system is clean — no extra packages found.")
		fmt.Println()
		return nil
	}

	showCleanPreview(result)

	if !dryRun {
		proceed, err := ui.Confirm(fmt.Sprintf("Remove %d packages?", result.TotalExtra()), false)
		if err != nil {
			return err
		}
		if !proceed {
			ui.Muted("Clean cancelled.")
			fmt.Println()
			return nil
		}
	}

	if err := cleaner.Execute(result, dryRun); err != nil {
		showCleanSummary(result)
		return fmt.Errorf("some packages failed to remove: %w", err)
	}

	fmt.Println()
	if dryRun {
		ui.Muted("Dry run complete — no changes were made.")
	} else {
		showCleanSummary(result)
		ui.Success("Clean complete!")
	}
	fmt.Println()
	return nil
}

func cleanFromFile(path string) (*cleaner.CleanResult, error) {
	ui.Info(fmt.Sprintf("Comparing against snapshot: %s", path))
	fmt.Println()

	snap, err := snapshot.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load snapshot: %w", err)
	}

	return cleaner.DiffFromSnapshot(snap)
}

func cleanFromRemote(userSlug string) (*cleaner.CleanResult, error) {
	ui.Info(fmt.Sprintf("Comparing against config: %s", userSlug))
	fmt.Println()

	var token string
	if stored, err := auth.LoadToken(); err == nil && stored != nil {
		token = stored.Token
	}

	rc, err := config.FetchRemoteConfig(userSlug, token)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote config: %w", err)
	}

	return cleaner.DiffFromLists(rc.Packages, rc.Casks, rc.Npm)
}

func cleanFromLocalSnapshot() (*cleaner.CleanResult, error) {
	ui.Info("Comparing against local snapshot (~/.openboot/snapshot.json)")
	fmt.Println()

	snap, err := snapshot.LoadLocal()
	if err != nil {
		return nil, fmt.Errorf("no local snapshot found — run 'openboot snapshot --local' first, or use --from or --user flags: %w", err)
	}

	return cleaner.DiffFromSnapshot(snap)
}

func showCleanSummary(result *cleaner.CleanResult) {
	fmt.Println()
	if result.TotalRemoved() > 0 {
		ui.Success(fmt.Sprintf("Removed %d package(s):", result.TotalRemoved()))
		if len(result.RemovedFormulae) > 0 {
			fmt.Printf("  formulae: %s\n", strings.Join(result.RemovedFormulae, ", "))
		}
		if len(result.RemovedCasks) > 0 {
			fmt.Printf("  casks: %s\n", strings.Join(result.RemovedCasks, ", "))
		}
		if len(result.RemovedNpm) > 0 {
			fmt.Printf("  npm: %s\n", strings.Join(result.RemovedNpm, ", "))
		}
	}
	if result.TotalFailed() > 0 {
		ui.Warn(fmt.Sprintf("Failed to remove %d package(s):", result.TotalFailed()))
		if len(result.FailedFormulae) > 0 {
			fmt.Printf("  formulae: %s\n", strings.Join(result.FailedFormulae, ", "))
		}
		if len(result.FailedCasks) > 0 {
			fmt.Printf("  casks: %s\n", strings.Join(result.FailedCasks, ", "))
		}
		if len(result.FailedNpm) > 0 {
			fmt.Printf("  npm: %s\n", strings.Join(result.FailedNpm, ", "))
		}
	}
}

func showCleanPreview(result *cleaner.CleanResult) {
	ui.Info(fmt.Sprintf("Found %d extra packages not in your config:", result.TotalExtra()))
	fmt.Println()

	if len(result.ExtraFormulae) > 0 {
		ui.Info(fmt.Sprintf("  Formulae (%d):", len(result.ExtraFormulae)))
		fmt.Printf("    %s\n", strings.Join(result.ExtraFormulae, ", "))
	}
	if len(result.ExtraCasks) > 0 {
		ui.Info(fmt.Sprintf("  Casks (%d):", len(result.ExtraCasks)))
		fmt.Printf("    %s\n", strings.Join(result.ExtraCasks, ", "))
	}
	if len(result.ExtraNpm) > 0 {
		ui.Info(fmt.Sprintf("  NPM (%d):", len(result.ExtraNpm)))
		fmt.Printf("    %s\n", strings.Join(result.ExtraNpm, ", "))
	}
	fmt.Println()
}
