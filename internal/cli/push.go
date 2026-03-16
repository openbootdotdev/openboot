package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push <file>",
	Short: "Upload a local config or snapshot file to openboot.dev",
	Long: `Upload a local JSON file to openboot.dev as a config.

Accepts both config and snapshot formats (auto-detected).
Use --slug to update an existing config instead of creating a new one.`,
	Example: `  # Upload a new config
  openboot push config.json

  # Update an existing config
  openboot push config.json --slug my-config`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		return runPush(args[0], slug)
	},
}

func init() {
	pushCmd.Flags().String("slug", "", "update an existing config by slug")
	rootCmd.AddCommand(pushCmd)
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

	name, desc, visibility, err := promptPushDetails()
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
	var rc config.RemoteConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if err := rc.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	name, desc, visibility, err := promptPushDetails()
	if err != nil {
		return err
	}

	// Convert RemoteConfig to API format: packages as [{name, type}]
	packages := remoteConfigToAPIPackages(&rc)

	reqBody := map[string]interface{}{
		"name":        name,
		"description": desc,
		"packages":    packages,
		"visibility":  visibility,
	}
	if len(rc.Taps) > 0 {
		reqBody["taps"] = rc.Taps
	}
	if rc.DotfilesRepo != "" {
		reqBody["dotfiles_repo"] = rc.DotfilesRepo
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
		uploadURL = fmt.Sprintf("%s/api/configs/%s", apiBase, slug)
	} else {
		uploadURL = fmt.Sprintf("%s/api/configs", apiBase)
	}
	return doUpload(uploadURL, bodyBytes, token, username, slug)
}

type apiPackage struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func remoteConfigToAPIPackages(rc *config.RemoteConfig) []apiPackage {
	pkgs := make([]apiPackage, 0, len(rc.Packages)+len(rc.Casks)+len(rc.Npm))
	for _, p := range rc.Packages {
		pkgs = append(pkgs, apiPackage{Name: p, Type: "formula"})
	}
	for _, c := range rc.Casks {
		pkgs = append(pkgs, apiPackage{Name: c, Type: "cask"})
	}
	for _, n := range rc.Npm {
		pkgs = append(pkgs, apiPackage{Name: n, Type: "npm"})
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
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if readErr != nil {
			return fmt.Errorf("upload failed (status %d): read response: %w", resp.StatusCode, readErr)
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

func promptPushDetails() (string, string, string, error) {
	fmt.Fprintln(os.Stderr)
	name, err := ui.Input("Config name", "My Mac Setup")
	if err != nil {
		return "", "", "", fmt.Errorf("get config name: %w", err)
	}
	if name == "" {
		name = "My Mac Setup"
	}

	fmt.Fprintln(os.Stderr)
	desc, err := ui.Input("Description (optional)", "")
	if err != nil {
		return "", "", "", fmt.Errorf("get description: %w", err)
	}

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
