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

	"github.com/charmbracelet/huh"
	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

// pushCmd is retained only to print a removal error — its functionality
// has moved to `openboot snapshot --publish`.
var pushCmd = &cobra.Command{
	Use:          "push",
	Short:        "[removed] Use 'openboot snapshot --publish' instead",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         removedError("push", "use 'openboot snapshot --publish' to upload your current state"),
}

func init() {
	rootCmd.AddCommand(pushCmd)
}

// runPushAuto captures the current system snapshot and uploads it to openboot.dev.
// If a sync source is configured, it updates that config silently; otherwise, it
// presents an interactive picker so the user can choose an existing config or create a new one.
func runPushAuto(slugOverride, message string) error {
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

	// Determine slug: --slug flag → sync source → interactive picker
	slug := slugOverride
	if slug == "" {
		if source, loadErr := syncpkg.LoadSource(); loadErr == nil && source != nil && source.Slug != "" {
			slug = source.Slug
		}
	}
	if slug == "" {
		slug, err = pickOrCreateConfig(stored.Token, apiBase)
		if err != nil {
			return err
		}
	}

	return pushSnapshot(data, slug, message, stored.Token, stored.Username, apiBase)
}

func runPush(filePath, slug, message string) error {
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
		return pushSnapshot(data, slug, message, stored.Token, stored.Username, apiBase)
	}
	return pushConfig(data, slug, stored.Token, stored.Username, apiBase)
}

func pushSnapshot(data []byte, slug, message, token, username, apiBase string) error {
	var snap snapshot.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("parse snapshot: %w", err)
	}

	var name, desc, visibility string
	if slug == "" {
		// New config: show what will be uploaded, then prompt for details.
		fmt.Fprintln(os.Stderr)
		printSnapshotSummary(&snap)
		var err error
		name, desc, visibility, err = promptPushDetails("")
		if err != nil {
			return err
		}
	} else {
		// Updating existing: show diff vs remote and confirm.
		// showPushDiff prints its own "nothing to push" / "cancelled" messages.
		confirmed, err := showPushDiff(&snap, slug, username, token, apiBase)
		if err != nil {
			return fmt.Errorf("push diff: %w", err)
		}
		if !confirmed {
			return nil
		}
	}

	reqBody := map[string]interface{}{
		"snapshot":   snap,
		"visibility": visibility,
	}
	if name != "" {
		reqBody["name"] = name
	}
	if desc != "" {
		reqBody["description"] = desc
	}
	if slug != "" {
		reqBody["config_slug"] = slug
	}
	if message != "" {
		reqBody["message"] = message
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

type remoteConfigSummary struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}

// fetchUserConfigs calls GET /api/configs and returns the user's existing configs.
func fetchUserConfigs(token, apiBase string) ([]remoteConfigSummary, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+"/api/configs", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("fetch configs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil // non-fatal — fall through to create-new flow
	}

	var result struct {
		Configs []remoteConfigSummary `json:"configs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}
	return result.Configs, nil
}

const createNewOption = "+ Create a new config"

// pickOrCreateConfig shows an interactive list of the user's existing configs plus a
// "Create new" option. Returns the chosen slug (non-empty = update existing), or ""
// (= create new, caller must ask for name/desc/visibility).
func pickOrCreateConfig(token, apiBase string) (string, error) {
	configs, _ := fetchUserConfigs(token, apiBase) // ignore fetch errors — just show create-new

	if len(configs) == 0 {
		return "", nil // no existing configs — skip picker, go straight to create-new
	}

	options := make([]string, 0, len(configs)+1)
	for _, c := range configs {
		label := c.Slug
		if c.Name != "" && c.Name != c.Slug {
			label = fmt.Sprintf("%s — %s", c.Slug, c.Name)
		}
		options = append(options, label)
	}
	options = append(options, createNewOption)

	fmt.Fprintln(os.Stderr)
	choice, err := ui.SelectOption("Push to which config?", options)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", huh.ErrUserAborted
		}
		return "", fmt.Errorf("select config: %w", err)
	}

	if choice == createNewOption {
		return "", nil // caller will prompt for name/desc/visibility
	}

	// Extract slug from "slug — Name" label
	slug := strings.SplitN(choice, " — ", 2)[0]
	return slug, nil
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
		if errors.Is(err, huh.ErrUserAborted) {
			return "", "", "", huh.ErrUserAborted
		}
		return "", "", "", fmt.Errorf("get config name: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "My Mac Setup"
	}

	fmt.Fprintln(os.Stderr)
	desc, err := ui.Input("Description (optional)", "")
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", "", "", huh.ErrUserAborted
		}
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
		if errors.Is(err, huh.ErrUserAborted) {
			return "", "", "", huh.ErrUserAborted
		}
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

// showPushDiff fetches the existing remote config, shows what will change, and
// asks for confirmation. Prints its own "nothing to push" / "cancelled" messages
// so the caller can just return nil when false.
func showPushDiff(snap *snapshot.Snapshot, slug, username, token, apiBase string) (bool, error) {
	fmt.Fprintln(os.Stderr)

	rc, fetchErr := config.FetchRemoteConfig(username+"/"+slug, token)
	if fetchErr != nil {
		// Can't fetch remote — fall back to showing upload summary.
		printSnapshotSummary(snap)
		confirmed, err := ui.Confirm("Push these changes?", true)
		if err != nil {
			return false, err
		}
		if !confirmed {
			ui.Info("Push cancelled.")
		}
		return confirmed, nil
	}

	// Package diff
	addFormulae, rmFormulae := diffSets(snap.Packages.Formulae, rc.Packages.Names())
	addCasks, rmCasks := diffSets(snap.Packages.Casks, rc.Casks.Names())
	addNpm, rmNpm := diffSets(snap.Packages.Npm, rc.Npm.Names())
	addTaps, rmTaps := diffSets(snap.Packages.Taps, rc.Taps)

	pkgTotal := len(addFormulae) + len(rmFormulae) + len(addCasks) + len(rmCasks) +
		len(addNpm) + len(rmNpm) + len(addTaps) + len(rmTaps)

	// macOS prefs diff: key by "domain.key"
	macChanged := diffMacOSPrefs(snap.MacOSPrefs, rc.MacOSPrefs)

	// Dotfiles diff
	dotfilesChanged := ""
	if snap.Dotfiles.RepoURL != rc.DotfilesRepo {
		dotfilesChanged = fmt.Sprintf("%s → %s", fallback(rc.DotfilesRepo, "(none)"), fallback(snap.Dotfiles.RepoURL, "(none)"))
	}

	total := pkgTotal + len(macChanged)
	if dotfilesChanged != "" {
		total++
	}

	if total == 0 {
		ui.Success("Nothing to push — remote config matches your system.")
		return false, nil
	}

	if pkgTotal > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", ui.Green("Package Changes"))
		printPushCategory("Formulae", addFormulae, rmFormulae)
		printPushCategory("Casks", addCasks, rmCasks)
		printPushCategory("NPM", addNpm, rmNpm)
		printPushCategory("Taps", addTaps, rmTaps)
		fmt.Fprintln(os.Stderr)
	}

	if len(macChanged) > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", ui.Green("macOS Changes"))
		for _, line := range macChanged {
			fmt.Fprintf(os.Stderr, "    %s\n", line)
		}
		fmt.Fprintln(os.Stderr)
	}

	if dotfilesChanged != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", ui.Green("Dotfiles"))
		fmt.Fprintf(os.Stderr, "    Repo: %s\n", dotfilesChanged)
		fmt.Fprintln(os.Stderr)
	}

	confirmed, err := ui.Confirm(fmt.Sprintf("Push %d change(s) to remote config?", total), true)
	if err != nil {
		return false, err
	}
	if !confirmed {
		ui.Info("Push cancelled.")
	}
	return confirmed, nil
}

// fallback returns s if non-empty, otherwise def.
func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// diffMacOSPrefs returns human-readable lines for each changed/added/removed macOS pref.
func diffMacOSPrefs(local []snapshot.MacOSPref, remote []config.RemoteMacOSPref) []string {
	key := func(domain, k string) string { return domain + "." + k }
	localMap := make(map[string]snapshot.MacOSPref, len(local))
	for _, p := range local {
		localMap[key(p.Domain, p.Key)] = p
	}
	remoteMap := make(map[string]config.RemoteMacOSPref, len(remote))
	for _, p := range remote {
		remoteMap[key(p.Domain, p.Key)] = p
	}

	var lines []string
	seen := make(map[string]bool, len(local)+len(remote))
	for k, lp := range localMap {
		seen[k] = true
		rp, exists := remoteMap[k]
		if !exists {
			lines = append(lines, fmt.Sprintf("+ %s: %s", label(lp.Desc, k), lp.Value))
			continue
		}
		if rp.Value != lp.Value {
			lines = append(lines, fmt.Sprintf("~ %s: %s → %s", label(lp.Desc, k), rp.Value, lp.Value))
		}
	}
	for k, rp := range remoteMap {
		if seen[k] {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", label(rp.Desc, k), rp.Value))
	}
	return lines
}

func label(desc, fallbackKey string) string {
	if desc != "" {
		return desc
	}
	return fallbackKey
}

// diffSets returns (toAdd, toRemove): items in local but not remote, and items in remote but not local.
func diffSets(local, remote []string) (toAdd, toRemove []string) {
	localSet := syncpkg.ToSet(local)
	remoteSet := syncpkg.ToSet(remote)
	for _, item := range local {
		if !remoteSet[item] {
			toAdd = append(toAdd, item)
		}
	}
	for _, item := range remote {
		if !localSet[item] {
			toRemove = append(toRemove, item)
		}
	}
	return toAdd, toRemove
}

func printPushCategory(category string, toAdd, toRemove []string) {
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return
	}
	if len(toAdd) > 0 {
		fmt.Fprintf(os.Stderr, "    + %s (%d): %s\n", category, len(toAdd), strings.Join(toAdd, ", "))
	}
	if len(toRemove) > 0 {
		fmt.Fprintf(os.Stderr, "    - %s (%d): %s\n", category, len(toRemove), strings.Join(toRemove, ", "))
	}
}

// printSnapshotSummary shows a brief count of what's in the snapshot being uploaded.
func printSnapshotSummary(snap *snapshot.Snapshot) {
	fmt.Fprintf(os.Stderr, "  %s\n", ui.Green("Uploading"))
	parts := []string{}
	if n := len(snap.Packages.Formulae); n > 0 {
		parts = append(parts, fmt.Sprintf("%d formulae", n))
	}
	if n := len(snap.Packages.Casks); n > 0 {
		parts = append(parts, fmt.Sprintf("%d casks", n))
	}
	if n := len(snap.Packages.Npm); n > 0 {
		parts = append(parts, fmt.Sprintf("%d npm packages", n))
	}
	if n := len(snap.Packages.Taps); n > 0 {
		parts = append(parts, fmt.Sprintf("%d taps", n))
	}
	if len(parts) == 0 {
		fmt.Fprintln(os.Stderr, "    (no packages)")
	} else {
		fmt.Fprintf(os.Stderr, "    %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintln(os.Stderr)
}
