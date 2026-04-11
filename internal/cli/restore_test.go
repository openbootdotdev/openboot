package cli

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestRevisionPackagesToRemoteConfig(t *testing.T) {
	tests := []struct {
		name     string
		pkgs     []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		username string
		slug     string
		wantRC   *config.RemoteConfig
	}{
		{
			name:     "empty packages",
			pkgs:     nil,
			username: "alice",
			slug:     "my-setup",
			wantRC: &config.RemoteConfig{
				Username: "alice",
				Slug:     "my-setup",
			},
		},
		{
			name: "formula only",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "git", Type: "formula"},
				{Name: "go", Type: "formula"},
			},
			username: "bob",
			slug:     "dev",
			wantRC: &config.RemoteConfig{
				Username: "bob",
				Slug:     "dev",
				Packages: config.PackageEntryList{
					{Name: "git"},
					{Name: "go"},
				},
			},
		},
		{
			name: "all package types",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "git", Type: "formula"},
				{Name: "docker", Type: "cask"},
				{Name: "typescript", Type: "npm"},
				{Name: "homebrew/cask-fonts", Type: "tap"},
			},
			username: "carol",
			slug:     "full",
			wantRC: &config.RemoteConfig{
				Username: "carol",
				Slug:     "full",
				Packages: config.PackageEntryList{{Name: "git"}},
				Casks:    config.PackageEntryList{{Name: "docker"}},
				Npm:      config.PackageEntryList{{Name: "typescript"}},
				Taps:     []string{"homebrew/cask-fonts"},
			},
		},
		{
			name: "unknown type is silently skipped",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "git", Type: "formula"},
				{Name: "unknown-thing", Type: "ruby"},
			},
			username: "dave",
			slug:     "slim",
			wantRC: &config.RemoteConfig{
				Username: "dave",
				Slug:     "slim",
				Packages: config.PackageEntryList{{Name: "git"}},
			},
		},
		{
			name: "multiple taps",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "hashicorp/tap", Type: "tap"},
				{Name: "homebrew/core", Type: "tap"},
			},
			username: "eve",
			slug:     "infra",
			wantRC: &config.RemoteConfig{
				Username: "eve",
				Slug:     "infra",
				Taps:     []string{"hashicorp/tap", "homebrew/core"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := revisionPackagesToRemoteConfig(tt.pkgs, tt.username, tt.slug)

			assert.Equal(t, tt.wantRC.Username, rc.Username)
			assert.Equal(t, tt.wantRC.Slug, rc.Slug)

			if len(tt.wantRC.Packages) == 0 {
				assert.Empty(t, rc.Packages)
			} else {
				assert.Equal(t, tt.wantRC.Packages, rc.Packages)
			}

			if len(tt.wantRC.Casks) == 0 {
				assert.Empty(t, rc.Casks)
			} else {
				assert.Equal(t, tt.wantRC.Casks, rc.Casks)
			}

			if len(tt.wantRC.Npm) == 0 {
				assert.Empty(t, rc.Npm)
			} else {
				assert.Equal(t, tt.wantRC.Npm, rc.Npm)
			}

			if len(tt.wantRC.Taps) == 0 {
				assert.Empty(t, rc.Taps)
			} else {
				assert.Equal(t, tt.wantRC.Taps, rc.Taps)
			}
		})
	}
}

func TestRevisionPackagesToRemoteConfig_PreservesOrder(t *testing.T) {
	pkgs := []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}{
		{Name: "zzz", Type: "formula"},
		{Name: "aaa", Type: "formula"},
		{Name: "mmm", Type: "formula"},
	}

	rc := revisionPackagesToRemoteConfig(pkgs, "user", "slug")

	assert.Equal(t, "zzz", rc.Packages[0].Name)
	assert.Equal(t, "aaa", rc.Packages[1].Name)
	assert.Equal(t, "mmm", rc.Packages[2].Name)
}
