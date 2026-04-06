package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

// SyncDiff holds all differences between the remote config and the local system.
type SyncDiff struct {
	// Packages
	MissingFormulae []string
	MissingCasks    []string
	MissingNpm      []string
	MissingTaps     []string
	ExtraFormulae   []string
	ExtraCasks      []string
	ExtraNpm        []string
	ExtraTaps       []string

	// Dotfiles
	DotfilesChanged bool
	RemoteDotfiles  string
	LocalDotfiles   string

	// macOS Preferences
	MacOSChanged []MacOSPrefDiff

	// Shell (non-nil when theme or plugins differ from remote)
	Shell *ShellDiff
}

// ShellDiff records a shell config difference between remote and local.
// RemoteTheme/RemotePlugins always reflect the remote config values.
type ShellDiff struct {
	ThemeChanged   bool
	RemoteTheme    string
	LocalTheme     string
	PluginsChanged bool
	RemotePlugins  []string
	LocalPlugins   []string
}

// MacOSPrefDiff records a single macOS preference that differs.
type MacOSPrefDiff struct {
	Domain      string
	Key         string
	Type        string
	Desc        string
	RemoteValue string
	LocalValue  string
}

// HasChanges reports whether the diff contains any differences at all.
func (d *SyncDiff) HasChanges() bool {
	return len(d.MissingFormulae) > 0 ||
		len(d.MissingCasks) > 0 ||
		len(d.MissingNpm) > 0 ||
		len(d.MissingTaps) > 0 ||
		len(d.ExtraFormulae) > 0 ||
		len(d.ExtraCasks) > 0 ||
		len(d.ExtraNpm) > 0 ||
		len(d.ExtraTaps) > 0 ||
		d.DotfilesChanged ||
		len(d.MacOSChanged) > 0 ||
		d.Shell != nil
}

// TotalMissing returns the count of items in remote but not on the local system.
func (d *SyncDiff) TotalMissing() int {
	return len(d.MissingFormulae) + len(d.MissingCasks) + len(d.MissingNpm) + len(d.MissingTaps)
}

// TotalExtra returns the count of items on the local system but not in remote.
func (d *SyncDiff) TotalExtra() int {
	return len(d.ExtraFormulae) + len(d.ExtraCasks) + len(d.ExtraNpm) + len(d.ExtraTaps)
}

// TotalChanged returns the count of values that differ (theme, dotfiles, macOS prefs, shell).
func (d *SyncDiff) TotalChanged() int {
	n := len(d.MacOSChanged)
	if d.DotfilesChanged {
		n++
	}
	if d.Shell != nil {
		n++
	}
	return n
}

// ComputeDiff compares a remote config against the local system state.
func ComputeDiff(rc *config.RemoteConfig) (*SyncDiff, error) {
	d := &SyncDiff{}

	// Capture local package state — fail fast on errors to prevent
	// false positives (showing everything as "missing" if brew is down).
	localFormulae, err := snapshot.CaptureFormulae()
	if err != nil {
		return nil, fmt.Errorf("capture local formulae: %w", err)
	}
	localCasks, err := snapshot.CaptureCasks()
	if err != nil {
		return nil, fmt.Errorf("capture local casks: %w", err)
	}
	localTaps, err := snapshot.CaptureTaps()
	if err != nil {
		return nil, fmt.Errorf("capture local taps: %w", err)
	}
	localNpm, err := snapshot.CaptureNpm()
	if err != nil {
		return nil, fmt.Errorf("capture local npm: %w", err)
	}

	// Package diffs — exclude cask names from formulae comparison
	casksSet := ToSet(rc.Casks.Names())
	remoteFormulae := make([]string, 0, len(rc.Packages))
	for _, p := range rc.Packages {
		if !casksSet[p.Name] {
			remoteFormulae = append(remoteFormulae, p.Name)
		}
	}
	d.MissingFormulae, d.ExtraFormulae = diffLists(remoteFormulae, localFormulae)
	d.MissingCasks, d.ExtraCasks = diffLists(rc.Casks.Names(), localCasks)
	d.MissingTaps, d.ExtraTaps = diffLists(rc.Taps, localTaps)
	d.MissingNpm, d.ExtraNpm = diffLists(rc.Npm.Names(), localNpm)

	// Dotfiles diff
	if rc.DotfilesRepo != "" {
		localURL := getLocalDotfilesURL()
		if localURL != rc.DotfilesRepo {
			d.DotfilesChanged = true
			d.RemoteDotfiles = rc.DotfilesRepo
			d.LocalDotfiles = localURL
		}
	}

	// Shell diff — only when remote config specifies oh-my-zsh
	if rc.Shell != nil && rc.Shell.OhMyZsh {
		localShell, err := snapshot.CaptureShell()
		if err != nil {
			return nil, fmt.Errorf("capture local shell: %w", err)
		}
		var sd *ShellDiff
		if rc.Shell.Theme != "" && rc.Shell.Theme != localShell.Theme {
			if sd == nil {
				sd = &ShellDiff{RemoteTheme: rc.Shell.Theme, LocalTheme: localShell.Theme, RemotePlugins: rc.Shell.Plugins, LocalPlugins: localShell.Plugins}
			}
			sd.ThemeChanged = true
		}
		if len(rc.Shell.Plugins) > 0 && !pluginsEqual(rc.Shell.Plugins, localShell.Plugins) {
			if sd == nil {
				sd = &ShellDiff{RemoteTheme: rc.Shell.Theme, LocalTheme: localShell.Theme, RemotePlugins: rc.Shell.Plugins, LocalPlugins: localShell.Plugins}
			}
			sd.PluginsChanged = true
		}
		d.Shell = sd
	}

	// macOS prefs diff
	if len(rc.MacOSPrefs) > 0 {
		localPrefs, prefsErr := snapshot.CaptureMacOSPrefs()
		if prefsErr != nil {
			return nil, fmt.Errorf("capture local macos prefs: %w", prefsErr)
		}

		type prefKey struct {
			Domain string
			Key    string
		}
		localMap := make(map[prefKey]string, len(localPrefs))
		for _, p := range localPrefs {
			localMap[prefKey{p.Domain, p.Key}] = p.Value
		}

		for _, rp := range rc.MacOSPrefs {
			localVal, exists := localMap[prefKey{rp.Domain, rp.Key}]
			if !exists || localVal != rp.Value {
				d.MacOSChanged = append(d.MacOSChanged, MacOSPrefDiff{
					Domain:      rp.Domain,
					Key:         rp.Key,
					Type:        rp.Type,
					Desc:        rp.Desc,
					RemoteValue: rp.Value,
					LocalValue:  localVal,
				})
			}
		}
	}

	return d, nil
}

// pluginsEqual reports whether two plugin lists contain the same elements
// regardless of order.
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

// diffLists returns (missing, extra) where missing = in remote but not local,
// extra = in local but not remote.
func diffLists(remote, local []string) (missing, extra []string) {
	remoteSet := ToSet(remote)
	localSet := ToSet(local)

	for item := range remoteSet {
		if !localSet[item] {
			missing = append(missing, item)
		}
	}
	for item := range localSet {
		if !remoteSet[item] {
			extra = append(extra, item)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

// ToSet converts a string slice to a set (map[string]bool).
func ToSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

// getLocalDotfilesURL reads the git remote URL from ~/.dotfiles if it exists.
func getLocalDotfilesURL() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dotfilesPath := filepath.Join(home, ".dotfiles")
	if _, err := os.Stat(filepath.Join(dotfilesPath, ".git")); err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", dotfilesPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
