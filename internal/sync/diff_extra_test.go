package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

// ---- diffDotfiles ----

func TestDiffDotfiles_SameURL(t *testing.T) {
	// When local and remote dotfiles URL are identical, no change should be recorded.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dotfilesDir := filepath.Join(tmpDir, ".dotfiles")
	require.NoError(t, os.MkdirAll(dotfilesDir, 0755))

	cmds := [][]string{
		{"git", "init", dotfilesDir},
		{"git", "-C", dotfilesDir, "remote", "add", "origin", "https://github.com/user/dotfiles.git"},
	}
	for _, args := range cmds {
		require.NoError(t, exec.Command(args[0], args[1:]...).Run())
	}

	rc := &config.RemoteConfig{DotfilesRepo: "https://github.com/user/dotfiles.git"}
	d := &SyncDiff{}
	diffDotfiles(rc, d)

	assert.False(t, d.DotfilesChanged)
	assert.Empty(t, d.RemoteDotfiles)
	assert.Empty(t, d.LocalDotfiles)
}

func TestDiffDotfiles_DifferentURL(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dotfilesDir := filepath.Join(tmpDir, ".dotfiles")
	require.NoError(t, os.MkdirAll(dotfilesDir, 0755))

	cmds := [][]string{
		{"git", "init", dotfilesDir},
		{"git", "-C", dotfilesDir, "remote", "add", "origin", "https://github.com/user/old-dotfiles.git"},
	}
	for _, args := range cmds {
		require.NoError(t, exec.Command(args[0], args[1:]...).Run())
	}

	rc := &config.RemoteConfig{DotfilesRepo: "https://github.com/user/new-dotfiles.git"}
	d := &SyncDiff{}
	diffDotfiles(rc, d)

	assert.True(t, d.DotfilesChanged)
	assert.Equal(t, "https://github.com/user/new-dotfiles.git", d.RemoteDotfiles)
	assert.Equal(t, "https://github.com/user/old-dotfiles.git", d.LocalDotfiles)
}

func TestDiffDotfiles_EmptyLocalURL(t *testing.T) {
	// When ~/.dotfiles doesn't exist, local URL is empty — change detected.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	rc := &config.RemoteConfig{DotfilesRepo: "https://github.com/user/dotfiles.git"}
	d := &SyncDiff{}
	diffDotfiles(rc, d)

	assert.True(t, d.DotfilesChanged)
	assert.Equal(t, "https://github.com/user/dotfiles.git", d.RemoteDotfiles)
	assert.Empty(t, d.LocalDotfiles)
}

func TestDiffDotfiles_EmptyRemoteURL(t *testing.T) {
	// When remote has no dotfiles repo, diffDotfiles should be a no-op.
	rc := &config.RemoteConfig{DotfilesRepo: ""}
	d := &SyncDiff{}
	diffDotfiles(rc, d)

	assert.False(t, d.DotfilesChanged)
}

// ---- diffShell ----

func TestDiffShell_NilShell(t *testing.T) {
	rc := &config.RemoteConfig{Shell: nil}
	d := &SyncDiff{}
	err := diffShell(rc, d)
	require.NoError(t, err)
	assert.Nil(t, d.Shell)
}

func TestDiffShell_OhMyZshFalse(t *testing.T) {
	rc := &config.RemoteConfig{
		Shell: &config.RemoteShellConfig{OhMyZsh: false, Theme: "robbyrussell"},
	}
	d := &SyncDiff{}
	err := diffShell(rc, d)
	require.NoError(t, err)
	assert.Nil(t, d.Shell)
}

func TestDiffShell_OhMyZsh_ThemeMatches(t *testing.T) {
	// Set up a HOME with a matching .zshrc so CaptureShell returns the same theme.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	zshrc := `ZSH_THEME="robbyrussell"` + "\n" + `plugins=(git z)`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".zshrc"), []byte(zshrc), 0644))

	rc := &config.RemoteConfig{
		Shell: &config.RemoteShellConfig{
			OhMyZsh: true,
			Theme:   "robbyrussell",
			Plugins: []string{"git", "z"},
		},
	}
	d := &SyncDiff{}
	err := diffShell(rc, d)
	require.NoError(t, err)
	// Theme and plugins match, so no shell diff should be recorded.
	assert.Nil(t, d.Shell)
}

func TestDiffShell_OhMyZsh_ThemeDiffers(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Local has "robbyrussell"; remote wants "agnoster"
	zshrc := `ZSH_THEME="robbyrussell"` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".zshrc"), []byte(zshrc), 0644))

	rc := &config.RemoteConfig{
		Shell: &config.RemoteShellConfig{
			OhMyZsh: true,
			Theme:   "agnoster",
		},
	}
	d := &SyncDiff{}
	err := diffShell(rc, d)
	require.NoError(t, err)
	require.NotNil(t, d.Shell)
	assert.True(t, d.Shell.ThemeChanged)
	assert.Equal(t, "agnoster", d.Shell.RemoteTheme)
	assert.Equal(t, "robbyrussell", d.Shell.LocalTheme)
}

func TestDiffShell_OhMyZsh_PluginsDiffer(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Local has "git"; remote wants "git z"
	zshrc := "ZSH_THEME=\"robbyrussell\"\nplugins=(git)\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".zshrc"), []byte(zshrc), 0644))

	rc := &config.RemoteConfig{
		Shell: &config.RemoteShellConfig{
			OhMyZsh: true,
			Theme:   "robbyrussell",
			Plugins: []string{"git", "z"},
		},
	}
	d := &SyncDiff{}
	err := diffShell(rc, d)
	require.NoError(t, err)
	require.NotNil(t, d.Shell)
	assert.True(t, d.Shell.PluginsChanged)
	assert.Equal(t, []string{"git", "z"}, d.Shell.RemotePlugins)
}

func TestDiffShell_OhMyZsh_NoZshrc(t *testing.T) {
	// No .zshrc in HOME — CaptureShell returns zero-value (no theme/plugins).
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	rc := &config.RemoteConfig{
		Shell: &config.RemoteShellConfig{
			OhMyZsh: true,
			Theme:   "robbyrussell",
		},
	}
	d := &SyncDiff{}
	err := diffShell(rc, d)
	require.NoError(t, err)
	// Remote has a theme, local has none (empty) → ThemeChanged.
	require.NotNil(t, d.Shell)
	assert.True(t, d.Shell.ThemeChanged)
}

// ---- diffMacOSPrefs ----

func TestDiffMacOSPrefs_EmptyPrefs(t *testing.T) {
	rc := &config.RemoteConfig{MacOSPrefs: nil}
	d := &SyncDiff{}
	err := diffMacOSPrefs(rc, d)
	require.NoError(t, err)
	assert.Empty(t, d.MacOSChanged)
}

func TestDiffMacOSPrefs_EmptyPrefsList(t *testing.T) {
	rc := &config.RemoteConfig{MacOSPrefs: []config.RemoteMacOSPref{}}
	d := &SyncDiff{}
	err := diffMacOSPrefs(rc, d)
	require.NoError(t, err)
	assert.Empty(t, d.MacOSChanged)
}

// ---- computeMacOSPrefDiff (pure-logic core) ----

func TestComputeMacOSPrefDiff_UnsetLocalCountsAsMissing(t *testing.T) {
	// Regression: a locally captured Unset pref carries the catalog default in
	// Value as a placeholder. The diff must treat it as "not on the machine"
	// so the remote pref still drives a write, instead of falsely matching.
	remote := []config.RemoteMacOSPref{
		{Domain: "com.apple.controlcenter", Key: "Bluetooth", Type: "int", Value: "18", Desc: "Always show Bluetooth"},
	}
	local := []snapshot.MacOSPref{
		{Domain: "com.apple.controlcenter", Key: "Bluetooth", Type: "int", Value: "18", Unset: true},
	}
	changed := computeMacOSPrefDiff(remote, local)
	require.Len(t, changed, 1)
	assert.Equal(t, "Bluetooth", changed[0].Key)
	assert.Equal(t, "18", changed[0].RemoteValue)
	assert.Empty(t, changed[0].LocalValue, "Unset local pref must not surface its placeholder value")
}

func TestComputeMacOSPrefDiff_ExplicitLocalMatchesRemote(t *testing.T) {
	// Sanity: an explicit (non-Unset) local value equal to remote → no diff.
	remote := []config.RemoteMacOSPref{
		{Domain: "com.apple.controlcenter", Key: "Sound", Type: "int", Value: "18"},
	}
	local := []snapshot.MacOSPref{
		{Domain: "com.apple.controlcenter", Key: "Sound", Type: "int", Value: "18"},
	}
	assert.Empty(t, computeMacOSPrefDiff(remote, local))
}

func TestComputeMacOSPrefDiff_ExplicitLocalDiffersFromRemote(t *testing.T) {
	remote := []config.RemoteMacOSPref{
		{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
	}
	local := []snapshot.MacOSPref{
		{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "false"},
	}
	changed := computeMacOSPrefDiff(remote, local)
	require.Len(t, changed, 1)
	assert.Equal(t, "true", changed[0].RemoteValue)
	assert.Equal(t, "false", changed[0].LocalValue)
}

// ---- ComputeDiff (pure-logic path — no packages) ----

func TestComputeDiff_EmptyRemoteConfig(t *testing.T) {
	if _, err := exec.LookPath("brew"); err != nil {
		t.Skip("brew not available; skipping ComputeDiff integration test")
	}

	rc := &config.RemoteConfig{}
	d, err := ComputeDiff(rc)
	require.NoError(t, err)
	require.NotNil(t, d)
	// With an empty remote config nothing can be "missing" from remote.
	assert.Empty(t, d.MissingFormulae)
	assert.Empty(t, d.MissingCasks)
	assert.Empty(t, d.MissingNpm)
	assert.Empty(t, d.MissingTaps)
	assert.False(t, d.DotfilesChanged)
	assert.Nil(t, d.Shell)
	assert.Empty(t, d.MacOSChanged)
}

func TestComputeDiff_MacOSPrefsOnly(t *testing.T) {
	if _, err := exec.LookPath("brew"); err != nil {
		t.Skip("brew not available; skipping ComputeDiff integration test")
	}

	rc := &config.RemoteConfig{
		MacOSPrefs: []config.RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "false"},
		},
	}
	d, err := ComputeDiff(rc)
	require.NoError(t, err)
	require.NotNil(t, d)
	// Either the local pref matches (no diff) or doesn't (diff detected).
	// We simply verify the call succeeds and returns a valid result.
	assert.NotNil(t, d)
}

// ---- SyncDiff TotalChanged with Shell ----

func TestSyncDiffTotalChanged_WithShell(t *testing.T) {
	d := &SyncDiff{
		Shell: &ShellDiff{ThemeChanged: true},
	}
	assert.Equal(t, 1, d.TotalChanged())
}

func TestSyncDiffTotalChanged_WithShellAndDotfiles(t *testing.T) {
	d := &SyncDiff{
		DotfilesChanged: true,
		Shell:           &ShellDiff{PluginsChanged: true},
	}
	assert.Equal(t, 2, d.TotalChanged())
}

func TestSyncDiffHasChanges_ShellDiff(t *testing.T) {
	d := &SyncDiff{Shell: &ShellDiff{ThemeChanged: true}}
	assert.True(t, d.HasChanges())
}

// ---- ShellDiff fields ----

func TestShellDiff_Fields(t *testing.T) {
	sd := &ShellDiff{
		ThemeChanged:   true,
		RemoteTheme:    "agnoster",
		LocalTheme:     "robbyrussell",
		PluginsChanged: true,
		RemotePlugins:  []string{"git", "z"},
		LocalPlugins:   []string{"git"},
	}
	assert.True(t, sd.ThemeChanged)
	assert.True(t, sd.PluginsChanged)
	assert.Equal(t, "agnoster", sd.RemoteTheme)
	assert.Equal(t, "robbyrussell", sd.LocalTheme)
	assert.Equal(t, []string{"git", "z"}, sd.RemotePlugins)
	assert.Equal(t, []string{"git"}, sd.LocalPlugins)
}

// ---- MacOSPrefDiff fields ----

func TestMacOSPrefDiff_Fields(t *testing.T) {
	mp := MacOSPrefDiff{
		Domain:      "com.apple.dock",
		Key:         "autohide",
		Type:        "bool",
		Desc:        "Auto-hide Dock",
		RemoteValue: "true",
		LocalValue:  "false",
	}
	assert.Equal(t, "com.apple.dock", mp.Domain)
	assert.Equal(t, "autohide", mp.Key)
	assert.Equal(t, "bool", mp.Type)
	assert.Equal(t, "Auto-hide Dock", mp.Desc)
	assert.Equal(t, "true", mp.RemoteValue)
	assert.Equal(t, "false", mp.LocalValue)
}

// ---- diffLists — edge-case additions ----

func TestDiffLists_AllIntersect(t *testing.T) {
	missing, extra := diffLists([]string{"a", "b", "c"}, []string{"a", "b", "c"})
	assert.Nil(t, missing)
	assert.Nil(t, extra)
}

func TestDiffLists_BothEmpty(t *testing.T) {
	missing, extra := diffLists([]string{}, []string{})
	assert.Nil(t, missing)
	assert.Nil(t, extra)
}

func TestDiffLists_NilInputs(t *testing.T) {
	missing, extra := diffLists(nil, nil)
	assert.Nil(t, missing)
	assert.Nil(t, extra)
}

func TestDiffLists_RemoteOnlyItems(t *testing.T) {
	// remote has "x", "y"; local has nothing → both missing
	missing, extra := diffLists([]string{"x", "y"}, []string{})
	assert.Equal(t, []string{"x", "y"}, missing)
	assert.Nil(t, extra)
}

func TestDiffLists_LocalOnlyItems(t *testing.T) {
	// remote has nothing; local has "a", "b" → both extra
	missing, extra := diffLists([]string{}, []string{"a", "b"})
	assert.Nil(t, missing)
	assert.Equal(t, []string{"a", "b"}, extra)
}
