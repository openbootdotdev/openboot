package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/ui"
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
	updateCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Preview what would be updated without updating")
	updateCmd.Flags().BoolVar(&selfUpdate, "self", false, "Update OpenBoot binary to latest release")
}

func runSelfUpdate() error {
	fmt.Println()
	ui.Header("OpenBoot Self-Update")
	fmt.Println()

	arch := runtime.GOARCH
	if arch == "" {
		arch = "arm64"
	}

	url := fmt.Sprintf("https://github.com/openbootdotdev/openboot/releases/latest/download/openboot-darwin-%s", arch)
	ui.Info(fmt.Sprintf("Downloading latest release (%s)...", arch))

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine binary path: %w", err)
	}

	tmpPath := binPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}
	f.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to replace binary: %w", err)
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
