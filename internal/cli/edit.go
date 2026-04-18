package cli

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/charmbracelet/huh"
	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open a config on openboot.dev in the browser",
	Long: `Pick a config from your openboot.dev account and open it in the browser.

Use --slug to skip the picker and open a specific config directly.`,
	Example: `  # Pick a config interactively
  openboot config edit

  # Open a specific config directly
  openboot config edit --slug my-config`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		return runEdit(slug)
	},
}

func init() {
	editCmd.Flags().String("slug", "", "config slug to open (skips the picker)")
	configCmd.AddCommand(editCmd)
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
		slug, err = pickConfig(stored.Token, auth.GetAPIBase())
		if err != nil {
			return err
		}
		if slug == "" {
			ui.Info("No config selected.")
			return nil
		}
	}

	url := fmt.Sprintf("https://openboot.dev/dashboard/edit/%s", slug)

	if err := exec.Command("open", url).Run(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}

	ui.Success(fmt.Sprintf("Opened %s", url))
	return nil
}

// pickConfig fetches the user's configs and shows an interactive select list.
// Returns the chosen slug, or "" if the user has no configs.
func pickConfig(token, apiBase string) (string, error) {
	configs, _ := fetchUserConfigs(token, apiBase)

	if len(configs) == 0 {
		return "", fmt.Errorf("no configs found — run 'openboot push' to create one")
	}

	options := make([]string, 0, len(configs))
	for _, c := range configs {
		label := c.Slug
		if c.Name != "" && c.Name != c.Slug {
			label = fmt.Sprintf("%s — %s", c.Slug, c.Name)
		}
		options = append(options, label)
	}

	fmt.Println()
	choice, err := ui.SelectOption("Which config would you like to edit?", options)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", nil
		}
		return "", fmt.Errorf("select config: %w", err)
	}

	// Extract slug from "slug — Name" label
	slug := splitBefore(choice, " — ")
	return slug, nil
}

func splitBefore(s, sep string) string {
	for i := 0; i < len(s)-len(sep)+1; i++ {
		if s[i:i+len(sep)] == sep {
			return s[:i]
		}
	}
	return s
}
