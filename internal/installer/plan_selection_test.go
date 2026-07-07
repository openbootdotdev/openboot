package installer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

func TestPlanFromSelectionDefaults(t *testing.T) {
	opts := &config.InstallOptions{Version: "1.0.0"}
	sel := config.GetPackagesForPreset("developer")
	require.NotEmpty(t, sel)

	plan := PlanFromSelection(opts, sel)

	assert.Equal(t, "1.0.0", plan.Version)
	assert.Positive(t, len(plan.Formulae)+len(plan.Casks)+len(plan.Npm), "packages categorized")
	assert.True(t, plan.InstallOhMyZsh, "shell installed by default")
	assert.NotEmpty(t, plan.DotfilesURL, "dotfiles default applied")
	assert.NotEmpty(t, plan.MacOSPrefs, "macOS prefs default applied")

	// Git identity: either reused from existing config or explicitly skipped —
	// never a half-filled state (the TUI never prompts).
	if plan.SkipGit {
		assert.Empty(t, plan.GitName)
		assert.Empty(t, plan.GitEmail)
	} else {
		assert.NotEmpty(t, plan.GitName)
		assert.NotEmpty(t, plan.GitEmail)
	}
}

func TestPlanFromSelectionPackagesOnly(t *testing.T) {
	opts := &config.InstallOptions{PackagesOnly: true}
	plan := PlanFromSelection(opts, config.GetPackagesForPreset("minimal"))

	assert.True(t, plan.PackagesOnly)
	assert.False(t, plan.InstallOhMyZsh)
	assert.Empty(t, plan.DotfilesURL)
	assert.Empty(t, plan.MacOSPrefs)
	assert.Positive(t, len(plan.Formulae)+len(plan.Casks), "packages still categorized")
}

func TestPlanFromSelectionSkipFlags(t *testing.T) {
	opts := &config.InstallOptions{Shell: "skip", Dotfiles: "skip", Macos: "skip"}
	plan := PlanFromSelection(opts, config.GetPackagesForPreset("minimal"))

	assert.False(t, plan.InstallOhMyZsh)
	assert.Empty(t, plan.DotfilesURL)
	assert.Empty(t, plan.MacOSPrefs)
}
