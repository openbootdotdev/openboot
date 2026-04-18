package config

import (
	"encoding/json"
	"fmt"
)

// Config holds all configuration for a single openboot run.
// See InstallOptions and InstallState for the split representation used
// internally by the installer package.
type Config struct {
	// --- Input (set by flags/env before run) ---

	Version          string // injected via -ldflags at build time
	Preset           string // -p / OPENBOOT_PRESET
	User             string // -u / OPENBOOT_USER
	DryRun           bool   // --dry-run
	Silent           bool   // -s / CI mode
	PackagesOnly     bool   // --packages-only
	Update           bool   // --update
	Shell            string // --shell (install|skip)
	Macos            string // --macos (configure|skip)
	Dotfiles         string // --dotfiles (clone|link|skip)
	GitName          string // OPENBOOT_GIT_NAME (silent mode)
	GitEmail         string // OPENBOOT_GIT_EMAIL (silent mode)
	PostInstall      string // --post-install
	AllowPostInstall bool   // --allow-post-install
	DotfilesURL      string // from remote config

	// --- Runtime state (populated during install) ---

	SelectedPkgs     map[string]bool    // set by UI package selector
	OnlinePkgs       []Package          // fetched from packages API
	SnapshotTaps     []string           // from snapshot capture
	RemoteConfig     *RemoteConfig      // fetched from openboot.dev at startup
	SnapshotGit      *SnapshotGitConfig // from snapshot capture
	SnapshotMacOS    []RemoteMacOSPref  // from snapshot capture
	SnapshotDotfiles    string             // from snapshot capture
	SnapshotShellOhMyZsh bool             // from snapshot capture
	SnapshotShellTheme   string           // from snapshot capture
	SnapshotShellPlugins []string         // from snapshot capture
}

// InstallOptions holds user-supplied inputs set from CLI flags and environment
// variables. All fields are read-only after Run() is called.
type InstallOptions struct {
	Version          string
	Preset           string
	User             string
	DryRun           bool
	Silent           bool
	PackagesOnly     bool
	Update           bool
	Shell            string
	Macos            string
	Dotfiles         string
	GitName          string
	GitEmail         string
	PostInstall      string
	AllowPostInstall bool
	DotfilesURL      string
}

// InstallState holds runtime values populated during installation.
// Fields are written by installer steps and read by subsequent steps.
type InstallState struct {
	SelectedPkgs     map[string]bool
	OnlinePkgs       []Package
	SnapshotTaps     []string
	RemoteConfig     *RemoteConfig
	SnapshotGit      *SnapshotGitConfig
	SnapshotMacOS    []RemoteMacOSPref
	SnapshotDotfiles    string
	SnapshotShellOhMyZsh bool
	SnapshotShellTheme   string
	SnapshotShellPlugins []string
}

type SnapshotGitConfig struct {
	UserName  string
	UserEmail string
}

// PackageEntry represents a package with an optional description.
type PackageEntry struct {
	Name string `json:"name"`
	Desc string `json:"desc,omitempty"`
}

// PackageEntryList is a list of PackageEntry that unmarshals from either
// ["git","curl"] (flat strings) or [{"name":"git","desc":"..."}] (objects).
type PackageEntryList []PackageEntry

// UnmarshalJSON handles both flat string arrays and object arrays.
func (p *PackageEntryList) UnmarshalJSON(data []byte) error {
	// Try flat string array first (most common from server responses).
	var names []string
	if err := json.Unmarshal(data, &names); err == nil {
		result := make([]PackageEntry, len(names))
		for i, n := range names {
			result[i] = PackageEntry{Name: n}
		}
		*p = result
		return nil
	}

	// Try object array [{name, desc}]. Reject if any entry has a "type"
	// field — those must be split by UnmarshalRemoteConfigFlexible instead.
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("packages must be a string array or object array: %w", err)
	}

	entries := make([]PackageEntry, 0, len(raw))
	for _, item := range raw {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(item, &probe); err == nil && probe.Type != "" {
			// Has a "type" field — bail so the caller's typed-object path handles it.
			return fmt.Errorf("object has type field; needs typed splitting")
		}
		var entry PackageEntry
		if err := json.Unmarshal(item, &entry); err != nil {
			return fmt.Errorf("invalid package entry: %w", err)
		}
		entries = append(entries, entry)
	}
	*p = entries
	return nil
}

// Names returns a slice of just the package names.
func (p PackageEntryList) Names() []string {
	names := make([]string, len(p))
	for i, e := range p {
		names[i] = e.Name
	}
	return names
}

// DescMap returns a map of name → desc for entries that have descriptions.
func (p PackageEntryList) DescMap() map[string]string {
	m := make(map[string]string, len(p))
	for _, e := range p {
		if e.Desc != "" {
			m[e.Name] = e.Desc
		}
	}
	return m
}

type RemoteConfig struct {
	Username     string             `json:"username"`
	Slug         string             `json:"slug"`
	Name         string             `json:"name"`
	Preset       string             `json:"preset"`
	Packages     PackageEntryList   `json:"packages"`
	Casks        PackageEntryList   `json:"casks"`
	Taps         []string           `json:"taps"`
	Npm          PackageEntryList   `json:"npm"`
	DotfilesRepo string             `json:"dotfiles_repo"`
	PostInstall  []string           `json:"post_install"`
	Shell        *RemoteShellConfig `json:"shell"`
	MacOSPrefs   []RemoteMacOSPref  `json:"macos_prefs"`
}

type RemoteShellConfig struct {
	OhMyZsh bool     `json:"oh_my_zsh"`
	Theme   string   `json:"theme"`
	Plugins []string `json:"plugins"`
}

type RemoteMacOSPref struct {
	Domain string `json:"domain"`
	Key    string `json:"key"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Desc   string `json:"desc"`
}

// typedPackage represents a package entry with name, type, and optional
// description, as returned by the openboot.dev API.
type typedPackage struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Desc string `json:"desc,omitempty"`
}

// Preset defines a named collection of CLI, cask, and npm packages.
type Preset struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	CLI         []string `yaml:"cli"`
	Cask        []string `yaml:"cask"`
	Npm         []string `yaml:"npm"`
}

type presetsData struct {
	Presets map[string]Preset `yaml:"presets"`
}

type screenRecordingData struct {
	Packages []string `yaml:"packages"`
}
