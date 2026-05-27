package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

func sampleCustomizerConfig() *config.RemoteConfig {
	return &config.RemoteConfig{
		Username: "alice", Slug: "dev",
		Packages: config.PackageEntryList{{Name: "git", Desc: "version control"}, {Name: "jq"}, {Name: "ripgrep"}},
		Casks:    config.PackageEntryList{{Name: "visual-studio-code"}, {Name: "docker"}},
		Npm:      config.PackageEntryList{{Name: "typescript"}, {Name: "eslint"}},
	}
}

func TestNewConfigCustomizer_BuildsThreeTabs(t *testing.T) {
	rc := sampleCustomizerConfig()
	m := NewConfigCustomizer(rc)
	require.Len(t, m.tabs, 3)
	assert.Equal(t, "Formulae", m.tabs[0].name)
	assert.Equal(t, "Casks", m.tabs[1].name)
	assert.Equal(t, "NPM", m.tabs[2].name)
	assert.Equal(t, 3, len(m.tabs[0].items))
	assert.Equal(t, 2, len(m.tabs[1].items))
	assert.Equal(t, 2, len(m.tabs[2].items))
}

func TestNewConfigCustomizer_AllItemsPreselected(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	for _, tab := range m.tabs {
		for _, item := range tab.items {
			assert.True(t, item.selected, "%s should be preselected", item.name)
		}
	}
}

func TestConfigCustomizer_SpaceToggles(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// cursor at first item ("git"), press Space
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)
	assert.False(t, m.tabs[0].items[0].selected)
}

func TestConfigCustomizer_TabSwitchesCategory(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(ConfigCustomizerModel)
	assert.Equal(t, 1, m.activeTab)
}

func TestConfigCustomizer_SelectAllInTab(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// Deselect first item in tab 0 so not all are selected
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)
	// Press 'a' — should select all (because not all were selected)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(ConfigCustomizerModel)
	for _, item := range m.tabs[0].items {
		assert.True(t, item.selected)
	}
	// Press 'a' again — should deselect all
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(ConfigCustomizerModel)
	for _, item := range m.tabs[0].items {
		assert.False(t, item.selected)
	}
}

func TestConfigCustomizer_EnterConfirms(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ConfigCustomizerModel)
	assert.True(t, m.confirmed)
}

func TestConfigCustomizer_PicksReflectsSelections(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// Deselect "git" (tab 0, cursor 0)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)

	picks := m.Picks()
	assert.False(t, picks["git"])
	assert.True(t, picks["jq"])
	assert.True(t, picks["ripgrep"])
	assert.True(t, picks["visual-studio-code"])
	assert.True(t, picks["docker"])
	assert.True(t, picks["typescript"])
	assert.True(t, picks["eslint"])
}

func TestConfigCustomizer_PicksOnlyIncludesSelected(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// Deselect "git" via Space
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)

	picks := m.Picks()
	// "git" should be absent from the map entirely (not present as false)
	_, present := picks["git"]
	assert.False(t, present, "deselected items should not appear in Picks()")
}
