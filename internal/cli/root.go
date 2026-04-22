package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/logging"
	"github.com/openbootdotdev/openboot/internal/updater"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "openboot",
	Short: "Set up your Mac dev environment",
	Long: `OpenBoot — Mac development environment setup tool

Automates installation of Homebrew packages, CLI tools, GUI apps, npm packages,
shell configuration, and macOS preferences.`,
	Example: `  # Interactive setup
  openboot install

  # Quick setup with a preset
  openboot install -p developer

  # Install from your cloud config
  openboot install -u githubusername

  # Install from a local config or snapshot file
  openboot install --from config.json

  # Capture your current environment
  openboot snapshot --json > my-setup.json`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Install always-on file logging; --verbose controls stderr level.
		// Failure here is never fatal — Init falls back to stderr internally.
		closer, err := logging.Init(version, verbose)
		if err != nil {
			return fmt.Errorf("init logging: %w", err)
		}
		logCloser = closer

		config.SetClientVersion(version)
		installCfg.Version = version

		// Only the install flow needs the package catalog and auto-update.
		// All other commands (snapshot, login, logout, etc.) run without
		// network overhead.
		if cmd.Name() == "install" {
			updater.AutoUpgrade(version)
			config.RefreshPackagesFromRemote()
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable debug logging to stderr")

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(updateCmd)

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

// verbose is set by the --verbose persistent flag.
var verbose bool

// logCloser is set by PersistentPreRunE and flushed by Execute on return.
var logCloser func()

func Execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	defer func() {
		if logCloser != nil {
			logCloser()
		}
	}()
	return rootCmd.ExecuteContext(ctx)
}
