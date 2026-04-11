package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/httputil"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show revision history for your config",
	Long: `List the revision history for your synced config on openboot.dev.

Each time you run 'openboot push', the previous version is saved as a revision.
Use 'openboot restore <revision-id>' to roll back to a previous state.`,
	Example: `  # Show revision history for your current config
  openboot log

  # Show history for a specific config slug
  openboot log --slug my-config`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		return runLog(slug)
	},
}

func init() {
	logCmd.Flags().String("slug", "", "config slug to show history for (default: current sync source)")
	rootCmd.AddCommand(logCmd)
}

type revisionSummary struct {
	ID           string  `json:"id"`
	Message      *string `json:"message"`
	CreatedAt    string  `json:"created_at"`
	PackageCount int     `json:"package_count"`
}

func runLog(slugOverride string) error {
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

	slug := slugOverride
	if slug == "" {
		source, loadErr := syncpkg.LoadSource()
		if loadErr == nil && source != nil && source.Slug != "" {
			slug = source.Slug
		}
	}
	if slug == "" {
		return fmt.Errorf("no config slug — use --slug or run 'openboot install <config>' first")
	}

	reqURL := fmt.Sprintf("%s/api/configs/%s/revisions", apiBase, url.PathEscape(slug))
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+stored.Token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return fmt.Errorf("fetch revisions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("config %q not found", slug)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("fetch revisions failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Revisions []revisionSummary `json:"revisions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	fmt.Println()
	ui.Header(fmt.Sprintf("Revision history: %s", slug))
	fmt.Println()

	if len(result.Revisions) == 0 {
		ui.Muted("  No revisions yet. Push a config update to create one.")
		fmt.Println()
		return nil
	}

	for _, rev := range result.Revisions {
		ts := rev.CreatedAt
		if t, parseErr := time.Parse("2006-01-02T15:04:05Z", rev.CreatedAt); parseErr == nil {
			ts = t.Local().Format("2006-01-02 15:04")
		} else if t2, parseErr2 := time.Parse("2006-01-02 15:04:05", rev.CreatedAt); parseErr2 == nil {
			ts = t2.Local().Format("2006-01-02 15:04")
		}

		msg := ""
		if rev.Message != nil && *rev.Message != "" {
			msg = fmt.Sprintf("  %s", *rev.Message)
		}

		fmt.Printf("  %s  %s  %d pkgs%s\n",
			ui.Cyan(rev.ID),
			ts,
			rev.PackageCount,
			msg,
		)
	}

	fmt.Println()
	ui.Muted("Use 'openboot restore <revision-id>' to roll back to a previous state.")
	fmt.Println()

	return nil
}
