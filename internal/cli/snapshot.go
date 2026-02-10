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
	snapshotCmd.Flags().Bool("dry-run", false, "Preview only, no save/upload")
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

	// --- CAPTURE PHASE ---

	// For --json: use silent Capture() (no progress UI, stdout must be clean)
	if jsonFlag {
		snap, err := snapshot.Capture()
		if err != nil {
			return err
		}
		catalogMatch := snapshot.MatchPackages(snap)
		snap.CatalogMatch = *catalogMatch
		snap.MatchedPreset = snapshot.DetectBestPreset(snap)
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal snapshot: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Interactive/local/dry-run: use progress UI
	snap, err := captureWithUI()
	if err != nil {
		return err
	}

	// Do matching
	catalogMatch := snapshot.MatchPackages(snap)
	snap.CatalogMatch = *catalogMatch
	snap.MatchedPreset = snapshot.DetectBestPreset(snap)

	if localFlag {
		path, err := snapshot.SaveLocal(snap)
		if err != nil {
			return fmt.Errorf("failed to save snapshot: %w", err)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapSuccessStyle.Render("✓ Snapshot saved to "+path))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	if dryRunFlag {
		showSnapshotPreview(snap)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Dry run — no changes made"))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	// --- EDITOR PHASE ---
	edited, confirmed, err := ui.RunSnapshotEditor(snap)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Snapshot cancelled."))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	// --- CONFIRM PHASE ---
	fmt.Fprintln(os.Stderr)
	upload, err := ui.Confirm("Upload this snapshot to openboot.dev?", false)
	if err != nil {
		return err
	}

	if !upload {
		// Offer local save as fallback
		saveLocal, err := ui.Confirm("Save snapshot locally instead?", true)
		if err != nil {
			return err
		}
		if saveLocal {
			path, err := snapshot.SaveLocal(edited)
			if err != nil {
				return fmt.Errorf("failed to save snapshot: %w", err)
			}
			fmt.Fprintln(os.Stderr, snapSuccessStyle.Render("✓ Snapshot saved to "+path))
		} else {
			fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Snapshot discarded."))
		}
		fmt.Fprintln(os.Stderr)
		return nil
	}

	// --- UPLOAD PHASE ---
	return uploadSnapshot(edited)
}

// captureWithUI runs CaptureWithProgress with the ScanProgress renderer.
func captureWithUI() (*snapshot.Snapshot, error) {
	fmt.Fprintln(os.Stderr)

	progress := ui.NewScanProgress(7)

	snap, err := snapshot.CaptureWithProgress(func(step snapshot.ScanStep) {
		progress.Update(step)
	})

	progress.Finish()

	if err != nil {
		return nil, fmt.Errorf("failed to capture snapshot: %w", err)
	}

	return snap, nil
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
		return fmt.Errorf("failed to load auth token: %w", err)
	}
	if stored == nil {
		return fmt.Errorf("no valid auth token found — please log in again")
	}

	fmt.Fprintln(os.Stderr)
	configName, err := ui.Input("Config name", "My Mac Setup")
	if err != nil {
		return fmt.Errorf("failed to get config name: %w", err)
	}
	configName = strings.TrimSpace(configName)
	if configName == "" {
		configName = "My Mac Setup"
	}

	reqBody := map[string]interface{}{
		"name":     configName,
		"snapshot": snap,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	uploadURL := fmt.Sprintf("%s/api/configs/from-snapshot", apiBase)
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", stored.Token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse upload response: %w", err)
	}

	// --- SUCCESS SCREEN ---
	configURL := fmt.Sprintf("%s/%s/%s", apiBase, stored.Username, result.Slug)
	installURL := fmt.Sprintf("curl -fsSL %s/%s/%s/install | bash", apiBase, stored.Username, result.Slug)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapSuccessStyle.Render("✓ Config uploaded successfully!"))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  View your config:"))
	fmt.Fprintf(os.Stderr, "    %s\n", configURL)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapBoldStyle.Render("  Share with others:"))
	fmt.Fprintf(os.Stderr, "    %s\n", installURL)
	fmt.Fprintln(os.Stderr)

	// Auto-open browser
	exec.Command("open", configURL).Start()
	fmt.Fprintln(os.Stderr, snapMutedStyle.Render("  Opening in browser..."))
	fmt.Fprintln(os.Stderr)

	return nil
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
	localPath := importPath
	if strings.HasPrefix(importPath, "http://") || strings.HasPrefix(importPath, "https://") {
		fmt.Fprintf(os.Stderr, "  Downloading snapshot from %s...\n", importPath)
		resp, err := http.Get(importPath)
		if err != nil {
			return fmt.Errorf("failed to download snapshot: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download snapshot: HTTP %d", resp.StatusCode)
		}
		tmpFile := filepath.Join(os.TempDir(), "openboot-snapshot-import.json")
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read snapshot response: %w", err)
		}
		if err := os.WriteFile(tmpFile, data, 0644); err != nil {
			return fmt.Errorf("failed to save snapshot: %w", err)
		}
		defer os.Remove(tmpFile)
		localPath = tmpFile
	}

	snap, err := snapshot.LoadFile(localPath)
	if err != nil {
		return err
	}

	catalogMatch := snapshot.MatchPackages(snap)
	snap.CatalogMatch = *catalogMatch
	snap.MatchedPreset = snapshot.DetectBestPreset(snap)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapTitleStyle.Render("=== Restoring from Snapshot ==="))
	fmt.Fprintf(os.Stderr, "  %s %s\n", snapBoldStyle.Render("Source:"), importPath)
	fmt.Fprintf(os.Stderr, "  %s %d formulae, %d casks, %d npm, %d taps\n",
		snapBoldStyle.Render("Packages:"),
		len(snap.Packages.Formulae), len(snap.Packages.Casks),
		len(snap.Packages.Npm), len(snap.Packages.Taps))
	fmt.Fprintln(os.Stderr)

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

	return installer.RunFromSnapshot(cfg)
}
