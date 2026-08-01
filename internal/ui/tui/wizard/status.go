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
		cmdline := "~ % openboot install"
		if m.srcLabel != "" {
			cmdline += " " + m.srcLabel
		}
		if m.opts.DryRun {
			cmdline += " --dry-run"
		}
		if m.probeIdx < len(m.probes) {
			return "BOOT", cInfo, "probing this Mac…", cmdline
		}
		return "BOOT", cInfo, "1 / 2 / 3 pick a loadout · c hand-pick from scratch · ↵ select", cmdline

	case scrSelect:
		action := "install"
		if m.opts.DryRun {
			action = "preview"
		}
		return "SELECT", cAccent,
			"←→ pane · ↑↓ move · space toggle · / filter · a all · x clear · ↵ " + action,
			fmt.Sprintf("%d pkgs · ~%d min", m.selCount(), m.estMin())

	case scrGit:
		return "GIT", cAccent, "↑↓/tab switch field · ↵ continue · esc back", "identity for your commits"

	default: // scrConfirm
		action := "install"
		if m.opts.DryRun {
			action = "preview"
		}
		return "REVIEW", cAccent, "↑↓ move · space toggle · ↵ " + action + " · esc back",
			fmt.Sprintf("%d pkgs · ~%d min", m.selCount(), m.estMin())
	}
}
