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

func (m Model) gitFieldRow(idx int, label, value, placeholder string) string {
	focused := m.gitField == idx
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
	return fg(cDim).Render(padTo(label, 7)) + " " + sep + " " + val
}
