package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/search"
	"github.com/sahilm/fuzzy"
)

var searchSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type onlineSearchResultMsg struct {
	Results []config.Package
	Query   string
	Err     error
}

type onlineSearchTickMsg struct{}

type searchSpinnerTickMsg struct{}

func searchSpinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return searchSpinnerTickMsg{}
	})
}

type toastClearMsg struct{}

const onlineSearchDebounce = 500 * time.Millisecond
const toastDuration = 1500 * time.Millisecond

func toastClearCmd() tea.Cmd {
	return tea.Tick(toastDuration, func(time.Time) tea.Msg {
		return toastClearMsg{}
	})
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
	toastMessage          string
	toastTime             time.Time
	toastIsAdd            bool
	searchSpinnerIdx      int
}

func NewSelector(presetName string) SelectorModel {
	return SelectorModel{
		categories:      config.GetCategories(),
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

	case toastClearMsg:
		m.toastMessage = ""
		return m, nil

	case searchSpinnerTickMsg:
		if m.searchMode && m.onlineSearching {
			m.searchSpinnerIdx = (m.searchSpinnerIdx + 1) % len(searchSpinnerFrames)
			return m, searchSpinnerTickCmd()
		}
		return m, nil

	case onlineSearchTickMsg:
		if m.onlineDebouncePending && m.searchQuery != "" && m.searchQuery == m.onlineSearchQuery {
			m.onlineDebouncePending = false
			m.onlineSearching = true
			m.searchSpinnerIdx = 0
			return m, tea.Batch(searchOnlineCmd(m.searchQuery), searchSpinnerTickCmd())
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
					if m.selected[pkg.Name] {
						m.toastMessage = "+ Added " + pkg.Name
						m.toastIsAdd = true
					} else {
						m.toastMessage = "- Removed " + pkg.Name
						m.toastIsAdd = false
					}
					m.toastTime = time.Now()
					return m, toastClearCmd()
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
					m.scrollOffset = 0
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
				if m.selected[pkg.Name] {
					m.toastMessage = "+ Added " + pkg.Name
					m.toastIsAdd = true
				} else {
					m.toastMessage = "- Removed " + pkg.Name
					m.toastIsAdd = false
				}
				m.toastTime = time.Now()
				return m, toastClearCmd()
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
			if !allSelected {
				m.toastMessage = fmt.Sprintf("✔ Selected all %d %s", len(cat.Packages), cat.Name)
				m.toastIsAdd = true
			} else {
				m.toastMessage = fmt.Sprintf("○ Deselected all %s", cat.Name)
				m.toastIsAdd = false
			}
			m.toastTime = time.Now()
			return m, toastClearCmd()
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
