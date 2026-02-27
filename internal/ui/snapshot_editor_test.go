package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openbootdotdev/openboot/internal/config"
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
			Npm:      []string{"typescript", "eslint"},
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

	assert.Equal(t, 5, len(m.tabs))
	assert.Equal(t, "Formulae", m.tabs[0].name)
	assert.Equal(t, "Casks", m.tabs[1].name)
	assert.Equal(t, "NPM", m.tabs[2].name)
	assert.Equal(t, "Taps", m.tabs[3].name)
	assert.Equal(t, "macOS Prefs", m.tabs[4].name)
}

func TestNewSnapshotEditorItems(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	assert.Equal(t, 3, len(m.tabs[0].items))
	assert.Equal(t, 2, len(m.tabs[1].items))
	assert.Equal(t, 2, len(m.tabs[2].items))
	assert.Equal(t, 1, len(m.tabs[3].items))
	assert.Equal(t, 2, len(m.tabs[4].items))

	assert.Equal(t, "git", m.tabs[0].items[0].name)
	assert.Equal(t, "visual-studio-code", m.tabs[1].items[0].name)
	assert.Equal(t, "typescript", m.tabs[2].items[0].name)
	assert.Equal(t, "homebrew/core", m.tabs[3].items[0].name)
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

	// 3 formulae + 2 casks + 2 npm + 1 tap + 2 prefs = 10
	total := m.totalSelected()
	assert.Equal(t, 10, total)

	m.tabs[0].items[0].selected = false
	assert.Equal(t, 9, m.totalSelected())
}

func TestSnapshotEditorSelectedCountsSummary(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	summary := m.selectedCountsSummary()
	assert.Contains(t, summary, "3 formulae")
	assert.Contains(t, summary, "2 casks")
	assert.Contains(t, summary, "2 npm")
	assert.Contains(t, summary, "1 taps")
	assert.Contains(t, summary, "2 preferences")
	assert.Contains(t, summary, "selected")
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
	assert.Equal(t, 3, updated.activeTab)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = result.(SnapshotEditorModel)
	assert.Equal(t, 4, updated.activeTab)

	// Wraps back to 0
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
	m = m.withFilteredItems()

	assert.Nil(t, m.filteredItems)
	assert.Nil(t, m.filteredRefs)
}

func TestSnapshotEditorUpdateFilteredItemsWithQuery(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.searchQuery = "git"
	m = m.withFilteredItems()

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

	m.tabs[0].items[0].selected = false // deselect "git" formula
	m.tabs[1].items[0].selected = false // deselect "visual-studio-code" cask
	m.tabs[2].items[0].selected = false // deselect "typescript" npm
	m.tabs[3].items[0].selected = false // deselect "homebrew/core" tap
	m.tabs[4].items[0].selected = false // deselect first macos pref

	edited := buildEditedSnapshot(snap, &m)

	assert.Equal(t, []string{"curl", "wget"}, edited.Packages.Formulae)
	assert.Equal(t, []string{"docker"}, edited.Packages.Casks)
	assert.Equal(t, []string{"eslint"}, edited.Packages.Npm)
	assert.Empty(t, edited.Packages.Taps)
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
}

func TestBuildEditedSnapshotAllSelected(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	edited := buildEditedSnapshot(snap, &m)

	assert.Equal(t, snap.Packages.Formulae, edited.Packages.Formulae)
	assert.Equal(t, snap.Packages.Casks, edited.Packages.Casks)
	assert.Equal(t, snap.Packages.Npm, edited.Packages.Npm)
	assert.Equal(t, snap.Packages.Taps, edited.Packages.Taps)
	assert.Len(t, edited.MacOSPrefs, len(snap.MacOSPrefs))
}

func TestSnapshotEditorMacOSPrefItemName(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	assert.Equal(t, "com.apple.dock.autohide", m.tabs[4].items[0].name)
	assert.Contains(t, m.tabs[4].items[0].description, "Auto-hide Dock")
}

func TestSnapshotEditorItemTypes(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	assert.Equal(t, editorItemFormula, m.tabs[0].items[0].itemType)
	assert.Equal(t, editorItemCask, m.tabs[1].items[0].itemType)
	assert.Equal(t, editorItemNpm, m.tabs[2].items[0].itemType)
	assert.Equal(t, editorItemTap, m.tabs[3].items[0].itemType)
	assert.Equal(t, editorItemMacOSPref, m.tabs[4].items[0].itemType)
}

func TestBuildEditedSnapshotWithAddedItems(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	// Simulate adding new items
	m.tabs[0].items = append(m.tabs[0].items, editorItem{
		name: "fzf", selected: true, itemType: editorItemFormula, isAdded: true,
	})
	m.tabs[2].items = append(m.tabs[2].items, editorItem{
		name: "prettier", selected: true, itemType: editorItemNpm, isAdded: true,
	})
	m.tabs[3].items = append(m.tabs[3].items, editorItem{
		name: "homebrew/cask-fonts", selected: true, itemType: editorItemTap, isAdded: true,
	})

	edited := buildEditedSnapshot(snap, &m)

	assert.Contains(t, edited.Packages.Formulae, "fzf")
	assert.Contains(t, edited.Packages.Npm, "prettier")
	assert.Contains(t, edited.Packages.Taps, "homebrew/cask-fonts")
}

func TestSnapshotEditorNpmTabItems(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	assert.Equal(t, "typescript", m.tabs[2].items[0].name)
	assert.Equal(t, "eslint", m.tabs[2].items[1].name)
	assert.True(t, m.tabs[2].items[0].selected)
}

func TestSnapshotEditorSelectedCountsSummaryNothingSelected(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	// Deselect everything
	for ti := range m.tabs {
		for ii := range m.tabs[ti].items {
			m.tabs[ti].items[ii].selected = false
		}
	}

	assert.Equal(t, "nothing selected", m.selectedCountsSummary())
}

// Phase 2: Online search tests

func TestPackageToEditorItem(t *testing.T) {
	tests := []struct {
		name     string
		pkg      config.Package
		expected editorItemType
	}{
		{"formula", config.Package{Name: "git", Description: "VCS"}, editorItemFormula},
		{"cask", config.Package{Name: "firefox", Description: "Browser", IsCask: true}, editorItemCask},
		{"npm", config.Package{Name: "typescript", Description: "TS compiler", IsNpm: true}, editorItemNpm},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := packageToEditorItem(tt.pkg)
			assert.Equal(t, tt.pkg.Name, item.name)
			assert.Equal(t, tt.pkg.Description, item.description)
			assert.Equal(t, tt.expected, item.itemType)
			assert.True(t, item.isAdded)
			assert.False(t, item.selected)
		})
	}
}

func TestTabIndexForItemType(t *testing.T) {
	assert.Equal(t, 0, tabIndexForItemType(editorItemFormula))
	assert.Equal(t, 1, tabIndexForItemType(editorItemCask))
	assert.Equal(t, 2, tabIndexForItemType(editorItemNpm))
	assert.Equal(t, 3, tabIndexForItemType(editorItemTap))
	assert.Equal(t, 4, tabIndexForItemType(editorItemMacOSPref))
}

func TestWithOnlineResultAdded(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	item := editorItem{name: "fzf", itemType: editorItemFormula, isAdded: true}
	m, toast := m.withOnlineResultAdded(item)

	assert.Contains(t, toast, "fzf")
	assert.Contains(t, toast, "Formulae")

	// Verify item was added to the correct tab
	found := false
	for _, i := range m.tabs[0].items {
		if i.name == "fzf" {
			found = true
			assert.True(t, i.selected)
			assert.True(t, i.isAdded)
		}
	}
	assert.True(t, found, "fzf should be in Formulae tab")
}

func TestWithOnlineResultAddedDuplicate(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)

	// "git" already exists in Formulae
	item := editorItem{name: "git", itemType: editorItemFormula, isAdded: true}
	_, toast := m.withOnlineResultAdded(item)

	assert.Empty(t, toast, "should return empty for duplicate")
}

func TestEditorOnlineSearchResultMsg(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)
	m.searchMode = true
	m.searchQuery = "fzf"

	// Simulate receiving online search results
	msg := editorOnlineSearchResultMsg{
		results: []config.Package{
			{Name: "fzf", Description: "Fuzzy finder"},
			{Name: "fzf-tab", Description: "FZF tab completion", IsNpm: true},
		},
		query: "fzf",
	}

	result, _ := m.Update(msg)
	updated := result.(SnapshotEditorModel)

	assert.False(t, updated.onlineSearching)
	assert.Len(t, updated.onlineResults, 2)
	assert.Equal(t, "fzf", updated.onlineResults[0].name)
	assert.Equal(t, editorItemFormula, updated.onlineResults[0].itemType)
	assert.Equal(t, editorItemNpm, updated.onlineResults[1].itemType)
}

func TestEditorOnlineSearchResultMsgStaleQuery(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)
	m.searchMode = true
	m.searchQuery = "newer-query"

	msg := editorOnlineSearchResultMsg{
		results: []config.Package{{Name: "old-result"}},
		query:   "old-query",
	}

	result, _ := m.Update(msg)
	updated := result.(SnapshotEditorModel)

	// Stale query results should be ignored
	assert.Nil(t, updated.onlineResults)
}

func TestEditorTotalSearchItems(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)
	m.searchQuery = "git"
	m = m.withFilteredItems()
	m.onlineResults = []editorItem{
		{name: "gitui", itemType: editorItemFormula},
	}

	total := m.totalSearchItems()
	assert.Equal(t, len(m.filteredItems)+1, total)
}

func TestEditorOnlineSearchResultMsgError(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)
	m.searchMode = true
	m.searchQuery = "fzf"
	m.onlineSearching = true

	msg := editorOnlineSearchResultMsg{
		results: nil,
		query:   "fzf",
		err:     fmt.Errorf("network error"),
	}

	result, cmd := m.Update(msg)
	updated := result.(SnapshotEditorModel)

	assert.False(t, updated.onlineSearching)
	assert.Contains(t, updated.toastMessage, "unavailable")
	assert.NotNil(t, cmd) // toast clear cmd
}

func TestEditorSearchEscClearsOnlineState(t *testing.T) {
	snap := makeTestSnapshot()
	m := NewSnapshotEditor(snap)
	m.searchMode = true
	m.searchQuery = "test"
	m.onlineSearching = true
	m.onlineDebouncePending = true
	m.onlineResults = []editorItem{{name: "test-result"}}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(SnapshotEditorModel)

	assert.False(t, updated.searchMode)
	assert.Empty(t, updated.searchQuery)
	assert.Nil(t, updated.onlineResults)
	assert.False(t, updated.onlineSearching)
	assert.False(t, updated.onlineDebouncePending)
}

// Phase 3: Manual add mode tests

func TestSnapshotEditorEnterAddMode(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	assert.False(t, m.addMode)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	updated := result.(SnapshotEditorModel)
	assert.True(t, updated.addMode)
	assert.Empty(t, updated.addInput)
}

func TestSnapshotEditorAddModeTypeAndEnter(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.addMode = true

	// Type "fzf"
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	updated := result.(SnapshotEditorModel)
	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	updated = result.(SnapshotEditorModel)
	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	updated = result.(SnapshotEditorModel)
	assert.Equal(t, "fzf", updated.addInput)

	// Press Enter
	result, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = result.(SnapshotEditorModel)

	assert.False(t, updated.addMode)
	assert.Contains(t, updated.toastMessage, "fzf")
	assert.NotNil(t, cmd) // toast clear cmd

	// Verify item was added to current (Formulae) tab
	found := false
	for _, item := range updated.tabs[0].items {
		if item.name == "fzf" {
			found = true
			assert.True(t, item.selected)
			assert.True(t, item.isAdded)
			assert.Equal(t, editorItemFormula, item.itemType)
		}
	}
	assert.True(t, found)
}

func TestSnapshotEditorAddModeEscCancels(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.addMode = true
	m.addInput = "something"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(SnapshotEditorModel)

	assert.False(t, updated.addMode)
	assert.Empty(t, updated.addInput)
}

func TestSnapshotEditorAddModeDuplicateIgnored(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.addMode = true
	m.addInput = "git" // already exists

	originalCount := len(m.tabs[0].items)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(SnapshotEditorModel)

	assert.False(t, updated.addMode)
	assert.Equal(t, originalCount, len(updated.tabs[0].items))
}

func TestSnapshotEditorAddModeBackspace(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.addMode = true
	m.addInput = "fz"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	updated := result.(SnapshotEditorModel)
	assert.Equal(t, "f", updated.addInput)
}

func TestSnapshotEditorAddToNpmTab(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.activeTab = 2 // NPM tab
	m.addMode = true
	m.addInput = "prettier"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(SnapshotEditorModel)

	found := false
	for _, item := range updated.tabs[2].items {
		if item.name == "prettier" {
			found = true
			assert.Equal(t, editorItemNpm, item.itemType)
			assert.True(t, item.isAdded)
		}
	}
	assert.True(t, found, "prettier should be in NPM tab")
}

func TestSnapshotEditorAddModeBlockedOnMacOSPrefs(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.activeTab = 4 // macOS Prefs tab

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	updated := result.(SnapshotEditorModel)

	assert.False(t, updated.addMode, "add mode should not activate on macOS Prefs tab")
	assert.Contains(t, updated.toastMessage, "Cannot manually add macOS prefs")
	assert.NotNil(t, cmd)
}

func TestSnapshotEditorAddedItemVisualBadge(t *testing.T) {
	m := NewSnapshotEditor(makeTestSnapshot())
	m.width = 80

	// Add a new item
	m.tabs[0].items = append(m.tabs[0].items, editorItem{
		name: "fzf", selected: true, itemType: editorItemFormula, isAdded: true,
	})

	view := m.View()
	assert.Contains(t, view, "[+]")
}
