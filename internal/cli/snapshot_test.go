package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
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

func TestBuildImportConfig_DryRunFlag(t *testing.T) {
	snap := &snapshot.Snapshot{}

	cfgDry := buildImportConfig(snap, true)
	cfgLive := buildImportConfig(snap, false)

	assert.True(t, cfgDry.DryRun)
	assert.False(t, cfgLive.DryRun)
}

func TestBuildImportConfig_ShellFields(t *testing.T) {
	snap := &snapshot.Snapshot{
		Shell: snapshot.ShellSnapshot{
			OhMyZsh: true,
			Theme:   "agnoster",
			Plugins: []string{"git", "zsh-autosuggestions"},
		},
	}

	cfg := buildImportConfig(snap, false)

	assert.True(t, cfg.SnapshotShellOhMyZsh)
	assert.Equal(t, "agnoster", cfg.SnapshotShellTheme)
	assert.Equal(t, []string{"git", "zsh-autosuggestions"}, cfg.SnapshotShellPlugins)
}

func TestBuildImportConfig_MacOSPrefs(t *testing.T) {
	snap := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "1", Desc: "Autohide dock"},
		},
	}

	cfg := buildImportConfig(snap, false)

	require.Len(t, cfg.SnapshotMacOS, 1)
	assert.Equal(t, "com.apple.dock", cfg.SnapshotMacOS[0].Domain)
	assert.Equal(t, "autohide", cfg.SnapshotMacOS[0].Key)
	assert.Equal(t, "1", cfg.SnapshotMacOS[0].Value)
	assert.Equal(t, "Autohide dock", cfg.SnapshotMacOS[0].Desc)
}

func TestBuildImportConfig_MacOSPrefs_PreservesUnset(t *testing.T) {
	snap := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "1", Desc: "set"},
			{Domain: "com.apple.dock", Key: "tilesize", Type: "int", Value: "48", Desc: "unset", Unset: true},
			{Domain: "com.apple.finder", Key: "ShowPathbar", Type: "bool", Value: "true", Desc: "set"},
		},
	}

	cfg := buildImportConfig(snap, false)

	// All three prefs propagate. Unset entries carry the catalog default
	// (the value we want to enforce anyway), so dropping them here would
	// silently lose categories whose keys aren't in the user's plist —
	// the exact bug that hid 8/9 Menu Bar items from openboot.dev.
	require.Len(t, cfg.SnapshotMacOS, 3)
	assert.Equal(t, "autohide", cfg.SnapshotMacOS[0].Key)
	assert.Equal(t, "tilesize", cfg.SnapshotMacOS[1].Key)
	assert.Equal(t, "ShowPathbar", cfg.SnapshotMacOS[2].Key)
}

func TestBuildImportConfig_InvalidDotfilesURL_Skipped(t *testing.T) {
	snap := &snapshot.Snapshot{
		Dotfiles: snapshot.DotfilesSnapshot{
			RepoURL: "not-a-valid-url",
		},
	}

	cfg := buildImportConfig(snap, false)

	// Invalid URL should be silently skipped.
	assert.Empty(t, cfg.SnapshotDotfiles)
	assert.Empty(t, cfg.DotfilesURL)
}

func TestBuildImportConfig_TapsPreserved(t *testing.T) {
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Taps: []string{"homebrew/cask-fonts", "homebrew/cask-versions"},
		},
	}

	cfg := buildImportConfig(snap, false)

	assert.Equal(t, []string{"homebrew/cask-fonts", "homebrew/cask-versions"}, cfg.SnapshotTaps)
}

// ── resolveTargetSlug ─────────────────────────────────────────────────────────

func TestResolveTargetSlug_ExplicitTakesPrecedence(t *testing.T) {
	// Even if a sync source exists on disk, explicit arg wins.
	t.Setenv("HOME", t.TempDir())
	writeSyncSourceForSnap(t, "existing-slug")

	result := resolveTargetSlug("my-explicit-slug")
	assert.Equal(t, "my-explicit-slug", result)
}

func TestResolveTargetSlug_FallsBackToSyncSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeSyncSourceForSnap(t, "stored-slug")

	result := resolveTargetSlug("")
	assert.Equal(t, "stored-slug", result)
}

func TestResolveTargetSlug_NoSyncSourceReturnsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // fresh dir, no sync source file

	result := resolveTargetSlug("")
	assert.Equal(t, "", result)
}

// writeSyncSourceForSnap writes a sync source to the temp HOME for snapshot tests.
func writeSyncSourceForSnap(t *testing.T, slug string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0700))
	src := syncpkg.SyncSource{
		Slug:        slug,
		Username:    "testuser",
		InstalledAt: time.Now(),
	}
	data, err := json.MarshalIndent(src, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sync_source.json"), data, 0600))
}

// ── loadSnapshot ──────────────────────────────────────────────────────────────

func TestLoadSnapshot_RejectsInsecureHTTP(t *testing.T) {
	_, err := loadSnapshot("http://example.com/snap.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insecure HTTP not allowed")
}

func TestLoadSnapshot_LocalFile(t *testing.T) {
	dir := t.TempDir()

	snap := &snapshot.Snapshot{
		CapturedAt: time.Now(),
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "ripgrep"},
			Casks:    []string{"firefox"},
		},
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	snapFile := filepath.Join(dir, "snap.json")
	require.NoError(t, os.WriteFile(snapFile, data, 0600))

	loaded, err := loadSnapshot(snapFile)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, []string{"git", "ripgrep"}, loaded.Packages.Formulae)
	assert.Equal(t, []string{"firefox"}, loaded.Packages.Casks)
}

func TestLoadSnapshot_LocalFile_NotFound(t *testing.T) {
	_, err := loadSnapshot("/tmp/this-file-should-not-exist-openboot-test.json")
	require.Error(t, err)
}

// ── recordPublishResult ───────────────────────────────────────────────────────

func withNoSnapshotBrowser(t *testing.T) {
	t.Helper()
	orig := openBrowser
	openBrowser = func(string) error { return nil }
	t.Cleanup(func() { openBrowser = orig })
}

func TestRecordPublishResult_NewConfig_SavesSyncSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	withNoSnapshotBrowser(t)

	// targetSlug="" means first publish → should save sync source.
	recordPublishResult("alice", "my-new-config", "", "public", "https://openboot.dev")

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "alice", loaded.Username)
	assert.Equal(t, "my-new-config", loaded.Slug)
	assert.Equal(t, "alice/my-new-config", loaded.UserSlug)
}

func TestRecordPublishResult_UpdateExisting_DoesNotOverrideSyncSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	withNoSnapshotBrowser(t)

	// Pre-write a sync source for a different config.
	writeSyncSourceForSnap(t, "original-slug")

	// targetSlug != "" → update path, should NOT overwrite sync source.
	recordPublishResult("alice", "original-slug", "original-slug", "unlisted", "https://openboot.dev")

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	// The existing sync source must be preserved (not overwritten by recordPublishResult).
	assert.Equal(t, "original-slug", loaded.Slug)
}

func TestRecordPublishResult_EmptyResultSlug_NoSave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	withNoSnapshotBrowser(t)

	// resultSlug="" and targetSlug="" — nothing to save.
	recordPublishResult("alice", "", "", "private", "https://openboot.dev")

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	// No sync source should have been created.
	assert.Nil(t, loaded)
}

// ── buildImportConfig online packages ────────────────────────────────────────

func TestBuildImportConfig_OnlinePackagesSeparatedFromCatalog(t *testing.T) {
	// Packages NOT in the catalog go into OnlinePkgs; catalog packages go into SelectedPkgs.
	// We use a definitely-not-in-catalog name to exercise the else branch.
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"__not_in_catalog_xyzabc__"},
			Casks:    []string{"__not_in_catalog_cask_xyz__"},
			Npm:      []string{"__not_in_catalog_npm_xyz__"},
		},
	}

	cfg := buildImportConfig(snap, false)

	// All three should land in OnlinePkgs because they don't match catalog names.
	require.Len(t, cfg.OnlinePkgs, 3)
	assert.Equal(t, "__not_in_catalog_xyzabc__", cfg.OnlinePkgs[0].Name)
	assert.False(t, cfg.OnlinePkgs[0].IsCask)
	assert.False(t, cfg.OnlinePkgs[0].IsNpm)
	assert.Equal(t, "__not_in_catalog_cask_xyz__", cfg.OnlinePkgs[1].Name)
	assert.True(t, cfg.OnlinePkgs[1].IsCask)
	assert.Equal(t, "__not_in_catalog_npm_xyz__", cfg.OnlinePkgs[2].Name)
	assert.True(t, cfg.OnlinePkgs[2].IsNpm)
}

// ── confirmInstallation ───────────────────────────────────────────────────────

func TestConfirmInstallation_DryRunSkipsPrompt(t *testing.T) {
	edited := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "vim"},
			Casks:    []string{"firefox"},
			Npm:      []string{"typescript"},
			Taps:     []string{"homebrew/cask-fonts"},
		},
	}

	// dryRun=true must return (true, nil) without prompting.
	ok, err := confirmInstallation(edited, true)
	require.NoError(t, err)
	assert.True(t, ok)
}

// ── saveSyncSourceIfRemote ────────────────────────────────────────────────────

func TestSaveSyncSourceIfRemote_NilRemoteConfigNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c := &config.Config{RemoteConfig: nil}
	// Should be a no-op — no panic, no file written.
	saveSyncSourceIfRemote(c)

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestSaveSyncSourceIfRemote_WithRemoteConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c := &config.Config{
		User: "alice/dev-setup",
		RemoteConfig: &config.RemoteConfig{
			Username: "alice",
			Slug:     "dev-setup",
		},
	}
	saveSyncSourceIfRemote(c)

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "alice/dev-setup", loaded.UserSlug)
	assert.Equal(t, "alice", loaded.Username)
	assert.Equal(t, "dev-setup", loaded.Slug)
}
