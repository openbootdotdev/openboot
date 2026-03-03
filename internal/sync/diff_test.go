package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffLists(t *testing.T) {
	tests := []struct {
		name        string
		remote      []string
		local       []string
		wantMissing []string
		wantExtra   []string
	}{
		{
			name:        "identical",
			remote:      []string{"a", "b", "c"},
			local:       []string{"a", "b", "c"},
			wantMissing: nil,
			wantExtra:   nil,
		},
		{
			name:        "missing items",
			remote:      []string{"a", "b", "c"},
			local:       []string{"a"},
			wantMissing: []string{"b", "c"},
			wantExtra:   nil,
		},
		{
			name:        "extra items",
			remote:      []string{"a"},
			local:       []string{"a", "b", "c"},
			wantMissing: nil,
			wantExtra:   []string{"b", "c"},
		},
		{
			name:        "both missing and extra",
			remote:      []string{"a", "b"},
			local:       []string{"b", "c"},
			wantMissing: []string{"a"},
			wantExtra:   []string{"c"},
		},
		{
			name:        "empty remote",
			remote:      []string{},
			local:       []string{"a", "b"},
			wantMissing: nil,
			wantExtra:   []string{"a", "b"},
		},
		{
			name:        "empty local",
			remote:      []string{"a", "b"},
			local:       []string{},
			wantMissing: []string{"a", "b"},
			wantExtra:   nil,
		},
		{
			name:        "both empty",
			remote:      []string{},
			local:       []string{},
			wantMissing: nil,
			wantExtra:   nil,
		},
		{
			name:        "nil inputs",
			remote:      nil,
			local:       nil,
			wantMissing: nil,
			wantExtra:   nil,
		},
		{
			name:        "duplicates ignored",
			remote:      []string{"a", "a", "b"},
			local:       []string{"b", "b", "c"},
			wantMissing: []string{"a"},
			wantExtra:   []string{"c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, extra := diffLists(tt.remote, tt.local)
			assert.Equal(t, tt.wantMissing, missing)
			assert.Equal(t, tt.wantExtra, extra)
		})
	}
}

func TestSyncDiffHasChanges(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		d := &SyncDiff{}
		assert.False(t, d.HasChanges())
	})

	t.Run("missing formulae", func(t *testing.T) {
		d := &SyncDiff{MissingFormulae: []string{"ripgrep"}}
		assert.True(t, d.HasChanges())
	})

	t.Run("extra casks", func(t *testing.T) {
		d := &SyncDiff{ExtraCasks: []string{"slack"}}
		assert.True(t, d.HasChanges())
	})

	t.Run("dotfiles changed", func(t *testing.T) {
		d := &SyncDiff{DotfilesChanged: true}
		assert.True(t, d.HasChanges())
	})

	t.Run("shell changed", func(t *testing.T) {
		d := &SyncDiff{ShellChanged: true}
		assert.True(t, d.HasChanges())
	})

	t.Run("macos prefs changed", func(t *testing.T) {
		d := &SyncDiff{MacOSChanged: []MacOSPrefDiff{{Domain: "com.apple.dock", Key: "autohide"}}}
		assert.True(t, d.HasChanges())
	})
}

func TestSyncDiffTotals(t *testing.T) {
	d := &SyncDiff{
		MissingFormulae: []string{"ripgrep", "fd"},
		MissingCasks:    []string{"raycast"},
		MissingNpm:      []string{"turbo"},
		MissingTaps:     []string{"homebrew/cask-fonts"},
		ExtraFormulae:   []string{"htop"},
		ExtraCasks:      []string{"slack"},
		ExtraNpm:        []string{"create-react-app"},
		ExtraTaps:       []string{"some/tap"},
		DotfilesChanged: true,
		ShellChanged:    true,
		ShellDiff: &ShellDiff{
			ThemeChanged:   true,
			MissingPlugins: []string{"zsh-autosuggestions"},
			ExtraPlugins:   []string{"old-plugin"},
		},
		MacOSChanged: []MacOSPrefDiff{{Domain: "com.apple.dock", Key: "autohide"}},
	}

	// Missing: 2 formulae + 1 cask + 1 npm + 1 tap + 1 plugin = 6
	assert.Equal(t, 6, d.TotalMissing())

	// Extra: 1 formula + 1 cask + 1 npm + 1 tap + 1 plugin = 5
	assert.Equal(t, 5, d.TotalExtra())

	// Changed: 1 dotfiles + 1 theme + 1 macos pref = 3
	assert.Equal(t, 3, d.TotalChanged())
}

func TestSyncDiffTotalsNilShellDiff(t *testing.T) {
	d := &SyncDiff{
		MissingFormulae: []string{"ripgrep"},
		ExtraFormulae:   []string{"htop"},
		DotfilesChanged: true,
		MacOSChanged:    []MacOSPrefDiff{{Domain: "d", Key: "k"}},
	}

	// No ShellDiff → plugins not counted
	assert.Equal(t, 1, d.TotalMissing())
	assert.Equal(t, 1, d.TotalExtra())
	// Changed: 1 dotfiles + 0 theme + 1 macos = 2
	assert.Equal(t, 2, d.TotalChanged())
}

func TestSyncDiffTotalChangedThemeOnly(t *testing.T) {
	d := &SyncDiff{
		ShellChanged: true,
		ShellDiff: &ShellDiff{
			ThemeChanged: true,
			RemoteTheme:  "agnoster",
			LocalTheme:   "robbyrussell",
		},
	}
	assert.Equal(t, 1, d.TotalChanged())
}

func TestSyncDiffTotalChangedNoTheme(t *testing.T) {
	d := &SyncDiff{
		ShellChanged: true,
		ShellDiff: &ShellDiff{
			ThemeChanged:   false,
			MissingPlugins: []string{"zsh-autosuggestions"},
		},
	}
	// ThemeChanged=false → no changed count, but MissingPlugins counted in TotalMissing
	assert.Equal(t, 0, d.TotalChanged())
	assert.Equal(t, 1, d.TotalMissing())
}

func TestSyncDiffHasChangesAllFields(t *testing.T) {
	fields := []struct {
		name string
		diff SyncDiff
	}{
		{"MissingCasks", SyncDiff{MissingCasks: []string{"x"}}},
		{"MissingNpm", SyncDiff{MissingNpm: []string{"x"}}},
		{"MissingTaps", SyncDiff{MissingTaps: []string{"x"}}},
		{"ExtraFormulae", SyncDiff{ExtraFormulae: []string{"x"}}},
		{"ExtraNpm", SyncDiff{ExtraNpm: []string{"x"}}},
		{"ExtraTaps", SyncDiff{ExtraTaps: []string{"x"}}},
	}
	for _, tt := range fields {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, tt.diff.HasChanges())
		})
	}
}

func TestToSet(t *testing.T) {
	s := ToSet([]string{"a", "b", "a"})
	assert.Equal(t, map[string]bool{"a": true, "b": true}, s)

	s = ToSet(nil)
	assert.Equal(t, map[string]bool{}, s)
}

func TestGetLocalDotfilesURL(t *testing.T) {
	t.Run("dotfiles repo exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		dotfilesDir := filepath.Join(tmpDir, ".dotfiles")
		require.NoError(t, os.MkdirAll(dotfilesDir, 0755))

		// Initialize a git repo with a remote
		cmds := [][]string{
			{"git", "init", dotfilesDir},
			{"git", "-C", dotfilesDir, "remote", "add", "origin", "https://github.com/user/dots.git"},
		}
		for _, args := range cmds {
			cmd := exec.Command(args[0], args[1:]...)
			require.NoError(t, cmd.Run())
		}

		url := getLocalDotfilesURL()
		assert.Equal(t, "https://github.com/user/dots.git", url)
	})

	t.Run("no dotfiles dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		url := getLocalDotfilesURL()
		assert.Equal(t, "", url)
	})

	t.Run("dotfiles dir without git", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		dotfilesDir := filepath.Join(tmpDir, ".dotfiles")
		require.NoError(t, os.MkdirAll(dotfilesDir, 0755))

		url := getLocalDotfilesURL()
		assert.Equal(t, "", url)
	})
}

