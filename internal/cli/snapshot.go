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

	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Capture your dev environment",
	Long: `Capture your Mac's Homebrew packages, npm globals, macOS preferences,
shell config, and dev tools. The destination is chosen by flag or TTY:

  openboot snapshot              Interactive menu (TTY) or JSON to stdout (pipe)
  openboot snapshot --local      Save to ~/.openboot/snapshot.json
  openboot snapshot --publish    Upload to openboot.dev
  openboot snapshot --json       Output JSON to stdout

Restore:
  openboot snapshot --import my-setup.json   Restore from a local file
  openboot snapshot --import https://...     Restore from a URL`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshot(cmd)
	},
}

func init() {
	snapshotCmd.Flags().Bool("local", false, "save to ~/.openboot/snapshot.json")
	snapshotCmd.Flags().Bool("publish", false, "upload to openboot.dev")
	snapshotCmd.Flags().String("slug", "", "target an existing config by slug (with --publish)")
	snapshotCmd.Flags().Bool("json", false, "output JSON to stdout")
	snapshotCmd.Flags().Bool("dry-run", false, "preview without modifying anything")
	snapshotCmd.Flags().String("import", "", "restore from a snapshot file or URL")
}

// stderr-only styles so stdout stays clean for --json piping
var (
	snapTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true)
	snapSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	snapMutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	snapBoldStyle    = lipgloss.NewStyle().Bold(true)
)

func runSnapshot(cmd *cobra.Command) error {
	importFile, _ := cmd.Flags().GetString("import")
	if importFile != "" {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		return runSnapshotImport(importFile, dryRun)
	}

	localFlag, _ := cmd.Flags().GetBool("local")
	publishFlag, _ := cmd.Flags().GetBool("publish")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	slugFlag, _ := cmd.Flags().GetString("slug")

	// Explicit flags: route directly to the requested destination(s).
	// Multiple flags combine (e.g. --local --publish does both).
	if jsonFlag {
		return captureJSONSnapshot()
	}

	if localFlag || publishFlag {
		snap, err := captureEnvironment()
		if err != nil {
			return err
		}
		if dryRunFlag {
			showSnapshotPreview(snap)
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Dry run — no changes made"))
			fmt.Fprintln(os.Stderr)
			return nil
		}
		if localFlag {
			path, err := snapshot.SaveLocal(snap)
			if err != nil {
				return fmt.Errorf("save snapshot: %w", err)
			}
			showLocalSaveSummary(snap, path)
		}
		if publishFlag {
			if err := publishSnapshot(snap, slugFlag); err != nil {
				return err
			}
		}
		return nil
	}

	// No explicit flag: interactive if TTY, JSON to stdout otherwise.
	if !isStdoutTTY() {
		return captureJSONSnapshot()
	}

	snap, err := captureEnvironment()
	if err != nil {
		return err
	}

	if dryRunFlag {
		showSnapshotPreview(snap)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Dry run — no changes made"))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	edited, confirmed, err := reviewSnapshot(snap)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	return interactiveSaveOrPublish(edited)
}

// interactiveSaveOrPublish asks the user where to send the captured snapshot.
// This is the TTY-interactive path; explicit flags bypass it.
func interactiveSaveOrPublish(snap *snapshot.Snapshot) error {
	fmt.Fprintln(os.Stderr)
	options := []string{
		"Save locally (~/.openboot/snapshot.json)",
		"Publish to openboot.dev",
		"Save locally and publish",
		"Discard",
	}
	choice, err := ui.SelectOption("What to do with this snapshot?", options)
	if err != nil {
		return err
	}

	switch choice {
	case options[0]:
		path, err := snapshot.SaveLocal(snap)
		if err != nil {
			return fmt.Errorf("save snapshot: %w", err)
		}
		showLocalSaveSummary(snap, path)
	case options[1]:
		return publishSnapshot(snap, "")
	case options[2]:
		path, err := snapshot.SaveLocal(snap)
		if err != nil {
			return fmt.Errorf("save snapshot: %w", err)
		}
		showLocalSaveSummary(snap, path)
		return publishSnapshot(snap, "")
	case options[3]:
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Snapshot discarded."))
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

// isStdoutTTY returns true when stdout is an interactive terminal.
func isStdoutTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func captureJSONSnapshot() error {
	fmt.Fprintln(os.Stderr, "Capturing environment snapshot...")
	snap, err := snapshot.Capture()
	if err != nil {
		return fmt.Errorf("capture snapshot: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Matching packages with catalog...")
	catalogMatch := snapshot.MatchPackages(snap)
	snap.CatalogMatch = *catalogMatch
	snap.MatchedPreset = snapshot.DetectBestPreset(snap)
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ Snapshot complete")
	fmt.Println(string(data))
	return nil
}

func captureEnvironment() (*snapshot.Snapshot, error) {
	snap, err := captureWithUI()
	if err != nil {
		return nil, err
	}
	catalogMatch := snapshot.MatchPackages(snap)
	snap.CatalogMatch = *catalogMatch
	snap.MatchedPreset = snapshot.DetectBestPreset(snap)
	return snap, nil
}

// captureWithUI runs CaptureWithProgress with the ScanProgress renderer.
func captureWithUI() (*snapshot.Snapshot, error) {
	fmt.Fprintln(os.Stderr)

	progress := ui.NewScanProgress(9)

	snap, err := snapshot.CaptureWithProgress(func(step snapshot.ScanStep) {
		progress.Update(step)
	})

	progress.Finish()

	if err != nil {
		return nil, fmt.Errorf("capture snapshot: %w", err)
	}

	if snap.Health.Partial {
		fmt.Fprintln(os.Stderr)
		ui.Warn(fmt.Sprintf("Snapshot is partial — %d step(s) failed: %s",
			len(snap.Health.FailedSteps),
			strings.Join(snap.Health.FailedSteps, ", ")))
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("  The snapshot was saved but may be incomplete."))
	}

	return snap, nil
}

func reviewSnapshot(snap *snapshot.Snapshot) (*snapshot.Snapshot, bool, error) {
	edited, confirmed, err := ui.RunSnapshotEditor(snap)
	if err != nil {
		return nil, false, err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Snapshot cancelled."))
		fmt.Fprintln(os.Stderr)
		return nil, false, nil
	}
	showSnapshotSummary(edited)
	return edited, true, nil
}

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

func showLocalSaveSummary(snap *snapshot.Snapshot, path string) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapSuccessStyle.Render("✓ Snapshot saved successfully!"))
	fmt.Fprintln(os.Stderr)

	totalFormulae := len(snap.Packages.Formulae)
	totalCasks := len(snap.Packages.Casks)
	totalTaps := len(snap.Packages.Taps)
	totalNpm := len(snap.Packages.Npm)

	fmt.Fprintf(os.Stderr, "  %s %d formulae, %d casks, %d taps, %d npm\n",
		snapBoldStyle.Render("Saved:"),
		totalFormulae, totalCasks, totalTaps, totalNpm)

	if snap.MatchedPreset != "" {
		matchRate := int(snap.CatalogMatch.MatchRate * 100)
		fmt.Fprintf(os.Stderr, "  %s Matches \"%s\" (%d%% similarity)\n",
			snapBoldStyle.Render("Preset:"),
			snap.MatchedPreset, matchRate)
	}

	fmt.Fprintf(os.Stderr, "  %s %s\n",
		snapBoldStyle.Render("Location:"),
		path)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  Restore this snapshot:"))
	fmt.Fprintf(os.Stderr, "    %s\n", snapMutedStyle.Render("openboot snapshot --import "+path))
	fmt.Fprintln(os.Stderr)
}

func showSnapshotSummary(snap *snapshot.Snapshot) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapTitleStyle.Render("=== Snapshot Summary ==="))
	fmt.Fprintln(os.Stderr)

	totalFormulae := len(snap.Packages.Formulae)
	totalCasks := len(snap.Packages.Casks)
	totalTaps := len(snap.Packages.Taps)
	totalNpm := len(snap.Packages.Npm)

	fmt.Fprintf(os.Stderr, "  %s %d formulae, %d casks, %d taps, %d npm packages\n",
		snapBoldStyle.Render("Packages:"),
		totalFormulae, totalCasks, totalTaps, totalNpm)

	if snap.MatchedPreset != "" {
		matchRate := int(snap.CatalogMatch.MatchRate * 100)
		fmt.Fprintf(os.Stderr, "  %s Matches \"%s\" (%d%% similarity)\n",
			snapBoldStyle.Render("Preset:"),
			snap.MatchedPreset, matchRate)
	} else {
		fmt.Fprintf(os.Stderr, "  %s Custom configuration\n",
			snapBoldStyle.Render("Preset:"))
	}

	if snap.Git.UserName != "" || snap.Git.UserEmail != "" {
		fmt.Fprintf(os.Stderr, "  %s %s <%s>\n",
			snapBoldStyle.Render("Git:"),
			snap.Git.UserName, snap.Git.UserEmail)
	} else {
		fmt.Fprintf(os.Stderr, "  %s Not configured\n",
			snapBoldStyle.Render("Git:"))
	}

	if len(snap.DevTools) > 0 {
		var toolNames []string
		for _, tool := range snap.DevTools {
			toolNames = append(toolNames, tool.Name)
		}
		fmt.Fprintf(os.Stderr, "  %s %s\n",
			snapBoldStyle.Render("Dev Tools:"),
			strings.Join(toolNames, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "  %s None detected\n",
			snapBoldStyle.Render("Dev Tools:"))
	}

	prefCount := len(snap.MacOSPrefs)
	fmt.Fprintf(os.Stderr, "  %s %d preferences captured\n",
		snapBoldStyle.Render("macOS:"),
		prefCount)

	fmt.Fprintln(os.Stderr)
	capturedTime := snap.CapturedAt.Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "  %s %s\n",
		snapMutedStyle.Render("Captured:"),
		snapMutedStyle.Render(capturedTime))

	fmt.Fprintln(os.Stderr)
}

func showSnapshotPreview(snap *snapshot.Snapshot) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapTitleStyle.Render("=== Snapshot Preview ==="))
	fmt.Fprintln(os.Stderr)

	fmt.Fprintf(os.Stderr, "  %s %d\n", snapBoldStyle.Render("Homebrew Formulae:"), len(snap.Packages.Formulae))
	printSnapshotList(snap.Packages.Formulae, 10)

	fmt.Fprintf(os.Stderr, "  %s %d\n", snapBoldStyle.Render("Homebrew Casks:"), len(snap.Packages.Casks))
	printSnapshotList(snap.Packages.Casks, 10)

	fmt.Fprintf(os.Stderr, "  %s %d\n", snapBoldStyle.Render("Taps:"), len(snap.Packages.Taps))
	printSnapshotList(snap.Packages.Taps, 10)

	fmt.Fprintf(os.Stderr, "  %s %d\n", snapBoldStyle.Render("NPM Packages:"), len(snap.Packages.Npm))
	printSnapshotList(snap.Packages.Npm, 10)

	fmt.Fprintf(os.Stderr, "  %s %d\n", snapBoldStyle.Render("macOS Preferences:"), len(snap.MacOSPrefs))
	for _, pref := range snap.MacOSPrefs {
		fmt.Fprintf(os.Stderr, "    %s.%s = %s\n", pref.Domain, pref.Key, pref.Value)
	}

	fmt.Fprintf(os.Stderr, "  %s %s <%s>\n",
		snapBoldStyle.Render("Git:"), snap.Git.UserName, snap.Git.UserEmail)

	fmt.Fprintf(os.Stderr, "  %s %d\n", snapBoldStyle.Render("Dev Tools:"), len(snap.DevTools))
	for _, tool := range snap.DevTools {
		fmt.Fprintf(os.Stderr, "    %s %s\n", tool.Name, tool.Version)
	}

}

func printSnapshotList(items []string, max int) {
	for i, item := range items {
		if i >= max {
			fmt.Fprintf(os.Stderr, "    ...and %d more\n", len(items)-max)
			break
		}
		fmt.Fprintf(os.Stderr, "    %s\n", item)
	}
}

func runSnapshotImport(importPath string, dryRun bool) error {
	snap, err := loadSnapshot(importPath)
	if err != nil {
		return err
	}

	if snap.Health.Partial {
		fmt.Fprintln(os.Stderr)
		ui.Warn(fmt.Sprintf("This snapshot is incomplete — %d capture step(s) failed: %s",
			len(snap.Health.FailedSteps),
			strings.Join(snap.Health.FailedSteps, ", ")))
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("  Some data may be missing. The restore will proceed with what was captured."))
		fmt.Fprintln(os.Stderr)
		proceed, err := ui.Confirm("Continue with partial snapshot?", false)
		if err != nil {
			return err
		}
		if !proceed {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Restore cancelled."))
			fmt.Fprintln(os.Stderr)
			return nil
		}
	}

	showRestoreInfo(snap, importPath)

	edited, confirmed, err := ui.RunSnapshotEditor(snap)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Restore cancelled."))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	ok, err := confirmInstallation(edited, dryRun)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Installation cancelled."))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	return installer.RunFromSnapshot(buildImportConfig(edited, dryRun))
}

func downloadSnapshotBytes(url string, client *http.Client) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("download snapshot request: %w", err)
	}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("download snapshot: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // standard HTTP body cleanup
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download snapshot: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read snapshot response: %w", err)
	}
	return data, nil
}

func loadSnapshot(importPath string) (*snapshot.Snapshot, error) {
	var snap *snapshot.Snapshot

	switch {
	case strings.HasPrefix(importPath, "http://"):
		return nil, fmt.Errorf("insecure HTTP not allowed for snapshot import — use https:// instead")
	case strings.HasPrefix(importPath, "https://"):
		fmt.Fprintf(os.Stderr, "  Downloading snapshot from %s...\n", importPath)
		data, err := downloadSnapshotBytes(importPath, &http.Client{Timeout: apiRequestTimeout})
		if err != nil {
			return nil, err
		}
		s, err := snapshot.ParseBytes(data)
		if err != nil {
			return nil, err
		}
		snap = s
	default:
		s, err := snapshot.LoadFile(importPath)
		if err != nil {
			return nil, err
		}
		snap = s
	}

	catalogMatch := snapshot.MatchPackages(snap)
	snap.CatalogMatch = *catalogMatch
	snap.MatchedPreset = snapshot.DetectBestPreset(snap)
	return snap, nil
}

func showRestoreInfo(snap *snapshot.Snapshot, source string) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapTitleStyle.Render("=== Restoring from Snapshot ==="))
	fmt.Fprintf(os.Stderr, "  %s %s\n", snapBoldStyle.Render("Source:"), source)
	fmt.Fprintf(os.Stderr, "  %s %d formulae, %d casks, %d npm, %d taps\n",
		snapBoldStyle.Render("Packages:"),
		len(snap.Packages.Formulae), len(snap.Packages.Casks),
		len(snap.Packages.Npm), len(snap.Packages.Taps))
	if snap.Git.UserName != "" || snap.Git.UserEmail != "" {
		fmt.Fprintf(os.Stderr, "  %s %s <%s>\n",
			snapBoldStyle.Render("Git:"), snap.Git.UserName, snap.Git.UserEmail)
	}
	fmt.Fprintln(os.Stderr)
}

func confirmInstallation(edited *snapshot.Snapshot, dryRun bool) (bool, error) {
	totalFormulae := len(edited.Packages.Formulae)
	totalCasks := len(edited.Packages.Casks)
	totalNpm := len(edited.Packages.Npm)
	totalTaps := len(edited.Packages.Taps)
	totalPkgs := totalFormulae + totalCasks + totalNpm + totalTaps

	fmt.Fprintln(os.Stderr)
	if dryRun {
		fmt.Fprintln(os.Stderr, snapTitleStyle.Render("=== Confirm Installation (DRY-RUN) ==="))
	} else {
		fmt.Fprintln(os.Stderr, snapTitleStyle.Render("=== Confirm Installation ==="))
	}
	fmt.Fprintf(os.Stderr, "  %s %d formulae, %d casks, %d npm, %d taps\n",
		snapBoldStyle.Render("About to install:"),
		totalFormulae, totalCasks, totalNpm, totalTaps)
	fmt.Fprintf(os.Stderr, "  %s %d total packages\n", snapBoldStyle.Render("Total:"), totalPkgs)
	fmt.Fprintln(os.Stderr)
	if dryRun {
		return true, nil
	}
	return ui.Confirm("Proceed with installation?", false)
}

func buildImportConfig(edited *snapshot.Snapshot, dryRun bool) *config.Config {
	catalogSet := make(map[string]bool)
	for _, cat := range config.GetCategories() {
		for _, pkg := range cat.Packages {
			catalogSet[pkg.Name] = true
		}
	}

	cfg := &config.Config{DryRun: dryRun}
	cfg.SelectedPkgs = make(map[string]bool)

	for _, name := range edited.Packages.Formulae {
		if catalogSet[name] {
			cfg.SelectedPkgs[name] = true
		} else {
			cfg.OnlinePkgs = append(cfg.OnlinePkgs, config.Package{Name: name})
		}
	}
	for _, name := range edited.Packages.Casks {
		if catalogSet[name] {
			cfg.SelectedPkgs[name] = true
		} else {
			cfg.OnlinePkgs = append(cfg.OnlinePkgs, config.Package{Name: name, IsCask: true})
		}
	}
	for _, name := range edited.Packages.Npm {
		if catalogSet[name] {
			cfg.SelectedPkgs[name] = true
		} else {
			cfg.OnlinePkgs = append(cfg.OnlinePkgs, config.Package{Name: name, IsNpm: true})
		}
	}

	cfg.SnapshotTaps = edited.Packages.Taps

	cfg.SnapshotGit = &config.SnapshotGitConfig{
		UserName:  edited.Git.UserName,
		UserEmail: edited.Git.UserEmail,
	}

	if edited.Dotfiles.RepoURL != "" {
		if err := config.ValidateDotfilesURL(edited.Dotfiles.RepoURL); err == nil {
			cfg.SnapshotDotfiles = edited.Dotfiles.RepoURL
			cfg.DotfilesURL = edited.Dotfiles.RepoURL
		}
		// Invalid URLs are silently skipped — validation at push time will catch them.
	}

	cfg.SnapshotMacOS = make([]config.RemoteMacOSPref, len(edited.MacOSPrefs))
	for i, p := range edited.MacOSPrefs {
		cfg.SnapshotMacOS[i] = config.RemoteMacOSPref{
			Domain: p.Domain,
			Key:    p.Key,
			Type:   p.Type,
			Value:  p.Value,
			Desc:   p.Desc,
		}
	}

	cfg.SnapshotShellOhMyZsh = edited.Shell.OhMyZsh
	cfg.SnapshotShellTheme = edited.Shell.Theme
	cfg.SnapshotShellPlugins = edited.Shell.Plugins

	return cfg
}
