package wizard

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/installer"
)

// The wizard plans; it does not install. Everything the user decides is
// resolved into an InstallPlan here, and Run hands that plan back to the CLI,
// which applies it on the normal terminal.
//
// The apply deliberately does NOT happen inside the alt-screen. A full-screen
// install looks good while it runs and then takes its own output with it: the
// alt-screen is torn down on exit and twenty minutes of package results vanish,
// leaving "check ~/.openboot/logs" as the only recourse. Streaming the apply
// into the scrollback instead keeps the record where the user already looks —
// scroll up, copy the error, pipe it — which is the whole point of a CLI.

// buildPlan resolves the current selection into an install plan — from the
// remote config in config mode, from the catalog selection otherwise.
func (m Model) buildPlan() installer.InstallPlan {
	if m.rc != nil {
		return installer.PlanForRemoteSelection(m.opts, m.rc, m.selected, m.selectedOnlinePkgs())
	}
	return installer.PlanFromSelection(m.opts, m.selected, m.selectedOnlinePkgs())
}

// finish accepts the reviewed plan and closes the wizard so the CLI can apply
// it. Confirm-screen toggles are folded in here: a step switched off must not
// reach the plan the installer receives.
func (m Model) finish() (tea.Model, tea.Cmd) {
	plan := m.buildPlan()

	// A git identity captured on the git screen (fresh Mac). When git is already
	// configured, these stay empty and the plan's existing config is used.
	if strings.TrimSpace(m.gitName) != "" && strings.TrimSpace(m.gitEmail) != "" {
		plan.GitName = m.gitName
		plan.GitEmail = m.gitEmail
		plan.SkipGit = false
	}
	if !m.confShell {
		plan.InstallOhMyZsh = false
		plan.ShellTheme = ""
		plan.ShellPlugins = nil
	}
	if !m.confDotfiles {
		plan.DotfilesURL = ""
	}
	if !m.confPrefs {
		plan.MacOSPrefs = nil
	}

	m.plan = plan
	m.confirmed = true
	m.quit = true
	return m, tea.Quit
}
