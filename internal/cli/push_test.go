package cli

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestRemoteConfigToAPIPackages(t *testing.T) {
	tests := []struct {
		name     string
		rc       *config.RemoteConfig
		expected []apiPackage
	}{
		{
			name:     "empty config",
			rc:       &config.RemoteConfig{},
			expected: []apiPackage{},
		},
		{
			name: "formulae only",
			rc: &config.RemoteConfig{
				Packages: config.PackageEntryList{{Name: "git"}, {Name: "go"}},
			},
			expected: []apiPackage{
				{Name: "git", Type: "formula"},
				{Name: "go", Type: "formula"},
			},
		},
		{
			name: "all types including taps",
			rc: &config.RemoteConfig{
				Packages: config.PackageEntryList{{Name: "git"}},
				Casks:    config.PackageEntryList{{Name: "docker"}},
				Npm:      config.PackageEntryList{{Name: "typescript"}},
				Taps:     []string{"homebrew/cask-fonts", "hashicorp/tap"},
			},
			expected: []apiPackage{
				{Name: "git", Type: "formula"},
				{Name: "docker", Type: "cask"},
				{Name: "typescript", Type: "npm"},
				{Name: "homebrew/cask-fonts", Type: "tap"},
				{Name: "hashicorp/tap", Type: "tap"},
			},
		},
		{
			name: "taps only",
			rc: &config.RemoteConfig{
				Taps: []string{"homebrew/core"},
			},
			expected: []apiPackage{
				{Name: "homebrew/core", Type: "tap"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := remoteConfigToAPIPackages(tt.rc)
			if len(tt.expected) == 0 {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestRemoteConfigToAPIPackagesImmutability(t *testing.T) {
	rc := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}},
		Taps:     []string{"homebrew/core"},
	}

	originalPackages := make(config.PackageEntryList, len(rc.Packages))
	copy(originalPackages, rc.Packages)
	originalTaps := make([]string, len(rc.Taps))
	copy(originalTaps, rc.Taps)

	_ = remoteConfigToAPIPackages(rc)

	assert.Equal(t, originalPackages, rc.Packages, "Packages slice should not be mutated")
	assert.Equal(t, originalTaps, rc.Taps, "Taps slice should not be mutated")
}

func TestTapsNotInRequestBodyAsTopLevelField(t *testing.T) {
	// This test verifies the structural expectation: taps should appear
	// in the packages array as {name, type:"tap"} entries, not as a
	// separate top-level "taps" field. The pushConfig function builds
	// reqBody without a "taps" key — this test documents that contract
	// by checking remoteConfigToAPIPackages includes taps.
	rc := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}},
		Taps:     []string{"hashicorp/tap"},
	}

	result := remoteConfigToAPIPackages(rc)

	tapEntries := []apiPackage{}
	for _, p := range result {
		if p.Type == "tap" {
			tapEntries = append(tapEntries, p)
		}
	}
	assert.Len(t, tapEntries, 1)
	assert.Equal(t, "hashicorp/tap", tapEntries[0].Name)
}
