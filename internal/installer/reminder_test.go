package installer

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestFindMatchingPackages(t *testing.T) {
	tests := []struct {
		name         string
		selectedPkgs map[string]bool
		onlinePkgs   []config.Package
		triggerPkgs  []string
		wantCount    int
		wantContains []string
	}{
		{
			name:         "zoom installed matches trigger",
			selectedPkgs: map[string]bool{"zoom": true, "git": true, "curl": true},
			triggerPkgs:  []string{"zoom", "microsoft-teams", "obs", "loom", "feishu", "lark"},
			wantCount:    1,
			wantContains: []string{"zoom"},
		},
		{
			name:         "multiple matches",
			selectedPkgs: map[string]bool{"zoom": true, "obs": true, "git": true},
			triggerPkgs:  []string{"zoom", "microsoft-teams", "obs", "loom", "feishu", "lark"},
			wantCount:    2,
			wantContains: []string{"zoom", "obs"},
		},
		{
			name:         "no matches - only non-trigger packages",
			selectedPkgs: map[string]bool{"git": true, "curl": true, "google-chrome": true},
			triggerPkgs:  []string{"zoom", "microsoft-teams", "obs", "loom", "feishu", "lark"},
			wantCount:    0,
		},
		{
			name:         "empty selected packages",
			selectedPkgs: map[string]bool{},
			triggerPkgs:  []string{"zoom", "microsoft-teams"},
			wantCount:    0,
		},
		{
			name:         "empty trigger list",
			selectedPkgs: map[string]bool{"zoom": true},
			triggerPkgs:  []string{},
			wantCount:    0,
		},
		{
			name:         "nil selected packages",
			selectedPkgs: nil,
			triggerPkgs:  []string{"zoom"},
			wantCount:    0,
		},
		{
			name:         "feishu matches",
			selectedPkgs: map[string]bool{"feishu": true},
			triggerPkgs:  []string{"zoom", "microsoft-teams", "obs", "loom", "feishu", "lark"},
			wantCount:    1,
			wantContains: []string{"feishu"},
		},
		{
			name:         "lark matches",
			selectedPkgs: map[string]bool{"lark": true, "node": true},
			triggerPkgs:  []string{"zoom", "microsoft-teams", "obs", "loom", "feishu", "lark"},
			wantCount:    1,
			wantContains: []string{"lark"},
		},
		{
			name:         "online packages match",
			selectedPkgs: map[string]bool{"git": true},
			onlinePkgs:   []config.Package{{Name: "zoom", IsCask: true}},
			triggerPkgs:  []string{"zoom", "microsoft-teams"},
			wantCount:    1,
			wantContains: []string{"zoom"},
		},
		{
			name:         "both selected and online match",
			selectedPkgs: map[string]bool{"zoom": true},
			onlinePkgs:   []config.Package{{Name: "loom", IsCask: true}},
			triggerPkgs:  []string{"zoom", "loom"},
			wantCount:    2,
			wantContains: []string{"zoom", "loom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				SelectedPkgs: tt.selectedPkgs,
				OnlinePkgs:   tt.onlinePkgs,
			}
			result := findMatchingPackages(cfg.ToInstallOptions(), cfg.ToInstallState(), tt.triggerPkgs)
			assert.Len(t, result, tt.wantCount)
			for _, want := range tt.wantContains {
				assert.Contains(t, result, want)
			}
		})
	}
}

func TestGetScreenRecordingPackages(t *testing.T) {
	pkgs := config.GetScreenRecordingPackages()
	assert.NotEmpty(t, pkgs)
	assert.Contains(t, pkgs, "zoom")
	assert.Contains(t, pkgs, "microsoft-teams")
	assert.Contains(t, pkgs, "obs")
	assert.Contains(t, pkgs, "loom")
	assert.Contains(t, pkgs, "feishu")
	assert.Contains(t, pkgs, "lark")
	// Verify excluded packages are NOT in the list
	assert.NotContains(t, pkgs, "google-chrome")
	assert.NotContains(t, pkgs, "slack")
	assert.NotContains(t, pkgs, "discord")
}
