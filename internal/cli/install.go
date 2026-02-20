package cli

import (
	"fmt"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/openbootdotdev/openboot/internal/updater"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [username/slug]",
	Short: "Set up your Mac dev environment",
	Long: `Install and configure your Mac development environment.

You can provide a username/slug to install from that user's openboot.dev config,
or run it interactively without arguments. If only a username is provided
without a slug, it defaults to the "default" config.`,
	Example: `  # Interactive setup with package selection
  openboot install

  # Install from a user's cloud config
  openboot install yourname/my-setup

  # Install default config for a user
  openboot install yourname

  # Quick setup with a preset
  openboot install -p developer

  # Preview changes without installing
  openboot install --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && cfg.User == "" {
			cfg.User = args[0]

			var token string
			if stored, err := auth.LoadToken(); err == nil && stored != nil {
				token = stored.Token
			}
			rc, err := config.FetchRemoteConfig(cfg.User, token)
			if err != nil {
				return fmt.Errorf("error fetching remote config: %v", err)
			}
			cfg.RemoteConfig = rc
			if cfg.Preset == "" {
				cfg.Preset = rc.Preset
			}
		}

		updater.AutoUpgrade(version)
		cfg.Version = version
		err := installer.Run(cfg)
		if err == installer.ErrUserCancelled {
			return nil
		}
		return err
	},
}

func init() {
	installCmd.Flags().SortFlags = false

	installCmd.Flags().StringVarP(&cfg.Preset, "preset", "p", "", "use a preset: minimal, developer, full")
	installCmd.Flags().StringVarP(&cfg.User, "user", "u", "", "install from openboot.dev/username config")
	installCmd.Flags().BoolVarP(&cfg.Silent, "silent", "s", false, "non-interactive mode (for CI/CD)")
	installCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "preview changes without installing")
	installCmd.Flags().BoolVar(&cfg.PackagesOnly, "packages-only", false, "install packages only, skip system config")

	installCmd.Flags().StringVar(&cfg.Shell, "shell", "", "shell setup: install, skip")
	installCmd.Flags().StringVar(&cfg.Macos, "macos", "", "macOS preferences: configure, skip")
	installCmd.Flags().StringVar(&cfg.Dotfiles, "dotfiles", "", "dotfiles: clone, link, skip")

	installCmd.Flags().BoolVar(&cfg.Update, "update", false, "update Homebrew before installing")
}
