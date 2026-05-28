package cli

import (
	"github.com/spf13/cobra"

	"github.com/openbootdotdev/openboot/internal/doctor"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and diagnose issues",
	Long: `Run diagnostic checks on your development environment.

Checks Homebrew, Git, Node.js, npm, shell configuration, Oh-My-Zsh,
PATH settings, and OpenBoot state. All checks are read-only.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		d := doctor.New(version)
		d.RunAll()
		return nil
	},
}
