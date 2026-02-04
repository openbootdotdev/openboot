package snapshot

import "time"

// Snapshot represents a complete capture of the developer environment state.
type Snapshot struct {
	Version       int             `json:"version"`
	CapturedAt    time.Time       `json:"captured_at"`
	Hostname      string          `json:"hostname"`
	Packages      PackageSnapshot `json:"packages"`
	MacOSPrefs    []MacOSPref     `json:"macos_prefs"`
	Shell         ShellSnapshot   `json:"shell"`
	Git           GitSnapshot     `json:"git"`
	DevTools      []DevTool       `json:"dev_tools"`
	MatchedPreset string          `json:"matched_preset"`
	CatalogMatch  CatalogMatch    `json:"catalog_match"`
}

// PackageSnapshot captures installed Homebrew packages.
type PackageSnapshot struct {
	Formulae []string `json:"formulae"`
	Casks    []string `json:"casks"`
	Taps     []string `json:"taps"`
}

// MacOSPref captures a single macOS defaults preference value.
type MacOSPref struct {
	Domain string `json:"domain"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Desc   string `json:"desc"`
}

// ShellSnapshot captures the shell environment configuration.
type ShellSnapshot struct {
	Default string   `json:"default"`
	OhMyZsh bool     `json:"oh_my_zsh"`
	Plugins []string `json:"plugins"`
	Theme   string   `json:"theme"`
}

// GitSnapshot captures global git user configuration.
type GitSnapshot struct {
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
}

// DevTool captures a detected development tool and its version.
type DevTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// CatalogMatch records how well the snapshot matches a catalog.
// Populated by the matching phase, not during capture.
type CatalogMatch struct {
	Matched   []string `json:"matched"`
	Unmatched []string `json:"unmatched"`
	MatchRate float64  `json:"match_rate"`
}
