package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Capture and upload your Mac's current dev environment",
	Long: `Scan your Mac for installed Homebrew packages, macOS preferences,
shell configuration, and development tools.

The snapshot can be saved locally, printed as JSON, or uploaded to
openboot.dev as a configuration that others can install.

Examples:
  openboot snapshot              # Interactive: capture, preview, and upload
  openboot snapshot --local      # Save snapshot to ~/.openboot/snapshot.json
  openboot snapshot --json       # Output snapshot as JSON (for piping)
  openboot snapshot --dry-run    # Preview what would be captured`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshot(cmd)
	},
}

func init() {
	snapshotCmd.Flags().Bool("local", false, "Save snapshot locally only")
	snapshotCmd.Flags().Bool("json", false, "Output as JSON to stdout")
	snapshotCmd.Flags().Bool("dry-run", false, "Preview only, no save/upload")
}

// stderr-only styles so stdout stays clean for --json piping
var (
	snapTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true)
	snapSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	snapMutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	snapBoldStyle    = lipgloss.NewStyle().Bold(true)
)

func runSnapshot(cmd *cobra.Command) error {
	localFlag, _ := cmd.Flags().GetBool("local")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapTitleStyle.Render("=== Scanning your Mac... ==="))
	fmt.Fprintln(os.Stderr)

	fmt.Fprintf(os.Stderr, "  Capturing environment...\n")
	snap, err := snapshot.Capture()
	if err != nil {
		return fmt.Errorf("failed to capture snapshot: %w", err)
	}

	catalogMatch := snapshot.MatchPackages(snap)
	snap.CatalogMatch = *catalogMatch
	snap.MatchedPreset = snapshot.DetectBestPreset(snap)

	showSnapshotPreview(snap)

	if jsonFlag {
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal snapshot: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if localFlag {
		path, err := snapshot.SaveLocal(snap)
		if err != nil {
			return fmt.Errorf("failed to save snapshot: %w", err)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapSuccessStyle.Render(fmt.Sprintf("✓ Snapshot saved to %s", path)))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	if dryRunFlag {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Dry run — no changes made"))
		fmt.Fprintln(os.Stderr)
		return nil
	}

	return uploadSnapshot(snap)
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

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, snapSuccessStyle.Render(
		fmt.Sprintf("✓ Config uploaded! View at: %s/%s/%s", apiBase, stored.Username, result.Slug),
	))
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
