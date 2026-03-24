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
	Formulae     []string          `json:"formulae"`
	Casks        []string          `json:"casks"`
	Taps         []string          `json:"taps"`
	Npm          []string          `json:"npm"`
	Descriptions map[string]string `json:"-"` // populated during unmarshal, not serialised
}

// UnmarshalJSON accepts three formats:
//   - Structured object: {"formulae":[],"casks":[],"taps":[],"npm":[]}
//   - Typed object array: [{"name":"git","type":"formula"},{"name":"docker","type":"cask"}]
//   - Flat string array:  ["git","curl"] (all treated as formulae)
func (ps *PackageSnapshot) UnmarshalJSON(data []byte) error {
	// Try structured object first (plain string arrays).
	type alias PackageSnapshot
	var obj alias
	if err := json.Unmarshal(data, &obj); err == nil {
		*ps = PackageSnapshot(obj)
		return nil
	}

	// Try structured object with entry objects: {"formulae":[{"name":"x","desc":"y"}],...}
	var richObj struct {
		Formulae []struct {
			Name string `json:"name"`
			Desc string `json:"desc"`
		} `json:"formulae"`
		Casks []struct {
			Name string `json:"name"`
			Desc string `json:"desc"`
		} `json:"casks"`
		Taps []string `json:"taps"`
		Npm  []struct {
			Name string `json:"name"`
			Desc string `json:"desc"`
		} `json:"npm"`
	}
	if err := json.Unmarshal(data, &richObj); err == nil &&
		(len(richObj.Formulae) > 0 || len(richObj.Casks) > 0 || len(richObj.Npm) > 0) {
		ps.Descriptions = make(map[string]string)
		for _, p := range richObj.Formulae {
			ps.Formulae = append(ps.Formulae, p.Name)
			if p.Desc != "" {
				ps.Descriptions[p.Name] = p.Desc
			}
		}
		for _, p := range richObj.Casks {
			ps.Casks = append(ps.Casks, p.Name)
			if p.Desc != "" {
				ps.Descriptions[p.Name] = p.Desc
			}
		}
		ps.Taps = richObj.Taps
		for _, p := range richObj.Npm {
			ps.Npm = append(ps.Npm, p.Name)
			if p.Desc != "" {
				ps.Descriptions[p.Name] = p.Desc
			}
		}
		return nil
	}

	// Try typed object array: [{"name":"x","type":"formula|cask|tap|npm","desc":"..."}]
	var typed []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Desc string `json:"desc"`
	}
	if err := json.Unmarshal(data, &typed); err == nil && len(typed) > 0 && typed[0].Name != "" {
		ps.Descriptions = make(map[string]string)
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
			if p.Desc != "" {
				ps.Descriptions[p.Name] = p.Desc
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

// MarshalJSON outputs packages as rich objects when descriptions exist,
// falling back to plain string arrays for backward compatibility.
func (ps PackageSnapshot) MarshalJSON() ([]byte, error) {
	if len(ps.Descriptions) == 0 {
		type alias PackageSnapshot
		return json.Marshal(alias(ps))
	}

	type entry struct {
		Name string `json:"name"`
		Desc string `json:"desc,omitempty"`
	}

	formulae := make([]entry, len(ps.Formulae))
	for i, name := range ps.Formulae {
		formulae[i] = entry{Name: name, Desc: ps.Descriptions[name]}
	}

	casks := make([]entry, len(ps.Casks))
	for i, name := range ps.Casks {
		casks[i] = entry{Name: name, Desc: ps.Descriptions[name]}
	}

	npm := make([]entry, len(ps.Npm))
	for i, name := range ps.Npm {
		npm[i] = entry{Name: name, Desc: ps.Descriptions[name]}
	}

	return json.Marshal(struct {
		Formulae []entry  `json:"formulae"`
		Casks    []entry  `json:"casks"`
		Taps     []string `json:"taps"`
		Npm      []entry  `json:"npm"`
	}{
		Formulae: formulae,
		Casks:    casks,
		Taps:     ps.Taps,
		Npm:      npm,
	})
}

type MacOSPref struct {
	Domain string `json:"domain"`
	Key    string `json:"key"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Desc   string `json:"desc"`
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
