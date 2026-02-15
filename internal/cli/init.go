package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/initializer"
	"github.com/openbootdotdev/openboot/internal/updater"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Set up project environment from .openboot.yml",
	Long: `Read .openboot.yml from the project directory and install declared dependencies,
run init scripts, and verify the environment is ready.`,
	Example: `  # Initialize from current directory
  openboot init

  # Initialize from specific directory
  openboot init /path/to/project

  # Preview changes without installing
  openboot init --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		updater.AutoUpgrade(version)

		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		absDir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("failed to resolve directory: %w", err)
		}

		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", absDir)
		}

		projectCfg, err := config.LoadProjectConfig(absDir)
		if err != nil {
			return err
		}

		initCfg := &initializer.Config{
			ProjectDir:    absDir,
			ProjectConfig: projectCfg,
			DryRun:        cfg.DryRun,
			Silent:        cfg.Silent,
			Update:        cfg.Update,
			Version:       version,
		}

		return initializer.Run(initCfg)
	},
}

func init() {
	initCmd.Flags().SortFlags = false
	initCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "preview changes without installing")
	initCmd.Flags().BoolVarP(&cfg.Silent, "silent", "s", false, "non-interactive mode (for CI/CD)")
	initCmd.Flags().BoolVar(&cfg.Update, "update", false, "update Homebrew before installing")
}
