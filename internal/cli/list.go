package cli

import (
	"fmt"
	"strings"

	"github.com/openbootdotdev/openboot/internal/auth"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List your configs on openboot.dev",
	Long: `Show all configs stored in your openboot.dev account.

The config currently linked to this machine (via sync source) is marked
with an arrow. Use 'openboot config delete <slug>' to remove a config.`,
	Example:      `  openboot config list`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList()
	},
}

func init() {
	configCmd.AddCommand(listCmd)
}

func runList() error {
	apiBase := auth.GetAPIBase()

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

	configs, err := fetchUserConfigs(stored.Token, apiBase)
	if err != nil {
		return fmt.Errorf("fetch configs: %w", err)
	}

	// Resolve the locally linked slug so we can mark it.
	linkedSlug := ""
	if source, loadErr := syncpkg.LoadSource(); loadErr == nil && source != nil {
		linkedSlug = source.Slug
	}

	fmt.Println()
	ui.Header(fmt.Sprintf("Configs for %s", stored.Username))
	fmt.Println()

	if len(configs) == 0 {
		ui.Muted("  No configs yet. Run 'openboot snapshot --publish' to create one.")
		fmt.Println()
		return nil
	}

	for _, c := range configs {
		marker := "  "
		if c.Slug == linkedSlug {
			marker = "→ "
		}

		name := c.Name
		if name == "" || name == c.Slug {
			name = ""
		}

		line := fmt.Sprintf("%s%s", marker, ui.Cyan(c.Slug))
		if name != "" {
			line += fmt.Sprintf("  %s", name)
		}
		if c.Visibility != "" && c.Visibility != "unlisted" {
			line += fmt.Sprintf("  [%s]", c.Visibility)
		}
		fmt.Println(line)
	}

	fmt.Println()
	ui.Muted(fmt.Sprintf(
		"Install: openboot install %s/<slug>  •  Edit: openboot config edit --slug <slug>  •  Delete: openboot config delete <slug>",
		stored.Username,
	))
	fmt.Println()

	// Warn if linked config is no longer in the list.
	if linkedSlug != "" && !slugInList(configs, linkedSlug) {
		ui.Warn(fmt.Sprintf(
			"Note: this machine is linked to '%s' but that config no longer exists.",
			linkedSlug,
		))
		fmt.Println()
	}

	return nil
}

func slugInList(configs []remoteConfigSummary, slug string) bool {
	for _, c := range configs {
		if strings.EqualFold(c.Slug, slug) {
			return true
		}
	}
	return false
}
