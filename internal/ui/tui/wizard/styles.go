package wizard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette.
//
// Rule: never hardcode a colour whose job is to contrast with the background.
// We don't know the background — the user picks it, and it may be translucent
// over a wallpaper, which lifts the effective background far above whatever a
// design mock assumed.
//
// The Redesign v5 tokens were a 10-step hex grey ramp mocked against #08080a.
// Measured against a translucent terminal (effective bg ≈ #2b4247), the bottom
// half of that ramp collapsed into the background — #3f3f46 landed at 1.0:1,
// i.e. the exact luminance of the background, which no terminal can render as
// visible text. Pending pipeline rows, sidebar counts and key hints simply
// disappeared. (v0.63's own greys — #444/#555/#666 — measure 1.1/1.4/1.9:1
// there and fail the same way; it only looked better because it used the
// terminal's default foreground for most text.)
//
// So the text ramp is now the terminal's own ANSI palette, which the user's
// theme guarantees is legible against the background they chose:
//
//	15 (bright white) — emphasis: cursor row, active phase, headings
//	 7 (white)        — normal body text and anything load-bearing
//	 8 (bright black) — decorative only: rules, placeholders, spent progress
//
// State is carried by glyph + accent hue (○ / spinner / ✓), never by fading a
// label toward the background — that reads as "missing", not as "pending".
//
// The brand/status hues stay hex: they're bright enough to hold up anywhere
// (measured 4.4–6.1:1 on the same translucent terminal) and they're the
// product's identity, shared with the internal/ui palette.
var (
	cAccent   = lipgloss.Color("#22c55e") // primary green
	cAccentHi = lipgloss.Color("#4ade80") // bright green — times, links
	cInfo     = lipgloss.Color("#06b6d4") // cyan — probing / active spinner
	cWarn     = lipgloss.Color("#f59e0b") // amber — active search

	// Text ramp — terminal-relative. Names kept so call sites read the same.
	cWhite  = lipgloss.Color("15") // cursor row name
	cTextHi = lipgloss.Color("15") // emphasized body text
	cText   = lipgloss.Color("7")  // body text
	cMuted  = lipgloss.Color("7")  // secondary text
	cMuted2 = lipgloss.Color("7")  // finished phase names
	cMuted3 = lipgloss.Color("7")  // status-bar info — teaches keys, must read
	cDim    = lipgloss.Color("7")  // package descriptions — chosen from, must read
	cDim2   = lipgloss.Color("7")  // status-bar key hints — must read
	cDim3   = lipgloss.Color("8")  // version, crumb, unselected box
	cDim4   = lipgloss.Color("8")  // pane headings, counts, placeholders
	cFaint  = lipgloss.Color("8")  // spent progress cells, pending glyph

	cInstalled = lipgloss.Color("2") // green — already-installed rows
	cBorder    = lipgloss.Color("8") // panel dividers
)

// fg returns a foreground-only style for c.
func fg(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

// hoverBg marks the row under the mouse pointer using reverse video (SGR 7)
// rather than a painted background colour. Any specific background we picked
// would be a guess about the user's — the design's #3d3d4a sat at 1.2:1
// against a translucent terminal, so the "highlight" was a no-op there.
// Reverse swaps whatever fg/bg the row already has, so it is guaranteed
// visible in every theme, at every colour depth, including monochrome.
//
// lipgloss ends each styled span with a FULL reset (\e[0m), which also clears
// the reverse attribute — so re-establish it after every reset, otherwise only
// the first span would be marked.
func hoverBg(s string) string {
	const rev = "\x1b[7m"
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+rev)
	return rev + s + "\x1b[0m"
}

// spinnerFrames matches the braille spinner used across the codebase and the design.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}
