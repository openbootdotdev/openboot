package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/httputil"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <revision-id>",
	Short: "Restore your config to a previous revision",
	Long: `Roll back your config to a previous revision from openboot.dev.

The current config is automatically saved as a new revision before restoring,
so you can always undo. After updating the server config, the changes are
applied to your local system via sync.

Use 'openboot log' to see available revision IDs.`,
	Example: `  # Restore to a specific revision
  openboot restore rev_abc1234567890123

  # Restore for a specific config slug
  openboot restore rev_abc1234567890123 --slug my-config

  # Preview what would change without applying
  openboot restore rev_abc1234567890123 --dry-run`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		yes, _ := cmd.Flags().GetBool("yes")
		return runRestore(args[0], slug, dryRun, yes)
	},
}

func init() {
	restoreCmd.Flags().String("slug", "", "config slug to restore (default: current sync source)")
	restoreCmd.Flags().Bool("dry-run", false, "preview changes without applying")
	restoreCmd.Flags().BoolP("yes", "y", false, "auto-confirm all prompts")
	rootCmd.AddCommand(restoreCmd)
}

func runRestore(revisionID, slugOverride string, dryRun, yes bool) error {
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

	// Resolve slug from flag, sync source, or error.
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

	fmt.Fprintln(os.Stderr)
	ui.Header(fmt.Sprintf("Restoring %s to revision %s", slug, revisionID))
	fmt.Fprintln(os.Stderr)

	// 1. Tell server to restore the revision (saves current as a new revision first).
	restoreURL := fmt.Sprintf(
		"%s/api/configs/%s/revisions/%s/restore",
		apiBase,
		url.PathEscape(slug),
		url.PathEscape(revisionID),
	)

	req, err := http.NewRequest(http.MethodPost, restoreURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+stored.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}

	var restoreResp struct {
		Restored   bool   `json:"restored"`
		RevisionID string `json:"revision_id"`
		Packages   []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"packages"`
	}

	if !dryRun {
		resp, doErr := httputil.Do(client, req)
		if doErr != nil {
			return fmt.Errorf("restore request: %w", doErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
			var errResp struct{ Error string `json:"error"` }
			if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error != "" {
				return fmt.Errorf("%s", errResp.Error)
			}
			return fmt.Errorf("revision %q not found for config %q", revisionID, slug)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
			return fmt.Errorf("restore failed (status %d): %s", resp.StatusCode, string(body))
		}

		if decErr := json.NewDecoder(resp.Body).Decode(&restoreResp); decErr != nil {
			return fmt.Errorf("parse restore response: %w", decErr)
		}
		ui.Success("Server config restored to revision")
	} else {
		// Dry-run: fetch the revision to preview what packages would be applied.
		revURL := fmt.Sprintf(
			"%s/api/configs/%s/revisions/%s",
			apiBase,
			url.PathEscape(slug),
			url.PathEscape(revisionID),
		)
		revReq, revErr := http.NewRequest(http.MethodGet, revURL, nil)
		if revErr != nil {
			return fmt.Errorf("build request: %w", revErr)
		}
		revReq.Header.Set("Authorization", "Bearer "+stored.Token)

		revResp, doErr := httputil.Do(client, revReq)
		if doErr != nil {
			return fmt.Errorf("fetch revision: %w", doErr)
		}
		defer revResp.Body.Close()

		if revResp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("revision %q not found for config %q", revisionID, slug)
		}
		if revResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(revResp.Body, 4<<10))
			return fmt.Errorf("fetch revision failed (status %d): %s", revResp.StatusCode, string(body))
		}

		if decErr := json.NewDecoder(revResp.Body).Decode(&restoreResp); decErr != nil {
			return fmt.Errorf("parse revision response: %w", decErr)
		}
		ui.Info(fmt.Sprintf("(dry-run) Would restore to %d packages from revision %s", len(restoreResp.Packages), revisionID))
	}

	// 2. Build a RemoteConfig from the revision packages and run local sync.
	rc := revisionPackagesToRemoteConfig(restoreResp.Packages, stored.Username, slug)

	diff, err := syncpkg.ComputeDiff(rc)
	if err != nil {
		return fmt.Errorf("compute diff: %w", err)
	}

	if !diff.HasChanges() {
		ui.Success("Your system already matches this revision — nothing to do.")
		fmt.Fprintln(os.Stderr)
		return nil
	}

	fmt.Fprintln(os.Stderr)
	printSyncDiff(diff)

	plan, err := buildSyncPlan(diff, rc, dryRun, false, yes)
	if err != nil {
		return err
	}

	if plan.IsEmpty() {
		ui.Info("No changes selected.")
		return nil
	}

	if !dryRun && !yes {
		confirmed, confErr := ui.Confirm(fmt.Sprintf("Apply %d changes?", plan.TotalActions()), true)
		if confErr != nil {
			return fmt.Errorf("confirm: %w", confErr)
		}
		if !confirmed {
			ui.Info("Restore cancelled.")
			return nil
		}
	}

	fmt.Fprintln(os.Stderr)
	result, execErr := syncpkg.Execute(plan, dryRun)

	fmt.Fprintln(os.Stderr)
	if result.Installed > 0 {
		ui.Success(fmt.Sprintf("Installed %d package(s)", result.Installed))
	}
	if result.Uninstalled > 0 {
		ui.Success(fmt.Sprintf("Removed %d package(s)", result.Uninstalled))
	}
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			ui.Error(fmt.Sprintf("Failed: %s", e))
		}
	}

	return execErr
}

// revisionPackagesToRemoteConfig converts a flat list of typed packages into a RemoteConfig
// so it can be fed into the existing sync pipeline.
func revisionPackagesToRemoteConfig(pkgs []struct {
	Name string `json:"name"`
	Type string `json:"type"`
}, username, slug string) *config.RemoteConfig {
	rc := &config.RemoteConfig{
		Username: username,
		Slug:     slug,
	}
	for _, p := range pkgs {
		entry := config.PackageEntry{Name: p.Name}
		switch p.Type {
		case "formula":
			rc.Packages = append(rc.Packages, entry)
		case "cask":
			rc.Casks = append(rc.Casks, entry)
		case "npm":
			rc.Npm = append(rc.Npm, entry)
		case "tap":
			rc.Taps = append(rc.Taps, p.Name)
		}
	}
	return rc
}
