package cli

import (
	"fmt"
	"os"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/spf13/cobra"
)

var (
	version = "0.7.1"
	cfg     = &config.Config{}
)

var rootCmd = &cobra.Command{
	Use:   "openboot",
	Short: "One-line macOS development environment setup",
	Long: `OpenBoot bootstraps your Mac development environment in minutes.
Install Homebrew, CLI tools, GUI apps, dotfiles, and Oh-My-Zsh with a single command.

Quick Start:
  openboot                    Interactive setup with package selection
  openboot -p minimal         Quick setup with essential tools only
  openboot -p developer       Full developer environment
  openboot -u username        Use your saved config from openboot.dev

Remote Config:
  Create your config at https://openboot.dev and run:
  openboot -u your-github-username

CI/Automation:
  export OPENBOOT_PRESET=developer
  openboot --silent`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Silent {
			if name := os.Getenv("OPENBOOT_GIT_NAME"); name != "" {
				cfg.GitName = name
			}
			if email := os.Getenv("OPENBOOT_GIT_EMAIL"); email != "" {
				cfg.GitEmail = email
			}
			if preset := os.Getenv("OPENBOOT_PRESET"); preset != "" && cfg.Preset == "" {
				cfg.Preset = preset
			}
		}

		if user := os.Getenv("OPENBOOT_USER"); user != "" && cfg.User == "" {
			cfg.User = user
		}

		if cfg.User != "" {
			rc, err := config.FetchRemoteConfig(cfg.User)
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
		return installer.Run(cfg)
	},
}

func init() {
	rootCmd.Flags().StringVarP(&cfg.Preset, "preset", "p", "", "Preset: minimal, developer, full")
	rootCmd.Flags().StringVarP(&cfg.User, "user", "u", "", "GitHub username for remote config")
	rootCmd.Flags().BoolVarP(&cfg.Silent, "silent", "s", false, "Non-interactive mode (for CI)")
	rootCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Preview changes without installing")
	rootCmd.Flags().BoolVar(&cfg.Update, "update", false, "Update Homebrew and packages")
	rootCmd.Flags().BoolVar(&cfg.Rollback, "rollback", false, "Restore backed up config files")
	rootCmd.Flags().StringVar(&cfg.Shell, "shell", "", "Shell setup: install, skip")
	rootCmd.Flags().StringVar(&cfg.Macos, "macos", "", "macOS prefs: configure, skip")
	rootCmd.Flags().StringVar(&cfg.Dotfiles, "dotfiles", "", "Dotfiles: clone, link, skip")
	rootCmd.Flags().BoolVar(&cfg.Resume, "resume", false, "Resume incomplete installation")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(snapshotCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("OpenBoot v%s\n", version)
	},
}

func Execute() error {
	return rootCmd.Execute()
}
