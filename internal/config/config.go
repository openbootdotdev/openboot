package config

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/system"
	"gopkg.in/yaml.v3"
)

// isAllowedAPIURL delegates to the shared implementation in system package.
var isAllowedAPIURL = system.IsAllowedAPIURL

var remoteHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

//go:embed data/presets.yaml
var presetsYAML embed.FS

//go:embed data/screen-recording-packages.yaml
var screenRecordingYAML embed.FS

type Config struct {
	Version      string
	Preset       string
	Silent       bool
	DryRun       bool
	Update       bool
	Shell        string
	Macos        string
	Dotfiles     string
	GitName      string
	GitEmail     string
	SelectedPkgs map[string]bool
	OnlinePkgs   []Package
	SnapshotTaps []string
	User         string
	RemoteConfig *RemoteConfig
	PackagesOnly bool

	SnapshotShell    *SnapshotShellConfig
	SnapshotGit      *SnapshotGitConfig
	SnapshotMacOS    []RemoteMacOSPref
	SnapshotDotfiles string
	DotfilesURL      string
	PostInstall      string
	AllowPostInstall bool
}

type SnapshotShellConfig struct {
	OhMyZsh bool
	Theme   string
	Plugins []string
}

type SnapshotGitConfig struct {
	UserName  string
	UserEmail string
}

type RemoteConfig struct {
	Username     string   `json:"username"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Preset       string   `json:"preset"`
	Packages     []string `json:"packages"`
	Casks        []string `json:"casks"`
	Taps         []string `json:"taps"`
	Npm          []string `json:"npm"`
	DotfilesRepo string   `json:"dotfiles_repo"`
	PostInstall  []string `json:"post_install"`
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

// typedPackage represents a package entry with name and type, as returned
// by the openboot.dev API (e.g. {"name":"git","type":"formula"}).
type typedPackage struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// UnmarshalRemoteConfigFlexible parses JSON into a RemoteConfig, accepting
// packages in either flat string array format (["git","curl"]) or typed
// object array format ([{"name":"git","type":"formula"}]).
func UnmarshalRemoteConfigFlexible(data []byte) (*RemoteConfig, error) {
	// Try direct unmarshal first (flat string arrays).
	var rc RemoteConfig
	if err := json.Unmarshal(data, &rc); err == nil {
		backfillMacOSPrefsFromSnapshot(&rc, data)
		return &rc, nil
	}

	// Extract packages as typed objects and convert to flat arrays.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	pkgData, ok := raw["packages"]
	if !ok {
		return nil, fmt.Errorf("missing packages field")
	}

	var typed []typedPackage
	if err := json.Unmarshal(pkgData, &typed); err != nil {
		return nil, fmt.Errorf("packages must be a string array or typed object array: %w", err)
	}

	var formulae, casks, taps, npm []string
	for _, p := range typed {
		switch p.Type {
		case "cask":
			casks = append(casks, p.Name)
		case "tap":
			taps = append(taps, p.Name)
		case "npm":
			npm = append(npm, p.Name)
		default:
			formulae = append(formulae, p.Name)
		}
	}

	// Replace packages with flat arrays and re-unmarshal.
	converted := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		converted[k] = v
	}
	if f, err := json.Marshal(formulae); err == nil {
		converted["packages"] = f
	}
	if len(casks) > 0 {
		if c, err := json.Marshal(casks); err == nil {
			converted["casks"] = c
		}
	}
	if len(taps) > 0 {
		if t, err := json.Marshal(taps); err == nil {
			converted["taps"] = t
		}
	}
	if len(npm) > 0 {
		if n, err := json.Marshal(npm); err == nil {
			converted["npm"] = n
		}
	}

	normalised, err := json.Marshal(converted)
	if err != nil {
		return nil, fmt.Errorf("normalise config: %w", err)
	}

	var result RemoteConfig
	if err := json.Unmarshal(normalised, &result); err != nil {
		return nil, err
	}
	backfillMacOSPrefsFromSnapshot(&result, data)
	return &result, nil
}

// backfillMacOSPrefsFromSnapshot copies macos_prefs from the embedded snapshot
// object when the top-level field is empty. This handles exported configs where
// macos_prefs are nested under "snapshot" rather than at the top level.
// Callers are responsible for calling Validate() on the returned RemoteConfig.
func backfillMacOSPrefsFromSnapshot(rc *RemoteConfig, data []byte) {
	if len(rc.MacOSPrefs) > 0 {
		return
	}
	var wrapper struct {
		Snapshot struct {
			MacOSPrefs []RemoteMacOSPref `json:"macos_prefs"`
		} `json:"snapshot"`
	}
	// Unmarshal error is intentionally ignored: data was already successfully
	// parsed once, so failure here means the snapshot sub-object is malformed
	// and we simply skip backfill rather than failing the entire load.
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Snapshot.MacOSPrefs) > 0 {
		rc.MacOSPrefs = wrapper.Snapshot.MacOSPrefs
	}
}

var (
	pkgNameRe = regexp.MustCompile(`^[a-zA-Z0-9@/_.-]+$`)
	tapNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+$`)
)

func (rc *RemoteConfig) Validate() error {
	for _, p := range rc.Packages {
		if !pkgNameRe.MatchString(p) {
			return fmt.Errorf("invalid package name: %q", p)
		}
	}
	for _, c := range rc.Casks {
		if !pkgNameRe.MatchString(c) {
			return fmt.Errorf("invalid cask name: %q", c)
		}
	}
	for _, n := range rc.Npm {
		if !pkgNameRe.MatchString(n) {
			return fmt.Errorf("invalid npm package name: %q", n)
		}
	}
	for _, t := range rc.Taps {
		if !tapNameRe.MatchString(t) {
			return fmt.Errorf("invalid tap name: %q (expected format: owner/repo)", t)
		}
	}
	if rc.DotfilesRepo != "" {
		if !strings.HasPrefix(rc.DotfilesRepo, "https://") && !strings.HasPrefix(rc.DotfilesRepo, "git@") {
			return fmt.Errorf("invalid dotfiles_repo: %q (only https:// or git@ URLs allowed)", rc.DotfilesRepo)
		}
	}
	validPrefTypes := map[string]bool{"": true, "string": true, "int": true, "bool": true, "float": true}
	for _, mp := range rc.MacOSPrefs {
		if !validPrefTypes[mp.Type] {
			return fmt.Errorf("invalid macos_prefs type: %q for %s %s (allowed: string, int, bool, float)", mp.Type, mp.Domain, mp.Key)
		}
	}
	return nil
}

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

var Presets map[string]Preset
var presetOrder = []string{"minimal", "developer", "full"}

func init() {
	data, err := presetsYAML.ReadFile("data/presets.yaml")
	if err != nil {
		log.Fatalf("Failed to read presets.yaml: %v", err)
	}

	var pd presetsData
	if err := yaml.Unmarshal(data, &pd); err != nil {
		log.Fatalf("Failed to parse presets.yaml: %v", err)
	}

	Presets = pd.Presets
}

func GetPreset(name string) (Preset, bool) {
	p, ok := Presets[name]
	return p, ok
}

func GetPresetNames() []string {
	return presetOrder
}

func getAPIBase() string {
	if base := os.Getenv("OPENBOOT_API_URL"); base != "" {
		if isAllowedAPIURL(base) {
			return base
		}
		fmt.Fprintf(os.Stderr, "Warning: ignoring insecure OPENBOOT_API_URL=%q (only https or http://localhost allowed)\n", base)
	}
	return "https://openboot.dev"
}

func fetchConfigBySlug(apiBase, username, slug, token string) (*http.Response, error) {
	configURL := fmt.Sprintf("%s/%s/%s/config", apiBase, url.PathEscape(username), url.PathEscape(slug))

	req, err := http.NewRequest("GET", configURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return remoteHTTPClient.Do(req)
}

func parseConfigResponse(resp *http.Response, username, slug, token string) (*RemoteConfig, error) {
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		if token == "" {
			return nil, fmt.Errorf("config %s/%s is private — run 'openboot login' first, then try again", username, slug)
		}
		return nil, fmt.Errorf("config %s/%s is private and you don't have access", username, slug)
	}

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("config not found: %s/%s", username, slug)
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("server error while fetching config %s/%s (status: %d)", username, slug, resp.StatusCode)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch config %s/%s: status %d", username, slug, resp.StatusCode)
	}

	var rc RemoteConfig
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := rc.Validate(); err != nil {
		return nil, fmt.Errorf("invalid remote config %s/%s: %w", username, slug, err)
	}

	return &rc, nil
}

// LoadRemoteConfigFromFile reads a JSON file and returns a RemoteConfig.
// It auto-detects whether the file is in RemoteConfig or Snapshot format.
// Snapshot files are converted by extracting the relevant fields.
func LoadRemoteConfigFromFile(path string) (*RemoteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Detect format: snapshot files have "captured_at" and nested "packages".
	var probe struct {
		CapturedAt string          `json:"captured_at"`
		Packages   json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if probe.CapturedAt != "" {
		return loadSnapshotAsRemoteConfig(data)
	}

	rc, err := UnmarshalRemoteConfigFlexible(data)
	if err != nil {
		return nil, fmt.Errorf("parse remote config: %w", err)
	}
	if err := rc.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return rc, nil
}

// snapshotFile mirrors the subset of snapshot.Snapshot needed for conversion,
// avoiding an import cycle with the snapshot package.
type snapshotFile struct {
	Packages struct {
		Formulae []string `json:"formulae"`
		Casks    []string `json:"casks"`
		Taps     []string `json:"taps"`
		Npm      []string `json:"npm"`
	} `json:"packages"`
	Shell struct {
		OhMyZsh bool     `json:"oh_my_zsh"`
		Theme   string   `json:"theme"`
		Plugins []string `json:"plugins"`
	} `json:"shell"`
	MacOSPrefs []RemoteMacOSPref `json:"macos_prefs"`
}

func loadSnapshotAsRemoteConfig(data []byte) (*RemoteConfig, error) {
	var snap snapshotFile
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot file: %w", err)
	}

	rc := &RemoteConfig{
		Packages: snap.Packages.Formulae,
		Casks:    snap.Packages.Casks,
		Taps:     snap.Packages.Taps,
		Npm:      snap.Packages.Npm,
		MacOSPrefs: snap.MacOSPrefs,
	}
	if snap.Shell.OhMyZsh {
		rc.Shell = &RemoteShellConfig{
			OhMyZsh: true,
			Theme:   snap.Shell.Theme,
			Plugins: snap.Shell.Plugins,
		}
	}
	if err := rc.Validate(); err != nil {
		return nil, fmt.Errorf("snapshot contains invalid data: %w", err)
	}
	// Note: snapshot files do not contain dotfiles_repo or post_install.
	// Those fields must be set manually on openboot.dev after upload.
	return rc, nil
}

func FetchRemoteConfig(userSlug string, token string) (*RemoteConfig, error) {
	parts := strings.SplitN(userSlug, "/", 2)
	slugExplicit := len(parts) > 1
	apiBase := getAPIBase()

	// If no explicit slug, try alias resolution first
	if !slugExplicit {
		alias := parts[0]
		rc, err := fetchConfigByAlias(apiBase, alias, token)
		if err == nil {
			return rc, nil
		}

		// Alias not found — try as username/default
		resp, err := fetchConfigBySlug(apiBase, alias, "default", token)
		if err != nil {
			return nil, fmt.Errorf("fetch config: %w", err)
		}
		return parseConfigResponse(resp, alias, "default", token)
	}

	// Explicit slug: fetch directly
	username := parts[0]
	slug := parts[1]
	resp, err := fetchConfigBySlug(apiBase, username, slug, token)
	if err != nil {
		return nil, fmt.Errorf("fetch config: %w", err)
	}
	return parseConfigResponse(resp, username, slug, token)
}

func fetchConfigByAlias(apiBase, alias, token string) (*RemoteConfig, error) {
	aliasURL := fmt.Sprintf("%s/api/configs/alias/%s", apiBase, url.PathEscape(alias))

	req, err := http.NewRequest("GET", aliasURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch alias: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("alias not found: %s", alias)
	}

	var rc RemoteConfig
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := rc.Validate(); err != nil {
		return nil, fmt.Errorf("invalid remote config (alias %s): %w", alias, err)
	}

	return &rc, nil
}

type screenRecordingData struct {
	Packages []string `yaml:"packages"`
}

func GetScreenRecordingPackages() []string {
	data, err := screenRecordingYAML.ReadFile("data/screen-recording-packages.yaml")
	if err != nil {
		return []string{}
	}

	var srd screenRecordingData
	if err := yaml.Unmarshal(data, &srd); err != nil {
		return []string{}
	}

	return srd.Packages
}
