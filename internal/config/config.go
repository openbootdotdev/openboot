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
	"sync"
	"time"

	"github.com/openbootdotdev/openboot/internal/system"
	"gopkg.in/yaml.v3"
)

var upgradeHintOnce sync.Once

// checkUpgradeHint reads the X-OpenBoot-Upgrade header from server responses.
// If the server says the CLI is too old, print a one-time hint to stderr.
func checkUpgradeHint(resp *http.Response) {
	if resp.Header.Get("X-OpenBoot-Upgrade") == "true" {
		upgradeHintOnce.Do(func() {
			minVer := resp.Header.Get("X-OpenBoot-Min-Version")
			if minVer == "" {
				minVer = "latest"
			}
			fmt.Fprintf(os.Stderr, "Notice: your CLI is older than the server expects (min %s). Run: brew upgrade openboot\n", minVer)
		})
	}
}

// isAllowedAPIURL delegates to the shared implementation in system package.
var isAllowedAPIURL = system.IsAllowedAPIURL

// clientVersion is set by the CLI at startup via SetClientVersion.
var clientVersion = "dev"

// SetClientVersion sets the version string sent in X-OpenBoot-Version headers.
func SetClientVersion(v string) { clientVersion = v }

// versionTransport wraps http.DefaultTransport to inject the version header.
type versionTransport struct{ base http.RoundTripper }

func (t *versionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-OpenBoot-Version", clientVersion)
	return t.base.RoundTrip(req)
}

var remoteHTTPClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: &versionTransport{base: http.DefaultTransport},
}

//go:embed data/presets.yaml
var presetsYAML embed.FS

//go:embed data/screen-recording-packages.yaml
var screenRecordingYAML embed.FS

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
	SnapshotDotfiles string             // from snapshot capture
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
	SnapshotDotfiles string
}

// ToInstallOptions extracts the read-only input fields from Config.
func (c *Config) ToInstallOptions() *InstallOptions {
	return &InstallOptions{
		Version:          c.Version,
		Preset:           c.Preset,
		User:             c.User,
		DryRun:           c.DryRun,
		Silent:           c.Silent,
		PackagesOnly:     c.PackagesOnly,
		Update:           c.Update,
		Shell:            c.Shell,
		Macos:            c.Macos,
		Dotfiles:         c.Dotfiles,
		GitName:          c.GitName,
		GitEmail:         c.GitEmail,
		PostInstall:      c.PostInstall,
		AllowPostInstall: c.AllowPostInstall,
		DotfilesURL:      c.DotfilesURL,
	}
}

// ToInstallState extracts the mutable runtime fields from Config.
func (c *Config) ToInstallState() *InstallState {
	return &InstallState{
		SelectedPkgs:     c.SelectedPkgs,
		OnlinePkgs:       c.OnlinePkgs,
		SnapshotTaps:     c.SnapshotTaps,
		RemoteConfig:     c.RemoteConfig,
		SnapshotGit:      c.SnapshotGit,
		SnapshotMacOS:    c.SnapshotMacOS,
		SnapshotDotfiles: c.SnapshotDotfiles,
	}
}

// ApplyState writes runtime state back into the Config (for callers that still
// use *Config as the shared context, e.g. CLI sync/diff commands).
func (c *Config) ApplyState(s *InstallState) {
	c.SelectedPkgs = s.SelectedPkgs
	c.OnlinePkgs = s.OnlinePkgs
	c.SnapshotTaps = s.SnapshotTaps
	c.RemoteConfig = s.RemoteConfig
	c.SnapshotGit = s.SnapshotGit
	c.SnapshotMacOS = s.SnapshotMacOS
	c.SnapshotDotfiles = s.SnapshotDotfiles
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
	Username     string           `json:"username"`
	Slug         string           `json:"slug"`
	Name         string           `json:"name"`
	Preset       string           `json:"preset"`
	Packages     PackageEntryList `json:"packages"`
	Casks        PackageEntryList `json:"casks"`
	Taps         []string         `json:"taps"`
	Npm          PackageEntryList `json:"npm"`
	DotfilesRepo string           `json:"dotfiles_repo"`
	PostInstall  []string         `json:"post_install"`
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

	var formulae, casks, npm PackageEntryList
	var taps []string
	for _, p := range typed {
		entry := PackageEntry{Name: p.Name, Desc: p.Desc}
		switch p.Type {
		case "cask":
			casks = append(casks, entry)
		case "tap":
			taps = append(taps, p.Name)
		case "npm":
			npm = append(npm, entry)
		default:
			formulae = append(formulae, entry)
		}
	}

	// Replace packages with typed arrays and re-unmarshal.
	converted := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		converted[k] = v
	}
	marshalInto := func(key string, items interface{}) {
		if data, err := json.Marshal(items); err == nil {
			converted[key] = data
		}
	}
	marshalInto("packages", formulae)
	if len(casks) > 0 {
		marshalInto("casks", casks)
	}
	if len(taps) > 0 {
		marshalInto("taps", taps)
	}
	if len(npm) > 0 {
		marshalInto("npm", npm)
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

const maxPackageNameLen = 200

// maxPostInstallCmdLen bounds each post_install command string. Keeps us well
// under any reasonable shell ARG_MAX limit per line and ensures a rogue or
// malformed remote config cannot flood the terminal with a multi-megabyte line.
const maxPostInstallCmdLen = 4096

var (
	pkgNameRe = regexp.MustCompile(`^[a-zA-Z0-9@/_.-]+$`)
	tapNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+$`)

	// dotfilesPathRe validates the path component: one or more segments of
	// alphanumeric, dash, underscore, or dot characters separated by slashes.
	dotfilesPathRe = regexp.MustCompile(`^/[a-zA-Z0-9._-]+(/[a-zA-Z0-9._-]+)*$`)
)

// ValidateDotfilesURL checks that a dotfiles repo URL uses HTTPS, has a
// valid path, max 500 chars, and no path traversal. Any HTTPS host is
// accepted (including self-hosted GitLab, Gitea, etc.).
func ValidateDotfilesURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	if len(rawURL) > 500 {
		return fmt.Errorf("dotfiles URL too long (%d chars, max 500)", len(rawURL))
	}

	if !strings.HasPrefix(rawURL, "https://") {
		return fmt.Errorf("dotfiles URL must use https:// (got %q); git@ URLs are not allowed", rawURL)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("dotfiles URL is not a valid URL: %w", err)
	}

	if parsed.Hostname() == "" {
		return fmt.Errorf("dotfiles URL is missing a hostname")
	}

	path := parsed.Path
	if strings.Contains(path, "..") {
		return fmt.Errorf("dotfiles URL path must not contain '..'")
	}
	if strings.Contains(path, "//") {
		return fmt.Errorf("dotfiles URL path must not contain '//'")
	}
	if !dotfilesPathRe.MatchString(path) {
		return fmt.Errorf("dotfiles URL has an invalid path %q; expected format: https://<host>/<owner>/<repo>", path)
	}

	return nil
}

func (rc *RemoteConfig) Validate() error {
	for _, p := range rc.Packages {
		if len(p.Name) > maxPackageNameLen {
			return fmt.Errorf("package name too long (%d chars, max %d): %q", len(p.Name), maxPackageNameLen, p.Name)
		}
		if !pkgNameRe.MatchString(p.Name) {
			return fmt.Errorf("invalid package name: %q", p.Name)
		}
	}
	for _, c := range rc.Casks {
		if len(c.Name) > maxPackageNameLen {
			return fmt.Errorf("cask name too long (%d chars, max %d): %q", len(c.Name), maxPackageNameLen, c.Name)
		}
		if !pkgNameRe.MatchString(c.Name) {
			return fmt.Errorf("invalid cask name: %q", c.Name)
		}
	}
	for _, n := range rc.Npm {
		if len(n.Name) > maxPackageNameLen {
			return fmt.Errorf("npm package name too long (%d chars, max %d): %q", len(n.Name), maxPackageNameLen, n.Name)
		}
		if !pkgNameRe.MatchString(n.Name) {
			return fmt.Errorf("invalid npm package name: %q", n.Name)
		}
	}
	for _, t := range rc.Taps {
		if len(t) > maxPackageNameLen {
			return fmt.Errorf("tap name too long (%d chars, max %d): %q", len(t), maxPackageNameLen, t)
		}
		if !tapNameRe.MatchString(t) {
			return fmt.Errorf("invalid tap name: %q (expected format: owner/repo)", t)
		}
	}
	if err := ValidateDotfilesURL(rc.DotfilesRepo); err != nil {
		return fmt.Errorf("invalid dotfiles_repo: %w", err)
	}
	validPrefTypes := map[string]bool{"": true, "string": true, "int": true, "bool": true, "float": true}
	for _, mp := range rc.MacOSPrefs {
		if !validPrefTypes[mp.Type] {
			return fmt.Errorf("invalid macos_prefs type: %q for %s %s (allowed: string, int, bool, float)", mp.Type, mp.Domain, mp.Key)
		}
		if strings.HasPrefix(mp.Domain, "-") {
			return fmt.Errorf("invalid macos_prefs domain: %q must not start with '-'", mp.Domain)
		}
		if strings.HasPrefix(mp.Key, "-") {
			return fmt.Errorf("invalid macos_prefs key: %q must not start with '-'", mp.Key)
		}
	}
	for i, cmd := range rc.PostInstall {
		if strings.TrimSpace(cmd) == "" {
			return fmt.Errorf("post_install[%d]: command must not be empty or whitespace only", i)
		}
		if strings.ContainsRune(cmd, 0) {
			return fmt.Errorf("post_install[%d]: command must not contain NUL bytes", i)
		}
		if len(cmd) > maxPostInstallCmdLen {
			return fmt.Errorf("post_install[%d]: command too long (%d chars, max %d)", i, len(cmd), maxPostInstallCmdLen)
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
	checkUpgradeHint(resp)

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
		Formulae PackageEntryList `json:"formulae"`
		Casks    PackageEntryList `json:"casks"`
		Taps     []string         `json:"taps"`
		Npm      PackageEntryList `json:"npm"`
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
		Packages:   snap.Packages.Formulae,
		Casks:      snap.Packages.Casks,
		Taps:       snap.Packages.Taps,
		Npm:        snap.Packages.Npm,
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
