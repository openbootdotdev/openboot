package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/config"
)

// ── selection helpers ──

func (m Model) isInstalled(name string) bool { return m.installed[name] }

func (m *Model) toggle(name string) { m.selected[name] = !m.selected[name] }

// selCount is the number of packages the user has selected.
func (m Model) selCount() int {
	n := 0
	for _, v := range m.selected {
		if v {
			n++
		}
	}
	return n
}

// toInstallCount is the number of selected packages not already installed.
func (m Model) toInstallCount() int {
	n := 0
	for name, v := range m.selected {
		if v && !m.installed[name] {
			n++
		}
	}
	return n
}

// skippedCount is the number of selected packages already present.
func (m Model) skippedCount() int { return m.selCount() - m.toInstallCount() }

func (m Model) estMin() int { return estMinutes(m.toInstallCount()) }

// pool returns the packages shown in the right pane: the active category, or —
// when a query is present — a substring match across the whole catalog.
func (m Model) pool() []config.Package {
	q := strings.TrimSpace(strings.ToLower(m.query))
	if q != "" {
		var out []config.Package
		for _, c := range m.cats {
			for _, p := range c.Packages {
				if strings.Contains(strings.ToLower(p.Name+" "+p.Description), q) {
					out = append(out, p)
				}
			}
		}
		return out
	}
	if m.catCur >= 0 && m.catCur < len(m.cats) {
		return m.cats[m.catCur].Packages
	}
	return nil
}

// selVisible is the number of package rows the list area can show. It must match
// selectList's own count (body height − search row − blank) exactly, or the
// scroll clamp and the render disagree: leaving blank rows at the bottom and
// rows the keyboard cursor can never reach. body height = m.height − title −
// status = m.height − 2; the list then spends 2 rows on the search + blank.
func (m Model) selVisible() int {
	v := m.height - 4
	if v < 3 {
		v = 3
	}
	return v
}

func (m Model) clampSelScroll() Model {
	visible := m.selVisible()
	if m.rowCur < m.scroll {
		m.scroll = m.rowCur
	}
	if m.rowCur >= m.scroll+visible {
		m.scroll = m.rowCur - visible + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	return m
}

// ── key handling ──

func (m Model) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocyclo // a keyboard dispatch table; splitting obscures the flow
	pool := m.pool()
	last := len(pool) - 1
	s := msg.String()

	if m.typing {
		switch s {
		case "esc":
			m.query, m.typing, m.rowCur, m.scroll = "", false, 0, 0
		case "enter":
			if strings.TrimSpace(m.query) != "" && len(pool) > 0 {
				p := pool[clamp(m.rowCur, 0, last)]
				if !m.isInstalled(p.Name) {
					m.toggle(p.Name)
				}
				m.query, m.typing, m.rowCur, m.scroll = "", false, 0, 0
			} else {
				return m.tryInstall()
			}
		case "backspace":
			if len(m.query) > 0 {
				m.query = trimLast(m.query)
				m.rowCur, m.scroll = 0, 0
			}
		case "up":
			if m.rowCur > 0 {
				m.rowCur--
			}
		case "down":
			if m.rowCur < last {
				m.rowCur++
			}
		default:
			// KeyRunes covers both single keystrokes and multi-rune input
			// (bubbletea coalesces pasted/fast text into one message).
			if msg.Type == tea.KeyRunes || s == " " {
				m.query += s
				m.rowCur, m.scroll = 0, 0
			}
		}
		return m.clampSelScroll(), nil
	}

	switch s {
	case "q":
		m.quit = true
		return m, tea.Quit
	case "/":
		// Filtering searches across categories, so it's a list operation —
		// pull focus to the list so ↑↓ behaves predictably after it clears.
		m.typing, m.query, m.rowCur, m.scroll, m.selFocus = true, "", 0, 0, focusList
	case "esc":
		m.query, m.rowCur, m.scroll = "", 0, 0
	case "left", "h":
		m.selFocus = focusCats
	case "right", "l":
		m.selFocus = focusList
	case "tab", "shift+tab":
		if m.selFocus == focusCats {
			m.selFocus = focusList
		} else {
			m.selFocus = focusCats
		}
	case "up", "k":
		m = m.selMoveUp()
	case "down", "j":
		m = m.selMoveDown()
	case " ":
		if len(pool) > 0 {
			p := pool[clamp(m.rowCur, 0, last)]
			if !m.isInstalled(p.Name) {
				m.toggle(p.Name)
			}
		}
	case "a":
		for _, p := range pool {
			if !m.isInstalled(p.Name) {
				m.selected[p.Name] = true
			}
		}
	case "x":
		m.selected = map[string]bool{}
	case "enter":
		return m.tryInstall()
	}
	return m.clampSelScroll(), nil
}

// selMoveUp/selMoveDown move the cursor within the focused pane: the category
// sidebar when it has focus (right pane live-previews the new category), the
// package list otherwise.
func (m Model) selMoveUp() Model {
	if m.selFocus == focusCats && m.query == "" {
		if m.catCur > 0 {
			m.catCur--
			m.rowCur, m.scroll = 0, 0
		}
		return m
	}
	if m.rowCur > 0 {
		m.rowCur--
	}
	return m
}

func (m Model) selMoveDown() Model {
	if m.selFocus == focusCats && m.query == "" {
		if m.catCur < len(m.cats)-1 {
			m.catCur++
			m.rowCur, m.scroll = 0, 0
		}
		return m
	}
	if m.rowCur < len(m.pool())-1 {
		m.rowCur++
	}
	return m
}

// ── mouse ──

type selHit int

const (
	hitNone selHit = iota
	hitCat
	hitPkg
)

// updateSelectMouse handles mouse events on the select screen: click a
// category to switch to it, click a package to toggle it, wheel to scroll the
// list, and hover to highlight the row under the cursor (no click needed).
func (m Model) updateSelectMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Hover tracking — fires on every mouse move even without a button pressed
	// (WithMouseAllMotion). Only highlight package rows, not categories or chrome.
	if msg.Action == tea.MouseActionMotion {
		kind, idx := m.selectHitTest(msg.X, msg.Y)
		if kind == hitPkg {
			m.hoverRow = idx
		} else {
			m.hoverRow = -1
		}
		return m.clampSelScroll(), nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.selFocus = focusList
		if m.rowCur > 0 {
			m.rowCur--
		}
	case tea.MouseButtonWheelDown:
		m.selFocus = focusList
		if m.rowCur < len(m.pool())-1 {
			m.rowCur++
		}
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		switch kind, idx := m.selectHitTest(msg.X, msg.Y); kind {
		case hitCat:
			m.catCur, m.query, m.typing = idx, "", false
			m.selFocus = focusCats
			m.rowCur, m.scroll = 0, 0
		case hitPkg:
			m.rowCur = idx
			m.selFocus = focusList
			pool := m.pool()
			if idx < len(pool) && !m.isInstalled(pool[idx].Name) {
				m.toggle(pool[idx].Name)
			}
		case hitNone:
			// click landed on chrome or blank space — nothing to do
		}
	default:
		// other buttons (middle, right, drag motion) aren't actionable here
		return m, nil
	}
	return m.clampSelScroll(), nil
}

// selectHitTest maps a screen coordinate to a category or package row. The
// geometry mirrors View + selectBody exactly: screen row 0 is the title bar so
// body row = y-1; the sidebar lists categories from body row 3, and the package
// list starts at body row 2 offset by the scroll. Kept a pure, testable
// function so the mapping is verified rather than eyeballed.
func (m Model) selectHitTest(x, y int) (selHit, int) {
	if !m.inBody(y) {
		return hitNone, -1
	}
	bodyRow := y - 1
	if x < sidebarW {
		if ci := bodyRow - 3; ci >= 0 && ci < len(m.cats) {
			return hitCat, ci
		}
		return hitNone, -1
	}
	if x == sidebarW {
		return hitNone, -1 // the vertical divider column — neither pane
	}
	if bodyRow < 2 {
		return hitNone, -1
	}
	pool := m.pool()
	if pj := m.scroll + bodyRow - 2; pj >= 0 && pj < len(pool) {
		return hitPkg, pj
	}
	return hitNone, -1
}

func (m Model) tryInstall() (tea.Model, tea.Cmd) {
	// Gate on a selection, not on new packages: an all-installed loadout still
	// carries git/shell/dotfiles/macOS steps worth reviewing and applying, so
	// zero *new* packages must not trap the user on the select screen.
	if m.selCount() == 0 {
		return m, nil // nothing selected — stay put
	}
	// Capture a git identity first when none is configured (fresh Mac), then
	// review the full plan before anything runs.
	if need, name, email := m.needsGitCapture(); need {
		m.gitName, m.gitEmail, m.gitField = name, email, 0
		m.screen, m.hoverRow = scrGit, -1
		return m, nil
	}
	return m.enterConfirm()
}

// ── rendering ──

const sidebarW = 22

func (m Model) selectBody(w, h int) string {
	left := m.selectSidebar(h)
	right := m.selectList(w-sidebarW-1, h)

	var lines []string
	for i := 0; i < h; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines = append(lines, padTo(truncCell(l, sidebarW), sidebarW)+fg(cBorder).Render("│")+r)
	}
	return strings.Join(lines, "\n")
}

func (m Model) selectSidebar(h int) []string {
	rows := make([]string, 0, h)
	rows = append(rows, "")
	rows = append(rows, " "+fg(cDim4).Render("CATALOG"))
	rows = append(rows, "")

	q := strings.TrimSpace(m.query)
	for i, c := range m.cats {
		selN, total := 0, len(c.Packages)
		for _, p := range c.Packages {
			if m.selected[p.Name] && !m.installed[p.Name] {
				selN++
			}
		}
		active := i == m.catCur && q == ""
		edge := " "
		nameStyle := fg(cDim)
		countStyle := fg(cDim4)
		if active {
			// A pointer arrow when the sidebar holds focus vs a plain bar when
			// it doesn't — a structural cue, so the focused pane reads without
			// relying on colour (piped output, colourblind users).
			if m.selFocus == focusCats {
				edge = fg(cAccent).Render("▸")
				nameStyle = fg(cTextHi)
			} else {
				edge = fg(cDim2).Render("▎")
				nameStyle = fg(cText)
			}
		}
		if selN > 0 {
			countStyle = fg(cAccentHi)
		}
		count := fmt.Sprintf("%d", total)
		if selN > 0 {
			count = fmt.Sprintf("%d/%d", selN, total)
		}
		name := nameStyle.Render(strings.ToLower(c.Name))
		rows = append(rows, bar(edge+" "+name, countStyle.Render(count)+" ", sidebarW))
	}

	// Footer pinned to the bottom of the sidebar.
	footer := []string{
		truncCell(fg(cAccent).Bold(true).Render(fmt.Sprintf("%d", m.selCount()))+
			fg(cDim).Render(fmt.Sprintf(" selected · ~%d min", m.estMin())), sidebarW),
	}
	if sk := m.skippedCount(); sk > 0 {
		footer = append(footer, truncCell(fg(cDim4).Render(fmt.Sprintf("%d already installed", sk)), sidebarW))
	}
	for len(rows) < h-len(footer)-1 {
		rows = append(rows, "")
	}
	rows = append(rows, "")
	rows = append(rows, footer...)
	return rows
}

func (m Model) selectList(w, h int) []string {
	rows := make([]string, 0, h)

	// Search / filter row.
	icon := fg(cDim3).Render("/")
	if strings.TrimSpace(m.query) != "" {
		icon = fg(cWarn).Bold(true).Render("/")
	}
	pool := m.pool()
	queryText := m.query
	if m.typing {
		queryText += "▌"
	}
	if queryText == "" {
		queryText = fg(cDim4).Render("type to filter — enter toggles the top hit")
	} else {
		queryText = fg(cTextHi).Render(queryText)
	}
	match := ""
	if strings.TrimSpace(m.query) != "" {
		match = fg(cDim3).Render(fmt.Sprintf("%d hits", len(pool)))
	}
	rows = append(rows, bar("  "+icon+"  "+queryText, match+" ", w))
	rows = append(rows, "")

	// Package rows.
	visible := h - len(rows)
	m2 := m.clampSelScroll()
	start := m2.scroll
	end := start + visible
	if end > len(pool) {
		end = len(pool)
	}
	for i := start; i < end; i++ {
		line := m.renderRow(pool[i], i == m2.rowCur, w)
		// Subtle background highlight on the row the mouse is hovering over, so the
		// user sees where a click would land — including when it's also the
		// keyboard cursor, so the "clickable" affordance is never dropped.
		if i == m.hoverRow {
			line = hoverBg(line)
		}
		rows = append(rows, line)
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return rows
}

func (m Model) renderRow(p config.Package, cursor bool, w int) string {
	installed := m.installed[p.Name]
	on := m.selected[p.Name]

	box := fg(cDim3).Render("◯")
	switch {
	case installed:
		box = fg(cInstalled).Render("✓")
	case on:
		box = fg(cAccent).Render("◉")
	}

	listFocused := m.selFocus == focusList
	nameStyle := fg(cMuted)
	switch {
	case installed:
		nameStyle = fg(cDim2)
	case cursor && listFocused:
		nameStyle = fg(cWhite).Bold(true)
	case cursor, on:
		nameStyle = fg(cText)
	}

	tail := fg(cDim3).Render(pkgType(p))
	if installed {
		tail = fg(cInstalled).Render("installed")
	}

	name := padTo(nameStyle.Render(p.Name), 20)
	rowPrefix := "  "
	if cursor && listFocused {
		// The cursor arrow shows only when the list holds focus, so exactly one
		// pane displays a pointer at a time — the focus cue, without colour.
		rowPrefix = fg(cAccent).Render("› ")
	}
	left := rowPrefix + box + "  " + name + " " + fg(cDim).Render(p.Description)
	return bar(truncCell(left, w-12), tail+" ", w)
}

func pkgType(p config.Package) string {
	switch {
	case p.IsNpm:
		return "npm"
	case p.IsCask:
		return "cask"
	default:
		return "brew"
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
