package cli

import (
	"fmt"
	"os/exec"

	"github.com/openbootdotdev/openboot/internal/auth"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open your config on openboot.dev in the browser",
	Long: `Open your remote config on openboot.dev in the default browser.

Resolves the config slug from the current sync source (set when you last ran
'openboot install' or 'openboot push'). Use --slug to open a specific config.`,
	Example: `  # Open your linked config in the browser
  openboot edit

  # Open a specific config
  openboot edit --slug my-config`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		return runEdit(slug)
	},
}

func init() {
	editCmd.Flags().String("slug", "", "config slug to open (default: current sync source)")
	rootCmd.AddCommand(editCmd)
}

func runEdit(slugOverride string) error {
	if !auth.IsAuthenticated() {
		return fmt.Errorf("not logged in — run 'openboot login' first")
	}

	stored, err := auth.LoadToken()
	if err != nil {
		return fmt.Errorf("load auth token: %w", err)
	}
	if stored == nil {
		return fmt.Errorf("no valid auth token found — please log in again")
	}

	slug := slugOverride
	if slug == "" {
		if source, loadErr := syncpkg.LoadSource(); loadErr == nil && source != nil && source.Slug != "" {
			slug = source.Slug
		}
	}
	if slug == "" {
		return fmt.Errorf("no config slug — use --slug or run 'openboot install <config>' first")
	}

	url := fmt.Sprintf("https://openboot.dev/%s/%s", stored.Username, slug)

	if err := exec.Command("open", url).Run(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}

	ui.Success(fmt.Sprintf("Opened %s", url))
	return nil
}
