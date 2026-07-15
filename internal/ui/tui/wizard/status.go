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
			"←→ pane · ↑↓ move · space toggle · / filter · a all · x clear · ↵ install",
			fmt.Sprintf("%d pkgs · ~%d min", m.selCount(), m.estMin())

	case scrGit:
		return "GIT", cAccent, "↑↓/tab switch field · ↵ continue · esc back", "identity for your commits"

	case scrConfirm:
		return "REVIEW", cAccent, "↑↓ move · space toggle · ↵ install · esc back",
			fmt.Sprintf("%d pkgs · ~%d min", m.selCount(), m.estMin())

	default: // scrInstall
		if m.done {
			return "DONE", cAccent, "r replay from boot · q quit",
				fmt.Sprintf("%d steps · %s", m.totalSteps(), fmtElapsed(m.elapsed()))
		}
		if m.aborting {
			return "ABORT", cDanger, "aborting — waiting for the current step to stop · ctrl+c again to force quit",
				fmt.Sprintf("%d/%d · %s", m.completedSteps(), m.totalSteps(), fmtElapsed(m.elapsed()))
		}
		return "INSTALL", cWarn, "installing — everything is logged to ~/.openboot/logs",
			fmt.Sprintf("%d/%d · %s", m.completedSteps(), m.totalSteps(), fmtElapsed(m.elapsed()))
	}
}
