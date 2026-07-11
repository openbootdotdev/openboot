package wizard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette — mirrors the OpenBoot TUI Redesign v5 design tokens. The two anchor
// colors (accent green #22c55e, info cyan #06b6d4) already match the existing
// internal/ui palette; the rest are the grey ramp and status hues the design
// leans on for depth.
var (
	cAccent   = lipgloss.Color("#22c55e") // primary green
	cAccentHi = lipgloss.Color("#4ade80") // bright green — times, links
	cInfo     = lipgloss.Color("#06b6d4") // cyan — probing / active spinner
	cWarn     = lipgloss.Color("#f59e0b") // amber — installing / active search
	cDanger   = lipgloss.Color("#ef4444") // red — failures

	cWhite  = lipgloss.Color("#ffffff") // cursor row name
	cTextHi = lipgloss.Color("#e4e4e7") // emphasized body text
	cText   = lipgloss.Color("#d4d4d8") // body text
	cMuted  = lipgloss.Color("#a1a1aa") // secondary text
	cMuted2 = lipgloss.Color("#8a8a92")
	cMuted3 = lipgloss.Color("#71717a")
	cDim    = lipgloss.Color("#63636c")
	cDim2   = lipgloss.Color("#52525b")
	cDim3   = lipgloss.Color("#3f3f46")
	cDim4   = lipgloss.Color("#3a3a41")
	cFaint  = lipgloss.Color("#2e2e34")

	cInstalled = lipgloss.Color("#3f6b4a") // dim green — already-installed rows
	cBorder    = lipgloss.Color("#2b2b33") // panel dividers (brighter than the
	//                                         design's #1c1c22 so it reads in a terminal)
)

// fg returns a foreground-only style for c.
func fg(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

// hoverBg paints a hard ANSI true-color background across the whole row.
// lipgloss ends every styled span with a FULL reset (\e[0m), which also clears
// the background — so re-establish the background after each reset, otherwise
// only the first span would be highlighted. cHover = #3d3d4a → rgb(61,61,74).
func hoverBg(s string) string {
	const bg = "\x1b[48;2;61;61;74m"
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+bg)
	return bg + s + "\x1b[0m"
}

// spinnerFrames matches the braille spinner used across the codebase and the design.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}
