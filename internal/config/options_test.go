package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ToInstallOptions ----

func TestToInstallOptions_AllFields(t *testing.T) {
	rc := &RemoteConfig{Username: "alice", Slug: "setup"}
	cfg := &Config{
		InstallOptions: InstallOptions{
			Version:          "1.2.3",
			Preset:           "developer",
			User:             "alice",
			DryRun:           true,
			Silent:           true,
			PackagesOnly:     true,
			Update:           true,
			Shell:            "install",
			Macos:            "configure",
			Dotfiles:         "clone",
			GitName:          "Alice",
			GitEmail:         "alice@example.com",
			PostInstall:      "mise install",
			AllowPostInstall: true,
			DotfilesURL:      "https://github.com/alice/dotfiles",
		},
		InstallState: InstallState{
			RemoteConfig: rc,
		},
	}

	opts := cfg.ToInstallOptions()
	require.NotNil(t, opts)

	assert.Equal(t, "1.2.3", opts.Version)
	assert.Equal(t, "developer", opts.Preset)
	assert.Equal(t, "alice", opts.User)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.Silent)
	assert.True(t, opts.PackagesOnly)
	assert.True(t, opts.Update)
	assert.Equal(t, "install", opts.Shell)
	assert.Equal(t, "configure", opts.Macos)
	assert.Equal(t, "clone", opts.Dotfiles)
	assert.Equal(t, "Alice", opts.GitName)
	assert.Equal(t, "alice@example.com", opts.GitEmail)
	assert.Equal(t, "mise install", opts.PostInstall)
	assert.True(t, opts.AllowPostInstall)
	assert.Equal(t, "https://github.com/alice/dotfiles", opts.DotfilesURL)
}

func TestToInstallOptions_ZeroConfig(t *testing.T) {
	cfg := &Config{}
	opts := cfg.ToInstallOptions()
	require.NotNil(t, opts)

	assert.Equal(t, "", opts.Version)
	assert.Equal(t, "", opts.Preset)
	assert.Equal(t, "", opts.User)
	assert.False(t, opts.DryRun)
	assert.False(t, opts.Silent)
	assert.False(t, opts.PackagesOnly)
	assert.False(t, opts.Update)
	assert.Equal(t, "", opts.Shell)
	assert.Equal(t, "", opts.Macos)
	assert.Equal(t, "", opts.Dotfiles)
	assert.Equal(t, "", opts.GitName)
	assert.Equal(t, "", opts.GitEmail)
	assert.Equal(t, "", opts.PostInstall)
	assert.False(t, opts.AllowPostInstall)
	assert.Equal(t, "", opts.DotfilesURL)
}

// ---- ToInstallState ----

func TestToInstallState_AllFields(t *testing.T) {
	rc := &RemoteConfig{Username: "bob", Slug: "dev"}
	git := &SnapshotGitConfig{UserName: "Bob", UserEmail: "bob@example.com"}
	macOSPrefs := []RemoteMacOSPref{
		{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
	}

	cfg := &Config{
		InstallState: InstallState{
			SelectedPkgs:         map[string]bool{"git": true, "curl": false},
			OnlinePkgs:           []Package{{Name: "git", Description: "VCS"}},
			SnapshotTaps:         []string{"homebrew/core"},
			RemoteConfig:         rc,
			SnapshotGit:          git,
			SnapshotMacOS:        macOSPrefs,
			SnapshotDotfiles:     "https://github.com/bob/dotfiles",
			SnapshotShellOhMyZsh: true,
			SnapshotShellTheme:   "agnoster",
			SnapshotShellPlugins: []string{"git", "z"},
		},
	}

	state := cfg.ToInstallState()
	require.NotNil(t, state)

	assert.Equal(t, map[string]bool{"git": true, "curl": false}, state.SelectedPkgs)
	assert.Len(t, state.OnlinePkgs, 1)
	assert.Equal(t, "git", state.OnlinePkgs[0].Name)
	assert.Equal(t, []string{"homebrew/core"}, state.SnapshotTaps)
	assert.Equal(t, rc, state.RemoteConfig)
	assert.Equal(t, git, state.SnapshotGit)
	assert.Equal(t, macOSPrefs, state.SnapshotMacOS)
	assert.Equal(t, "https://github.com/bob/dotfiles", state.SnapshotDotfiles)
	assert.True(t, state.SnapshotShellOhMyZsh)
	assert.Equal(t, "agnoster", state.SnapshotShellTheme)
	assert.Equal(t, []string{"git", "z"}, state.SnapshotShellPlugins)
}

func TestToInstallState_ZeroConfig(t *testing.T) {
	cfg := &Config{}
	state := cfg.ToInstallState()
	require.NotNil(t, state)

	assert.Nil(t, state.SelectedPkgs)
	assert.Nil(t, state.OnlinePkgs)
	assert.Nil(t, state.SnapshotTaps)
	assert.Nil(t, state.RemoteConfig)
	assert.Nil(t, state.SnapshotGit)
	assert.Nil(t, state.SnapshotMacOS)
	assert.Equal(t, "", state.SnapshotDotfiles)
	assert.False(t, state.SnapshotShellOhMyZsh)
	assert.Equal(t, "", state.SnapshotShellTheme)
	assert.Nil(t, state.SnapshotShellPlugins)
}

// ---- ApplyState ----

func TestApplyState_AllFields(t *testing.T) {
	rc := &RemoteConfig{Username: "carol", Slug: "home"}
	git := &SnapshotGitConfig{UserName: "Carol", UserEmail: "carol@example.com"}
	macOSPrefs := []RemoteMacOSPref{
		{Domain: "NSGlobalDomain", Key: "AppleShowScrollBars", Type: "string", Value: "Always"},
	}

	state := &InstallState{
		SelectedPkgs:         map[string]bool{"ripgrep": true},
		OnlinePkgs:           []Package{{Name: "ripgrep", Description: "Search tool"}},
		SnapshotTaps:         []string{"homebrew/cask"},
		RemoteConfig:         rc,
		SnapshotGit:          git,
		SnapshotMacOS:        macOSPrefs,
		SnapshotDotfiles:     "https://github.com/carol/dotfiles",
		SnapshotShellOhMyZsh: true,
		SnapshotShellTheme:   "powerlevel10k",
		SnapshotShellPlugins: []string{"git", "zsh-autosuggestions"},
	}

	cfg := &Config{}
	cfg.ApplyState(state)

	assert.Equal(t, map[string]bool{"ripgrep": true}, cfg.SelectedPkgs)
	assert.Len(t, cfg.OnlinePkgs, 1)
	assert.Equal(t, "ripgrep", cfg.OnlinePkgs[0].Name)
	assert.Equal(t, []string{"homebrew/cask"}, cfg.SnapshotTaps)
	assert.Equal(t, rc, cfg.RemoteConfig)
	assert.Equal(t, git, cfg.SnapshotGit)
	assert.Equal(t, macOSPrefs, cfg.SnapshotMacOS)
	assert.Equal(t, "https://github.com/carol/dotfiles", cfg.SnapshotDotfiles)
	assert.True(t, cfg.SnapshotShellOhMyZsh)
	assert.Equal(t, "powerlevel10k", cfg.SnapshotShellTheme)
	assert.Equal(t, []string{"git", "zsh-autosuggestions"}, cfg.SnapshotShellPlugins)
}

func TestApplyState_ZeroState(t *testing.T) {
	cfg := &Config{
		InstallState: InstallState{
			SelectedPkgs:         map[string]bool{"old": true},
			SnapshotDotfiles:     "https://github.com/old/dotfiles",
			SnapshotShellOhMyZsh: true,
		},
	}

	state := &InstallState{}
	cfg.ApplyState(state)

	// All fields should be overwritten with zero values from state.
	assert.Nil(t, cfg.SelectedPkgs)
	assert.Equal(t, "", cfg.SnapshotDotfiles)
	assert.False(t, cfg.SnapshotShellOhMyZsh)
}

// ---- Round-trip: ToInstallState → ApplyState ----

func TestInstallState_RoundTrip(t *testing.T) {
	rc := &RemoteConfig{Username: "eve", Slug: "workstation"}
	original := &Config{
		InstallState: InstallState{
			SelectedPkgs:         map[string]bool{"fd": true, "bat": true},
			OnlinePkgs:           []Package{{Name: "fd"}, {Name: "bat"}},
			SnapshotTaps:         []string{"homebrew/core", "homebrew/cask"},
			RemoteConfig:         rc,
			SnapshotDotfiles:     "https://github.com/eve/dots",
			SnapshotShellOhMyZsh: true,
			SnapshotShellTheme:   "robbyrussell",
			SnapshotShellPlugins: []string{"git"},
		},
	}

	state := original.ToInstallState()

	restored := &Config{}
	restored.ApplyState(state)

	assert.Equal(t, original.SelectedPkgs, restored.SelectedPkgs)
	assert.Equal(t, original.OnlinePkgs, restored.OnlinePkgs)
	assert.Equal(t, original.SnapshotTaps, restored.SnapshotTaps)
	assert.Equal(t, original.RemoteConfig, restored.RemoteConfig)
	assert.Equal(t, original.SnapshotDotfiles, restored.SnapshotDotfiles)
	assert.Equal(t, original.SnapshotShellOhMyZsh, restored.SnapshotShellOhMyZsh)
	assert.Equal(t, original.SnapshotShellTheme, restored.SnapshotShellTheme)
	assert.Equal(t, original.SnapshotShellPlugins, restored.SnapshotShellPlugins)
}

// ToInstallOptions does NOT include state fields (runtime-only fields).
func TestToInstallOptions_DoesNotLeakStateFields(t *testing.T) {
	cfg := &Config{
		InstallState: InstallState{
			SelectedPkgs: map[string]bool{"git": true},
			OnlinePkgs:   []Package{{Name: "git"}},
		},
	}
	opts := cfg.ToInstallOptions()
	// InstallOptions has no SelectedPkgs or OnlinePkgs — simply confirm it compiles
	// and returns a valid struct with the correct type.
	require.NotNil(t, opts)
	assert.IsType(t, &InstallOptions{}, opts)
}
