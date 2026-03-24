package cli

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildImportConfig_DotfilesPopulated(t *testing.T) {
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git"},
		},
		Dotfiles: snapshot.DotfilesSnapshot{
			RepoURL: "https://github.com/testuser/dotfiles",
		},
	}

	cfg := buildImportConfig(snap, false)

	assert.Equal(t, "https://github.com/testuser/dotfiles", cfg.SnapshotDotfiles)
	assert.Equal(t, "https://github.com/testuser/dotfiles", cfg.DotfilesURL)
}

func TestBuildImportConfig_EmptyDotfiles(t *testing.T) {
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git"},
		},
	}

	cfg := buildImportConfig(snap, false)

	assert.Empty(t, cfg.SnapshotDotfiles)
	assert.Empty(t, cfg.DotfilesURL)
}

func TestBuildImportConfig_GitPopulated(t *testing.T) {
	snap := &snapshot.Snapshot{
		Git: snapshot.GitSnapshot{
			UserName:  "Test User",
			UserEmail: "test@example.com",
		},
	}

	cfg := buildImportConfig(snap, false)

	require.NotNil(t, cfg.SnapshotGit)
	assert.Equal(t, "Test User", cfg.SnapshotGit.UserName)
}

func TestBuildImportConfig_EmptySnapshot(t *testing.T) {
	snap := &snapshot.Snapshot{}

	cfg := buildImportConfig(snap, false)

	require.NotNil(t, cfg)
	require.NotNil(t, cfg.SelectedPkgs)
	assert.Empty(t, cfg.SelectedPkgs)
	assert.Empty(t, cfg.OnlinePkgs)
}
