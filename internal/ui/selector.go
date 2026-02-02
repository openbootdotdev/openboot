package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/config"
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
)

type SelectorModel struct {
	categories   []config.Category
	selected     map[string]bool
	activeTab    int
	cursor       int
	confirmed    bool
	width        int
	height       int
	scrollOffset int
}

func NewSelector(presetName string) SelectorModel {
	return SelectorModel{
		categories: config.Categories,
		selected:   config.GetPackagesForPreset(presetName),
		activeTab:  0,
		cursor:     0,
	}
}

func (m SelectorModel) Init() tea.Cmd {
	return nil
}

func (m SelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, keys.Tab), key.Matches(msg, keys.Right):
			m.activeTab = (m.activeTab + 1) % len(m.categories)
			m.cursor = 0
			m.scrollOffset = 0

		case key.Matches(msg, keys.ShiftTab), key.Matches(msg, keys.Left):
			m.activeTab = (m.activeTab - 1 + len(m.categories)) % len(m.categories)
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
			m.confirmed = true
			return m, tea.Quit

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

func (m SelectorModel) View() string {
	var lines []string

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
		lines = append(lines, line)
	}

	clearLine := strings.Repeat(" ", 80)
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
	lines = append(lines, helpStyle.Render("Tab/←→: switch category • ↑↓: navigate • Space: toggle • a: select all • Enter: confirm • q: quit"))

	return strings.Join(lines, "\n")
}

func (m SelectorModel) Selected() map[string]bool {
	return m.selected
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

func RunSelector(presetName string) (map[string]bool, bool, error) {
	model := NewSelector(presetName)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, false, err
	}

	m := finalModel.(SelectorModel)
	return m.Selected(), m.Confirmed(), nil
}
