package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

// CompareSnapshots performs a full diff between the current system snapshot and a reference snapshot.
func CompareSnapshots(system, reference *snapshot.Snapshot, source Source) *DiffResult {
	return &DiffResult{
		Source:   source,
		Packages: diffPackages(system, reference),
		MacOS:    diffMacOS(system.MacOSPrefs, reference.MacOSPrefs),
		DevTools: diffDevTools(system.DevTools, reference.DevTools),
		Dotfiles: diffDotfiles(system.Dotfiles.RepoURL, reference.Dotfiles.RepoURL),
	}
}

// CompareSnapshotToRemote compares the system snapshot against a remote config.
// The remote API includes casks in the packages list, so we exclude them to get pure formulae.
func CompareSnapshotToRemote(system *snapshot.Snapshot, remote *config.RemoteConfig, source Source) *DiffResult {
	// Remote packages list contains both formulae and casks — filter out casks
	caskSet := toSet(remote.Casks.Names())
	var formulaeOnly []string
	for _, name := range remote.Packages.Names() {
		if !caskSet[name] {
			formulaeOnly = append(formulaeOnly, name)
		}
	}

	result := &DiffResult{
		Source: source,
		Packages: PackageDiff{
			Formulae: DiffLists(system.Packages.Formulae, formulaeOnly),
			Casks:    DiffLists(system.Packages.Casks, remote.Casks.Names()),
			Npm:      DiffLists(system.Packages.Npm, remote.Npm.Names()),
			Taps:     DiffLists(system.Packages.Taps, remote.Taps),
		},
	}

	// Dotfiles comparison
	result.Dotfiles = diffDotfiles(system.Dotfiles.RepoURL, remote.DotfilesRepo)

	// macOS preferences comparison
	if len(remote.MacOSPrefs) > 0 {
		refPrefs := make([]snapshot.MacOSPref, len(remote.MacOSPrefs))
		for i, p := range remote.MacOSPrefs {
			refPrefs[i] = snapshot.MacOSPref{
				Domain: p.Domain,
				Key:    p.Key,
				Type:   p.Type,
				Value:  p.Value,
				Desc:   p.Desc,
			}
		}
		result.MacOS = diffMacOS(system.MacOSPrefs, refPrefs)
	}

	// Shell configuration comparison — only when remote specifies oh-my-zsh
	if remote.Shell != nil && remote.Shell.OhMyZsh {
		result.Shell = diffShell(remote.Shell.Theme, remote.Shell.Plugins)
	}

	return result
}

// diffShell captures the local shell state and compares it against reference values.
func diffShell(refTheme string, refPlugins []string) *ShellDiff {
	local, err := snapshot.CaptureShell()
	if err != nil || local == nil {
		return nil
	}

	var sd *ShellDiff

	if refTheme != "" && refTheme != local.Theme {
		sd = &ShellDiff{
			ThemeChanged:     true,
			LocalTheme:       local.Theme,
			ReferenceTheme:   refTheme,
			LocalPlugins:     local.Plugins,
			ReferencePlugins: refPlugins,
		}
	}

	if len(refPlugins) > 0 && !pluginsEqual(refPlugins, local.Plugins) {
		if sd == nil {
			sd = &ShellDiff{
				LocalTheme:       local.Theme,
				ReferenceTheme:   refTheme,
				LocalPlugins:     local.Plugins,
				ReferencePlugins: refPlugins,
			}
		}
		sd.PluginsChanged = true
	}

	return sd
}

// pluginsEqual reports whether two plugin lists contain the same elements regardless of order.
func pluginsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, p := range a {
		set[p] = true
	}
	for _, p := range b {
		if !set[p] {
			return false
		}
	}
	return true
}

func diffPackages(system, reference *snapshot.Snapshot) PackageDiff {
	return PackageDiff{
		Formulae: DiffLists(system.Packages.Formulae, reference.Packages.Formulae),
		Casks:    DiffLists(system.Packages.Casks, reference.Packages.Casks),
		Npm:      DiffLists(system.Packages.Npm, reference.Packages.Npm),
		Taps:     DiffLists(system.Packages.Taps, reference.Packages.Taps),
	}
}

func diffMacOS(system, reference []snapshot.MacOSPref) *MacOSDiff {
	type prefKey struct {
		Domain string
		Key    string
	}

	sysMap := make(map[prefKey]string, len(system))
	for _, p := range system {
		sysMap[prefKey{p.Domain, p.Key}] = p.Value
	}

	refMap := make(map[prefKey]string, len(reference))
	for _, p := range reference {
		refMap[prefKey{p.Domain, p.Key}] = p.Value
	}

	md := &MacOSDiff{}

	// Find missing and changed
	for _, p := range reference {
		pk := prefKey{p.Domain, p.Key}
		sysVal, exists := sysMap[pk]
		if !exists {
			md.Missing = append(md.Missing, MacOSPrefEntry{
				Domain: p.Domain, Key: p.Key, Value: p.Value,
			})
		} else if sysVal != p.Value {
			md.Changed = append(md.Changed, MacOSPrefChange{
				Domain: p.Domain, Key: p.Key, System: sysVal, Reference: p.Value,
			})
		}
	}

	// Find extra (in system but not in reference)
	for _, p := range system {
		pk := prefKey{p.Domain, p.Key}
		if _, exists := refMap[pk]; !exists {
			md.Extra = append(md.Extra, MacOSPrefEntry{
				Domain: p.Domain, Key: p.Key, Value: p.Value,
			})
		}
	}

	return md
}

func diffDevTools(system, reference []snapshot.DevTool) *DevToolDiff {
	sysMap := make(map[string]string, len(system))
	for _, t := range system {
		sysMap[t.Name] = t.Version
	}

	refMap := make(map[string]string, len(reference))
	for _, t := range reference {
		refMap[t.Name] = t.Version
	}

	dd := &DevToolDiff{}

	// Find missing and changed
	for _, t := range reference {
		sysVer, exists := sysMap[t.Name]
		if !exists {
			dd.Missing = append(dd.Missing, t.Name)
		} else if sysVer != t.Version {
			dd.Changed = append(dd.Changed, DevToolDelta{
				Name: t.Name, System: sysVer, Reference: t.Version,
			})
		} else {
			dd.Common++
		}
	}

	// Find extra
	for _, t := range system {
		if _, exists := refMap[t.Name]; !exists {
			dd.Extra = append(dd.Extra, t.Name)
		}
	}

	sort.Strings(dd.Missing)
	sort.Strings(dd.Extra)

	return dd
}

// diffDotfiles compares dotfiles repo URLs and checks local repo health.
func diffDotfiles(systemURL, referenceURL string) *DotfilesDiff {
	dd := &DotfilesDiff{}

	// Compare repo URLs (normalize trailing .git)
	sysNorm := strings.TrimSuffix(strings.TrimSpace(systemURL), ".git")
	refNorm := strings.TrimSuffix(strings.TrimSpace(referenceURL), ".git")
	if sysNorm != refNorm && refNorm != "" {
		dd.RepoChanged = &ValueChange{System: systemURL, Reference: referenceURL}
	}

	// Only check local dotfiles repo state if dotfiles are actually configured
	// If both URLs are empty, there's no dotfiles setup to check
	if sysNorm == "" && refNorm == "" {
		return dd
	}

	// Check local dotfiles repo for dirty state
	home, err := os.UserHomeDir()
	if err != nil {
		return dd
	}
	dotfilesPath := filepath.Join(home, ".dotfiles")
	if _, err := os.Stat(filepath.Join(dotfilesPath, ".git")); err != nil {
		return dd
	}

	// Uncommitted changes (staged + unstaged + untracked)
	out, err := exec.Command("git", "-C", dotfilesPath, "status", "--porcelain").Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		dd.Dirty = true
	}

	// Unpushed commits
	out, err = exec.Command("git", "-C", dotfilesPath, "log", "--oneline", "@{upstream}..HEAD").Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		dd.Unpushed = true
	}

	return dd
}
