package config

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

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
		return base
	}
	return "https://openboot.dev"
}

func FetchRemoteConfig(userSlug string, token string) (*RemoteConfig, error) {
	parts := strings.SplitN(userSlug, "/", 2)
	username := parts[0]
	slug := "default"
	if len(parts) > 1 {
		slug = parts[1]
	}

	apiBase := getAPIBase()
	url := fmt.Sprintf("%s/%s/%s/config", apiBase, username, slug)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}
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
		return nil, fmt.Errorf("failed to fetch config %s/%s (status: %d)", username, slug, resp.StatusCode)
	}

	var rc RemoteConfig
	if err := json.NewDecoder(resp.Body).Decode(&rc); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
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
