package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

type editorItemType int

const (
	editorItemFormula editorItemType = iota
	editorItemCask
	editorItemNpm
	editorItemTap
	editorItemMacOSPref
)

type editorItem struct {
	name        string
	description string
	selected    bool
	itemType    editorItemType
	isAdded     bool // true = user added this, not from original snapshot
}

type editorTab struct {
	name     string
	icon     string
	items    []editorItem
	itemType editorItemType
}

type editorFilteredRef struct {
	tabIdx  int
	itemIdx int
}

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

	// Online search state
	onlineResults         []editorItem
	onlineSearching       bool
	onlineSearchQuery     string
	onlineDebouncePending bool
	searchSpinnerIdx      int

	// Toast notification
	toastMessage string

	// Manual add mode
	addMode  bool
	addInput string
}

func NewSnapshotEditor(snap *snapshot.Snapshot) SnapshotEditorModel {
	tabs := make([]editorTab, 5)

	formulaeItems := make([]editorItem, len(snap.Packages.Formulae))
	for i, pkg := range snap.Packages.Formulae {
		formulaeItems[i] = editorItem{name: pkg, selected: true, itemType: editorItemFormula}
	}
	tabs[0] = editorTab{name: "Formulae", icon: "🍺", items: formulaeItems, itemType: editorItemFormula}

	caskItems := make([]editorItem, len(snap.Packages.Casks))
	for i, pkg := range snap.Packages.Casks {
		caskItems[i] = editorItem{name: pkg, selected: true, itemType: editorItemCask}
	}
	tabs[1] = editorTab{name: "Casks", icon: "📦", items: caskItems, itemType: editorItemCask}

	npmItems := make([]editorItem, len(snap.Packages.Npm))
	for i, pkg := range snap.Packages.Npm {
		npmItems[i] = editorItem{name: pkg, selected: true, itemType: editorItemNpm}
	}
	tabs[2] = editorTab{name: "NPM", icon: "📜", items: npmItems, itemType: editorItemNpm}

	tapItems := make([]editorItem, len(snap.Packages.Taps))
	for i, tap := range snap.Packages.Taps {
		tapItems[i] = editorItem{name: tap, selected: true, itemType: editorItemTap}
	}
	tabs[3] = editorTab{name: "Taps", icon: "🔌", items: tapItems, itemType: editorItemTap}

	prefItems := make([]editorItem, len(snap.MacOSPrefs))
	for i, p := range snap.MacOSPrefs {
		prefItems[i] = editorItem{
			name:        fmt.Sprintf("%s.%s", p.Domain, p.Key),
			description: fmt.Sprintf("= %s (%s)", p.Value, p.Desc),
			selected:    true,
			itemType:    editorItemMacOSPref,
		}
	}
	tabs[4] = editorTab{name: "macOS Prefs", icon: "⚙️", items: prefItems, itemType: editorItemMacOSPref}

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

	case editorOnlineSearchResultMsg:
		if msg.query == m.searchQuery {
			m.onlineSearching = false
			if msg.err != nil {
				m.toastMessage = "Online search unavailable"
				return m, editorToastClearCmd()
			}
			m.onlineResults = nil
			for _, pkg := range msg.results {
				m.onlineResults = append(m.onlineResults, packageToEditorItem(pkg))
			}
			if total := m.totalSearchItems(); total > 0 && m.cursor >= total {
				m.cursor = total - 1
			}
		}
		return m, nil

	case editorOnlineSearchTickMsg:
		if m.onlineDebouncePending && m.searchQuery != "" && m.searchQuery == m.onlineSearchQuery {
			m.onlineDebouncePending = false
			m.onlineSearching = true
			m.searchSpinnerIdx = 0
			return m, tea.Batch(editorSearchOnlineCmd(m.searchQuery), editorSearchSpinnerTickCmd())
		}
		m.onlineDebouncePending = false
		return m, nil

	case editorSearchSpinnerTickMsg:
		if m.searchMode && m.onlineSearching {
			m.searchSpinnerIdx = (m.searchSpinnerIdx + 1) % len(searchSpinnerFrames)
			return m, editorSearchSpinnerTickCmd()
		}
		return m, nil

	case editorToastClearMsg:
		m.toastMessage = ""
		return m, nil

	case tea.KeyMsg:
		if m.addMode {
			return m.updateAddMode(msg)
		}
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
			m = m.withFilteredItems()

		case msg.String() == "+":
			// Block manual add on macOS Prefs tab (prefs require domain.key structure)
			if m.tabs[m.activeTab].itemType == editorItemMacOSPref {
				m.toastMessage = "Cannot manually add macOS prefs"
				return m, editorToastClearCmd()
			}
			m.addMode = true
			m.addInput = ""

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
		m.onlineResults = nil
		m.onlineSearching = false
		m.onlineDebouncePending = false
		m.onlineSearchQuery = ""
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil
	case " ", "enter":
		return m.toggleOrAddSearchItem()

	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m = m.withFilteredItems()
			m.cursor = 0
			m.scrollOffset = 0
			m.onlineSearchQuery = m.searchQuery
			m.onlineDebouncePending = true
			m.onlineResults = nil
			if m.searchQuery == "" {
				m.onlineDebouncePending = false
				m.onlineSearching = false
			}
			return m, editorOnlineSearchDebounceCmd()
		}
		return m, nil
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down":
		if m.cursor < m.totalSearchItems()-1 {
			m.cursor++
		}
		return m, nil
	default:
		if len(msg.String()) == 1 && msg.String() >= " " {
			m.searchQuery += msg.String()
			m = m.withFilteredItems()
			m.cursor = 0
			m.onlineSearchQuery = m.searchQuery
			m.onlineDebouncePending = true
			m.onlineResults = nil
			return m, editorOnlineSearchDebounceCmd()
		}
	}
	return m, nil
}

func (m SnapshotEditorModel) updateAddMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addMode = false
		m.addInput = ""
		return m, nil
	case "enter":
		if m.addInput != "" {
			tab := &m.tabs[m.activeTab]
			// Check for duplicates
			duplicate := false
			for _, item := range tab.items {
				if item.name == m.addInput {
					duplicate = true
					break
				}
			}
			if !duplicate {
				tab.items = append(tab.items, editorItem{
					name:     m.addInput,
					selected: true,
					itemType: m.tabs[m.activeTab].itemType,
					isAdded:  true,
				})
				m.toastMessage = fmt.Sprintf("+ Added %s", m.addInput)
				m.addMode = false
				m.addInput = ""
				// Move cursor to the new item
				m.cursor = len(tab.items) - 1
				return m, editorToastClearCmd()
			}
		}
		m.addMode = false
		m.addInput = ""
		return m, nil
	case "backspace":
		if len(m.addInput) > 0 {
			m.addInput = m.addInput[:len(m.addInput)-1]
		}
		return m, nil
	default:
		if len(msg.String()) == 1 && msg.String() >= " " {
			m.addInput += msg.String()
		}
		return m, nil
	}
}

func (m SnapshotEditorModel) withFilteredItems() SnapshotEditorModel {
	if m.searchQuery == "" {
		m.filteredItems = nil
		m.filteredRefs = nil
		return m
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
	return m
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

	if m.addMode {
		lines = append(lines, "")
		tabName := m.tabs[m.activeTab].name
		lines = append(lines, activeTabStyle.Render(fmt.Sprintf("Add to %s: %s▌", tabName, m.addInput)))
		lines = append(lines, descStyle.Render("  Type a package name and press Enter to add, Esc to cancel"))
		lines = append(lines, "")
	}

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
			if item.isAdded && item.selected {
				checkbox = "[+]"
				style = selectedStyle
			} else if item.selected {
				checkbox = "[✓]"
				style = selectedStyle
			}

			line := fmt.Sprintf("%s%s %s", cursor, checkbox, style.Render(item.name))
			if item.description != "" {
				line += " " + descStyle.Render(item.description)
			}
			lines = append(lines, truncateLine(line, m.width))
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

	if m.toastMessage != "" {
		lines = append(lines, "")
		lines = append(lines, selectedStyle.Render(m.toastMessage))
	}

	lines = append(lines, "")
	lines = append(lines, countStyle.Render(m.selectedCountsSummary()))

	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("/: search online • +: add manually • Space: toggle • a: all • Tab/←→: switch • Enter: confirm • q: cancel"))

	return strings.Join(lines, "\n")
}

func (m SnapshotEditorModel) viewSearch() string {
	var lines []string

	searchBox := fmt.Sprintf("Search: %s▌", m.searchQuery)
	lines = append(lines, activeTabStyle.Render(searchBox))
	lines = append(lines, "")

	visibleItems := m.getVisibleItems()
	totalItems := m.totalSearchItems()

	if totalItems == 0 && !m.onlineSearching {
		if m.searchQuery == "" {
			lines = append(lines, descStyle.Render("Type to search items..."))
		} else {
			lines = append(lines, descStyle.Render("No items found"))
		}
	} else {
		rendered := 0

		// Render local filtered items
		for i := 0; i < len(m.filteredItems) && rendered < visibleItems; i++ {
			ref := m.filteredRefs[i]
			item := m.tabs[ref.tabIdx].items[ref.itemIdx]
			lines = append(lines, m.renderSearchItem(item, i))
			rendered++
		}

		// Online results separator
		if len(m.onlineResults) > 0 && rendered < visibleItems {
			lines = append(lines, onlineHeaderStyle.Render("── Online Results ──"))
			rendered++
		}

		// Render online results
		for i := 0; i < len(m.onlineResults) && rendered < visibleItems; i++ {
			globalIdx := len(m.filteredItems) + i
			lines = append(lines, m.renderSearchItem(m.onlineResults[i], globalIdx))
			rendered++
		}

		// Spinner for ongoing search
		if m.onlineSearching && rendered < visibleItems {
			spinner := searchSpinnerFrames[m.searchSpinnerIdx]
			lines = append(lines, onlineSearchingStyle.Render(fmt.Sprintf("  %s Searching online...", spinner)))
			rendered++
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

	// Toast notification
	if m.toastMessage != "" {
		lines = append(lines, "")
		lines = append(lines, selectedStyle.Render(m.toastMessage))
	}

	totalSelected := m.totalSelected()
	lines = append(lines, "")
	lines = append(lines, countStyle.Render(fmt.Sprintf("Selected: %d items • Local: %d • Online: %d", totalSelected, len(m.filteredItems), len(m.onlineResults))))
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("↑↓: navigate • Space/Enter: toggle/add • Esc: exit search"))

	return strings.Join(lines, "\n")
}

func (m SnapshotEditorModel) toggleOrAddSearchItem() (SnapshotEditorModel, tea.Cmd) {
	total := m.totalSearchItems()
	if total > 0 && m.cursor < total {
		if m.cursor < len(m.filteredItems) {
			ref := m.filteredRefs[m.cursor]
			m.tabs[ref.tabIdx].items[ref.itemIdx].selected = !m.tabs[ref.tabIdx].items[ref.itemIdx].selected
		} else {
			onlineIdx := m.cursor - len(m.filteredItems)
			if onlineIdx < len(m.onlineResults) {
				item := m.onlineResults[onlineIdx]
				m, toast := m.withOnlineResultAdded(item)
				if toast != "" {
					m.toastMessage = toast
					return m, editorToastClearCmd()
				}
			}
		}
	}
	return m, nil
}

func (m SnapshotEditorModel) renderSearchItem(item editorItem, globalIdx int) string {
	cursor := "  "
	if globalIdx == m.cursor {
		cursor = "> "
	}

	checkbox := "[ ]"
	style := itemStyle
	if item.isAdded && item.selected {
		checkbox = "[+]"
		style = selectedStyle
	} else if item.selected {
		checkbox = "[✓]"
		style = selectedStyle
	}

	line := fmt.Sprintf("%s%s %s", cursor, checkbox, style.Render(item.name))
	if item.description != "" {
		line += " " + descStyle.Render(item.description)
	}
	return truncateLine(line, m.width)
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
	counts := make(map[editorItemType]int)
	for _, tab := range m.tabs {
		for _, item := range tab.items {
			if item.selected {
				counts[item.itemType]++
			}
		}
	}

	var parts []string
	if c := counts[editorItemFormula]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d formulae", c))
	}
	if c := counts[editorItemCask]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d casks", c))
	}
	if c := counts[editorItemNpm]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d npm", c))
	}
	if c := counts[editorItemTap]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d taps", c))
	}
	if c := counts[editorItemMacOSPref]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d preferences", c))
	}

	if len(parts) == 0 {
		return "nothing selected"
	}
	return strings.Join(parts, ", ") + " selected"
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

func buildEditedSnapshot(original *snapshot.Snapshot, m *SnapshotEditorModel) *snapshot.Snapshot {
	edited := &snapshot.Snapshot{
		Version:       original.Version,
		CapturedAt:    original.CapturedAt,
		Hostname:      original.Hostname,
		Shell:         original.Shell,
		Git:           original.Git,
		DevTools:      original.DevTools,
		MatchedPreset: original.MatchedPreset,
		CatalogMatch:  original.CatalogMatch,
	}

	// Build lookup maps for original snapshot items (for macOS prefs matching)
	originalPrefs := make(map[string]snapshot.MacOSPref, len(original.MacOSPrefs))
	for _, p := range original.MacOSPrefs {
		originalPrefs[fmt.Sprintf("%s.%s", p.Domain, p.Key)] = p
	}

	for _, tab := range m.tabs {
		for _, item := range tab.items {
			if !item.selected {
				continue
			}
			switch item.itemType {
			case editorItemFormula:
				edited.Packages.Formulae = append(edited.Packages.Formulae, item.name)
			case editorItemCask:
				edited.Packages.Casks = append(edited.Packages.Casks, item.name)
			case editorItemNpm:
				edited.Packages.Npm = append(edited.Packages.Npm, item.name)
			case editorItemTap:
				edited.Packages.Taps = append(edited.Packages.Taps, item.name)
			case editorItemMacOSPref:
				if pref, ok := originalPrefs[item.name]; ok {
					edited.MacOSPrefs = append(edited.MacOSPrefs, pref)
				}
			}
		}
	}

	return edited
}
