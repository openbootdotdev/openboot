package config

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var remoteHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

//go:embed data/presets.yaml
var presetsYAML embed.FS

type Config struct {
	Version      string
	Preset       string
	Silent       bool
	DryRun       bool
	Update       bool
	Rollback     bool
	Resume       bool
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

func FetchRemoteConfig(userSlug string) (*RemoteConfig, error) {
	parts := strings.SplitN(userSlug, "/", 2)
	username := parts[0]
	slug := "default"
	if len(parts) > 1 {
		slug = parts[1]
	}

	url := fmt.Sprintf("https://openboot.dev/%s/%s/config", username, slug)

	resp, err := remoteHTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("config not found: %s/%s", username, slug)
	}

	var rc RemoteConfig
	if err := json.NewDecoder(resp.Body).Decode(&rc); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &rc, nil
}
