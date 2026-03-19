package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/macos"
)

// MacOSSelectorModel is a bubbletea model for selecting macOS preferences by category.
type MacOSSelectorModel struct {
	categories      []macos.PrefCategory
	selected        map[string]bool // key: macos.PrefKey(pref)
	activeTab       int
	cursor          int
	scrollOffset    int
	cursorPositions map[int]int
	showConfirmation bool
	confirmed       bool
	toastMessage    string
	toastIsAdd      bool
	width           int
	height          int
}

func NewMacOSSelector() MacOSSelectorModel {
	return MacOSSelectorModel{
		categories:      macos.DefaultCategories,
		selected:        macos.AllPrefsSelected(),
		cursorPositions: make(map[int]int),
	}
}

func (m MacOSSelectorModel) Init() tea.Cmd {
	return nil
}

func (m MacOSSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case toastClearMsg:
		m.toastMessage = ""
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

		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, keys.Tab), key.Matches(msg, keys.Right):
			m.cursorPositions[m.activeTab] = m.cursor
			m.activeTab = (m.activeTab + 1) % len(m.categories)
			m.cursor = m.cursorPositions[m.activeTab]
			if m.cursor >= len(m.categories[m.activeTab].Prefs) {
				m.cursor = 0
			}
			m.scrollOffset = 0

		case key.Matches(msg, keys.ShiftTab), key.Matches(msg, keys.Left):
			m.cursorPositions[m.activeTab] = m.cursor
			m.activeTab = (m.activeTab - 1 + len(m.categories)) % len(m.categories)
			m.cursor = m.cursorPositions[m.activeTab]
			if m.cursor >= len(m.categories[m.activeTab].Prefs) {
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
			prefs := m.categories[m.activeTab].Prefs
			if m.cursor < len(prefs)-1 {
				m.cursor++
				visibleItems := m.macosVisibleItems()
				if m.cursor >= m.scrollOffset+visibleItems {
					m.scrollOffset = m.cursor - visibleItems + 1
				}
			}

		case key.Matches(msg, keys.Space):
			prefs := m.categories[m.activeTab].Prefs
			if m.cursor < len(prefs) {
				pref := prefs[m.cursor]
				k := macos.PrefKey(pref)
				m.selected[k] = !m.selected[k]
				if m.selected[k] {
					m.toastMessage = "+ Enabled " + pref.Desc
					m.toastIsAdd = true
				} else {
					m.toastMessage = "- Disabled " + pref.Desc
					m.toastIsAdd = false
				}
				return m, toastClearCmd()
			}

		case key.Matches(msg, keys.Enter):
			m.showConfirmation = true
			return m, nil

		case key.Matches(msg, keys.SelectAll):
			cat := m.categories[m.activeTab]
			allEnabled := true
			for _, p := range cat.Prefs {
				if !m.selected[macos.PrefKey(p)] {
					allEnabled = false
					break
				}
			}
			for _, p := range cat.Prefs {
				m.selected[macos.PrefKey(p)] = !allEnabled
			}
			if !allEnabled {
				m.toastMessage = fmt.Sprintf("✔ Enabled all %s", cat.Name)
				m.toastIsAdd = true
			} else {
				m.toastMessage = fmt.Sprintf("○ Disabled all %s", cat.Name)
				m.toastIsAdd = false
			}
			return m, toastClearCmd()
		}
	}

	return m, nil
}

func (m MacOSSelectorModel) macosVisibleItems() int {
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

func (m MacOSSelectorModel) macosRenderTabBar() string {
	totalTabs := len(m.categories)

	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555"))
	neighborStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#444"))
	posStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555"))

	cat := m.categories[m.activeTab]
	count := 0
	for _, p := range cat.Prefs {
		if m.selected[macos.PrefKey(p)] {
			count++
		}
	}
	activeRendered := activeTabStyle.Render(fmt.Sprintf("%s %s (%d)", cat.Icon, cat.Name, count))
	posRendered := posStyle.Render(fmt.Sprintf("  %d/%d", m.activeTab+1, totalTabs))

	hasLeft := m.activeTab > 0
	hasRight := m.activeTab < totalTabs-1
	leftArrow := "  "
	if hasLeft {
		leftArrow = arrowStyle.Render("‹ ")
	}
	rightArrow := "  "
	if hasRight {
		rightArrow = arrowStyle.Render(" ›")
	}

	termWidth := m.width
	if termWidth == 0 {
		termWidth = 80
	}

	sep := sepStyle.Render(" │ ")
	sepW := lipgloss.Width(sep)
	baseWidth := lipgloss.Width(leftArrow) + lipgloss.Width(activeRendered) + lipgloss.Width(rightArrow) + lipgloss.Width(posRendered)
	remaining := termWidth - baseWidth

	var leftNeighbors []string
	var rightNeighbors []string
	li := m.activeTab - 1
	ri := m.activeTab + 1

	for remaining > 0 && (li >= 0 || ri < totalTabs) {
		added := false
		if li >= 0 {
			rendered := neighborStyle.Render(m.categories[li].Name)
			w := lipgloss.Width(rendered) + sepW
			if w <= remaining {
				leftNeighbors = append([]string{rendered}, leftNeighbors...)
				remaining -= w
				li--
				added = true
			} else {
				li = -1
			}
		}
		if ri < totalTabs {
			rendered := neighborStyle.Render(m.categories[ri].Name)
			w := lipgloss.Width(rendered) + sepW
			if w <= remaining {
				rightNeighbors = append(rightNeighbors, rendered)
				remaining -= w
				ri++
				added = true
			} else {
				ri = totalTabs
			}
		}
		if !added {
			break
		}
	}

	var result strings.Builder
	result.WriteString(leftArrow)
	for _, n := range leftNeighbors {
		result.WriteString(n)
		result.WriteString(sep)
	}
	result.WriteString(activeRendered)
	for _, n := range rightNeighbors {
		result.WriteString(sep)
		result.WriteString(n)
	}
	result.WriteString(rightArrow)
	result.WriteString(posRendered)

	return result.String()
}

func (m MacOSSelectorModel) View() string {
	if m.showConfirmation {
		return m.macosConfirmationView()
	}

	var lines []string
	lines = append(lines, m.macosRenderTabBar())
	lines = append(lines, "")

	cat := m.categories[m.activeTab]
	prefs := cat.Prefs
	visibleItems := m.macosVisibleItems()

	scrollOffset := m.scrollOffset
	if scrollOffset > len(prefs)-visibleItems {
		scrollOffset = len(prefs) - visibleItems
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	endIdx := scrollOffset + visibleItems
	if endIdx > len(prefs) {
		endIdx = len(prefs)
	}

	for i := scrollOffset; i < endIdx; i++ {
		pref := prefs[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		checkbox := "[ ]"
		style := itemStyle
		if m.selected[macos.PrefKey(pref)] {
			checkbox = "[✓]"
			style = selectedStyle
		}
		line := fmt.Sprintf("%s%s %s  %s", cursor, checkbox, style.Render(pref.Key), descStyle.Render(pref.Desc))
		if m.width > 0 {
			line = padLine(truncateLine(line, m.width-2), m.width)
		}
		lines = append(lines, line)
	}

	clearWidth := m.width
	if clearWidth <= 0 {
		clearWidth = 80
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
	if m.toastMessage != "" {
		toastStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Italic(true)
		if !m.toastIsAdd {
			toastStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Italic(true)
		}
		lines = append(lines, toastStyle.Render(m.toastMessage))
	} else {
		lines = append(lines, countStyle.Render(fmt.Sprintf("Selected: %d preferences", totalSelected)))
	}
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("Tab/←→: switch • ↑↓: navigate • Space: toggle • a: all • Enter: confirm • q: quit"))

	return strings.Join(lines, "\n")
}

func (m MacOSSelectorModel) macosConfirmationView() string {
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
	content.WriteString(headerStyle.Render("macOS Preferences Summary"))
	content.WriteString("\n\n")

	total := 0
	for _, v := range m.selected {
		if v {
			total++
		}
	}
	content.WriteString(fmt.Sprintf("Total: %d preferences\n\n", total))

	for _, cat := range m.categories {
		var enabled []string
		for _, p := range cat.Prefs {
			if m.selected[macos.PrefKey(p)] {
				enabled = append(enabled, p.Desc)
			}
		}
		if len(enabled) == 0 {
			continue
		}
		content.WriteString(sectionStyle.Render(fmt.Sprintf("%s %s (%d)", cat.Icon, cat.Name, len(enabled))))
		content.WriteString("\n")
		for _, desc := range enabled {
			content.WriteString(listStyle.Render("  • " + desc))
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	content.WriteString(instructionStyle.Render("[Enter] Apply Preferences"))
	content.WriteString("\n")
	content.WriteString(instructionStyle.Render("[Esc] Go Back"))

	return boxStyle.Render(content.String())
}

// SelectedPreferences returns the list of preferences the user enabled.
func (m MacOSSelectorModel) SelectedPreferences() []macos.Preference {
	var result []macos.Preference
	for _, cat := range m.categories {
		for _, p := range cat.Prefs {
			if m.selected[macos.PrefKey(p)] {
				result = append(result, p)
			}
		}
	}
	return result
}

func (m MacOSSelectorModel) Confirmed() bool {
	return m.confirmed
}

// RunMacOSSelector runs the interactive macOS preferences TUI and returns the
// selected preferences, whether the user confirmed, and any error.
func RunMacOSSelector() ([]macos.Preference, bool, error) {
	model := NewMacOSSelector()
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, false, err
	}

	m, ok := finalModel.(MacOSSelectorModel)
	if !ok {
		return nil, false, fmt.Errorf("unexpected model type returned from macOS selector")
	}
	return m.SelectedPreferences(), m.Confirmed(), nil
}

