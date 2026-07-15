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

	plan := PlanFromSelection(opts, sel, nil)

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
	plan := PlanFromSelection(opts, config.GetPackagesForPreset("minimal"), nil)

	assert.True(t, plan.PackagesOnly)
	assert.False(t, plan.InstallOhMyZsh)
	assert.Empty(t, plan.DotfilesURL)
	assert.Empty(t, plan.MacOSPrefs)
	assert.Positive(t, len(plan.Formulae)+len(plan.Casks), "packages still categorized")
}

func TestPlanFromSelectionSkipFlags(t *testing.T) {
	opts := &config.InstallOptions{Shell: "skip", Dotfiles: "skip", Macos: "skip"}
	plan := PlanFromSelection(opts, config.GetPackagesForPreset("minimal"), nil)

	assert.False(t, plan.InstallOhMyZsh)
	assert.Empty(t, plan.DotfilesURL)
	assert.Empty(t, plan.MacOSPrefs)
}

// Online picks come from openboot.dev search — not in the local catalog, so
// their type info must flow through OnlinePkgs into categorization.
func TestPlanFromSelectionIncludesOnlinePicks(t *testing.T) {
	opts := &config.InstallOptions{PackagesOnly: true}
	online := []config.Package{{Name: "web-only-tool", IsNpm: true}}
	plan := PlanFromSelection(opts, map[string]bool{"web-only-tool": true}, online)
	assert.Contains(t, plan.Npm, "web-only-tool")
	assert.Equal(t, online, plan.OnlinePkgs)
}

// Config-mode plans: the remote config filtered by the wizard's selection,
// with openboot.dev picks appended and all declarative fields carried.
func TestPlanForRemoteSelection(t *testing.T) {
	rc := &config.RemoteConfig{
		Packages:     config.PackageEntryList{{Name: "cowsay"}, {Name: "fortune"}},
		Casks:        config.PackageEntryList{{Name: "warp"}},
		Npm:          config.PackageEntryList{{Name: "left-pad"}},
		Taps:         []string{"acme/tap"},
		DotfilesRepo: "https://github.com/alice/dotfiles",
		Shell:        &config.RemoteShellConfig{OhMyZsh: true, Theme: "agnoster", Plugins: []string{"git"}},
		PostInstall:  []string{"echo hi"},
	}
	sel := map[string]bool{"cowsay": true, "warp": true, "web-x": true} // fortune & left-pad deselected
	online := []config.Package{{Name: "web-x", IsNpm: true}}

	plan := PlanForRemoteSelection(&config.InstallOptions{}, rc, sel, online)

	assert.Equal(t, []string{"cowsay"}, plan.Formulae)
	assert.Equal(t, []string{"warp"}, plan.Casks)
	assert.Equal(t, []string{"web-x"}, plan.Npm, "online pick lands with its npm type")
	assert.Equal(t, []string{"acme/tap"}, plan.Taps, "taps ride along untouched")
	assert.Equal(t, "https://github.com/alice/dotfiles", plan.DotfilesURL)
	assert.True(t, plan.InstallOhMyZsh)
	assert.Equal(t, "agnoster", plan.ShellTheme)
	assert.Equal(t, []string{"echo hi"}, plan.PostInstall)
	assert.True(t, plan.SelectedPkgs["web-x"])
	assert.False(t, plan.SelectedPkgs["fortune"])
	assert.Equal(t, online, plan.OnlinePkgs)
}
