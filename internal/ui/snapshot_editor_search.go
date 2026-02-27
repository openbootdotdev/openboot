package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/search"
)

// Message types for editor online search
type editorOnlineSearchResultMsg struct {
	results []config.Package
	query   string
	err     error
}

type editorOnlineSearchTickMsg struct{}

type editorSearchSpinnerTickMsg struct{}

type editorToastClearMsg struct{}

const editorOnlineSearchDebounce = 500 * time.Millisecond
const editorToastDuration = 1500 * time.Millisecond

// Commands
func editorSearchOnlineCmd(query string) tea.Cmd {
	return func() tea.Msg {
		results, err := search.SearchOnline(query)
		return editorOnlineSearchResultMsg{results: results, query: query, err: err}
	}
}

func editorOnlineSearchDebounceCmd() tea.Cmd {
	return tea.Tick(editorOnlineSearchDebounce, func(time.Time) tea.Msg {
		return editorOnlineSearchTickMsg{}
	})
}

func editorSearchSpinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return editorSearchSpinnerTickMsg{}
	})
}

func editorToastClearCmd() tea.Cmd {
	return tea.Tick(editorToastDuration, func(time.Time) tea.Msg {
		return editorToastClearMsg{}
	})
}

// packageToEditorItem converts a config.Package from online search to an editorItem.
func packageToEditorItem(pkg config.Package) editorItem {
	itemType := editorItemFormula
	if pkg.IsCask {
		itemType = editorItemCask
	} else if pkg.IsNpm {
		itemType = editorItemNpm
	}
	return editorItem{
		name:        pkg.Name,
		description: pkg.Description,
		selected:    false,
		itemType:    itemType,
		isAdded:     true,
	}
}

// tabIndexForItemType returns the tab index for a given item type.
func tabIndexForItemType(t editorItemType) int {
	switch t {
	case editorItemFormula:
		return 0
	case editorItemCask:
		return 1
	case editorItemNpm:
		return 2
	case editorItemTap:
		return 3
	case editorItemMacOSPref:
		return 4
	default:
		return 0
	}
}

// tabNameForItemType returns a display label for a given item type.
func tabNameForItemType(t editorItemType) string {
	switch t {
	case editorItemFormula:
		return "Formulae"
	case editorItemCask:
		return "Casks"
	case editorItemNpm:
		return "NPM"
	case editorItemTap:
		return "Taps"
	case editorItemMacOSPref:
		return "macOS Prefs"
	default:
		return "Unknown"
	}
}

// withOnlineResultAdded adds an online search result to the appropriate tab.
// Returns the updated model and toast message. If the item already exists, toast is empty.
func (m SnapshotEditorModel) withOnlineResultAdded(item editorItem) (SnapshotEditorModel, string) {
	tabIdx := tabIndexForItemType(item.itemType)

	// Check for duplicates
	for _, existing := range m.tabs[tabIdx].items {
		if existing.name == item.name {
			return m, ""
		}
	}

	item.selected = true
	m.tabs[tabIdx].items = append(m.tabs[tabIdx].items, item)
	return m, fmt.Sprintf("+ Added %s to %s", item.name, tabNameForItemType(item.itemType))
}

// totalSearchItems returns the total count of local filtered + online results.
func (m SnapshotEditorModel) totalSearchItems() int {
	return len(m.filteredItems) + len(m.onlineResults)
}
