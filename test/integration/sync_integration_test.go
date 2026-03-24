//go:build integration

package integration

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Execute_DryRun_EmptyPlan(t *testing.T) {
	plan := &syncpkg.SyncPlan{}

	result, err := syncpkg.Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Installed)
	assert.Equal(t, 0, result.Uninstalled)
	assert.Equal(t, 0, result.Updated)
	assert.Empty(t, result.Errors)
}

func TestIntegration_Execute_DryRun_InstallOnly(t *testing.T) {
	plan := &syncpkg.SyncPlan{
		InstallFormulae: []string{"ripgrep", "fd"},
		InstallCasks:    []string{"raycast"},
		InstallNpm:      []string{"turbo"},
		InstallTaps:     []string{"homebrew/cask-fonts"},
	}

	result, err := syncpkg.Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 5, result.Installed)
	assert.Equal(t, 0, result.Uninstalled)
	assert.Equal(t, 0, result.Updated)
	assert.Empty(t, result.Errors)
}

func TestIntegration_Execute_DryRun_UninstallOnly(t *testing.T) {
	plan := &syncpkg.SyncPlan{
		UninstallFormulae: []string{"htop"},
		UninstallCasks:    []string{"slack"},
		UninstallNpm:      []string{"create-react-app"},
		UninstallTaps:     []string{"old/tap"},
	}

	result, err := syncpkg.Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Installed)
	assert.Equal(t, 4, result.Uninstalled)
	assert.Equal(t, 0, result.Updated)
	assert.Empty(t, result.Errors)
}

func TestIntegration_Execute_DryRun_MacOSPrefs(t *testing.T) {
	plan := &syncpkg.SyncPlan{
		UpdateMacOSPrefs: []config.RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "true", Desc: "Dock auto-hide"},
			{Domain: "com.apple.finder", Key: "ShowPathbar", Value: "true", Desc: "Show path bar"},
		},
	}

	result, err := syncpkg.Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Installed)
	assert.Equal(t, 0, result.Uninstalled)
	assert.Equal(t, 2, result.Updated)
	assert.Empty(t, result.Errors)
}

func TestIntegration_Execute_DryRun_FullPlan(t *testing.T) {
	plan := &syncpkg.SyncPlan{
		InstallFormulae:   []string{"ripgrep"},
		InstallCasks:      []string{"raycast"},
		InstallNpm:        []string{"turbo"},
		InstallTaps:       []string{"homebrew/cask-fonts"},
		UninstallFormulae: []string{"htop"},
		UpdateDotfiles: "https://github.com/user/dots",
		UpdateMacOSPrefs: []config.RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
		},
	}

	result, err := syncpkg.Execute(plan, true)
	require.NoError(t, err)
	// 1 formulae + 1 cask + 1 npm + 1 tap = 4 installed
	assert.Equal(t, 4, result.Installed)
	// 1 formulae uninstalled
	assert.Equal(t, 1, result.Uninstalled)
	// dotfiles(1) + macos(1) = 2 updated
	assert.Equal(t, 2, result.Updated)
	assert.Empty(t, result.Errors)
}
