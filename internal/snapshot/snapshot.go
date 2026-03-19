package snapshot

import (
	"encoding/json"
	"fmt"
	"time"
)

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
	Dotfiles      DotfilesSnapshot `json:"dotfiles"`
	DevTools      []DevTool       `json:"dev_tools"`
	MatchedPreset string          `json:"matched_preset"`
	CatalogMatch  CatalogMatch    `json:"catalog_match"`
	Health        CaptureHealth   `json:"health"`
}

type DotfilesSnapshot struct {
	RepoURL string `json:"repo_url,omitempty"`
}

type PackageSnapshot struct {
	Formulae []string `json:"formulae"`
	Casks    []string `json:"casks"`
	Taps     []string `json:"taps"`
	Npm      []string `json:"npm"`
}

// UnmarshalJSON accepts three formats:
//   - Structured object: {"formulae":[],"casks":[],"taps":[],"npm":[]}
//   - Typed object array: [{"name":"git","type":"formula"},{"name":"docker","type":"cask"}]
//   - Flat string array:  ["git","curl"] (all treated as formulae)
func (ps *PackageSnapshot) UnmarshalJSON(data []byte) error {
	// Try structured object first.
	type alias PackageSnapshot
	var obj alias
	if err := json.Unmarshal(data, &obj); err == nil {
		*ps = PackageSnapshot(obj)
		return nil
	}

	// Try typed object array: [{"name":"x","type":"formula|cask|tap|npm"}]
	var typed []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typed); err == nil && len(typed) > 0 && typed[0].Name != "" {
		for _, p := range typed {
			switch p.Type {
			case "cask":
				ps.Casks = append(ps.Casks, p.Name)
			case "tap":
				ps.Taps = append(ps.Taps, p.Name)
			case "npm":
				ps.Npm = append(ps.Npm, p.Name)
			default:
				ps.Formulae = append(ps.Formulae, p.Name)
			}
		}
		return nil
	}

	// Try flat string array.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		ps.Formulae = arr
		return nil
	}

	return fmt.Errorf("packages must be an object {formulae,casks,taps,npm} or an array")
}

type MacOSPref struct {
	Domain string `json:"domain"`
	Key    string `json:"key"`
	Type   string `json:"type"`
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
