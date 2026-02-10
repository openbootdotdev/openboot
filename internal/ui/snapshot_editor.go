package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

// editorItem represents a single toggleable item in the snapshot editor.
type editorItem struct {
	name        string
	description string
	selected    bool
}

// editorTab represents a tab in the snapshot editor.
type editorTab struct {
	name  string
	icon  string
	items []editorItem
}

// editorFilteredRef maps a filtered search result back to its source tab and item.
type editorFilteredRef struct {
	tabIdx  int
	itemIdx int
}

// SnapshotEditorModel is a Bubbletea model for reviewing and editing a captured snapshot.
type SnapshotEditorModel struct {
	tabs          []editorTab
	activeTab     int
	cursor        int
	confirmed     bool
	width         int
	height        int
	scrollOffset  int
	searchMode    bool
	searchQuery   string
	filteredItems []editorItem
	filteredRefs  []editorFilteredRef
	snapshot      *snapshot.Snapshot
}

// NewSnapshotEditor creates a new SnapshotEditorModel from a captured snapshot.
func NewSnapshotEditor(snap *snapshot.Snapshot) SnapshotEditorModel {
	tabs := make([]editorTab, 3)

	formulaeItems := make([]editorItem, len(snap.Packages.Formulae))
	for i, pkg := range snap.Packages.Formulae {
		formulaeItems[i] = editorItem{name: pkg, selected: true}
	}
	tabs[0] = editorTab{name: "Formulae", icon: "🍺", items: formulaeItems}

	caskItems := make([]editorItem, len(snap.Packages.Casks))
	for i, pkg := range snap.Packages.Casks {
		caskItems[i] = editorItem{name: pkg, selected: true}
	}
	tabs[1] = editorTab{name: "Casks", icon: "📦", items: caskItems}

	prefItems := make([]editorItem, len(snap.MacOSPrefs))
	for i, p := range snap.MacOSPrefs {
		prefItems[i] = editorItem{
			name:        fmt.Sprintf("%s.%s", p.Domain, p.Key),
			description: fmt.Sprintf("= %s (%s)", p.Value, p.Desc),
			selected:    true,
		}
	}
	tabs[2] = editorTab{name: "macOS Prefs", icon: "⚙️", items: prefItems}

	return SnapshotEditorModel{
		tabs:      tabs,
		activeTab: 0,
		cursor:    0,
		snapshot:  snap,
	}
}

func (m SnapshotEditorModel) Init() tea.Cmd {
	return nil
}

func (m SnapshotEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.searchMode {
			return m.updateSearch(msg)
		}

		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

		case msg.String() == "/":
			m.searchMode = true
			m.searchQuery = ""
			m.cursor = 0
			m.scrollOffset = 0
			m.updateFilteredItems()

		case key.Matches(msg, keys.Tab), key.Matches(msg, keys.Right):
			m.activeTab = (m.activeTab + 1) % len(m.tabs)
			m.cursor = 0
			m.scrollOffset = 0

		case key.Matches(msg, keys.ShiftTab), key.Matches(msg, keys.Left):
			m.activeTab = (m.activeTab - 1 + len(m.tabs)) % len(m.tabs)
			m.cursor = 0
			m.scrollOffset = 0

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scrollOffset {
					m.scrollOffset = m.cursor
				}
			}

		case key.Matches(msg, keys.Down):
			tab := m.tabs[m.activeTab]
			if m.cursor < len(tab.items)-1 {
				m.cursor++
				visibleItems := m.getVisibleItems()
				if m.cursor >= m.scrollOffset+visibleItems {
					m.scrollOffset = m.cursor - visibleItems + 1
				}
			}

		case key.Matches(msg, keys.Space):
			tab := &m.tabs[m.activeTab]
			if m.cursor < len(tab.items) {
				tab.items[m.cursor].selected = !tab.items[m.cursor].selected
			}

		case key.Matches(msg, keys.Enter):
			m.confirmed = true
			return m, tea.Quit

		case key.Matches(msg, keys.SelectAll):
			tab := &m.tabs[m.activeTab]
			allSelected := true
			for _, item := range tab.items {
				if !item.selected {
					allSelected = false
					break
				}
			}
			for i := range tab.items {
				tab.items[i].selected = !allSelected
			}
		}
	}

	return m, nil
}

func (m SnapshotEditorModel) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchQuery = ""
		m.filteredItems = nil
		m.filteredRefs = nil
		m.cursor = 0
		m.scrollOffset = 0
	case "enter", " ":
		if len(m.filteredRefs) > 0 && m.cursor < len(m.filteredRefs) {
			ref := m.filteredRefs[m.cursor]
			m.tabs[ref.tabIdx].items[ref.itemIdx].selected = !m.tabs[ref.tabIdx].items[ref.itemIdx].selected
		}
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.updateFilteredItems()
			m.cursor = 0
		}
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(m.filteredItems)-1 {
			m.cursor++
		}
	default:
		if len(msg.String()) == 1 && msg.String() >= " " {
			m.searchQuery += msg.String()
			m.updateFilteredItems()
			m.cursor = 0
		}
	}
	return m, nil
}

func (m *SnapshotEditorModel) updateFilteredItems() {
	if m.searchQuery == "" {
		m.filteredItems = nil
		m.filteredRefs = nil
		return
	}

	query := strings.ToLower(m.searchQuery)
	m.filteredItems = nil
	m.filteredRefs = nil

	for ti, tab := range m.tabs {
		for ii, item := range tab.items {
			if strings.Contains(strings.ToLower(item.name), query) ||
				strings.Contains(strings.ToLower(item.description), query) {
				m.filteredItems = append(m.filteredItems, item)
				m.filteredRefs = append(m.filteredRefs, editorFilteredRef{tabIdx: ti, itemIdx: ii})
			}
		}
	}
}

func (m SnapshotEditorModel) getVisibleItems() int {
	if m.height == 0 {
		return 15
	}
	// Account for: title(2) + tabs(2) + summary(4) + counts(1) + help(2) + padding(2) = ~13 lines
	available := m.height - 13
	if available < 5 {
		available = 5
	}
	if available > 20 {
		available = 20
	}
	return available
}

func (m SnapshotEditorModel) View() string {
	if m.searchMode {
		return m.viewSearch()
	}

	var lines []string

	lines = append(lines, "")
	lines = append(lines, activeTabStyle.Render("📋 Snapshot Editor — Review your captured environment"))
	lines = append(lines, "")

	var tabs []string
	for i, tab := range m.tabs {
		count := 0
		for _, item := range tab.items {
			if item.selected {
				count++
			}
		}
		label := fmt.Sprintf("%s %s (%d)", tab.icon, tab.name, count)
		if i == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, tabStyle.Render(label))
		}
	}
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	lines = append(lines, "")

	tab := m.tabs[m.activeTab]
	visibleItems := m.getVisibleItems()

	if len(tab.items) == 0 {
		lines = append(lines, descStyle.Render("  No items"))
	} else {
		scrollOffset := m.scrollOffset
		if scrollOffset > len(tab.items)-visibleItems {
			scrollOffset = len(tab.items) - visibleItems
		}
		if scrollOffset < 0 {
			scrollOffset = 0
		}

		endIdx := scrollOffset + visibleItems
		if endIdx > len(tab.items) {
			endIdx = len(tab.items)
		}

		for i := scrollOffset; i < endIdx; i++ {
			item := tab.items[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			checkbox := "[ ]"
			style := itemStyle
			if item.selected {
				checkbox = "[✓]"
				style = selectedStyle
			}

			line := fmt.Sprintf("%s%s %s", cursor, checkbox, style.Render(item.name))
			if item.description != "" {
				line += " " + descStyle.Render(item.description)
			}
			if m.width > 0 && len(line) > m.width {
				if m.width < 10 {
					line = line[:m.width]
				} else {
					line = line[:m.width-3] + "..."
				}
			}
			lines = append(lines, line)
		}
	}

	clearWidth := 80
	if m.width > 0 && m.width < 80 {
		clearWidth = m.width
	}
	clearLine := strings.Repeat(" ", clearWidth)
	for len(lines) < visibleItems+5 {
		lines = append(lines, clearLine)
	}

	lines = append(lines, "")
	lines = append(lines, m.shellSummary())
	lines = append(lines, m.gitSummary())
	lines = append(lines, m.devToolsSummary())

	lines = append(lines, "")
	lines = append(lines, countStyle.Render(m.selectedCountsSummary()))

	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("Tab/←→: switch • ↑↓: navigate • Space: toggle • /: search • a: all • Enter: confirm • q: cancel"))

	return strings.Join(lines, "\n")
}

func (m SnapshotEditorModel) viewSearch() string {
	var lines []string

	searchBox := fmt.Sprintf("Search: %s▌", m.searchQuery)
	lines = append(lines, activeTabStyle.Render(searchBox))
	lines = append(lines, "")

	visibleItems := m.getVisibleItems()

	if len(m.filteredItems) == 0 {
		if m.searchQuery == "" {
			lines = append(lines, descStyle.Render("Type to search items..."))
		} else {
			lines = append(lines, descStyle.Render("No items found"))
		}
	} else {
		endIdx := visibleItems
		if endIdx > len(m.filteredItems) {
			endIdx = len(m.filteredItems)
		}

		for i := 0; i < endIdx; i++ {
			ref := m.filteredRefs[i]
			item := m.tabs[ref.tabIdx].items[ref.itemIdx]

			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			checkbox := "[ ]"
			style := itemStyle
			if item.selected {
				checkbox = "[✓]"
				style = selectedStyle
			}

			line := fmt.Sprintf("%s%s %s", cursor, checkbox, style.Render(item.name))
			if item.description != "" {
				line += " " + descStyle.Render(item.description)
			}
			if m.width > 0 && len(line) > m.width {
				if m.width < 10 {
					line = line[:m.width]
				} else {
					line = line[:m.width-3] + "..."
				}
			}
			lines = append(lines, line)
		}
	}

	clearWidth := 80
	if m.width > 0 && m.width < 80 {
		clearWidth = m.width
	}
	clearLine := strings.Repeat(" ", clearWidth)
	for len(lines) < visibleItems+2 {
		lines = append(lines, clearLine)
	}

	totalSelected := m.totalSelected()
	lines = append(lines, "")
	lines = append(lines, countStyle.Render(fmt.Sprintf("Selected: %d items • Found: %d", totalSelected, len(m.filteredItems))))
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("↑↓: navigate • Space: toggle • Esc: exit search • Enter: toggle selected"))

	return strings.Join(lines, "\n")
}

func (m SnapshotEditorModel) shellSummary() string {
	snap := m.snapshot
	summary := fmt.Sprintf("Shell: %s", snap.Shell.Default)
	if snap.Shell.OhMyZsh {
		summary += fmt.Sprintf(" (Oh-My-Zsh: installed, Theme: %s)", snap.Shell.Theme)
	}
	return descStyle.Render("  " + summary)
}

func (m SnapshotEditorModel) gitSummary() string {
	snap := m.snapshot
	if snap.Git.UserName == "" && snap.Git.UserEmail == "" {
		return descStyle.Render("  Git: not configured")
	}
	return descStyle.Render(fmt.Sprintf("  Git: %s <%s>", snap.Git.UserName, snap.Git.UserEmail))
}

func (m SnapshotEditorModel) devToolsSummary() string {
	snap := m.snapshot
	if len(snap.DevTools) == 0 {
		return descStyle.Render("  Dev Tools: none detected")
	}
	var parts []string
	for _, dt := range snap.DevTools {
		parts = append(parts, fmt.Sprintf("%s %s", dt.Name, dt.Version))
	}
	return descStyle.Render(fmt.Sprintf("  Dev Tools: %s", strings.Join(parts, ", ")))
}

func (m SnapshotEditorModel) selectedCountsSummary() string {
	counts := make([]int, len(m.tabs))
	for i, tab := range m.tabs {
		for _, item := range tab.items {
			if item.selected {
				counts[i]++
			}
		}
	}
	return fmt.Sprintf("%d formulae, %d casks, %d preferences selected", counts[0], counts[1], counts[2])
}

func (m SnapshotEditorModel) totalSelected() int {
	total := 0
	for _, tab := range m.tabs {
		for _, item := range tab.items {
			if item.selected {
				total++
			}
		}
	}
	return total
}

// RunSnapshotEditor launches the snapshot editor TUI and returns the edited snapshot.
// Returns (editedSnapshot, confirmed, error). If the user cancels, confirmed is false.
func RunSnapshotEditor(snap *snapshot.Snapshot) (*snapshot.Snapshot, bool, error) {
	model := NewSnapshotEditor(snap)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, false, err
	}

	m := finalModel.(SnapshotEditorModel)
	if !m.confirmed {
		return nil, false, nil
	}

	edited := buildEditedSnapshot(snap, &m)
	return edited, true, nil
}

// buildEditedSnapshot creates a new snapshot with deselected items removed.
func buildEditedSnapshot(original *snapshot.Snapshot, m *SnapshotEditorModel) *snapshot.Snapshot {
	edited := &snapshot.Snapshot{
		Version:       original.Version,
		CapturedAt:    time.Now(),
		Hostname:      original.Hostname,
		Shell:         original.Shell,
		Git:           original.Git,
		DevTools:      original.DevTools,
		MatchedPreset: original.MatchedPreset,
		CatalogMatch:  original.CatalogMatch,
	}

	for i, item := range m.tabs[0].items {
		if item.selected {
			edited.Packages.Formulae = append(edited.Packages.Formulae, original.Packages.Formulae[i])
		}
	}

	for i, item := range m.tabs[1].items {
		if item.selected {
			edited.Packages.Casks = append(edited.Packages.Casks, original.Packages.Casks[i])
		}
	}

	edited.Packages.Taps = original.Packages.Taps

	for i, item := range m.tabs[2].items {
		if item.selected {
			edited.MacOSPrefs = append(edited.MacOSPrefs, original.MacOSPrefs[i])
		}
	}

	return edited
}
