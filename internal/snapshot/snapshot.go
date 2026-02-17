package snapshot

import "time"

type CaptureHealth struct {
	FailedSteps []string `json:"failed_steps"`
	Partial     bool     `json:"partial"`
}

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
	Health        CaptureHealth   `json:"health"`
}

type PackageSnapshot struct {
	Formulae []string `json:"formulae"`
	Casks    []string `json:"casks"`
	Taps     []string `json:"taps"`
	Npm      []string `json:"npm"`
}

type MacOSPref struct {
	Domain string `json:"domain"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Desc   string `json:"desc"`
}

type ShellSnapshot struct {
	Default string   `json:"default"`
	OhMyZsh bool     `json:"oh_my_zsh"`
	Plugins []string `json:"plugins"`
	Theme   string   `json:"theme"`
}

type GitSnapshot struct {
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
}

type DevTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type CatalogMatch struct {
	Matched   []string `json:"matched"`
	Unmatched []string `json:"unmatched"`
	MatchRate float64  `json:"match_rate"`
}
