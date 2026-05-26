package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
)

var openBrowser = func(url string) error {
	return exec.Command("open", url).Run() //nolint:gosec // "open" is macOS system binary; url is validated by caller
}

// stderr-only styles so stdout stays clean for --json piping
var (
	snapTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true)
	snapSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	snapMutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	snapBoldStyle    = lipgloss.NewStyle().Bold(true)
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
			return fmt.Errorf("capture environment: %w", err)
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
			if err := publishSnapshot(cmd.Context(), snap, slugFlag); err != nil {
				return fmt.Errorf("publish snapshot: %w", err)
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
		return fmt.Errorf("capture environment: %w", err)
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
		return fmt.Errorf("review snapshot: %w", err)
	}
	if !confirmed {
		return nil
	}

	return interactiveSaveOrPublish(cmd.Context(), edited)
}

// isStdoutTTY returns true when stdout is an interactive terminal.
func isStdoutTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveSaveOrPublish asks the user where to send the captured snapshot.
// This is the TTY-interactive path; explicit flags bypass it.
func interactiveSaveOrPublish(ctx context.Context, snap *snapshot.Snapshot) error {
	fmt.Fprintln(os.Stderr)
	options := []string{
		"Save locally (~/.openboot/snapshot.json)",
		"Publish to openboot.dev",
		"Save locally and publish",
		"Discard",
	}
	choice, err := ui.SelectOption("What to do with this snapshot?", options)
	if err != nil {
		return fmt.Errorf("select snapshot action: %w", err)
	}

	switch choice {
	case options[0]:
		path, err := snapshot.SaveLocal(snap)
		if err != nil {
			return fmt.Errorf("save snapshot: %w", err)
		}
		showLocalSaveSummary(snap, path)
	case options[1]:
		return publishSnapshot(ctx, snap, "")
	case options[2]:
		path, err := snapshot.SaveLocal(snap)
		if err != nil {
			return fmt.Errorf("save snapshot: %w", err)
		}
		showLocalSaveSummary(snap, path)
		return publishSnapshot(ctx, snap, "")
	case options[3]:
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, snapMutedStyle.Render("Snapshot discarded."))
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

func promptPushDetails(defaultName string) (string, string, string, error) {
	var (
		name string
		err  error
	)

	fmt.Fprintln(os.Stderr)
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
		return nil, fmt.Errorf("capture with ui: %w", err)
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
		return nil, false, fmt.Errorf("snapshot editor: %w", err)
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

	setCount := 0
	for _, pref := range snap.MacOSPrefs {
		if !pref.Unset {
			setCount++
		}
	}
	fmt.Fprintf(os.Stderr, "  %s %d (%d set, %d unset)\n",
		snapBoldStyle.Render("macOS Preferences:"), len(snap.MacOSPrefs),
		setCount, len(snap.MacOSPrefs)-setCount)
	for _, pref := range snap.MacOSPrefs {
		marker := ""
		if pref.Unset {
			marker = " (unset)"
		}
		fmt.Fprintf(os.Stderr, "    %s.%s = %s%s\n", pref.Domain, pref.Key, pref.Value, marker)
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
