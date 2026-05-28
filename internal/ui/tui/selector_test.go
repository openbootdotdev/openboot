package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

func TestTruncateLine(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		maxWidth     int
		wantLen      int // visual width of result
		wantSuffix   string
		wantPassthru bool
	}{
		{"within limit", "hello", 20, 5, "", true},
		{"exact limit", "hello", 5, 5, "", true},
		{"zero width passthrough", "hello", 0, 5, "", true},
		{"negative width passthrough", "hello", -1, 5, "", true},
		{"maxWidth < 10 truncates without ellipsis", "hello world", 7, 7, "", false},
		{"maxWidth >= 10 truncates with ellipsis", "hello world foo", 12, 12, "...", false},
		{"maxWidth >= 10 exact boundary", "hello world!", 15, 12, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateLine(tt.input, tt.maxWidth)
			if tt.wantPassthru {
				assert.Equal(t, tt.input, result)
			} else {
				assert.Equal(t, tt.wantLen, lipgloss.Width(result))
				if tt.wantSuffix != "" {
					assert.True(t, strings.HasSuffix(result, tt.wantSuffix))
				}
			}
		})
	}
}

func TestPadLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		width   int
		wantLen int
	}{
		{"pads short line", "hi", 10, 10},
		{"no padding needed", "hello world", 5, 11}, // already wider, returned as-is
		{"zero width passthrough", "hi", 0, 2},
		{"exact width no-op", "hello", 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padLine(tt.input, tt.width)
			assert.Equal(t, tt.wantLen, lipgloss.Width(result))
		})
	}
}

func TestHighlightMatchesNoMatchIndexes(t *testing.T) {
	result := highlightMatches("hello", []int{})
	assert.Equal(t, "hello", result)
}

func TestHighlightMatchesNilIndexes(t *testing.T) {
	result := highlightMatches("hello", nil)
	assert.Equal(t, "hello", result)
}

func TestHighlightMatchesWithIndexesDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		result := highlightMatches("hello", []int{0, 2, 4})
		assert.NotEmpty(t, result)
	})
}

func TestGetTypeBadge(t *testing.T) {
	tests := []struct {
		name     string
		pkg      config.Package
		contains string
	}{
		{"npm package", config.Package{IsNpm: true}, "📦"},
		{"cask package", config.Package{IsCask: true}, "🖥"},
		{"formula", config.Package{}, "⚙"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTypeBadge(tt.pkg)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestSelectorModelGetVisibleItems(t *testing.T) {
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
			m := NewSelector("scratch")
			m.height = tt.height
			assert.Equal(t, tt.expected, m.getVisibleItems())
		})
	}
}

func TestSelectorModelTotalSearchItems(t *testing.T) {
	m := NewSelector("scratch")

	assert.Equal(t, 0, m.totalSearchItems())

	m.filteredPkgs = []config.Package{{Name: "git"}, {Name: "curl"}}
	assert.Equal(t, 2, m.totalSearchItems())

	m.onlineResults = []config.Package{{Name: "fzf"}}
	assert.Equal(t, 3, m.totalSearchItems())
}

func TestSelectorModelSearchItemAt(t *testing.T) {
	m := NewSelector("scratch")
	m.filteredPkgs = []config.Package{{Name: "git"}, {Name: "curl"}}
	m.onlineResults = []config.Package{{Name: "fzf"}}

	pkg, isOnline := m.searchItemAt(0)
	assert.Equal(t, "git", pkg.Name)
	assert.False(t, isOnline)

	pkg, isOnline = m.searchItemAt(1)
	assert.Equal(t, "curl", pkg.Name)
	assert.False(t, isOnline)

	pkg, isOnline = m.searchItemAt(2)
	assert.Equal(t, "fzf", pkg.Name)
	assert.True(t, isOnline)

	pkg, isOnline = m.searchItemAt(99)
	assert.Equal(t, "", pkg.Name)
	assert.False(t, isOnline)
}

func TestSelectorModelUpdateFilteredPackagesEmptyQuery(t *testing.T) {
	m := NewSelector("scratch")
	m.filteredPkgs = []config.Package{{Name: "git"}}
	m.searchQuery = ""
	m.updateFilteredPackages()

	assert.Nil(t, m.filteredPkgs)
	assert.Nil(t, m.fuzzyMatches)
}

func TestSelectorModelUpdateFilteredPackagesWithQuery(t *testing.T) {
	m := NewSelector("scratch")
	m.searchQuery = "git"
	m.updateFilteredPackages()

	require.NotEmpty(t, m.filteredPkgs, "expected at least one package matching 'git'")
	for _, pkg := range m.filteredPkgs {
		assert.NotEmpty(t, pkg.Name)
	}
}

func TestSelectorModelNavigateDown(t *testing.T) {
	m := NewSelector("scratch")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := result.(SelectorModel)
	assert.Equal(t, 1, updated.cursor)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated = result.(SelectorModel)
	assert.Equal(t, 2, updated.cursor)
}

func TestSelectorModelNavigateUp(t *testing.T) {
	m := NewSelector("scratch")
	m.cursor = 2

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := result.(SelectorModel)
	assert.Equal(t, 1, updated.cursor)
}

func TestSelectorModelCursorDoesNotGoNegative(t *testing.T) {
	m := NewSelector("scratch")
	assert.Equal(t, 0, m.cursor)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := result.(SelectorModel)
	assert.Equal(t, 0, updated.cursor)
}

func TestSelectorModelTabSwitching(t *testing.T) {
	m := NewSelector("scratch")
	require.NotEmpty(t, m.categories)
	initialTab := m.activeTab

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated := result.(SelectorModel)
	assert.Equal(t, (initialTab+1)%len(m.categories), updated.activeTab)
}

func TestSelectorModelEnterSearchMode(t *testing.T) {
	m := NewSelector("scratch")
	assert.False(t, m.searchMode)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	updated := result.(SelectorModel)
	assert.True(t, updated.searchMode)
	assert.Equal(t, "", updated.searchQuery)
}

func TestSelectorModelSearchTypeAndBackspace(t *testing.T) {
	m := NewSelector("scratch")
	m.searchMode = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	updated := result.(SelectorModel)
	assert.Equal(t, "g", updated.searchQuery)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	updated = result.(SelectorModel)
	assert.Equal(t, "gi", updated.searchQuery)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	updated = result.(SelectorModel)
	assert.Equal(t, "g", updated.searchQuery)
}

func TestSelectorModelExitSearchMode(t *testing.T) {
	m := NewSelector("scratch")
	m.searchMode = true
	m.searchQuery = "git"
	m.filteredPkgs = []config.Package{{Name: "git"}}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(SelectorModel)
	assert.False(t, updated.searchMode)
	assert.Equal(t, "", updated.searchQuery)
	assert.Nil(t, updated.filteredPkgs)
}

func TestSelectorModelSpaceTogglesPackage(t *testing.T) {
	m := NewSelector("scratch")
	cat := m.categories[m.activeTab]
	require.NotEmpty(t, cat.Packages, "active tab must have packages")

	firstPkg := cat.Packages[0]
	initiallySelected := m.selected[firstPkg.Name]

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	updated := result.(SelectorModel)
	assert.Equal(t, !initiallySelected, updated.selected[firstPkg.Name])

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	updated = result.(SelectorModel)
	assert.Equal(t, initiallySelected, updated.selected[firstPkg.Name])
}

func TestSelectorModelEnterShowsConfirmation(t *testing.T) {
	m := NewSelector("scratch")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(SelectorModel)
	assert.True(t, updated.showConfirmation)
}

func TestSelectorModelConfirmationEnterConfirms(t *testing.T) {
	m := NewSelector("scratch")
	m.showConfirmation = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(SelectorModel)
	assert.True(t, updated.confirmed)
	assert.NotNil(t, cmd)
}

func TestSelectorModelConfirmationEscGoesBack(t *testing.T) {
	m := NewSelector("scratch")
	m.showConfirmation = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(SelectorModel)
	assert.False(t, updated.showConfirmation)
	assert.False(t, updated.confirmed)
}

func TestSelectorModelSelectAll(t *testing.T) {
	m := NewSelector("scratch")
	cat := m.categories[m.activeTab]
	require.NotEmpty(t, cat.Packages)

	for _, pkg := range cat.Packages {
		m.selected[pkg.Name] = false
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated := result.(SelectorModel)
	for _, pkg := range cat.Packages {
		assert.True(t, updated.selected[pkg.Name])
	}

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated = result.(SelectorModel)
	for _, pkg := range cat.Packages {
		assert.False(t, updated.selected[pkg.Name])
	}
}

func TestSelectorModelWindowResize(t *testing.T) {
	m := NewSelector("scratch")

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := result.(SelectorModel)
	assert.Equal(t, 120, updated.width)
	assert.Equal(t, 40, updated.height)
}

func TestSelectorModelSelected(t *testing.T) {
	m := NewSelector("scratch")
	m.selected["git"] = true
	m.selected["curl"] = false

	selected := m.Selected()
	assert.True(t, selected["git"])
	assert.False(t, selected["curl"])
}

func TestSelectorModelConfirmed(t *testing.T) {
	m := NewSelector("scratch")
	assert.False(t, m.Confirmed())

	m.confirmed = true
	assert.True(t, m.Confirmed())
}

func TestSelectorModelToastOnToggle(t *testing.T) {
	m := NewSelector("scratch")
	cat := m.categories[m.activeTab]
	require.NotEmpty(t, cat.Packages)

	m.selected[cat.Packages[0].Name] = false

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	updated := result.(SelectorModel)
	assert.Contains(t, updated.toastMessage, cat.Packages[0].Name)
}

func TestSelectorModelSearchNavigateDownUp(t *testing.T) {
	m := NewSelector("scratch")
	m.searchMode = true
	m.filteredPkgs = []config.Package{
		{Name: "git"},
		{Name: "curl"},
		{Name: "wget"},
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated := result.(SelectorModel)
	assert.Equal(t, 1, updated.cursor)

	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated = result.(SelectorModel)
	assert.Equal(t, 0, updated.cursor)
}

func TestSelectorModelOnlineSelectedEmpty(t *testing.T) {
	m := NewSelector("scratch")
	result := m.OnlineSelected()
	assert.Empty(t, result)
}
