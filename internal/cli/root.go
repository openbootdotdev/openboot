package cli

import (
	"fmt"
	"os"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/updater"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	cfg     = &config.Config{}
)

var rootCmd = &cobra.Command{
	Use:   "openboot",
	Short: "Set up your Mac dev environment in one command",
	Long: `OpenBoot - Mac development environment setup tool

Automates installation of Homebrew packages, CLI tools, GUI apps, npm packages,
shell configuration, and macOS preferences.`,
	Example: `  # Interactive setup with package selection
  openboot

  # Quick setup with a preset
  openboot -p developer

  # Install from your cloud config
  openboot -u githubusername

  # Install from a local config or snapshot file
  openboot --from config.json

  # Capture your current environment
  openboot snapshot --json > my-setup.json`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		config.SetClientVersion(version)

		// Skip network operations for lightweight commands that don't
		// need the package catalog or auto-update check.
		lightweightCmds := map[string]bool{
			"version": true,
			"help":    true,
		}
		if !lightweightCmds[cmd.Name()] {
			updater.AutoUpgrade(version)
			config.RefreshPackagesFromRemote()
		}

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

		if cfg.User != "" {
			var token string
			if stored, err := auth.LoadToken(); err == nil && stored != nil {
				token = stored.Token
			}
			rc, err := config.FetchRemoteConfig(cfg.User, token)
			if err != nil {
				return fmt.Errorf("fetch remote config: %w", err)
			}
			cfg.RemoteConfig = rc
			if cfg.Preset == "" {
				cfg.Preset = rc.Preset
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// `openboot` (no subcommand) is equivalent to `openboot install`.
		return runInstallCmd(cmd, args)
	},
}

func init() {
	rootCmd.Flags().SortFlags = false

	rootCmd.Flags().StringVarP(&cfg.Preset, "preset", "p", "", "use a preset: minimal, developer, full")
	rootCmd.Flags().StringVarP(&cfg.User, "user", "u", "", "install from openboot.dev/username config")
	rootCmd.Flags().String("from", "", "install from a local config or snapshot JSON file")
	rootCmd.Flags().BoolVarP(&cfg.Silent, "silent", "s", false, "non-interactive mode (for CI/CD)")
	rootCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "preview changes without installing")
	rootCmd.Flags().BoolVar(&cfg.PackagesOnly, "packages-only", false, "install packages only, skip system config")

	rootCmd.Flags().StringVar(&cfg.Shell, "shell", "", "shell setup: install, skip")
	rootCmd.Flags().StringVar(&cfg.Macos, "macos", "", "macOS preferences: configure, skip")
	rootCmd.Flags().StringVar(&cfg.Dotfiles, "dotfiles", "", "dotfiles: clone, link, skip")
	rootCmd.Flags().StringVar(&cfg.PostInstall, "post-install", "", "post-install script: skip")
	rootCmd.Flags().BoolVar(&cfg.AllowPostInstall, "allow-post-install", false, "allow post-install scripts in silent mode")

	rootCmd.Flags().BoolVar(&cfg.Update, "update", false, "update Homebrew before installing")

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)

	rootCmd.SetUsageTemplate(usageTemplate)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("OpenBoot v%s\n", version)
	},
}

const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}

Learn more:
  Documentation: https://openboot.dev/docs
  GitHub:        https://github.com/openbootdotdev/openboot
`

func Execute() error {
	return rootCmd.Execute()
}
