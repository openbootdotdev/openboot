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

	"gopkg.in/yaml.v3"
)

func isAllowedAPIURL(u string) bool {
	if strings.HasPrefix(u, "https://") {
		return true
	}
	if strings.HasPrefix(u, "http://localhost") || strings.HasPrefix(u, "http://127.0.0.1") {
		return true
	}
	return false
}

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
	SnapshotMacOS    []SnapshotMacOSPref
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

type SnapshotMacOSPref struct {
	Domain string
	Key    string
	Value  string
	Desc   string
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
