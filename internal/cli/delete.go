package cli

import (
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"time"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Delete a config from openboot.dev",
	Long:  `Delete a config from your openboot.dev account by its slug.`,
	Example: `  # Delete a config (with confirmation prompt)
  openboot delete my-config

  # Delete without confirmation (for scripting)
  openboot delete my-config --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return runDelete(args[0], force)
	},
}

func init() {
	deleteCmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(slug string, force bool) error {
	apiBase := auth.GetAPIBase()

	if !auth.IsAuthenticated() {
		fmt.Fprintln(os.Stderr)
		ui.Info("You need to log in to delete configs.")
		fmt.Fprintln(os.Stderr)
		if _, err := auth.LoginInteractive(apiBase); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	stored, err := auth.LoadToken()
	if err != nil {
		return fmt.Errorf("load auth token: %w", err)
	}
	if stored == nil {
		return fmt.Errorf("no valid auth token found — please log in again")
	}

	if !force {
		confirmed, err := ui.Confirm(
			fmt.Sprintf("Delete config '%s'? This cannot be undone", slug), false)
		if err != nil {
			return fmt.Errorf("confirm: %w", err)
		}
		if !confirmed {
			ui.Info("Delete cancelled.")
			return nil
		}
	}

	url := fmt.Sprintf("%s/api/configs/%s", apiBase, neturl.PathEscape(slug))

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+stored.Token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		fmt.Fprintln(os.Stderr)
		ui.Success(fmt.Sprintf("Config '%s' deleted.", slug))
		fmt.Fprintln(os.Stderr)
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("not authorized — please log in again with 'openboot login'")
	case http.StatusNotFound:
		return fmt.Errorf("config '%s' not found", slug)
	default:
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if readErr != nil {
			return fmt.Errorf("delete failed (status %d): read response: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("delete failed (status %d): %s", resp.StatusCode, string(body))
	}
}
