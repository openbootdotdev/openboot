package cli

import (
	"fmt"
	"os"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/spf13/cobra"
)

var (
	version = "0.14.0"
	cfg     = &config.Config{}
)

var rootCmd = &cobra.Command{
	Use:   "openboot",
	Short: "Set up your Mac dev environment in one command",
	Long: `OpenBoot sets up your Mac development environment in minutes.
Homebrew, CLI tools, GUI apps, npm packages, shell, and macOS preferences — all in one go.

Install:
  curl -fsSL https://openboot.dev/install | bash

Quick Start:
  openboot                              Interactive setup with package selection
  openboot snapshot                     Capture your current environment
  openboot snapshot --import setup.json Restore from a snapshot

Remote Config:
  openboot -u <github-username>         Install from your openboot.dev config
  openboot -p developer                 Use a built-in preset

Self-Update:
  openboot update --self                Update OpenBoot to the latest version`,
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
		err := installer.Run(cfg)
		if err == installer.ErrUserCancelled {
			return nil
		}
		return err
	},
}

func init() {
	rootCmd.Flags().StringVarP(&cfg.Preset, "preset", "p", "", "Use a preset (minimal, developer, full)")
	rootCmd.Flags().StringVarP(&cfg.User, "user", "u", "", "Install from openboot.dev user config")
	rootCmd.Flags().BoolVarP(&cfg.Silent, "silent", "s", false, "Non-interactive mode for CI")
	rootCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "preview without installing or modifying anything")
	rootCmd.Flags().BoolVar(&cfg.Update, "update", false, "Update Homebrew and upgrade packages")
	rootCmd.Flags().BoolVar(&cfg.Rollback, "rollback", false, "Restore backed-up config files")
	rootCmd.Flags().StringVar(&cfg.Shell, "shell", "", "Shell setup (install, skip)")
	rootCmd.Flags().StringVar(&cfg.Macos, "macos", "", "macOS preferences (configure, skip)")
	rootCmd.Flags().StringVar(&cfg.Dotfiles, "dotfiles", "", "Dotfiles (clone, link, skip)")
	rootCmd.Flags().BoolVar(&cfg.Resume, "resume", false, "Resume an incomplete installation")
	rootCmd.Flags().BoolVar(&cfg.PackagesOnly, "packages-only", false, "Install packages only, skip system configuration")

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
