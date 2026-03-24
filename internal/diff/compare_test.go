package diff

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/stretchr/testify/assert"
)

func TestCompareSnapshots_Identical(t *testing.T) {
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "curl"},
			Casks:    []string{"firefox"},
			Npm:      []string{"typescript"},
			Taps:     []string{"homebrew/core"},
		},
		Git: snapshot.GitSnapshot{
			UserName:  "Alice",
			UserEmail: "alice@example.com",
		},
	}

	source := Source{Kind: "local", Path: "~/.openboot/snapshot.json"}
	result := CompareSnapshots(snap, snap, source)

	assert.False(t, result.HasChanges())
	assert.Equal(t, 2, result.Packages.Formulae.Common)
	assert.Equal(t, 1, result.Packages.Casks.Common)
	assert.Empty(t, result.Packages.Formulae.Missing)
	assert.Empty(t, result.Packages.Formulae.Extra)
}

func TestCompareSnapshots_PackageDifferences(t *testing.T) {
	system := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "wget"},
			Casks:    []string{"firefox", "chrome"},
		},
	}
	reference := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "ripgrep"},
			Casks:    []string{"firefox", "slack"},
		},
	}

	result := CompareSnapshots(system, reference, Source{Kind: "file", Path: "ref.json"})

	assert.Equal(t, []string{"ripgrep"}, result.Packages.Formulae.Missing)
	assert.Equal(t, []string{"wget"}, result.Packages.Formulae.Extra)
	assert.Equal(t, 1, result.Packages.Formulae.Common)

	assert.Equal(t, []string{"slack"}, result.Packages.Casks.Missing)
	assert.Equal(t, []string{"chrome"}, result.Packages.Casks.Extra)
	assert.Equal(t, 1, result.Packages.Casks.Common)
}

func TestCompareSnapshots_MacOSDifferences(t *testing.T) {
	system := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "NSGlobalDomain", Key: "AppleShowAllExtensions", Value: "true"},
			{Domain: "com.apple.dock", Key: "autohide", Value: "false"},
		},
	}
	reference := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "NSGlobalDomain", Key: "AppleShowAllExtensions", Value: "true"},
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
			{Domain: "com.apple.dock", Key: "tilesize", Value: "48"},
		},
	}

	result := CompareSnapshots(system, reference, Source{})

	assert.NotNil(t, result.MacOS)
	assert.Len(t, result.MacOS.Changed, 1)
	assert.Equal(t, "autohide", result.MacOS.Changed[0].Key)
	assert.Len(t, result.MacOS.Missing, 1)
	assert.Equal(t, "tilesize", result.MacOS.Missing[0].Key)
	assert.Empty(t, result.MacOS.Extra)
}

func TestCompareSnapshots_DevToolDifferences(t *testing.T) {
	system := &snapshot.Snapshot{
		DevTools: []snapshot.DevTool{
			{Name: "go", Version: "1.22"},
			{Name: "node", Version: "20.0"},
			{Name: "python", Version: "3.12"},
		},
	}
	reference := &snapshot.Snapshot{
		DevTools: []snapshot.DevTool{
			{Name: "go", Version: "1.24"},
			{Name: "node", Version: "20.0"},
			{Name: "rust", Version: "1.75"},
		},
	}

	result := CompareSnapshots(system, reference, Source{})

	assert.NotNil(t, result.DevTools)
	assert.Equal(t, []string{"rust"}, result.DevTools.Missing)
	assert.Equal(t, []string{"python"}, result.DevTools.Extra)
	assert.Len(t, result.DevTools.Changed, 1)
	assert.Equal(t, "go", result.DevTools.Changed[0].Name)
	assert.Equal(t, "1.22", result.DevTools.Changed[0].System)
	assert.Equal(t, "1.24", result.DevTools.Changed[0].Reference)
	assert.Equal(t, 1, result.DevTools.Common) // node
}

func TestCompareSnapshotToRemote(t *testing.T) {
	system := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "wget"},
			Casks:    []string{"firefox"},
			Npm:      []string{"typescript"},
			Taps:     []string{"homebrew/core"},
		},
		Git: snapshot.GitSnapshot{UserName: "Alice"},
	}
	remote := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}, {Name: "ripgrep"}},
		Casks:    config.PackageEntryList{{Name: "firefox"}, {Name: "slack"}},
		Npm:      config.PackageEntryList{{Name: "typescript"}},
		Taps:     []string{"homebrew/core"},
	}

	source := Source{Kind: "remote", Path: "alice/my-config"}
	result := CompareSnapshotToRemote(system, remote, source)

	// Packages should be compared
	assert.Equal(t, []string{"ripgrep"}, result.Packages.Formulae.Missing)
	assert.Equal(t, []string{"wget"}, result.Packages.Formulae.Extra)
	assert.Equal(t, []string{"slack"}, result.Packages.Casks.Missing)

	// Non-package sections should be nil for remote configs
	assert.Nil(t, result.MacOS)
	assert.Nil(t, result.DevTools)
}

func TestCompareSnapshotToRemote_EmptyRemote(t *testing.T) {
	system := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git"},
		},
	}
	remote := &config.RemoteConfig{}

	result := CompareSnapshotToRemote(system, remote, Source{})

	assert.Equal(t, []string{"git"}, result.Packages.Formulae.Extra)
	assert.Empty(t, result.Packages.Formulae.Missing)
}

func TestCompareSnapshots_MacOSExtraPrefs(t *testing.T) {
	system := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
			{Domain: "com.apple.finder", Key: "ShowHardDrivesOnDesktop", Value: "true"},
		},
	}
	reference := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
		},
	}

	result := CompareSnapshots(system, reference, Source{})

	assert.NotNil(t, result.MacOS)
	assert.Empty(t, result.MacOS.Changed)
	assert.Empty(t, result.MacOS.Missing)
	assert.Len(t, result.MacOS.Extra, 1)
	assert.Equal(t, "ShowHardDrivesOnDesktop", result.MacOS.Extra[0].Key)
}

func TestCompareSnapshots_EmptySnapshots(t *testing.T) {
	system := &snapshot.Snapshot{}
	reference := &snapshot.Snapshot{}

	result := CompareSnapshots(system, reference, Source{})

	assert.False(t, result.HasChanges())
	assert.Equal(t, 0, result.TotalMissing())
	assert.Equal(t, 0, result.TotalExtra())
	assert.Equal(t, 0, result.TotalChanged())
}
