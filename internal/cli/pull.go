package cli

import (
	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull remote config and apply changes (alias for sync)",
	Long: `Fetch the latest remote config and apply changes to your local system.

Like 'git pull', this fetches from the remote and applies changes locally.
Run 'openboot push' to upload your current system state to openboot.dev.

The sync source is automatically saved when you run 'openboot install <config>'.
If no source is saved, falls back to your logged-in openboot.dev config.
You can override it with --source.`,
	Example: `  # Pull latest config changes
  openboot pull

  # Preview changes without applying
  openboot pull --dry-run

  # Pull and apply all changes without prompts (for CI)
  openboot pull --yes

  # Pull from a specific config
  openboot pull --source alice/my-setup`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSync(cmd)
	},
}

func init() {
	pullCmd.Flags().String("source", "", "override remote config source (alias or username/slug)")
	pullCmd.Flags().Bool("dry-run", false, "preview changes without applying")
	pullCmd.Flags().Bool("install-only", false, "only install missing packages, skip removal prompts")
	pullCmd.Flags().BoolP("yes", "y", false, "auto-confirm all prompts (non-interactive)")
}
