package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestSnapshot() *snapshot.Snapshot {
	return &snapshot.Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test-machine",
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "curl", "wget"},
			Casks:    []string{"visual-studio-code", "docker"},
			Taps:     []string{"homebrew/core"},
		},
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "1", Desc: "Auto-hide Dock"},
			{Domain: "com.apple.finder", Key: "ShowPathbar", Value: "1", Desc: "Show path bar"},
		},
		Shell: snapshot.ShellSnapshot{
			Default: "zsh",
			OhMyZsh: true,
			Theme:   "robbyrussell",
			Plugins: []string{"git", "zsh-autosuggestions"},
		},
		Git: snapshot.GitSnapshot{
			UserName:  "Test User",
			UserEmail: "test@example.com",
		},
		DevTools: []snapshot.DevTool{
			{Name: "go", Version: "1.24.0"},
			{Name: "node", Version: "20.0.0"},
		},
	}
}

func TestNewSnapshotEditorTabs(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	assert.Equal(t, 3, len(m.tabs))
	assert.Equal(t, "Formulae", m.tabs[0].name)
	assert.Equal(t, "Casks", m.tabs[1].name)
	assert.Equal(t, "macOS Prefs", m.tabs[2].name)
}

func TestNewSnapshotEditorItems(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	assert.Equal(t, 3, len(m.tabs[0].items))
	assert.Equal(t, 2, len(m.tabs[1].items))
	assert.Equal(t, 2, len(m.tabs[2].items))

	assert.Equal(t, "git", m.tabs[0].items[0].name)
	assert.Equal(t, "visual-studio-code", m.tabs[1].items[0].name)
}

func TestNewSnapshotEditorAllItemsSelected(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	for _, tab := range m.tabs {
		for _, item := range tab.items {
			assert.True(t, item.selected, "item %q should start selected", item.name)
		}
	}
}

func TestSnapshotEditorGetVisibleItems(t *testing.T) {
	tests := []struct {
		name     string
		height   int
		expected int
	}{
		{"no height defaults to 15", 0, 15},
		{"small terminal clamps to 5", 15, 5},
		{"normal terminal", 28, 15},
		{"large terminal clamps to 20", 80, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewSnapshotEditor(makeTestSnapshot())
			m.height = tt.height
			assert.Equal(t, tt.expected, m.getVisibleItems())
		})
	}
}

func TestSnapshotEditorTotalSelected(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	total := m.totalSelected()
	assert.Equal(t, 3+2+2, total)

	m.tabs[0].items[0].selected = false
	assert.Equal(t, 3+2+2-1, m.totalSelected())
}

func TestSnapshotEditorSelectedCountsSummary(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	summary := m.selectedCountsSummary()
	assert.Contains(t, summary, "3 formulae")
	assert.Contains(t, summary, "2 casks")
	assert.Contains(t, summary, "2 preferences")
}

func TestSnapshotEditorShellSummary(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	summary := m.shellSummary()
	assert.Contains(t, summary, "zsh")
	assert.Contains(t, summary, "Oh-My-Zsh")
	assert.Contains(t, summary, "robbyrussell")
}

func TestSnapshotEditorShellSummaryNoOhMyZsh(t *testing.T) {
	snap := makeTestSnapshot()
	snap.Shell.OhMyZsh = false
	m := NewSnapshotEditor(snap)

	summary := m.shellSummary()
	assert.Contains(t, summary, "zsh")
	assert.NotContains(t, summary, "Oh-My-Zsh")
}

func TestSnapshotEditorGitSummaryWithConfig(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	summary := m.gitSummary()
	assert.Contains(t, summary, "Test User")
	assert.Contains(t, summary, "test@example.com")
}

func TestSnapshotEditorGitSummaryNotConfigured(t *testing.T) {
	snap := makeTestSnapshot()
	snap.Git = snapshot.GitSnapshot{}
	m := NewSnapshotEditor(snap)

	summary := m.gitSummary()
	assert.Contains(t, summary, "not configured")
}

func TestSnapshotEditorDevToolsSummary(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	summary := m.devToolsSummary()
	assert.Contains(t, summary, "go")
	assert.Contains(t, summary, "node")
}

func TestSnapshotEditorDevToolsSummaryNone(t *testing.T) {
	snap := makeTestSnapshot()
	snap.DevTools = nil
	m := NewSnapshotEditor(snap)

	summary := m.devToolsSummary()
	assert.Contains(t, summary, "none detected")
}

func TestSnapshotEditorKeyNavigateDown(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	assert.Equal(t, 0, m.cursor)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := result.(SnapshotEditorModel)
	assert.Equal(t, 1, updated.cursor)
}

func TestSnapshotEditorKeyNavigateUp(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.cursor = 2

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := result.(SnapshotEditorModel)
	assert.Equal(t, 1, updated.cursor)
}

func TestSnapshotEditorCursorDoesNotGoNegative(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := result.(SnapshotEditorModel)
	assert.Equal(t, 0, updated.cursor)
}

func TestSnapshotEditorTabSwitch(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	assert.Equal(t, 0, m.activeTab)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated := result.(SnapshotEditorModel)
	assert.Equal(t, 1, updated.activeTab)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = result.(SnapshotEditorModel)
	assert.Equal(t, 2, updated.activeTab)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = result.(SnapshotEditorModel)
	assert.Equal(t, 0, updated.activeTab)
}

func TestSnapshotEditorSpaceTogglesItem(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	assert.True(t, m.tabs[0].items[0].selected)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	updated := result.(SnapshotEditorModel)
	assert.False(t, updated.tabs[0].items[0].selected)
}

func TestSnapshotEditorSelectAll(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated := result.(SnapshotEditorModel)
	for _, item := range updated.tabs[updated.activeTab].items {
		assert.False(t, item.selected)
	}

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated = result.(SnapshotEditorModel)
	for _, item := range updated.tabs[updated.activeTab].items {
		assert.True(t, item.selected)
	}
}

func TestSnapshotEditorEnterSearchMode(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	assert.False(t, m.searchMode)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	updated := result.(SnapshotEditorModel)
	assert.True(t, updated.searchMode)
}

func TestSnapshotEditorSearchTypeAndBackspace(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.searchMode = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	updated := result.(SnapshotEditorModel)
	assert.Equal(t, "g", updated.searchQuery)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	updated = result.(SnapshotEditorModel)
	assert.Equal(t, "", updated.searchQuery)
}

func TestSnapshotEditorSearchExitWithEsc(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.searchMode = true
	m.searchQuery = "git"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(SnapshotEditorModel)
	assert.False(t, updated.searchMode)
	assert.Equal(t, "", updated.searchQuery)
}

func TestSnapshotEditorUpdateFilteredItemsEmpty(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.searchQuery = ""
	m.updateFilteredItems()

	assert.Nil(t, m.filteredItems)
	assert.Nil(t, m.filteredRefs)
}

func TestSnapshotEditorUpdateFilteredItemsWithQuery(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.searchQuery = "git"
	m.updateFilteredItems()

	require.NotEmpty(t, m.filteredItems)
	for _, item := range m.filteredItems {
		assert.True(t,
			strings.Contains(strings.ToLower(item.name), "git") ||
				strings.Contains(strings.ToLower(item.description), "git"),
		)
	}
}

func TestSnapshotEditorWindowResize(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated := result.(SnapshotEditorModel)
	assert.Equal(t, 100, updated.width)
	assert.Equal(t, 40, updated.height)
}

func TestBuildEditedSnapshotFiltersDeselected(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	m.tabs[0].items[0].selected = false
	m.tabs[1].items[0].selected = false
	m.tabs[2].items[0].selected = false

	edited := buildEditedSnapshot(snap, &m)

	assert.Equal(t, []string{"curl", "wget"}, edited.Packages.Formulae)
	assert.Equal(t, []string{"docker"}, edited.Packages.Casks)
	assert.Len(t, edited.MacOSPrefs, 1)
	assert.Equal(t, "ShowPathbar", edited.MacOSPrefs[0].Key)
}

func TestBuildEditedSnapshotPreservesMetadata(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	edited := buildEditedSnapshot(snap, &m)

	assert.Equal(t, snap.Version, edited.Version)
	assert.Equal(t, snap.Hostname, edited.Hostname)
	assert.Equal(t, snap.Shell, edited.Shell)
	assert.Equal(t, snap.Git, edited.Git)
	assert.Equal(t, snap.DevTools, edited.DevTools)
	assert.Equal(t, snap.Packages.Taps, edited.Packages.Taps)
}

func TestBuildEditedSnapshotAllSelected(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	edited := buildEditedSnapshot(snap, &m)

	assert.Equal(t, snap.Packages.Formulae, edited.Packages.Formulae)
	assert.Equal(t, snap.Packages.Casks, edited.Packages.Casks)
	assert.Len(t, edited.MacOSPrefs, len(snap.MacOSPrefs))
}

func TestSnapshotEditorMacOSPrefItemName(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	assert.Equal(t, "com.apple.dock.autohide", m.tabs[2].items[0].name)
	assert.Contains(t, m.tabs[2].items[0].description, "Auto-hide Dock")
}
