package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// Shared primitives for the remaining snapshot editor and remote-config
// customizer. These used to live in selector.go/selector_view.go even though
// both independent models consumed them, which made deleting the legacy
// install selector accidentally delete their UI foundation too.

var searchSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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

var (
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

	onlineHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f59e0b")).
				Bold(true)

	onlineSearchingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888")).
				Italic(true)
)

func truncateLine(line string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(line) <= maxWidth {
		return line
	}
	if maxWidth < 10 {
		return lipgloss.NewStyle().MaxWidth(maxWidth).Render(line)
	}
	return lipgloss.NewStyle().MaxWidth(maxWidth-3).Render(line) + "..."
}

func padLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	visualWidth := lipgloss.Width(line)
	if visualWidth >= width {
		return line
	}
	return line + strings.Repeat(" ", width-visualWidth)
}

func padAllLines(s string, width int) string {
	if width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = padLine(line, width)
	}
	return strings.Join(lines, "\n")
}
