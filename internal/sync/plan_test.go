package sync

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSyncPlanTotalActions(t *testing.T) {
	plan := &SyncPlan{
		InstallFormulae:   []string{"ripgrep", "fd"},
		InstallCasks:      []string{"raycast"},
		InstallNpm:        []string{"turbo"},
		InstallTaps:       []string{"homebrew/cask-fonts"},
		UninstallFormulae: []string{"htop"},
		UpdateDotfiles:    "https://github.com/user/dots",
		UpdateMacOSPrefs:  []config.RemoteMacOSPref{{Domain: "com.apple.dock", Key: "autohide", Value: "true"}},
	}

	// 2 + 1 + 1 + 1 + 1 + 1(dotfiles) + 1(macos) = 8
	assert.Equal(t, 8, plan.TotalActions())
}

func TestSyncPlanIsEmpty(t *testing.T) {
	assert.True(t, (&SyncPlan{}).IsEmpty())
	assert.False(t, (&SyncPlan{InstallFormulae: []string{"ripgrep"}}).IsEmpty())
}

func TestSyncPlanEmptySlices(t *testing.T) {
	plan := &SyncPlan{
		InstallFormulae:   []string{},
		InstallCasks:      []string{},
		InstallNpm:        []string{},
		InstallTaps:       []string{},
		UninstallFormulae: []string{},
		UninstallCasks:    []string{},
		UninstallNpm:      []string{},
		UninstallTaps:     []string{},
		UpdateMacOSPrefs: []config.RemoteMacOSPref{},
	}

	assert.True(t, plan.IsEmpty())
	assert.Equal(t, 0, plan.TotalActions())
}

func TestSyncPlanTotalActionsUninstallOnly(t *testing.T) {
	plan := &SyncPlan{
		UninstallFormulae: []string{"htop", "jq"},
		UninstallCasks:    []string{"slack"},
		UninstallNpm:      []string{"create-react-app"},
		UninstallTaps:     []string{"old/tap"},
	}
	assert.Equal(t, 5, plan.TotalActions())
	assert.False(t, plan.IsEmpty())
}

func TestSyncPlanTotalActionsDotfilesOnly(t *testing.T) {
	plan := &SyncPlan{
		UpdateDotfiles: "https://github.com/user/dots",
	}
	assert.Equal(t, 1, plan.TotalActions())
	assert.False(t, plan.IsEmpty())
}

func TestSyncPlanTotalActionsMacOSOnly(t *testing.T) {
	plan := &SyncPlan{
		UpdateMacOSPrefs: []config.RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
			{Domain: "com.apple.finder", Key: "ShowPathbar", Value: "true"},
		},
	}
	assert.Equal(t, 2, plan.TotalActions())
}

func TestSyncResultDefaults(t *testing.T) {
	r := &SyncResult{}
	assert.Equal(t, 0, r.Installed)
	assert.Equal(t, 0, r.Uninstalled)
	assert.Equal(t, 0, r.Updated)
	assert.Empty(t, r.Errors)
}
