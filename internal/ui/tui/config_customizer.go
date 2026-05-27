package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/config"
)

type customizerItem struct {
	name        string
	description string
	selected    bool
}

type customizerTab struct {
	name  string
	icon  string
	items []customizerItem
}

// ConfigCustomizerModel is a bubbletea Model that lets the user toggle which
// packages from a RemoteConfig should be installed. Three tabs: Formulae,
// Casks, NPM. All items are preselected on entry.
type ConfigCustomizerModel struct {
	tabs         []customizerTab
	activeTab    int
	cursor       int
	scrollOffset int
	width        int
	height       int
	confirmed    bool
}

// NewConfigCustomizer builds the model from a RemoteConfig.
func NewConfigCustomizer(rc *config.RemoteConfig) ConfigCustomizerModel {
	formulae := make([]customizerItem, len(rc.Packages))
	for i, e := range rc.Packages {
		formulae[i] = customizerItem{name: e.Name, description: e.Desc, selected: true}
	}
	casks := make([]customizerItem, len(rc.Casks))
	for i, e := range rc.Casks {
		casks[i] = customizerItem{name: e.Name, description: e.Desc, selected: true}
	}
	npm := make([]customizerItem, len(rc.Npm))
	for i, e := range rc.Npm {
		npm[i] = customizerItem{name: e.Name, description: e.Desc, selected: true}
	}
	return ConfigCustomizerModel{
		tabs: []customizerTab{
			{name: "Formulae", icon: "🍺", items: formulae},
			{name: "Casks", icon: "📦", items: casks},
			{name: "NPM", icon: "📜", items: npm},
		},
	}
}

func (m ConfigCustomizerModel) Init() tea.Cmd { return nil }

func (m ConfigCustomizerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

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
			if m.cursor < len(m.tabs[m.activeTab].items)-1 {
				m.cursor++
				visible := m.getVisibleItems()
				if m.cursor >= m.scrollOffset+visible {
					m.scrollOffset = m.cursor - visible + 1
				}
			}

		case key.Matches(msg, keys.Space):
			tab := &m.tabs[m.activeTab]
			if m.cursor < len(tab.items) {
				tab.items[m.cursor].selected = !tab.items[m.cursor].selected
			}

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

		case key.Matches(msg, keys.Enter):
			m.confirmed = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ConfigCustomizerModel) View() string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, activeTabStyle.Render("📋 Customize packages"))
	lines = append(lines, "")

	// Tab bar
	var tabParts []string
	for i, tab := range m.tabs {
		selectedCount := 0
		for _, item := range tab.items {
			if item.selected {
				selectedCount++
			}
		}
		label := fmt.Sprintf("%s %s (%d/%d)", tab.icon, tab.name, selectedCount, len(tab.items))
		if i == m.activeTab {
			tabParts = append(tabParts, activeTabStyle.Render(label))
		} else {
			tabParts = append(tabParts, itemStyle.Render(label))
		}
	}
	lines = append(lines, strings.Join(tabParts, "  │  "))
	lines = append(lines, "")

	// Items
	tab := m.tabs[m.activeTab]
	visible := m.getVisibleItems()
	if len(tab.items) == 0 {
		lines = append(lines, descStyle.Render("  (no items)"))
	} else {
		end := m.scrollOffset + visible
		if end > len(tab.items) {
			end = len(tab.items)
		}
		for i := m.scrollOffset; i < end; i++ {
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
			lines = append(lines, padLine(truncateLine(line, m.width), m.width))
		}
	}

	lines = append(lines, "")
	lines = append(lines, countStyle.Render(m.summary()))
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("Space: toggle • a: all in tab • Tab/←→: switch • Enter: confirm • q: cancel"))
	return padAllLines(strings.Join(lines, "\n"), m.width)
}

func (m ConfigCustomizerModel) getVisibleItems() int {
	if m.height == 0 {
		return 15
	}
	available := m.height - 10
	if available < 5 {
		available = 5
	}
	if available > 20 {
		available = 20
	}
	return available
}

func (m ConfigCustomizerModel) summary() string {
	var parts []string
	for _, tab := range m.tabs {
		count := 0
		for _, item := range tab.items {
			if item.selected {
				count++
			}
		}
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, strings.ToLower(tab.name)))
		}
	}
	if len(parts) == 0 {
		return "nothing selected"
	}
	return strings.Join(parts, ", ") + " selected"
}

// Picks returns the set of selected item names across all tabs.
func (m ConfigCustomizerModel) Picks() map[string]bool {
	out := map[string]bool{}
	for _, tab := range m.tabs {
		for _, item := range tab.items {
			if item.selected {
				out[item.name] = true
			}
		}
	}
	return out
}

// Confirmed returns true if the user pressed Enter to confirm.
func (m ConfigCustomizerModel) Confirmed() bool { return m.confirmed }

// RunConfigCustomizer launches the customizer TUI and returns the user's
// picks and whether they confirmed. If they cancelled (q / Ctrl-C), picks
// is nil and confirmed is false.
func RunConfigCustomizer(rc *config.RemoteConfig) (picks map[string]bool, confirmed bool, err error) {
	p := tea.NewProgram(NewConfigCustomizer(rc), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, false, fmt.Errorf("run customizer: %w", err)
	}
	m := finalModel.(ConfigCustomizerModel)
	if !m.Confirmed() {
		return nil, false, nil
	}
	return m.Picks(), true, nil
}
