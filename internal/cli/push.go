package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push [file]",
	Short: "Push your system state to openboot.dev",
	Long: `Upload your current system state (or a local config file) to openboot.dev.

Like 'git push', running without arguments captures a snapshot of your current
Mac environment and uploads it. If a sync source is configured (from a previous
'openboot install'), it updates that config automatically.

You can also push a local JSON file directly (config or snapshot format, auto-detected).
Use --slug to target a specific existing config.`,
	Example: `  # Push current system state (auto-capture snapshot)
  openboot push

  # Push a local config or snapshot file
  openboot push config.json

  # Push and update a specific existing config
  openboot push --slug my-config
  openboot push config.json --slug my-config`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		if len(args) == 0 {
			return runPushAuto(slug)
		}
		return runPush(args[0], slug)
	},
}

func init() {
	pushCmd.Flags().String("slug", "", "update an existing config by slug")
	rootCmd.AddCommand(pushCmd)
}

// runPushAuto captures the current system snapshot and uploads it to openboot.dev.
// If a sync source is configured, it updates that config; otherwise, creates a new one.
func runPushAuto(slugOverride string) error {
	apiBase := auth.GetAPIBase()

	if !auth.IsAuthenticated() {
		fmt.Fprintln(os.Stderr)
		ui.Info("You need to log in to upload configs.")
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

	// Capture current system state
	fmt.Fprintln(os.Stderr)
	ui.Header("Capturing system snapshot...")
	snap, err := captureEnvironment()
	if err != nil {
		return err
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	// Determine slug: use override, then sync source, then blank (create new)
	slug := slugOverride
	if slug == "" {
		if source, loadErr := syncpkg.LoadSource(); loadErr == nil && source != nil && source.Slug != "" {
			slug = source.Slug
		}
	}

	return pushSnapshot(data, slug, stored.Token, stored.Username, apiBase)
}

func runPush(filePath, slug string) error {
	apiBase := auth.GetAPIBase()

	if !auth.IsAuthenticated() {
		fmt.Fprintln(os.Stderr)
		ui.Info("You need to log in to upload configs.")
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

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Detect format
	var probe struct {
		CapturedAt string `json:"captured_at"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("parse file: %w", err)
	}

	if probe.CapturedAt != "" {
		return pushSnapshot(data, slug, stored.Token, stored.Username, apiBase)
	}
	return pushConfig(data, slug, stored.Token, stored.Username, apiBase)
}

func pushSnapshot(data []byte, slug, token, username, apiBase string) error {
	var snap snapshot.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("parse snapshot: %w", err)
	}

	name, desc, visibility, err := promptPushDetails("")
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{
		"name":        name,
		"description": desc,
		"snapshot":    snap,
		"visibility":  visibility,
	}
	if slug != "" {
		reqBody["config_slug"] = slug
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	uploadURL := fmt.Sprintf("%s/api/configs/from-snapshot", apiBase)
	return doUpload(uploadURL, bodyBytes, token, username, slug)
}

func pushConfig(data []byte, slug, token, username, apiBase string) error {
	rc, err := config.UnmarshalRemoteConfigFlexible(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if err := rc.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	name, desc, visibility, err := promptPushDetails("")
	if err != nil {
		return err
	}

	// Convert RemoteConfig to API format: packages as [{name, type}]
	packages := remoteConfigToAPIPackages(rc)

	reqBody := map[string]interface{}{
		"name":        name,
		"description": desc,
		"packages":    packages,
		"visibility":  visibility,
	}
	if rc.DotfilesRepo != "" {
		reqBody["dotfiles_repo"] = rc.DotfilesRepo
	}
	if len(rc.PostInstall) > 0 {
		reqBody["custom_script"] = strings.Join(rc.PostInstall, "\n")
	}
	if rc.Preset != "" {
		reqBody["base_preset"] = rc.Preset
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	var uploadURL string
	if slug != "" {
		uploadURL = fmt.Sprintf("%s/api/configs/%s", apiBase, url.PathEscape(slug))
	} else {
		uploadURL = fmt.Sprintf("%s/api/configs", apiBase)
	}
	return doUpload(uploadURL, bodyBytes, token, username, slug)
}

type apiPackage struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Desc string `json:"desc,omitempty"`
}

func remoteConfigToAPIPackages(rc *config.RemoteConfig) []apiPackage {
	totalCap := len(rc.Packages) + len(rc.Casks) + len(rc.Npm) + len(rc.Taps)
	pkgs := make([]apiPackage, 0, totalCap)
	appendEntries := func(entries config.PackageEntryList, typeName string) {
		for _, e := range entries {
			pkgs = append(pkgs, apiPackage{Name: e.Name, Type: typeName, Desc: e.Desc})
		}
	}
	appendEntries(rc.Packages, "formula")
	appendEntries(rc.Casks, "cask")
	appendEntries(rc.Npm, "npm")
	for _, t := range rc.Taps {
		pkgs = append(pkgs, apiPackage{Name: t, Type: "tap"})
	}
	return pkgs
}

func doUpload(url string, body []byte, token, username, slug string) error {
	method := "POST"
	if slug != "" {
		method = "PUT"
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if readErr != nil {
			return fmt.Errorf("upload failed (status %d): read response: %w", resp.StatusCode, readErr)
		}

		if resp.StatusCode == http.StatusConflict {
			var errResp struct {
				Message string `json:"message"`
				Error   string `json:"error"`
			}
			if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil {
				msg := errResp.Message
				if msg == "" {
					msg = errResp.Error
				}
				if msg != "" && strings.Contains(strings.ToLower(msg), "maximum") {
					return errors.New("config limit reached (max 20): delete an existing config with 'openboot delete <slug>' first")
				}
				if msg != "" {
					return errors.New(msg)
				}
			}
			return fmt.Errorf("conflict: %s", string(respBody))
		}

		return fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	resultSlug := result.Slug
	if resultSlug == "" {
		resultSlug = slug
	}

	fmt.Fprintln(os.Stderr)
	ui.Success("Config uploaded successfully!")
	fmt.Fprintln(os.Stderr)
	if resultSlug != "" {
		fmt.Fprintf(os.Stderr, "  View:    https://openboot.dev/%s/%s\n", username, resultSlug)
		fmt.Fprintf(os.Stderr, "  Install: openboot -u %s/%s\n", username, resultSlug)
	}
	fmt.Fprintln(os.Stderr)

	return nil
}

func promptPushDetails(defaultName string) (string, string, string, error) {
	fmt.Fprintln(os.Stderr)
	var name string
	var err error
	if defaultName != "" {
		name, err = ui.InputWithDefault("Config name", "My Mac Setup", defaultName)
	} else {
		name, err = ui.Input("Config name", "My Mac Setup")
	}
	if err != nil {
		return "", "", "", fmt.Errorf("get config name: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "My Mac Setup"
	}

	fmt.Fprintln(os.Stderr)
	desc, err := ui.Input("Description (optional)", "")
	if err != nil {
		return "", "", "", fmt.Errorf("get description: %w", err)
	}
	desc = strings.TrimSpace(desc)

	fmt.Fprintln(os.Stderr)
	options := []string{
		"Public - Anyone can discover and use this config",
		"Unlisted - Only people with the link can access",
		"Private - Only you can see this config",
	}
	choice, err := ui.SelectOption("Who can see this config?", options)
	if err != nil {
		return "", "", "", fmt.Errorf("select visibility: %w", err)
	}

	visibility := "unlisted"
	switch {
	case strings.HasPrefix(choice, "Public"):
		visibility = "public"
	case strings.HasPrefix(choice, "Private"):
		visibility = "private"
	}

	return name, desc, visibility, nil
}
