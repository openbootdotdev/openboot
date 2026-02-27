package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Capture or restore your dev environment",
	Long: `Capture your Mac's Homebrew packages, npm globals, macOS preferences,
shell config, and dev tools into a portable JSON snapshot.

Export:
  openboot snapshot                            Capture interactively (save or upload)
  openboot snapshot --local                    Save to ~/.openboot/snapshot.json
  openboot snapshot --json > my-setup.json     Export as JSON

Import:
  openboot snapshot --import my-setup.json     Restore from a local file
  openboot snapshot --import https://...       Restore from a URL`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshot(cmd)
	},
}

func init() {
	snapshotCmd.Flags().Bool("local", false, "Save snapshot locally only")
	snapshotCmd.Flags().Bool("json", false, "Output as JSON to stdout")
	snapshotCmd.Flags().Bool("dry-run", false, "preview without installing or modifying anything")
	snapshotCmd.Flags().String("import", "", "Restore from a snapshot file or URL")
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
	jsonFlag, _ := cmd.Flags().GetBool("json")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")

	if jsonFlag {
		return captureJSONSnapshot()
	}

	snap, err := captureEnvironment()
	if err != nil {
		return err
	}

	if localFlag {
		path, err := snapshot.SaveLocal(snap)
		if err != nil {
			return fmt.Errorf("save snapshot: %w", err)
		}
		showLocalSaveSummary(snap, path)
		return nil
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

	fmt.Fprintln(os.Stderr)
	upload, err := ui.Confirm("Upload this snapshot to openboot.dev?", false)
	if err != nil {
		return err
	}

	if !upload {
		fmt.Fprintln(os.Stderr)
		saveLocal, err := ui.Confirm("Save snapshot locally instead?", true)
		if err != nil {
			return err
		}
		if saveLocal {
			path, err := snapshot.SaveLocal(edited)
			if err != nil {
				return fmt.Errorf("save snapshot: %w", err)
			}
			showLocalSaveSummary(edited, path)
		} else {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Snapshot discarded."))
			fmt.Fprintln(os.Stderr)
		}
		return nil
	}

	return uploadSnapshot(edited)
}

func captureJSONSnapshot() error {
	fmt.Fprintln(os.Stderr, "Capturing environment snapshot...")
	snap, err := snapshot.Capture()
	if err != nil {
		return err
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

	progress := ui.NewScanProgress(8)

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

func uploadSnapshot(snap *snapshot.Snapshot) error {
	apiBase := auth.GetAPIBase()

	if !auth.IsAuthenticated() {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "  You need to log in to upload your snapshot.\n")
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

	configName, configDesc, visibility, err := promptConfigDetails()
	if err != nil {
		return err
	}

	slug, err := postSnapshotToAPI(snap, configName, configDesc, visibility, stored.Token, apiBase)
	if err != nil {
		return err
	}

	configURL := fmt.Sprintf("%s/%s/%s", apiBase, stored.Username, slug)
	installURL := fmt.Sprintf("openboot -u %s/%s", stored.Username, slug)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapSuccessStyle.Render("✓ Config uploaded successfully!"))
	fmt.Fprintln(os.Stderr)
	showUploadedConfigInfo(visibility, configURL, installURL)
	fmt.Fprintln(os.Stderr)

	return nil
}

func promptConfigDetails() (string, string, string, error) {
	fmt.Fprintln(os.Stderr)
	configName, err := ui.Input("Config name", "My Mac Setup")
	if err != nil {
		return "", "", "", fmt.Errorf("get config name: %w", err)
	}
	configName = strings.TrimSpace(configName)
	if configName == "" {
		configName = "My Mac Setup"
	}

	fmt.Fprintln(os.Stderr)
	configDesc, err := ui.Input("Description (optional)", "")
	if err != nil {
		return "", "", "", fmt.Errorf("get config description: %w", err)
	}
	configDesc = strings.TrimSpace(configDesc)

	fmt.Fprintln(os.Stderr)
	visibilityOptions := []string{
		"Public - Anyone can discover and use this config",
		"Unlisted - Only people with the link can access",
		"Private - Only you can see this config",
	}
	visibilityChoice, err := ui.SelectOption("Who can see this config?", visibilityOptions)
	if err != nil {
		return "", "", "", fmt.Errorf("select visibility: %w", err)
	}

	visibility := "unlisted"
	if strings.HasPrefix(visibilityChoice, "Public") {
		visibility = "public"
	} else if strings.HasPrefix(visibilityChoice, "Private") {
		visibility = "private"
	}

	return configName, configDesc, visibility, nil
}

func postSnapshotToAPI(snap *snapshot.Snapshot, configName, configDesc, visibility, token, apiBase string) (string, error) {
	reqBody := map[string]interface{}{
		"name":        configName,
		"description": configDesc,
		"snapshot":    snap,
		"visibility":  visibility,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	uploadURL := fmt.Sprintf("%s/api/configs/from-snapshot", apiBase)
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("upload failed (status %d): read response: %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}
	return result.Slug, nil
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
		if err := exec.Command("open", configURL).Start(); err != nil {
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

	omzStatus := "not installed"
	if snap.Shell.OhMyZsh {
		theme := snap.Shell.Theme
		if theme == "" {
			theme = "default"
		}
		pluginCount := len(snap.Shell.Plugins)
		omzStatus = fmt.Sprintf("Oh-My-Zsh (%s theme, %d plugins)", theme, pluginCount)
	}
	fmt.Fprintf(os.Stderr, "  %s %s + %s\n",
		snapBoldStyle.Render("Shell:"),
		snap.Shell.Default, omzStatus)

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

	omzStatus := "not installed"
	if snap.Shell.OhMyZsh {
		omzStatus = "installed"
	}
	theme := snap.Shell.Theme
	if theme == "" {
		theme = "none"
	}
	plugins := "none"
	if len(snap.Shell.Plugins) > 0 {
		plugins = strings.Join(snap.Shell.Plugins, ", ")
	}
	fmt.Fprintf(os.Stderr, "  %s %s (Oh-My-Zsh: %s, Theme: %s, Plugins: %s)\n",
		snapBoldStyle.Render("Shell:"), snap.Shell.Default, omzStatus, theme, plugins)

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

func loadSnapshot(importPath string) (*snapshot.Snapshot, error) {
	localPath := importPath
	if strings.HasPrefix(importPath, "http://") || strings.HasPrefix(importPath, "https://") {
		fmt.Fprintf(os.Stderr, "  Downloading snapshot from %s...\n", importPath)
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(importPath)
		if err != nil {
			return nil, fmt.Errorf("download snapshot: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("download snapshot: HTTP %d", resp.StatusCode)
		}
		tmpFile := filepath.Join(os.TempDir(), "openboot-snapshot-import.json")
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read snapshot response: %w", err)
		}
		if err := os.WriteFile(tmpFile, data, 0644); err != nil {
			return nil, fmt.Errorf("save snapshot: %w", err)
		}
		defer os.Remove(tmpFile)
		localPath = tmpFile
	}

	snap, err := snapshot.LoadFile(localPath)
	if err != nil {
		return nil, err
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
	if snap.Shell.OhMyZsh {
		theme := snap.Shell.Theme
		if theme == "" {
			theme = "default"
		}
		plugins := "none"
		if len(snap.Shell.Plugins) > 0 {
			plugins = strings.Join(snap.Shell.Plugins, ", ")
		}
		fmt.Fprintf(os.Stderr, "  %s Oh-My-Zsh (theme: %s, plugins: %s)\n",
			snapBoldStyle.Render("Shell:"), theme, plugins)
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
		fmt.Fprintf(os.Stderr, "  %s", snapMutedStyle.Render("Proceed with installation? [y/N] (dry-run mode) "))
	} else {
		fmt.Fprintf(os.Stderr, "  %s", snapMutedStyle.Render("Proceed with installation? [y/N] "))
	}

	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))

	return response == "y" || response == "yes", nil
}

func buildImportConfig(edited *snapshot.Snapshot, dryRun bool) *config.Config {
	catalogSet := make(map[string]bool)
	for _, cat := range config.Categories {
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

	cfg.SnapshotShell = &config.SnapshotShellConfig{
		OhMyZsh: edited.Shell.OhMyZsh,
		Theme:   edited.Shell.Theme,
		Plugins: edited.Shell.Plugins,
	}

	cfg.SnapshotMacOS = make([]config.SnapshotMacOSPref, len(edited.MacOSPrefs))
	for i, p := range edited.MacOSPrefs {
		cfg.SnapshotMacOS[i] = config.SnapshotMacOSPref{
			Domain: p.Domain,
			Key:    p.Key,
			Value:  p.Value,
			Desc:   p.Desc,
		}
	}

	return cfg
}
