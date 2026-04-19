package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// publishSnapshot uploads a captured snapshot to openboot.dev. Slug resolution
// follows the v1.0 spec (P7):
//   - explicit --slug X → update X (error if X does not exist)
//   - no --slug, sync source exists → update the sync source's config
//   - otherwise → create new (prompts for name/description/visibility)
//
// Updating an existing config does not prompt for metadata; visibility is
// preserved from the existing config.
func publishSnapshot(snap *snapshot.Snapshot, explicitSlug string) error {
	apiBase := auth.GetAPIBase()

	if !auth.IsAuthenticated() {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  You need to log in to publish your snapshot.")
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

	targetSlug := resolveTargetSlug(explicitSlug)

	var configName, configDesc, visibility string
	if targetSlug != "" {
		fmt.Fprintln(os.Stderr)
		ui.Info(fmt.Sprintf("Publishing to @%s/%s (updating)", stored.Username, targetSlug))
	} else {
		fmt.Fprintln(os.Stderr)
		ui.Info("Publishing as a new config on openboot.dev")
		configName, configDesc, visibility, err = promptPushDetails("")
		if err != nil {
			return err
		}
	}

	resultSlug, err := postSnapshotToAPI(snap, configName, configDesc, visibility, stored.Token, apiBase, targetSlug)
	if err != nil {
		return err
	}

	recordPublishResult(stored.Username, resultSlug, targetSlug, visibility, apiBase)
	return nil
}

// resolveTargetSlug returns the slug to update: explicit arg takes precedence,
// then the current machine's sync source, then empty (create new).
func resolveTargetSlug(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if source, _ := syncpkg.LoadSource(); source != nil && source.Slug != "" {
		return source.Slug
	}
	return ""
}

// recordPublishResult saves the new config as the machine's sync source (on
// first publish) and prints the success output.
func recordPublishResult(username, resultSlug, targetSlug, visibility, apiBase string) {
	if targetSlug == "" && resultSlug != "" {
		src := &syncpkg.SyncSource{
			UserSlug:    fmt.Sprintf("%s/%s", username, resultSlug),
			Username:    username,
			Slug:        resultSlug,
			InstalledAt: time.Now(),
			SyncedAt:    time.Now(),
		}
		if err := syncpkg.SaveSource(src); err != nil {
			ui.Warn(fmt.Sprintf("Failed to save sync source: %v", err))
		}
	}

	configURL := fmt.Sprintf("%s/%s/%s", apiBase, username, resultSlug)
	installURL := fmt.Sprintf("openboot install %s/%s", username, resultSlug)

	fmt.Fprintln(os.Stderr)
	if targetSlug != "" {
		fmt.Fprintln(os.Stderr, snapSuccessStyle.Render("✓ Config updated successfully!"))
	} else {
		fmt.Fprintln(os.Stderr, snapSuccessStyle.Render("✓ Config published successfully!"))
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("  Future 'openboot snapshot --publish' will update this config."))
	}
	fmt.Fprintln(os.Stderr)
	showUploadedConfigInfo(visibility, configURL, installURL)
	fmt.Fprintln(os.Stderr)
}

func postSnapshotToAPI(snap *snapshot.Snapshot, configName, configDesc, visibility, token, apiBase, slug string) (string, error) {
	method := "POST"
	reqBody := map[string]interface{}{
		"name":        configName,
		"description": configDesc,
		"snapshot":    snap,
		"visibility":  visibility,
	}
	if slug != "" {
		method = "PUT"
		reqBody["config_slug"] = slug
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	uploadURL := fmt.Sprintf("%s/api/configs/from-snapshot", apiBase)
	req, err := http.NewRequest(method, uploadURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: apiRequestTimeout}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return "", fmt.Errorf("upload snapshot: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // standard HTTP body cleanup

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if readErr != nil {
			return "", fmt.Errorf("upload failed (status %d): read response: %w", resp.StatusCode, readErr)
		}
		if resp.StatusCode == http.StatusConflict {
			return "", parseConflictError(respBody)
		}
		return "", fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}
	resultSlug := result.Slug
	if resultSlug == "" {
		resultSlug = slug
	}
	return resultSlug, nil
}

func showUploadedConfigInfo(visibility, configURL, installURL string) {
	switch visibility {
	case "public":
		fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  View your config:"))
		fmt.Fprintf(os.Stderr, "    %s\n", configURL)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  Share with others:"))
		fmt.Fprintf(os.Stderr, "    %s\n", installURL)
		fmt.Fprintln(os.Stderr)
		if err := openBrowser(configURL); err != nil {
			ui.Warn(fmt.Sprintf("Could not open browser: %v", err))
		}
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("  Opening in browser..."))
	case "unlisted":
		fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  View your config:"))
		fmt.Fprintf(os.Stderr, "    %s\n", configURL)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  Share with people who have the link:"))
		fmt.Fprintf(os.Stderr, "    %s\n", installURL)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("  (This config is unlisted - only people with the link can access it)"))
	default:
		fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  Manage your config:"))
		fmt.Fprintf(os.Stderr, "    %s\n", configURL)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("  (This config is private - only you can see it)"))
	}
}
