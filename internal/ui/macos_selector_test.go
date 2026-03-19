package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMacOSSelector_InitialState(t *testing.T) {
	m := NewMacOSSelector()

	assert.Equal(t, macos.DefaultCategories, m.categories)
	assert.Equal(t, 0, m.activeTab)
	assert.Equal(t, 0, m.cursor)
	assert.False(t, m.confirmed)
	assert.False(t, m.showConfirmation)
	assert.NotNil(t, m.selected)
	assert.NotNil(t, m.cursorPositions)
}

func TestNewMacOSSelector_AllPrefsSelectedByDefault(t *testing.T) {
	m := NewMacOSSelector()

	for _, cat := range macos.DefaultCategories {
		for _, p := range cat.Prefs {
			k := macos.PrefKey(p)
			assert.True(t, m.selected[k], "expected pref %q to be selected by default", k)
		}
	}
}

func TestMacOSSelectorModel_VisibleItems(t *testing.T) {
	tests := []struct {
		name     string
		height   int
		expected int
	}{
		{"no height defaults to 15", 0, 15},
		{"small terminal clamps to 5", 10, 5},
		{"normal terminal", 23, 15},
		{"large terminal clamps to 20", 80, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMacOSSelector()
			m.height = tt.height
			assert.Equal(t, tt.expected, m.macosVisibleItems())
		})
	}
}

func TestMacOSSelectorModel_NavigateDown(t *testing.T) {
	m := NewMacOSSelector()

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, 1, updated.cursor)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated = result.(MacOSSelectorModel)
	assert.Equal(t, 2, updated.cursor)
}

func TestMacOSSelectorModel_NavigateUp(t *testing.T) {
	m := NewMacOSSelector()
	m.cursor = 2

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, 1, updated.cursor)
}

func TestMacOSSelectorModel_NavigateUpAtTop(t *testing.T) {
	m := NewMacOSSelector()
	m.cursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, 0, updated.cursor, "cursor should not go below 0")
}

func TestMacOSSelectorModel_NavigateDownAtBottom(t *testing.T) {
	m := NewMacOSSelector()
	lastIdx := len(m.categories[0].Prefs) - 1
	m.cursor = lastIdx

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, lastIdx, updated.cursor, "cursor should not go past last item")
}

func TestMacOSSelectorModel_TogglePref(t *testing.T) {
	m := NewMacOSSelector()
	m.cursor = 0
	pref := m.categories[0].Prefs[0]
	k := macos.PrefKey(pref)

	// starts selected — toggle off
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	updated := result.(MacOSSelectorModel)
	assert.False(t, updated.selected[k])
	assert.Contains(t, updated.toastMessage, "Disabled")

	// toggle back on
	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	updated = result.(MacOSSelectorModel)
	assert.True(t, updated.selected[k])
	assert.Contains(t, updated.toastMessage, "Enabled")
}

func TestMacOSSelectorModel_SelectAllToggle(t *testing.T) {
	m := NewMacOSSelector()
	// all start selected; pressing 'a' should deselect all in active category
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated := result.(MacOSSelectorModel)

	for _, p := range updated.categories[0].Prefs {
		assert.False(t, updated.selected[macos.PrefKey(p)], "expected all prefs in category to be deselected")
	}
	assert.Contains(t, updated.toastMessage, "Disabled all")

	// press 'a' again to re-select all
	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated = result.(MacOSSelectorModel)
	for _, p := range updated.categories[0].Prefs {
		assert.True(t, updated.selected[macos.PrefKey(p)], "expected all prefs in category to be re-enabled")
	}
	assert.Contains(t, updated.toastMessage, "Enabled all")
}

func TestMacOSSelectorModel_TabSwitching(t *testing.T) {
	m := NewMacOSSelector()
	assert.Equal(t, 0, m.activeTab)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, 1, updated.activeTab)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated = result.(MacOSSelectorModel)
	assert.Equal(t, 0, updated.activeTab)
}

func TestMacOSSelectorModel_TabWrap(t *testing.T) {
	m := NewMacOSSelector()
	last := len(m.categories) - 1
	m.activeTab = last

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, 0, updated.activeTab, "tab should wrap around to first")

	m.activeTab = 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated = result.(MacOSSelectorModel)
	assert.Equal(t, last, updated.activeTab, "tab should wrap around to last")
}

func TestMacOSSelectorModel_CursorMemoryOnTabSwitch(t *testing.T) {
	m := NewMacOSSelector()
	m.cursor = 2

	// switch tab, cursor for tab 0 should be remembered
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, 0, updated.cursor, "new tab starts at cursor 0")
	assert.Equal(t, 2, updated.cursorPositions[0], "cursor position saved for previous tab")

	// switch back, cursor should be restored
	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated = result.(MacOSSelectorModel)
	assert.Equal(t, 2, updated.cursor, "cursor restored when returning to previous tab")
}

func TestMacOSSelectorModel_EnterShowsConfirmation(t *testing.T) {
	m := NewMacOSSelector()
	assert.False(t, m.showConfirmation)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(MacOSSelectorModel)
	assert.True(t, updated.showConfirmation)
}

func TestMacOSSelectorModel_EscFromConfirmationGoesBack(t *testing.T) {
	m := NewMacOSSelector()
	m.showConfirmation = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(MacOSSelectorModel)
	assert.False(t, updated.showConfirmation)
	assert.False(t, updated.confirmed)
}

func TestMacOSSelectorModel_EnterOnConfirmationConfirms(t *testing.T) {
	m := NewMacOSSelector()
	m.showConfirmation = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(MacOSSelectorModel)
	assert.True(t, updated.confirmed)
}

func TestMacOSSelectorModel_WindowSizeMsg(t *testing.T) {
	m := NewMacOSSelector()
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := result.(MacOSSelectorModel)
	assert.Equal(t, 120, updated.width)
	assert.Equal(t, 40, updated.height)
}

func TestMacOSSelectorModel_SelectedPreferences_AllSelected(t *testing.T) {
	m := NewMacOSSelector()
	prefs := m.SelectedPreferences()
	assert.Equal(t, len(macos.DefaultPreferences), len(prefs))
}

func TestMacOSSelectorModel_SelectedPreferences_NoneSelected(t *testing.T) {
	m := NewMacOSSelector()
	for k := range m.selected {
		m.selected[k] = false
	}
	prefs := m.SelectedPreferences()
	assert.Empty(t, prefs)
}

func TestMacOSSelectorModel_SelectedPreferences_PartialSelection(t *testing.T) {
	m := NewMacOSSelector()
	// deselect everything, then select only first pref of first category
	for k := range m.selected {
		m.selected[k] = false
	}
	firstPref := m.categories[0].Prefs[0]
	m.selected[macos.PrefKey(firstPref)] = true

	prefs := m.SelectedPreferences()
	require.Len(t, prefs, 1)
	assert.Equal(t, firstPref.Domain, prefs[0].Domain)
	assert.Equal(t, firstPref.Key, prefs[0].Key)
}

func TestMacOSSelectorModel_SelectedPreferences_OrderMatchesCategories(t *testing.T) {
	m := NewMacOSSelector()
	prefs := m.SelectedPreferences()

	// build expected order from categories
	var expected []macos.Preference
	for _, cat := range m.categories {
		for _, p := range cat.Prefs {
			if m.selected[macos.PrefKey(p)] {
				expected = append(expected, p)
			}
		}
	}
	assert.Equal(t, expected, prefs)
}

func TestMacOSSelectorModel_ToastClearMsg(t *testing.T) {
	m := NewMacOSSelector()
	m.toastMessage = "some toast"

	result, _ := m.Update(toastClearMsg{})
	updated := result.(MacOSSelectorModel)
	assert.Empty(t, updated.toastMessage)
}

func TestMacOSSelectorModel_Confirmed(t *testing.T) {
	m := NewMacOSSelector()
	assert.False(t, m.Confirmed())
	m.confirmed = true
	assert.True(t, m.Confirmed())
}

func TestMacOSSelectorModel_ViewRendersTabBar(t *testing.T) {
	m := NewMacOSSelector()
	m.width = 80
	m.height = 30

	view := m.View()
	assert.NotEmpty(t, view)
	// active tab name should appear
	assert.Contains(t, view, m.categories[0].Name)
}

func TestMacOSSelectorModel_ViewRendersCheckboxes(t *testing.T) {
	m := NewMacOSSelector()
	m.width = 100
	m.height = 30

	view := m.View()
	assert.Contains(t, view, "[✓]", "selected prefs should show checked checkbox")
}

func TestMacOSSelectorModel_ViewRendersHelpText(t *testing.T) {
	m := NewMacOSSelector()
	m.width = 100
	m.height = 30

	view := m.View()
	assert.Contains(t, view, "Tab")
	assert.Contains(t, view, "Space")
	assert.Contains(t, view, "Enter")
}

func TestMacOSSelectorModel_ViewRendersUncheckedWhenDeselected(t *testing.T) {
	m := NewMacOSSelector()
	// deselect all prefs in first category
	for _, p := range m.categories[0].Prefs {
		m.selected[macos.PrefKey(p)] = false
	}
	m.width = 100
	m.height = 30

	view := m.View()
	assert.Contains(t, view, "[ ]", "deselected prefs should show unchecked checkbox")
}

func TestMacOSSelectorModel_ViewShowsConfirmation(t *testing.T) {
	m := NewMacOSSelector()
	m.showConfirmation = true
	m.width = 100
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "macOS Preferences Summary")
	assert.Contains(t, view, "[Enter] Apply Preferences")
	assert.Contains(t, view, "[Esc] Go Back")
}

func TestMacOSSelectorModel_ConfirmationView_ListsSelectedByCategory(t *testing.T) {
	m := NewMacOSSelector()
	// deselect everything except first pref of Finder category
	for k := range m.selected {
		m.selected[k] = false
	}
	var finderCat *macos.PrefCategory
	for i := range m.categories {
		if m.categories[i].Name == "Finder" {
			finderCat = &m.categories[i]
			break
		}
	}
	require.NotNil(t, finderCat)
	selectedPref := finderCat.Prefs[0]
	m.selected[macos.PrefKey(selectedPref)] = true
	m.showConfirmation = true
	m.width = 100
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "Finder")
	assert.Contains(t, view, selectedPref.Desc)
}

func TestMacOSSelectorModel_ConfirmationView_HidesEmptyCategories(t *testing.T) {
	m := NewMacOSSelector()
	// deselect all
	for k := range m.selected {
		m.selected[k] = false
	}
	m.showConfirmation = true
	m.width = 100
	m.height = 40

	view := m.View()
	// no category names should appear in body since all are deselected
	assert.NotContains(t, view, "Finder (")
	assert.NotContains(t, view, "Dock (")
}

func TestMacOSSelectorModel_RenderTabBar_NoWidth(t *testing.T) {
	m := NewMacOSSelector()
	// zero width should not panic
	assert.NotPanics(t, func() {
		result := m.macosRenderTabBar()
		assert.NotEmpty(t, result)
	})
}

func TestMacOSSelectorModel_RenderTabBar_ShowsCount(t *testing.T) {
	m := NewMacOSSelector()
	m.width = 100
	// deselect all in first category so count shows 0
	for _, p := range m.categories[0].Prefs {
		m.selected[macos.PrefKey(p)] = false
	}
	bar := m.macosRenderTabBar()
	// strip ANSI and check for (0)
	stripped := stripANSI(bar)
	assert.Contains(t, stripped, "(0)")
}

func TestMacOSSelectorModel_ViewPadsToWidth(t *testing.T) {
	m := NewMacOSSelector()
	m.width = 100
	m.height = 30

	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		w := lipgloss.Width(line)
		assert.LessOrEqual(t, w, 100, "no line should exceed terminal width")
	}
}

// stripANSI removes ANSI escape sequences for plain-text assertions.
func stripANSI(s string) string {
	var result strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
