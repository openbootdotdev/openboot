package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
)

// ── fallbackStr ───────────────────────────────────────────────────────────────

func TestFallbackStr(t *testing.T) {
	tests := []struct {
		name string
		s    string
		def  string
		want string
	}{
		{"non-empty string returned as-is", "actual", "(none)", "actual"},
		{"empty string returns default", "", "(none)", "(none)"},
		{"empty default with non-empty string", "value", "", "value"},
		{"both empty returns empty default", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fallbackStr(tt.s, tt.def))
		})
	}
}

// ── sourceLabel ───────────────────────────────────────────────────────────────

func TestSourceLabel(t *testing.T) {
	tests := []struct {
		name   string
		source *syncpkg.SyncSource
		want   string
	}{
		{
			name:   "username and slug both set",
			source: &syncpkg.SyncSource{Username: "alice", Slug: "dev-setup", UserSlug: "alice/dev-setup"},
			want:   "@alice/dev-setup",
		},
		{
			name:   "only userslug set falls back to userslug",
			source: &syncpkg.SyncSource{UserSlug: "alice/dev-setup"},
			want:   "alice/dev-setup",
		},
		{
			name:   "username without slug falls back to userslug",
			source: &syncpkg.SyncSource{Username: "alice", UserSlug: "alice/dev-setup"},
			want:   "alice/dev-setup",
		},
		{
			name:   "slug without username falls back to userslug",
			source: &syncpkg.SyncSource{Slug: "dev-setup", UserSlug: "alice/dev-setup"},
			want:   "alice/dev-setup",
		},
		{
			name:   "all empty returns empty string",
			source: &syncpkg.SyncSource{},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sourceLabel(tt.source))
		})
	}
}

// ── sourceLabelForConfig ──────────────────────────────────────────────────────

func TestSourceLabelForConfig(t *testing.T) {
	tests := []struct {
		name string
		rc   *config.RemoteConfig
		want string
	}{
		{
			name: "username and slug set",
			rc:   &config.RemoteConfig{Username: "bob", Slug: "my-config"},
			want: "@bob/my-config",
		},
		{
			name: "only username, no slug",
			rc:   &config.RemoteConfig{Username: "bob"},
			want: "",
		},
		{
			name: "only slug, no username",
			rc:   &config.RemoteConfig{Slug: "my-config"},
			want: "",
		},
		{
			name: "both empty",
			rc:   &config.RemoteConfig{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sourceLabelForConfig(tt.rc))
		})
	}
}

// ── buildInstallPlan ──────────────────────────────────────────────────────────

func TestBuildInstallPlan_PackagesOnly(t *testing.T) {
	diff := &syncpkg.SyncDiff{
		MissingFormulae: []string{"git", "ripgrep"},
		MissingCasks:    []string{"firefox"},
		MissingNpm:      []string{"typescript"},
		MissingTaps:     []string{"homebrew/cask-fonts"},
	}
	rc := &config.RemoteConfig{}

	plan := buildInstallPlan(diff, rc)

	assert.Equal(t, []string{"git", "ripgrep"}, plan.InstallFormulae)
	assert.Equal(t, []string{"firefox"}, plan.InstallCasks)
	assert.Equal(t, []string{"typescript"}, plan.InstallNpm)
	assert.Equal(t, []string{"homebrew/cask-fonts"}, plan.InstallTaps)
	// Install plan is additive: no uninstalls should be set.
	assert.Empty(t, plan.UninstallFormulae)
	assert.Empty(t, plan.UninstallCasks)
	assert.Empty(t, plan.UninstallNpm)
	assert.Empty(t, plan.UninstallTaps)
}

func TestBuildInstallPlan_EmptyDiff(t *testing.T) {
	diff := &syncpkg.SyncDiff{}
	rc := &config.RemoteConfig{}

	plan := buildInstallPlan(diff, rc)

	require.NotNil(t, plan)
	assert.Empty(t, plan.InstallFormulae)
	assert.Empty(t, plan.InstallCasks)
	assert.Empty(t, plan.InstallNpm)
	assert.Empty(t, plan.InstallTaps)
	assert.False(t, plan.UpdateShell)
	assert.Empty(t, plan.UpdateDotfiles)
	assert.Empty(t, plan.UpdateMacOSPrefs)
}

func TestBuildInstallPlan_ShellChanges(t *testing.T) {
	diff := &syncpkg.SyncDiff{
		Shell: &syncpkg.ShellDiff{
			ThemeChanged:   true,
			RemoteTheme:    "agnoster",
			PluginsChanged: false,
		},
	}
	rc := &config.RemoteConfig{
		Shell: &config.RemoteShellConfig{
			OhMyZsh: true,
			Theme:   "agnoster",
			Plugins: []string{"git", "zsh-autosuggestions"},
		},
	}

	plan := buildInstallPlan(diff, rc)

	assert.True(t, plan.UpdateShell)
	assert.True(t, plan.ShellOhMyZsh)
	assert.Equal(t, "agnoster", plan.ShellTheme)
	assert.Equal(t, []string{"git", "zsh-autosuggestions"}, plan.ShellPlugins)
}

func TestBuildInstallPlan_ShellChanges_NilRCShell(t *testing.T) {
	// When diff.Shell is non-nil but rc.Shell is nil, UpdateShell must be false
	// (guard condition in buildInstallPlan requires both to be non-nil).
	diff := &syncpkg.SyncDiff{
		Shell: &syncpkg.ShellDiff{ThemeChanged: true, RemoteTheme: "robbyrussell"},
	}
	rc := &config.RemoteConfig{Shell: nil}

	plan := buildInstallPlan(diff, rc)

	assert.False(t, plan.UpdateShell)
}

func TestBuildInstallPlan_DotfilesChanged(t *testing.T) {
	diff := &syncpkg.SyncDiff{
		DotfilesChanged: true,
		RemoteDotfiles:  "https://github.com/alice/dotfiles",
	}
	rc := &config.RemoteConfig{}

	plan := buildInstallPlan(diff, rc)

	assert.Equal(t, "https://github.com/alice/dotfiles", plan.UpdateDotfiles)
}

func TestBuildInstallPlan_MacOSPrefs(t *testing.T) {
	diff := &syncpkg.SyncDiff{
		MacOSChanged: []syncpkg.MacOSPrefDiff{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Desc: "Autohide dock", RemoteValue: "1"},
			{Domain: "NSGlobalDomain", Key: "ApplePressAndHoldEnabled", Type: "bool", Desc: "", RemoteValue: "0"},
		},
	}
	rc := &config.RemoteConfig{}

	plan := buildInstallPlan(diff, rc)

	require.Len(t, plan.UpdateMacOSPrefs, 2)
	assert.Equal(t, "com.apple.dock", plan.UpdateMacOSPrefs[0].Domain)
	assert.Equal(t, "autohide", plan.UpdateMacOSPrefs[0].Key)
	assert.Equal(t, "1", plan.UpdateMacOSPrefs[0].Value)
	assert.Equal(t, "Autohide dock", plan.UpdateMacOSPrefs[0].Desc)
	assert.Equal(t, "NSGlobalDomain", plan.UpdateMacOSPrefs[1].Domain)
}

// ── updateSyncedAt ────────────────────────────────────────────────────────────

func TestUpdateSyncedAt_PersistsSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	now := time.Now().Add(-1 * time.Hour)
	source := &syncpkg.SyncSource{
		UserSlug:    "alice/dev-setup",
		Username:    "alice",
		Slug:        "dev-setup",
		InstalledAt: now,
	}
	rc := &config.RemoteConfig{Username: "alice", Slug: "dev-setup"}

	// Should not panic and should save without error.
	updateSyncedAt(source, "", rc)

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "alice/dev-setup", loaded.UserSlug)
	assert.Equal(t, "alice", loaded.Username)
	assert.Equal(t, "dev-setup", loaded.Slug)
	assert.Equal(t, now.Unix(), loaded.InstalledAt.Unix())
	assert.False(t, loaded.SyncedAt.IsZero(), "SyncedAt should be stamped")
}

func TestUpdateSyncedAt_OverrideUserSlug(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	source := &syncpkg.SyncSource{UserSlug: "alice/dev-setup"}
	rc := &config.RemoteConfig{Username: "alice", Slug: "new-slug"}

	updateSyncedAt(source, "alice/new-slug", rc)

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "alice/new-slug", loaded.UserSlug)
}

func TestUpdateSyncedAt_ZeroInstalledAt_UseNow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	source := &syncpkg.SyncSource{UserSlug: "alice/dev-setup"}
	rc := &config.RemoteConfig{Username: "alice", Slug: "dev-setup"}

	before := time.Now()
	updateSyncedAt(source, "", rc)
	after := time.Now()

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	// InstalledAt was zero → should be stamped to ~now.
	assert.True(t, !loaded.InstalledAt.Before(before.Add(-time.Second)), "InstalledAt should be recent")
	assert.True(t, !loaded.InstalledAt.After(after.Add(time.Second)), "InstalledAt should be recent")
}

// ── syncPipelinePhases ────────────────────────────────────────────────────────
