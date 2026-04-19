package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
)

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
