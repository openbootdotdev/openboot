package wizard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

// Config mode: install <slug> / -u / --from / alias runs the full wizard over
// the remote config's own packages instead of the catalog.

func testRemoteConfig() *config.RemoteConfig {
	return &config.RemoteConfig{
		Username:     "alice",
		Slug:         "dev",
		Packages:     config.PackageEntryList{{Name: "cowsay", Desc: "talking cow"}, {Name: "fortune"}},
		Casks:        config.PackageEntryList{{Name: "warp"}},
		Npm:          config.PackageEntryList{{Name: "left-pad"}},
		DotfilesRepo: "https://github.com/alice/dotfiles",
		Shell:        &config.RemoteShellConfig{OhMyZsh: true, Theme: "agnoster"},
		PostInstall:  []string{"echo hi"},
	}
}

func sizedConfig(rc *config.RemoteConfig) Model {
	m := NewForConfig("1.4.0", &config.InstallOptions{Version: "1.4.0"}, rc)
	return send(m, tea.WindowSizeMsg{Width: 96, Height: 30})
}

func TestConfigModeShowsConfigPackagesPreselected(t *testing.T) {
	m := sizedConfig(testRemoteConfig())
	require.Len(t, m.cats, 3, "cli tools / apps / npm categories from the config")
	assert.Equal(t, 4, m.selCount(), "everything preselected")
	assert.Contains(t, m.View(), "openboot install alice/dev", "status bar names the source")

	m = finishProbes(m)
	assert.Equal(t, scrSelect, m.screen, "config mode skips the loadout question")
	pool := m.pool()
	require.NotEmpty(t, pool)
	assert.Equal(t, "cowsay", pool[0].Name)
}

func TestConfigModeSkipsGitScreenAndPlansFromConfig(t *testing.T) {
	defer stubGitConfig("", "")()
	m := finishProbes(sizedConfig(testRemoteConfig()))
	m.installed = map[string]bool{}

	// Deselect "fortune" on the select screen, then continue.
	m.rowCur = 1
	m = send(m, key("space"))
	require.False(t, m.selected["fortune"])
	m = send(m, key("enter"))

	require.Equal(t, scrConfirm, m.screen, "declarative installs never interpose the git prompt")
	assert.NotContains(t, m.preview.Formulae, "fortune", "deselected package leaves the plan")
	assert.Contains(t, m.preview.Formulae, "cowsay")
	assert.Contains(t, m.preview.Casks, "warp")
	assert.Contains(t, m.preview.Npm, "left-pad")
	assert.Equal(t, "https://github.com/alice/dotfiles", m.preview.DotfilesURL)
	assert.True(t, m.preview.InstallOhMyZsh)
	assert.Equal(t, "agnoster", m.preview.ShellTheme)
	assert.Equal(t, []string{"echo hi"}, m.preview.PostInstall)
}

func TestConfigModeConfirmShowsPostInstallAndShiftsHitTest(t *testing.T) {
	defer stubGitConfig("", "")()
	m := finishProbes(sizedConfig(testRemoteConfig()))
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrConfirm, m.screen)

	assert.Contains(t, m.View(), "runs after the wizard", "post-install surfaced on the review screen")
	require.Equal(t, 9, m.confirmHeaderRows(), "post-install line shifts the toggle rows down")
	kind, idx := m.confirmHitTest(10) // body row 9 = first toggle row
	assert.Equal(t, "row", kind)
	assert.Equal(t, 0, idx)
	_, miss := m.confirmHitTest(9)
	assert.Equal(t, -1, miss, "the info line itself is not clickable")
}

// The wizard hands the plan back intact and installs nothing itself: the CLI
// applies it on the normal terminal, where post-install gets its own preview
// and confirm, and Silent stays as the run asked for rather than being forced
// on to keep prompts out of an alt-screen.
func TestConfigModeConfirmReturnsPlanWithoutInstalling(t *testing.T) {
	defer stubGitConfig("", "")()
	m := finishProbes(sizedConfig(testRemoteConfig()))
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrConfirm, m.screen)
	m = send(m, key("enter"))

	require.True(t, m.confirmed, "↵ on the review screen accepts the plan")
	require.True(t, m.quit, "and closes the wizard so the CLI can apply it")
	assert.Equal(t, []string{"echo hi"}, m.plan.PostInstall,
		"the config's post-install script reaches the CLI intact")
	assert.False(t, m.plan.Silent, "an interactive run stays interactive")
	assert.Contains(t, m.plan.Formulae, "cowsay")
}
