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

// selVisible is the number of package rows the list area can show.
func (m Model) selVisible() int {
	v := m.height - 6 // chrome (2) + search row + blank + slack
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
				m.query = m.query[:len(m.query)-1]
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
		m.typing, m.query, m.rowCur, m.scroll = true, "", 0, 0
	case "esc":
		m.query, m.rowCur, m.scroll = "", 0, 0
	case "up", "k":
		if m.rowCur > 0 {
			m.rowCur--
		}
	case "down", "j":
		if m.rowCur < last {
			m.rowCur++
		}
	case "tab", "right":
		if m.query == "" && len(m.cats) > 0 {
			m.catCur = (m.catCur + 1) % len(m.cats)
			m.rowCur, m.scroll = 0, 0
		}
	case "shift+tab", "left":
		if m.query == "" && len(m.cats) > 0 {
			m.catCur = (m.catCur - 1 + len(m.cats)) % len(m.cats)
			m.rowCur, m.scroll = 0, 0
		}
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

func (m Model) tryInstall() (tea.Model, tea.Cmd) {
	if m.toInstallCount() == 0 {
		return m, nil // nothing to install — stay put
	}
	// Capture a git identity first when none is configured (fresh Mac).
	if need, name, email := m.needsGitCapture(); need {
		m.gitName, m.gitEmail, m.gitField = name, email, 0
		m.screen = scrGit
		return m, nil
	}
	return m.startInstall()
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
			edge = fg(cAccent).Render("▎")
			nameStyle = fg(cTextHi)
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
		rows = append(rows, m.renderRow(pool[i], i == m2.rowCur, w))
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

	nameStyle := fg(cMuted)
	switch {
	case installed:
		nameStyle = fg(cDim2)
	case cursor:
		nameStyle = fg(cWhite).Bold(true)
	case on:
		nameStyle = fg(cText)
	}

	tail := fg(cDim3).Render(pkgType(p))
	if installed {
		tail = fg(cInstalled).Render("installed")
	}

	name := padTo(nameStyle.Render(p.Name), 20)
	rowPrefix := "  "
	if cursor {
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
