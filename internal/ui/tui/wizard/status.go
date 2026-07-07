package wizard

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// statusContent returns the bottom status-bar pieces for the current screen:
// the mode badge label + color, the keybinding hint, and the right-aligned info.
func (m Model) statusContent() (mode string, color lipgloss.Color, keys, right string) {
	switch m.screen {
	case scrBoot:
		if m.probeIdx < len(m.probes) {
			return "BOOT", cInfo, "probing this Mac…", "~ % openboot install"
		}
		return "BOOT", cInfo, "1 / 2 / 3 pick a loadout · c hand-pick from scratch · ↵ select", "~ % openboot install"

	case scrSelect:
		return "SELECT", cAccent,
			"↑↓/jk move · space toggle · ⇥ category · / filter · a all · x clear · ↵ install",
			fmt.Sprintf("%d pkgs · ~%d min", m.selCount(), m.estMin())

	case scrGit:
		return "GIT", cAccent, "↑↓/tab switch field · ↵ continue · esc back", "identity for your commits"

	default: // scrInstall
		if m.done {
			return "DONE", cAccent, "r replay from boot · q quit",
				fmt.Sprintf("%d steps · %ds", m.totalSteps(), m.elapsed())
		}
		return "INSTALL", cWarn, "installing — everything is logged to ~/.openboot/logs",
			fmt.Sprintf("%d/%d · %ds", m.completedSteps(), m.totalSteps(), m.elapsed())
	}
}
