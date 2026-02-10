package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/search"
	"github.com/sahilm/fuzzy"
)

var (
	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#666"))

	activeTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#22c55e")).
			Bold(true).
			Underline(true)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fff"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e"))

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444")).
			MarginTop(1)

	countStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888"))

	badgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666"))

	boldStyle = lipgloss.NewStyle().
			Bold(true)

	onlineHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f59e0b")).
				Bold(true)

	onlineSearchingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888")).
				Italic(true)
)

type onlineSearchResultMsg struct {
	Results []config.Package
	Query   string
	Err     error
}

type onlineSearchTickMsg struct{}

const onlineSearchDebounce = 500 * time.Millisecond

type SelectorModel struct {
	categories            []config.Category
	selected              map[string]bool
	selectedOnline        map[string]config.Package
	activeTab             int
	cursor                int
	confirmed             bool
	width                 int
	height                int
	scrollOffset          int
	searchMode            bool
	searchQuery           string
	filteredPkgs          []config.Package
	fuzzyMatches          []fuzzy.Match
	cursorPositions       map[int]int
	onlineResults         []config.Package
	onlineSearching       bool
	onlineSearchQuery     string
	onlineDebouncePending bool
	showConfirmation      bool
}

func NewSelector(presetName string) SelectorModel {
	return SelectorModel{
		categories:      config.Categories,
		selected:        config.GetPackagesForPreset(presetName),
		selectedOnline:  make(map[string]config.Package),
		activeTab:       0,
		cursor:          0,
		cursorPositions: make(map[int]int),
	}
}

func (m SelectorModel) Init() tea.Cmd {
	return nil
}

func searchOnlineCmd(query string) tea.Cmd {
	return func() tea.Msg {
		results, err := search.SearchOnline(query)
		return onlineSearchResultMsg{Results: results, Query: query, Err: err}
	}
}

func onlineSearchDebounceCmd() tea.Cmd {
	return tea.Tick(onlineSearchDebounce, func(time.Time) tea.Msg {
		return onlineSearchTickMsg{}
	})
}

func (m SelectorModel) totalSearchItems() int {
	return len(m.filteredPkgs) + len(m.onlineResults)
}

func (m SelectorModel) searchItemAt(index int) (config.Package, bool) {
	if index < len(m.filteredPkgs) {
		return m.filteredPkgs[index], false
	}
	onlineIdx := index - len(m.filteredPkgs)
	if onlineIdx < len(m.onlineResults) {
		return m.onlineResults[onlineIdx], true
	}
	return config.Package{}, false
}

func (m SelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case onlineSearchResultMsg:
		if msg.Query == m.searchQuery {
			m.onlineSearching = false
			m.onlineResults = msg.Results
			if total := m.totalSearchItems(); total > 0 && m.cursor >= total {
				m.cursor = total - 1
			}
		}
		return m, nil

	case onlineSearchTickMsg:
		if m.onlineDebouncePending && m.searchQuery != "" && m.searchQuery == m.onlineSearchQuery {
			m.onlineDebouncePending = false
			m.onlineSearching = true
			return m, searchOnlineCmd(m.searchQuery)
		}
		m.onlineDebouncePending = false
		return m, nil

	case tea.KeyMsg:
		if m.showConfirmation {
			switch msg.String() {
			case "enter":
				m.confirmed = true
				return m, tea.Quit
			case "esc":
				m.showConfirmation = false
				return m, nil
			}
			return m, nil
		}

		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.filteredPkgs = nil
				m.onlineResults = nil
				m.onlineSearching = false
				m.onlineDebouncePending = false
				m.cursor = 0
				m.scrollOffset = 0
				return m, nil
			case " ":
				total := m.totalSearchItems()
				if total > 0 && m.cursor < total {
					pkg, isOnline := m.searchItemAt(m.cursor)
					m.selected[pkg.Name] = !m.selected[pkg.Name]
					if isOnline {
						if m.selected[pkg.Name] {
							m.selectedOnline[pkg.Name] = pkg
						} else {
							delete(m.selectedOnline, pkg.Name)
						}
					}
				}
				return m, nil
			case "enter":
				m.showConfirmation = true
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.updateFilteredPackages()
					m.cursor = 0
					m.scrollOffset = 0
					m.onlineSearchQuery = m.searchQuery
					m.onlineDebouncePending = true
					m.onlineResults = nil
					if m.searchQuery == "" {
						m.onlineDebouncePending = false
						m.onlineSearching = false
					}
					return m, onlineSearchDebounceCmd()
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
					m.updateFilteredPackages()
					m.cursor = 0
					m.onlineSearchQuery = m.searchQuery
					m.onlineDebouncePending = true
					m.onlineResults = nil
					return m, onlineSearchDebounceCmd()
				}
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

		case msg.String() == "/":
			m.searchMode = true
			m.searchQuery = ""
			m.cursor = 0
			m.updateFilteredPackages()

		case key.Matches(msg, keys.Tab), key.Matches(msg, keys.Right):
			m.cursorPositions[m.activeTab] = m.cursor
			m.activeTab = (m.activeTab + 1) % len(m.categories)
			m.cursor = m.cursorPositions[m.activeTab]
			if m.cursor >= len(m.categories[m.activeTab].Packages) {
				m.cursor = 0
			}
			m.scrollOffset = 0

		case key.Matches(msg, keys.ShiftTab), key.Matches(msg, keys.Left):
			m.cursorPositions[m.activeTab] = m.cursor
			m.activeTab = (m.activeTab - 1 + len(m.categories)) % len(m.categories)
			m.cursor = m.cursorPositions[m.activeTab]
			if m.cursor >= len(m.categories[m.activeTab].Packages) {
				m.cursor = 0
			}
			m.scrollOffset = 0

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scrollOffset {
					m.scrollOffset = m.cursor
				}
			}

		case key.Matches(msg, keys.Down):
			cat := m.categories[m.activeTab]
			if m.cursor < len(cat.Packages)-1 {
				m.cursor++
				visibleItems := m.getVisibleItems()
				if m.cursor >= m.scrollOffset+visibleItems {
					m.scrollOffset = m.cursor - visibleItems + 1
				}
			}

		case key.Matches(msg, keys.Space):
			cat := m.categories[m.activeTab]
			if m.cursor < len(cat.Packages) {
				pkg := cat.Packages[m.cursor]
				m.selected[pkg.Name] = !m.selected[pkg.Name]
			}

		case key.Matches(msg, keys.Enter):
			m.showConfirmation = true
			return m, nil

		case key.Matches(msg, keys.SelectAll):
			cat := m.categories[m.activeTab]
			allSelected := true
			for _, pkg := range cat.Packages {
				if !m.selected[pkg.Name] {
					allSelected = false
					break
				}
			}
			for _, pkg := range cat.Packages {
				m.selected[pkg.Name] = !allSelected
			}
		}
	}

	return m, nil
}

func (m *SelectorModel) updateFilteredPackages() {
	if m.searchQuery == "" {
		m.filteredPkgs = nil
		m.fuzzyMatches = nil
		return
	}

	var allPackages []config.Package
	var packageNames []string

	for _, cat := range m.categories {
		for _, pkg := range cat.Packages {
			allPackages = append(allPackages, pkg)
			packageNames = append(packageNames, pkg.Name)
		}
	}

	matches := fuzzy.Find(m.searchQuery, packageNames)

	m.filteredPkgs = nil
	m.fuzzyMatches = nil

	for _, match := range matches {
		m.filteredPkgs = append(m.filteredPkgs, allPackages[match.Index])
		m.fuzzyMatches = append(m.fuzzyMatches, match)
	}
}

func (m SelectorModel) getVisibleItems() int {
	if m.height == 0 {
		return 15
	}
	available := m.height - 8
	if available < 5 {
		available = 5
	}
	if available > 20 {
		available = 20
	}
	return available
}

func getTypeBadge(pkg config.Package) string {
	if pkg.IsNpm {
		return badgeStyle.Render("📦 ")
	}
	if pkg.IsCask {
		return badgeStyle.Render("🖥 ")
	}
	return badgeStyle.Render("⚙ ")
}

func highlightMatches(text string, matchedIndexes []int) string {
	if len(matchedIndexes) == 0 {
		return text
	}

	var result strings.Builder
	matchSet := make(map[int]bool)
	for _, idx := range matchedIndexes {
		matchSet[idx] = true
	}

	for i, char := range text {
		if matchSet[i] {
			result.WriteString(boldStyle.Render(string(char)))
		} else {
			result.WriteRune(char)
		}
	}

	return result.String()
}

func truncateLine(line string, maxWidth int) string {
	if maxWidth <= 0 || len(line) <= maxWidth {
		return line
	}
	if maxWidth < 10 {
		return line[:maxWidth]
	}
	return line[:maxWidth-3] + "..."
}

func (m SelectorModel) View() string {
	if m.showConfirmation {
		return m.confirmationView()
	}

	var lines []string

	if m.searchMode {
		return m.viewSearch()
	}

	var tabs []string
	for i, cat := range m.categories {
		count := 0
		for _, pkg := range cat.Packages {
			if m.selected[pkg.Name] {
				count++
			}
		}
		label := fmt.Sprintf("%s %s (%d)", cat.Icon, cat.Name, count)
		if i == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, tabStyle.Render(label))
		}
	}
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	lines = append(lines, "")

	cat := m.categories[m.activeTab]
	visibleItems := m.getVisibleItems()

	if m.scrollOffset > len(cat.Packages)-visibleItems {
		m.scrollOffset = len(cat.Packages) - visibleItems
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	endIdx := m.scrollOffset + visibleItems
	if endIdx > len(cat.Packages) {
		endIdx = len(cat.Packages)
	}

	for i := m.scrollOffset; i < endIdx; i++ {
		pkg := cat.Packages[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		checkbox := "[ ]"
		style := itemStyle
		if m.selected[pkg.Name] {
			checkbox = "[✓]"
			style = selectedStyle
		}

		line := fmt.Sprintf("%s%s %s %s", cursor, checkbox, style.Render(pkg.Name), descStyle.Render(pkg.Description))
		if m.width > 0 {
			line = truncateLine(line, m.width-2)
		}
		lines = append(lines, line)
	}

	clearWidth := 80
	if m.width > 0 && m.width < 80 {
		clearWidth = m.width
	}
	clearLine := strings.Repeat(" ", clearWidth)
	for len(lines) < visibleItems+2 {
		lines = append(lines, clearLine)
	}

	totalSelected := 0
	for _, v := range m.selected {
		if v {
			totalSelected++
		}
	}

	lines = append(lines, "")
	lines = append(lines, countStyle.Render(fmt.Sprintf("Selected: %d packages", totalSelected)))
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("Tab/←→: switch • ↑↓: navigate • Space: toggle • /: search • a: all • Enter: confirm • q: quit"))

	return strings.Join(lines, "\n")
}

func (m SelectorModel) confirmationView() string {
	var formulae, casks, npm []string
	for name, selected := range m.selected {
		if !selected {
			continue
		}

		var pkg *config.Package
		for _, cat := range m.categories {
			for i := range cat.Packages {
				if cat.Packages[i].Name == name {
					pkg = &cat.Packages[i]
					break
				}
			}
			if pkg != nil {
				break
			}
		}

		if pkg == nil {
			if op, ok := m.selectedOnline[name]; ok {
				pkg = &op
			}
		}

		if pkg != nil {
			if pkg.IsNpm {
				npm = append(npm, pkg.Name)
			} else if pkg.IsCask {
				casks = append(casks, pkg.Name)
			} else {
				formulae = append(formulae, pkg.Name)
			}
		}
	}

	totalPackages := len(formulae) + len(casks) + len(npm)

	estimatedSeconds := len(formulae)*15 + len(casks)*30 + len(npm)*5
	estimatedMinutes := estimatedSeconds / 60

	boxWidth := 60
	if m.width > 0 && m.width < 70 {
		boxWidth = m.width - 10
		if boxWidth < 40 {
			boxWidth = 40
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#22c55e")).
		Padding(1, 2).
		Width(boxWidth)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#22c55e")).
		Bold(true)

	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#fff")).
		Bold(true)

	listStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888"))

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666")).
		Italic(true)

	var content strings.Builder

	content.WriteString(headerStyle.Render("Install Summary"))
	content.WriteString("\n\n")
	content.WriteString(fmt.Sprintf("Total: %d packages\n\n", totalPackages))

	if len(formulae) > 0 {
		content.WriteString(sectionStyle.Render(fmt.Sprintf("⚙  Formulae (%d)", len(formulae))))
		content.WriteString("\n")
		if len(formulae) <= 10 {
			content.WriteString(listStyle.Render("  " + strings.Join(formulae, ", ")))
		} else {
			content.WriteString(listStyle.Render("  " + strings.Join(formulae[:10], ", ")))
			content.WriteString(listStyle.Render(fmt.Sprintf(" and %d more...", len(formulae)-10)))
		}
		content.WriteString("\n\n")
	}

	if len(casks) > 0 {
		content.WriteString(sectionStyle.Render(fmt.Sprintf("🖥  Applications (%d)", len(casks))))
		content.WriteString("\n")
		if len(casks) <= 10 {
			content.WriteString(listStyle.Render("  " + strings.Join(casks, ", ")))
		} else {
			content.WriteString(listStyle.Render("  " + strings.Join(casks[:10], ", ")))
			content.WriteString(listStyle.Render(fmt.Sprintf(" and %d more...", len(casks)-10)))
		}
		content.WriteString("\n\n")
	}

	if len(npm) > 0 {
		content.WriteString(sectionStyle.Render(fmt.Sprintf("📦  NPM (%d)", len(npm))))
		content.WriteString("\n")
		if len(npm) <= 10 {
			content.WriteString(listStyle.Render("  " + strings.Join(npm, ", ")))
		} else {
			content.WriteString(listStyle.Render("  " + strings.Join(npm[:10], ", ")))
			content.WriteString(listStyle.Render(fmt.Sprintf(" and %d more...", len(npm)-10)))
		}
		content.WriteString("\n\n")
	}

	content.WriteString(fmt.Sprintf("Estimated time: ~%d minutes\n\n", estimatedMinutes))
	content.WriteString(instructionStyle.Render("[Enter] Confirm & Install"))
	content.WriteString("\n")
	content.WriteString(instructionStyle.Render("[Esc] Go Back"))

	return boxStyle.Render(content.String())
}

func (m SelectorModel) viewSearch() string {
	var lines []string

	searchBox := fmt.Sprintf("Search: %s▌", m.searchQuery)
	lines = append(lines, activeTabStyle.Render(searchBox))
	lines = append(lines, "")

	visibleItems := m.getVisibleItems()
	itemsRendered := 0

	if len(m.filteredPkgs) == 0 && len(m.onlineResults) == 0 && !m.onlineSearching {
		if m.searchQuery == "" {
			lines = append(lines, descStyle.Render("Type to search packages..."))
		} else {
			lines = append(lines, descStyle.Render("No packages found"))
		}
	} else {
		endIdx := visibleItems
		if endIdx > len(m.filteredPkgs) {
			endIdx = len(m.filteredPkgs)
		}

		for i := 0; i < endIdx; i++ {
			pkg := m.filteredPkgs[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			checkbox := "[ ]"
			style := itemStyle
			if m.selected[pkg.Name] {
				checkbox = "[✓]"
				style = selectedStyle
			}

			badge := getTypeBadge(pkg)

			var displayName string
			if i < len(m.fuzzyMatches) {
				displayName = highlightMatches(pkg.Name, m.fuzzyMatches[i].MatchedIndexes)
			} else {
				displayName = pkg.Name
			}

			line := fmt.Sprintf("%s%s %s%s %s", cursor, checkbox, badge, style.Render(displayName), descStyle.Render(pkg.Description))
			if m.width > 0 {
				line = truncateLine(line, m.width-2)
			}
			lines = append(lines, line)
			itemsRendered++
		}

		if m.onlineSearching {
			lines = append(lines, "")
			lines = append(lines, onlineSearchingStyle.Render("  Searching online..."))
			itemsRendered += 2
		} else if len(m.onlineResults) > 0 {
			lines = append(lines, "")
			lines = append(lines, onlineHeaderStyle.Render("── Online Results ──"))
			itemsRendered += 2

			onlineVisibleLimit := visibleItems - itemsRendered
			if onlineVisibleLimit < 1 {
				onlineVisibleLimit = 1
			}
			onlineEnd := onlineVisibleLimit
			if onlineEnd > len(m.onlineResults) {
				onlineEnd = len(m.onlineResults)
			}

			offlineCount := len(m.filteredPkgs)
			for i := 0; i < onlineEnd; i++ {
				pkg := m.onlineResults[i]
				globalIdx := offlineCount + i
				cursor := "  "
				if globalIdx == m.cursor {
					cursor = "> "
				}

				checkbox := "[ ]"
				style := itemStyle
				if m.selected[pkg.Name] {
					checkbox = "[✓]"
					style = selectedStyle
				}

				badge := getTypeBadge(pkg)
				line := fmt.Sprintf("%s%s %s%s %s", cursor, checkbox, badge, style.Render(pkg.Name), descStyle.Render(pkg.Description))
				if m.width > 0 {
					line = truncateLine(line, m.width-2)
				}
				lines = append(lines, line)
				itemsRendered++
			}
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

	totalSelected := 0
	for _, v := range m.selected {
		if v {
			totalSelected++
		}
	}

	foundCount := len(m.filteredPkgs) + len(m.onlineResults)
	lines = append(lines, "")
	lines = append(lines, countStyle.Render(fmt.Sprintf("Selected: %d packages • Found: %d", totalSelected, foundCount)))
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("↑↓: navigate • Space: toggle • Esc: exit search • Enter: confirm"))

	return strings.Join(lines, "\n")
}

func (m SelectorModel) Selected() map[string]bool {
	return m.selected
}

func (m SelectorModel) OnlineSelected() []config.Package {
	var result []config.Package
	for _, pkg := range m.selectedOnline {
		if m.selected[pkg.Name] {
			result = append(result, pkg)
		}
	}
	return result
}

func (m SelectorModel) Confirmed() bool {
	return m.confirmed
}

type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	Tab       key.Binding
	ShiftTab  key.Binding
	Space     key.Binding
	Enter     key.Binding
	SelectAll key.Binding
	Quit      key.Binding
}

var keys = keyMap{
	Up:        key.NewBinding(key.WithKeys("up", "k")),
	Down:      key.NewBinding(key.WithKeys("down", "j")),
	Left:      key.NewBinding(key.WithKeys("left", "h")),
	Right:     key.NewBinding(key.WithKeys("right", "l")),
	Tab:       key.NewBinding(key.WithKeys("tab")),
	ShiftTab:  key.NewBinding(key.WithKeys("shift+tab")),
	Space:     key.NewBinding(key.WithKeys(" ")),
	Enter:     key.NewBinding(key.WithKeys("enter")),
	SelectAll: key.NewBinding(key.WithKeys("a")),
	Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c")),
}

func RunSelector(presetName string) (map[string]bool, []config.Package, bool, error) {
	model := NewSelector(presetName)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, nil, false, err
	}

	m := finalModel.(SelectorModel)
	return m.Selected(), m.OnlineSelected(), m.Confirmed(), nil
}
