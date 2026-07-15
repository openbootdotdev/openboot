package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The confirm screen is the last stop before the engine runs: it shows exactly
// what this run will do — packages, git identity, and the three system-config
// steps as toggleable rows (default on, preserving the design's defaults-on
// spirit) — so nothing mutates the system without having been on screen.

// confirmRow identifies one toggleable system-config row.
type confirmRow int

const (
	rowShell confirmRow = iota
	rowDotfiles
	rowPrefs
)

// confirmRows returns the toggleable rows present for the previewed plan.
func (m Model) confirmRows() []confirmRow {
	var rows []confirmRow
	if m.preview.InstallOhMyZsh {
		rows = append(rows, rowShell)
	}
	if m.preview.DotfilesURL != "" {
		rows = append(rows, rowDotfiles)
	}
	if len(m.preview.MacOSPrefs) > 0 {
		rows = append(rows, rowPrefs)
	}
	return rows
}

// enterConfirm computes the plan preview and shows the confirm screen.
func (m Model) enterConfirm() (tea.Model, tea.Cmd) {
	m.preview = m.buildPlan()
	m.confShell, m.confDotfiles, m.confPrefs = true, true, true
	m.confCur, m.hoverRow = 0, -1
	m.screen = scrConfirm
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.confirmRows()
	switch msg.String() {
	case "esc":
		m.screen = scrSelect
	case "up", "k":
		if m.confCur > 0 {
			m.confCur--
		}
	case "down", "j":
		if m.confCur < len(rows)-1 {
			m.confCur++
		}
	case " ":
		if len(rows) > 0 {
			switch rows[clamp(m.confCur, 0, len(rows)-1)] {
			case rowShell:
				m.confShell = !m.confShell
			case rowDotfiles:
				m.confDotfiles = !m.confDotfiles
			case rowPrefs:
				m.confPrefs = !m.confPrefs
			}
		}
	case "q":
		m.quit = true
		return m, tea.Quit
	case "enter":
		return m.startInstall()
	}
	return m, nil
}

func (m Model) confirmBody(_, _ int) string {
	const pad = "   "
	var b []string
	b = append(b, "")
	b = append(b, "")
	b = append(b, pad+fg(cTextHi).Bold(true).Render("Ready to install"))
	b = append(b, pad+fg(cDim3).Render("Everything below runs when you press ↵ — space toggles a step off."))
	b = append(b, "")

	// Packages summary (informational, not toggleable).
	toInstall := m.toInstallCount()
	skipped := m.selCount() - toInstall
	pkgLine := fg(cAccentHi).Render(fmt.Sprintf("%d to install", toInstall)) +
		fg(cDim).Render(fmt.Sprintf(" · ~%d min", m.estMin()))
	if skipped > 0 {
		pkgLine += fg(cDim3).Render(fmt.Sprintf(" · %d already present", skipped))
	}
	b = append(b, pad+"  "+fg(cDim2).Render(padTo("packages", 13))+pkgLine)

	// Git identity (informational).
	gitVal := m.gitName + " <" + m.gitEmail + ">"
	if strings.TrimSpace(m.gitName) == "" {
		switch {
		case m.preview.SkipGit:
			gitVal = "not configured — skipped"
		case strings.TrimSpace(m.preview.GitName) == "":
			// Config-mode plans carry no identity: the git step keeps an
			// existing config and no-ops when there is none.
			gitVal = "existing config kept — skipped when absent"
		default:
			gitVal = m.preview.GitName + " <" + m.preview.GitEmail + ">"
		}
	}
	b = append(b, pad+"  "+fg(cDim2).Render(padTo("git", 13))+fg(cMuted).Render(gitVal))
	// Post-install (informational): the script can't run inside the
	// alt-screen, so it executes after the wizard, with its own confirm.
	if n := len(m.preview.PostInstall); n > 0 {
		b = append(b, pad+"  "+fg(cDim2).Render(padTo("post-install", 13))+
			fg(cMuted).Render(fmt.Sprintf("%d command(s) · runs after the wizard, asks first", n)))
	}
	b = append(b, "")

	rows := m.confirmRows()
	for i, r := range rows {
		line := m.renderConfirmRow(r, i == clamp(m.confCur, 0, len(rows)-1))
		if i == m.hoverRow {
			line = hoverBg(line)
		}
		b = append(b, pad+line)
	}
	if len(rows) > 0 {
		b = append(b, "")
	}
	b = append(b, pad+fg(cDim3).Render("↑↓ move · space toggle · ↵ install · esc back"))
	return strings.Join(b, "\n")
}

func (m Model) renderConfirmRow(r confirmRow, cursor bool) string {
	var on bool
	var name, desc string
	switch r {
	case rowShell:
		on, name = m.confShell, "oh-my-zsh"
		desc = "zsh setup, theme & plugins"
	case rowDotfiles:
		on, name = m.confDotfiles, "dotfiles"
		desc = m.preview.DotfilesURL + " → symlinked (backups kept)"
	case rowPrefs:
		on, name = m.confPrefs, "macOS prefs"
		desc = fmt.Sprintf("%d preferences · restarts Dock & Finder", len(m.preview.MacOSPrefs))
	}

	box := fg(cDim3).Render("◯")
	nameStyle := fg(cMuted)
	if on {
		box = fg(cAccent).Render("◉")
		nameStyle = fg(cText)
	}
	prefix := "  "
	if cursor {
		prefix = fg(cAccent).Render("› ")
		nameStyle = fg(cWhite).Bold(true)
	}
	return prefix + box + " " + nameStyle.Render(padTo(name, 12)) + fg(cDim).Render(desc)
}

// ── confirm: mouse ──

func (m Model) updateConfirmMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionMotion {
		_, idx := m.confirmHitTest(msg.Y)
		m.hoverRow = idx
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if _, idx := m.confirmHitTest(msg.Y); idx >= 0 {
		m.confCur = idx
		rows := m.confirmRows()
		switch rows[clamp(idx, 0, len(rows)-1)] {
		case rowShell:
			m.confShell = !m.confShell
		case rowDotfiles:
			m.confDotfiles = !m.confDotfiles
		case rowPrefs:
			m.confPrefs = !m.confPrefs
		}
	}
	return m, nil
}

// confirmHeaderRows is the number of body rows above the first toggleable row
// (2 blanks + title + subtitle + blank + packages + git + blank = 8, plus one
// for the post-install line when the plan carries a script).
func (m Model) confirmHeaderRows() int {
	n := 8
	if len(m.preview.PostInstall) > 0 {
		n++
	}
	return n
}

// confirmHitTest maps a screen Y coordinate to a confirm-row index.
func (m Model) confirmHitTest(y int) (string, int) {
	if !m.inBody(y) {
		return "", -1
	}
	bodyRow := y - 1
	idx := bodyRow - m.confirmHeaderRows()
	if idx >= 0 && idx < len(m.confirmRows()) {
		return "row", idx
	}
	return "", -1
}
