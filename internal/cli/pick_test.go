package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

func TestParsePicks(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]bool
	}{
		{"empty", "", map[string]bool{}},
		{"single", "git", map[string]bool{"git": true}},
		{"multiple", "git,jq,ripgrep", map[string]bool{"git": true, "jq": true, "ripgrep": true}},
		{"whitespace trimmed", " git , jq ,ripgrep ", map[string]bool{"git": true, "jq": true, "ripgrep": true}},
		{"empty entries skipped", "git,,jq,", map[string]bool{"git": true, "jq": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePicks(tt.raw)
			assert.Equal(t, tt.want, got)
		})
	}
}

func sampleRemoteConfig() *config.RemoteConfig {
	return &config.RemoteConfig{
		Username:     "alice",
		Slug:         "dev",
		Packages:     config.PackageEntryList{{Name: "git"}, {Name: "jq"}, {Name: "ripgrep"}},
		Casks:        config.PackageEntryList{{Name: "visual-studio-code"}, {Name: "docker"}},
		Npm:          config.PackageEntryList{{Name: "typescript"}, {Name: "eslint"}},
		Taps:         []string{"homebrew/cask-fonts"},
		DotfilesRepo: "https://github.com/alice/dotfiles",
		PostInstall:  []string{"echo done"},
	}
}

func TestApplyPicks_FiltersFormulae(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, unknown := ApplyPicks(rc, map[string]bool{"git": true})
	require.Empty(t, unknown)
	assert.Equal(t, []string{"git"}, filtered.Packages.Names())
	assert.Empty(t, filtered.Casks)
	assert.Empty(t, filtered.Npm)
}

func TestApplyPicks_FiltersAcrossCategories(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, unknown := ApplyPicks(rc, map[string]bool{
		"git": true, "docker": true, "typescript": true,
	})
	require.Empty(t, unknown)
	assert.Equal(t, []string{"git"}, filtered.Packages.Names())
	assert.Equal(t, []string{"docker"}, filtered.Casks.Names())
	assert.Equal(t, []string{"typescript"}, filtered.Npm.Names())
}

func TestApplyPicks_PreservesNonPackageFields(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, _ := ApplyPicks(rc, map[string]bool{"git": true})
	assert.Equal(t, rc.Taps, filtered.Taps)
	assert.Equal(t, rc.DotfilesRepo, filtered.DotfilesRepo)
	assert.Equal(t, rc.PostInstall, filtered.PostInstall)
	assert.Equal(t, rc.Username, filtered.Username)
	assert.Equal(t, rc.Slug, filtered.Slug)
}

func TestApplyPicks_ReportsUnknownNames(t *testing.T) {
	rc := sampleRemoteConfig()
	_, unknown := ApplyPicks(rc, map[string]bool{"git": true, "nope": true, "alsonope": true})
	assert.ElementsMatch(t, []string{"nope", "alsonope"}, unknown)
}

func TestApplyPicks_EmptyPicksProducesEmptyLists(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, unknown := ApplyPicks(rc, map[string]bool{})
	assert.Empty(t, unknown)
	assert.Empty(t, filtered.Packages)
	assert.Empty(t, filtered.Casks)
	assert.Empty(t, filtered.Npm)
}

func TestApplyPicks_DoesNotMutateInput(t *testing.T) {
	rc := sampleRemoteConfig()
	origCount := len(rc.Packages)
	ApplyPicks(rc, map[string]bool{"git": true})
	assert.Equal(t, origCount, len(rc.Packages))
}
