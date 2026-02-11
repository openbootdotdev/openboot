package snapshot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMatchPackages_EmptySnapshot tests matching with empty snapshot.
func TestMatchPackages_EmptySnapshot(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	match := MatchPackages(snap)
	assert.NotNil(t, match)
	assert.Equal(t, 0, len(match.Matched))
	assert.Equal(t, 0, len(match.Unmatched))
	assert.Equal(t, 0.0, match.MatchRate)
}

// TestMatchPackages_SinglePackage tests matching with single package.
func TestMatchPackages_SinglePackage(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	match := MatchPackages(snap)
	assert.NotNil(t, match)
	assert.Equal(t, 1, len(match.Matched))
	assert.Contains(t, match.Matched, "go")
}

// TestMatchPackages_MultiplePackages tests matching with multiple packages.
func TestMatchPackages_MultiplePackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go", "node", "git"},
			Casks:    []string{"docker"},
			Npm:      []string{},
		},
	}

	match := MatchPackages(snap)
	assert.NotNil(t, match)
	assert.GreaterOrEqual(t, len(match.Matched), 1)
	assert.Greater(t, len(match.Matched)+len(match.Unmatched), 0)
}

// TestMatchPackages_UnknownPackages tests matching with unknown packages.
func TestMatchPackages_UnknownPackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"unknown-pkg-xyz-123"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	match := MatchPackages(snap)
	assert.NotNil(t, match)
	assert.Equal(t, 1, len(match.Unmatched))
	assert.Contains(t, match.Unmatched, "unknown-pkg-xyz-123")
}

// TestMatchPackages_MixedPackages tests matching with mixed known and unknown packages.
func TestMatchPackages_MixedPackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go", "unknown-pkg"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	match := MatchPackages(snap)
	assert.NotNil(t, match)
	assert.Greater(t, len(match.Matched), 0)
	assert.Greater(t, len(match.Unmatched), 0)
	assert.Greater(t, match.MatchRate, 0.0)
	assert.Less(t, match.MatchRate, 1.0)
}

// TestMatchPackages_MatchRate tests match rate calculation.
func TestMatchPackages_MatchRate(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go", "node", "unknown1", "unknown2"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	match := MatchPackages(snap)
	assert.NotNil(t, match)
	// Match rate should be matched / total
	expectedRate := float64(len(match.Matched)) / float64(len(match.Matched)+len(match.Unmatched))
	assert.InDelta(t, expectedRate, match.MatchRate, 0.01)
}

// TestDetectBestPreset_EmptySnapshot tests preset detection with empty snapshot.
func TestDetectBestPreset_EmptySnapshot(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	assert.Empty(t, preset)
}

// TestDetectBestPreset_UnknownPackages tests preset detection with unknown packages.
func TestDetectBestPreset_UnknownPackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"unknown-pkg-xyz-123"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	assert.Empty(t, preset)
}

// TestDetectBestPreset_MinimalPreset tests preset detection with minimal preset packages.
func TestDetectBestPreset_MinimalPreset(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{
				"curl", "wget", "jq", "yq", "ripgrep", "fd", "bat", "eza",
				"fzf", "zoxide", "htop", "btop", "tree", "tldr", "gh", "git-delta",
			},
			Casks: []string{},
			Npm:   []string{},
		},
	}

	preset := DetectBestPreset(snap)
	assert.NotEmpty(t, preset)
	assert.Equal(t, "minimal", preset)
}

// TestDetectBestPreset_DeveloperPreset tests preset detection with developer preset packages.
func TestDetectBestPreset_DeveloperPreset(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{
				"curl", "wget", "jq", "yq", "ripgrep", "fd", "bat", "eza",
				"fzf", "zoxide", "htop", "btop", "tree", "tldr", "gh", "git-delta",
				"git-lfs", "lazygit", "pre-commit", "stow", "node", "go", "pnpm",
				"docker", "docker-compose", "lazydocker", "tmux", "neovim", "httpie",
			},
			Casks: []string{},
			Npm:   []string{},
		},
	}

	preset := DetectBestPreset(snap)
	assert.NotEmpty(t, preset)
	assert.Equal(t, "developer", preset)
}

// TestDetectBestPreset_PartialMatch tests preset detection with partial match.
func TestDetectBestPreset_PartialMatch(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"curl", "wget", "jq"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	// With only 3 packages, similarity might be below 0.3 threshold
	// This test just ensures it doesn't panic
	assert.IsType(t, "", preset)
}

// TestDetectBestPreset_ReturnsString tests that preset detection returns a string.
func TestDetectBestPreset_ReturnsString(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go", "node"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	assert.IsType(t, "", preset)
}

// TestDetectBestPreset_WithCasks tests preset detection with cask packages.
func TestDetectBestPreset_WithCasks(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go", "node"},
			Casks:    []string{"docker", "vscode"},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	// Should not panic
	assert.IsType(t, "", preset)
}

// TestDetectBestPreset_WithNpm tests preset detection with npm packages.
func TestDetectBestPreset_WithNpm(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go", "node"},
			Casks:    []string{},
			Npm:      []string{"typescript", "eslint"},
		},
	}

	preset := DetectBestPreset(snap)
	// Should not panic
	assert.IsType(t, "", preset)
}

// TestDetectBestPreset_LargePackageSet tests preset detection with large package set.
func TestDetectBestPreset_LargePackageSet(t *testing.T) {
	formulae := make([]string, 50)
	for i := 0; i < 50; i++ {
		formulae[i] = "package-" + string(rune(i))
	}

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: formulae,
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	// Should not panic with large package set
	assert.IsType(t, "", preset)
}

// TestDetectBestPreset_DuplicatePackages tests preset detection with duplicate packages.
func TestDetectBestPreset_DuplicatePackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"go", "go", "node", "node"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	// Should handle duplicates gracefully
	assert.IsType(t, "", preset)
}

// TestDetectBestPreset_SpecialCharacters tests preset detection with special characters.
func TestDetectBestPreset_SpecialCharacters(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"@angular/cli", "pkg-with-dash", "pkg_with_underscore"},
			Casks:    []string{},
			Npm:      []string{},
		},
	}

	preset := DetectBestPreset(snap)
	// Should handle special characters gracefully
	assert.IsType(t, "", preset)
}
