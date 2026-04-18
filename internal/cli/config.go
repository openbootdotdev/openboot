package cli

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage your openboot.dev configs",
	Long: `Manage configs on your openboot.dev account.

Configs are reusable Mac setup blueprints. Use:
  openboot config list       Show your configs
  openboot config edit       Open a config in the browser
  openboot config delete     Remove a config`,
	Example: `  openboot config list
  openboot config edit --slug my-setup
  openboot config delete old-setup`,
}

func init() {
	rootCmd.AddCommand(configCmd)
}
