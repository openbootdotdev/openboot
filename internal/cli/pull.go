package cli

import (
	"github.com/spf13/cobra"
)

// pullCmd is retained only to print a removal error — its functionality
// has moved to `openboot install`.
var pullCmd = &cobra.Command{
	Use:          "pull",
	Short:        "[removed] Use 'openboot install' instead",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("pull", "use 'openboot install' to sync from your saved config"),
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
