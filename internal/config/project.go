package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const ProjectConfigFileName = ".openboot.yml"

// ProjectConfig represents the .openboot.yml file in a project repository
type ProjectConfig struct {
	Version string            `yaml:"version"`
	Brew    *BrewConfig       `yaml:"brew,omitempty"`
	Npm     []string          `yaml:"npm,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Init    []string          `yaml:"init,omitempty"`
	Verify  []string          `yaml:"verify,omitempty"`
}

// BrewConfig represents Homebrew package configuration
type BrewConfig struct {
	Taps     []string `yaml:"taps,omitempty"`
	Packages []string `yaml:"packages,omitempty"`
	Casks    []string `yaml:"casks,omitempty"`
}

// LoadProjectConfig reads and parses .openboot.yml from the specified directory
func LoadProjectConfig(dir string) (*ProjectConfig, error) {
	configPath := filepath.Join(dir, ProjectConfigFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s not found in %s", ProjectConfigFileName, dir)
		}
		return nil, fmt.Errorf("failed to read %s: %w", ProjectConfigFileName, err)
	}

	var pc ProjectConfig
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", ProjectConfigFileName, err)
	}

	if err := pc.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &pc, nil
}

// Validate checks if the project config is valid
func (pc *ProjectConfig) Validate() error {
	if pc.Version == "" {
		return fmt.Errorf("version field is required")
	}

	if pc.Version != "1.0" {
		return fmt.Errorf("unsupported version: %s (supported: 1.0)", pc.Version)
	}

	return nil
}

// HasPackages returns true if the config has any packages to install
func (pc *ProjectConfig) HasPackages() bool {
	if pc.Brew != nil {
		if len(pc.Brew.Packages) > 0 || len(pc.Brew.Casks) > 0 || len(pc.Brew.Taps) > 0 {
			return true
		}
	}
	return len(pc.Npm) > 0
}

// HasInit returns true if the config has init scripts
func (pc *ProjectConfig) HasInit() bool {
	return len(pc.Init) > 0
}

// HasVerify returns true if the config has verify scripts
func (pc *ProjectConfig) HasVerify() bool {
	return len(pc.Verify) > 0
}

// HasEnv returns true if the config has environment variables
func (pc *ProjectConfig) HasEnv() bool {
	return len(pc.Env) > 0
}
