package wizard

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/system"
)

// gitConfigLookup is a seam so tests can stub the existing-identity probe.
var gitConfigLookup = system.GetExistingGitConfig

// needsGitCapture reports whether the wizard should prompt for a git identity
// before installing: only when system config is not fully set and the run
// configures system state. It also returns any partial existing values to
// prefill.
func (m Model) needsGitCapture() (need bool, name, email string) {
	if m.opts.PackagesOnly {
		return false, "", ""
	}
	// Config-mode installs are declarative — the git step already no-ops
	// safely when no identity exists (matching the slug pipeline path), so
	// don't interpose a prompt the config never asked for.
	if m.rc != nil {
		return false, "", ""
	}
	name, email = gitConfigLookup()
	if name != "" && email != "" {
		return false, name, email
	}
	return true, name, email
}

func (m Model) updateGit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = scrSelect
		return m, nil
	case "tab", "down", "up", "shift+tab":
		m.gitField = (m.gitField + 1) % 2
	case "enter":
		if m.gitField == 0 {
			m.gitField = 1
			return m, nil
		}
		if strings.TrimSpace(m.gitName) != "" && strings.TrimSpace(m.gitEmail) != "" {
			return m.enterConfirm()
		}
		// Focus whichever field is still empty.
		if strings.TrimSpace(m.gitName) == "" {
			m.gitField = 0
		}
		return m, nil
	case "backspace":
		if m.gitField == 0 {
			m.gitName = trimLast(m.gitName)
		} else {
			m.gitEmail = trimLast(m.gitEmail)
		}
	default:
		// KeyRunes covers both single keystrokes and multi-rune input
		// (bubbletea coalesces pasted/fast text into one message).
		if s := msg.String(); msg.Type == tea.KeyRunes || s == " " {
			if m.gitField == 0 {
				m.gitName += s
			} else {
				m.gitEmail += s
			}
		}
	}
	return m, nil
}

// trimLast removes the final rune (not byte) — a byte slice would leave
// invalid UTF-8 behind after backspacing multi-byte input like "张" or "é".
func trimLast(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

func (m Model) gitBody(_, _ int) string {
	const pad = "   "
	var b []string
	b = append(b, "")
	b = append(b, "")
	b = append(b, pad+fg(cTextHi).Bold(true).Render("Set your git identity"))
	b = append(b, pad+fg(cDim3).Render("No git config found — used to author your commits on this Mac."))
	b = append(b, "")
	b = append(b, pad+m.gitFieldRow(0, "Name", m.gitName, "Jane Developer"))
	b = append(b, pad+m.gitFieldRow(1, "Email", m.gitEmail, "jane@example.com"))
	b = append(b, "")
	b = append(b, pad+fg(cDim3).Render("↑↓/tab switch field · ↵ continue · esc back"))
	return strings.Join(b, "\n")
}

func (m Model) updateGitMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionMotion {
		_, idx := m.gitHitTest(msg.X, msg.Y)
		m.hoverRow = idx // field index or -1
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if _, idx := m.gitHitTest(msg.X, msg.Y); idx >= 0 {
		m.gitField = idx
	}
	return m, nil
}

func (m Model) gitHitTest(x, y int) (string, int) {
	if !m.inBody(y) {
		return "", -1
	}
	bodyRow := y - 1
	// gitBody layout: 2 blank + title + subtitle + blank = 5 rows before fields
	if bodyRow == 5 {
		return "name", 0
	}
	if bodyRow == 6 {
		return "email", 1
	}
	return "", -1
}

func (m Model) gitFieldRow(idx int, label, value, placeholder string) string {
	focused := m.gitField == idx
	hovered := m.hoverRow == idx
	sep := fg(cBorder).Render("┃")
	var val string
	switch {
	case value == "" && !focused:
		val = fg(cDim4).Render(placeholder)
	case focused:
		sep = fg(cAccent).Render("┃")
		val = fg(cTextHi).Render(value) + fg(cAccent).Render("▌")
	default:
		val = fg(cTextHi).Render(value)
	}
	line := fg(cDim).Render(padTo(label, 7)) + " " + sep + " " + val
	if hovered {
		line = hoverBg(line)
	}
	return line
}
