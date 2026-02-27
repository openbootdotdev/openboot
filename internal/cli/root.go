package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
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

  # Capture your current environment
  openboot snapshot --json > my-setup.json`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
				return fmt.Errorf("error fetching remote config: %v", err)
			}
			cfg.RemoteConfig = rc
			if cfg.Preset == "" {
				cfg.Preset = rc.Preset
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		updater.AutoUpgrade(version)
		cfg.Version = version
		err := installer.Run(cfg)
		if errors.Is(err, installer.ErrUserCancelled) {
			return nil
		}
		return err
	},
}

func init() {
	rootCmd.Flags().SortFlags = false

	rootCmd.Flags().StringVarP(&cfg.Preset, "preset", "p", "", "use a preset: minimal, developer, full")
	rootCmd.Flags().StringVarP(&cfg.User, "user", "u", "", "install from openboot.dev/username config")
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
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(cleanCmd)
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
