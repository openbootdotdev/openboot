package cli

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/openbootdotdev/openboot/internal/updater"
	"github.com/spf13/cobra"
)

var selfUpdate bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Homebrew packages or OpenBoot itself",
	Long: `Update Homebrew package definitions and upgrade all installed packages.

Use --self to update the OpenBoot binary to the latest release.

Examples:
  openboot update          # Update Homebrew packages
  openboot update --self   # Update OpenBoot itself`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if selfUpdate {
			return runSelfUpdate()
		}
		return runUpdateCommand()
	},
}

func init() {
	updateCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "preview without installing or modifying anything")
	updateCmd.Flags().BoolVar(&selfUpdate, "self", false, "Update OpenBoot binary to latest release")
}

func runSelfUpdate() error {
	fmt.Println()
	ui.Header("OpenBoot Self-Update")
	fmt.Println()

	ui.Info("Downloading latest release...")
	if err := updater.DownloadAndReplace(); err != nil {
		return err
	}

	fmt.Println()
	ui.Success("OpenBoot updated to latest version!")
	fmt.Println()
	return nil
}

func runUpdateCommand() error {
	fmt.Println()
	ui.Header("OpenBoot Update")
	fmt.Println()

	if cfg.DryRun {
		ui.Muted("[DRY-RUN MODE - No changes will be made]")
		fmt.Println()
	}

	if !brew.IsInstalled() {
		ui.Error("Homebrew is not installed. Run 'openboot' to install it first.")
		return fmt.Errorf("homebrew not installed")
	}

	ui.Info("Checking for outdated packages...")
	outdated, err := brew.ListOutdated()
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to check outdated packages: %v", err))
	} else if len(outdated) == 0 {
		ui.Success("All packages are up to date!")
		fmt.Println()
		return nil
	} else {
		fmt.Println()
		ui.Info(fmt.Sprintf("Found %d outdated packages:", len(outdated)))
		for _, pkg := range outdated {
			fmt.Printf("  %s: %s -> %s\n", pkg.Name, pkg.Current, pkg.Latest)
		}
		fmt.Println()
	}

	if cfg.DryRun {
		ui.Info("Would run: brew update && brew upgrade && brew cleanup")
		return nil
	}

	if err := brew.Update(false); err != nil {
		return err
	}

	brew.Cleanup()

	fmt.Println()
	ui.Success("Update complete!")
	fmt.Println()
	return nil
}
