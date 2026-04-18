package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// removedError returns a cobra RunE that reports a removed command with
// a migration hint.
func removedError(name, hint string) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		if hint == "" {
			return fmt.Errorf("'%s' has been removed in v1.0", name)
		}
		return fmt.Errorf("'%s' has been removed in v1.0 — %s", name, hint)
	}
}

var diffCmd = &cobra.Command{
	Use:          "diff",
	Short:        "[removed] Use 'openboot install --dry-run' instead",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("diff", "use 'openboot install --dry-run' to preview changes"),
}

var cleanCmd = &cobra.Command{
	Use:          "clean",
	Short:        "[removed] OpenBoot no longer manages package removal",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("clean", "OpenBoot no longer manages package removal. Use 'brew uninstall' directly"),
}

var logCmd = &cobra.Command{
	Use:          "log",
	Short:        "[removed] Version history is no longer supported",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("log", "version history is no longer supported"),
}

var restoreCmd = &cobra.Command{
	Use:          "restore",
	Short:        "[removed] Version history is no longer supported",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("restore", "version history is no longer supported"),
}

var initCmd = &cobra.Command{
	Use:          "init",
	Short:        "[removed] OpenBoot no longer scans project dependencies",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("init", "project-level dependency scanning was removed in v1.0 — use your project's own tooling (npm/pip/go/cargo)"),
}

var setupAgentCmd = &cobra.Command{
	Use:          "setup-agent",
	Short:        "[removed] AI agent integration is no longer bundled",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("setup-agent", "AI agent integration was tied to 'openboot init', which is removed"),
}

var doctorCmd = &cobra.Command{
	Use:          "doctor",
	Short:        "[removed] Use 'brew doctor' and 'git config' directly",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("doctor", "use 'brew doctor' and 'git config --list' directly"),
}

var updateCmd = &cobra.Command{
	Use:          "update",
	Short:        "[removed] Use 'brew upgrade' directly",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("update", "use 'brew upgrade' for packages; OpenBoot self-updates automatically on launch"),
}

func init() {
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(setupAgentCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(updateCmd)
}
