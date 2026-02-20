package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProjectConfig(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			content: `version: "1.0"
brew:
  packages:
    - git
    - node
  casks:
    - visual-studio-code
npm:
  - typescript
  - eslint
env:
  NODE_ENV: development
init:
  - npm install
verify:
  - npm --version`,
			wantErr: false,
		},
		{
			name: "missing version",
			content: `brew:
  packages:
    - git`,
			wantErr:     true,
			errContains: "version field is required",
		},
		{
			name:        "unsupported version",
			content:     `version: "2.0"`,
			wantErr:     true,
			errContains: "unsupported version",
		},
		{
			name:        "invalid yaml",
			content:     `version: "1.0"\ninvalid yaml content [[[`,
			wantErr:     true,
			errContains: "parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, ProjectConfigFileName)
			err := os.WriteFile(configPath, []byte(tt.content), 0644)
			require.NoError(t, err)

			cfg, err := LoadProjectConfig(tmpDir)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
				assert.Equal(t, "1.0", cfg.Version)
			}
		})
	}
}

func TestLoadProjectConfig_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := LoadProjectConfig(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestProjectConfig_HasMethods(t *testing.T) {
	tests := []struct {
		name        string
		config      *ProjectConfig
		hasPackages bool
		hasInit     bool
		hasVerify   bool
		hasEnv      bool
	}{
		{
			name: "all fields populated",
			config: &ProjectConfig{
				Version: "1.0",
				Brew: &BrewConfig{
					Packages: []string{"git"},
				},
				Npm:    []string{"typescript"},
				Init:   []string{"npm install"},
				Verify: []string{"npm --version"},
				Env:    map[string]string{"NODE_ENV": "development"},
			},
			hasPackages: true,
			hasInit:     true,
			hasVerify:   true,
			hasEnv:      true,
		},
		{
			name: "only brew taps",
			config: &ProjectConfig{
				Version: "1.0",
				Brew: &BrewConfig{
					Taps: []string{"homebrew/cask-fonts"},
				},
			},
			hasPackages: true,
			hasInit:     false,
			hasVerify:   false,
			hasEnv:      false,
		},
		{
			name: "only npm packages",
			config: &ProjectConfig{
				Version: "1.0",
				Npm:     []string{"typescript"},
			},
			hasPackages: true,
			hasInit:     false,
			hasVerify:   false,
			hasEnv:      false,
		},
		{
			name: "empty config",
			config: &ProjectConfig{
				Version: "1.0",
			},
			hasPackages: false,
			hasInit:     false,
			hasVerify:   false,
			hasEnv:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.hasPackages, tt.config.HasPackages())
			assert.Equal(t, tt.hasInit, tt.config.HasInit())
			assert.Equal(t, tt.hasVerify, tt.config.HasVerify())
			assert.Equal(t, tt.hasEnv, tt.config.HasEnv())
		})
	}
}

func TestProjectConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *ProjectConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid version 1.0",
			config: &ProjectConfig{
				Version: "1.0",
			},
			wantErr: false,
		},
		{
			name: "missing version",
			config: &ProjectConfig{
				Version: "",
			},
			wantErr:     true,
			errContains: "version field is required",
		},
		{
			name: "unsupported version",
			config: &ProjectConfig{
				Version: "2.0",
			},
			wantErr:     true,
			errContains: "unsupported version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
