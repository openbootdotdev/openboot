package wizard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/progress"
)

// send routes a message through Update and returns the resulting Model.
func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func key(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "space":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// finishProbes drives the boot probes to completion with representative data.
func finishProbes(m Model) Model {
	m = send(m, probeDoneMsg{idx: 0, result: "macOS 15.5 · Apple M3 Pro · 18 GB · arm64", ok: true})
	m = send(m, probeDoneMsg{idx: 1, result: "Homebrew 4.5.11 — healthy", ok: true})
	m = send(m, probeDoneMsg{idx: 2, result: "6 tools already on this Mac — will be skipped", ok: true,
		installed: map[string]bool{"git": true, "python": true}})
	m = send(m, probeDoneMsg{idx: 3, result: "catalog ready · 240 packages · 61 macOS prefs", ok: true})
	return m
}

// TestDumpFrames renders each screen for manual visual inspection:
//
//	go test ./internal/ui/tui/wizard -run TestDumpFrames -v
func TestDumpFrames(t *testing.T) {
	const W, H = 96, 30
	m := New("1.4.0", &config.InstallOptions{Version: "1.4.0"})
	m = send(m, tea.WindowSizeMsg{Width: W, Height: H})

	// Boot: mid-probe (probe 2 running).
	mid := send(m, probeDoneMsg{idx: 0, result: "macOS 15.5 · Apple M3 Pro · 18 GB · arm64", ok: true})
	mid = send(mid, probeDoneMsg{idx: 1, result: "Homebrew 4.5.11 — healthy", ok: true})
	t.Log("\n===== BOOT (probing) =====\n" + mid.View())

	// Boot: done, loadout picker.
	m = finishProbes(m)
	t.Log("\n===== BOOT (loadouts) =====\n" + m.View())

	// Select: Developer loadout.
	sel := send(m, key("2"))
	t.Log("\n===== SELECT (developer) =====\n" + sel.View())

	// Select: filtered.
	f := send(sel, key("/"))
	for _, r := range "doc" {
		f = send(f, key(string(r)))
	}
	t.Log("\n===== SELECT (filter 'doc') =====\n" + f.View())

	// Git identity capture.
	g := send(sel, key("2")) // reuse a select model
	g.screen = scrGit
	g.gitName = "Jane Developer"
	g.gitField = 1
	t.Log("\n===== GIT (capture) =====\n" + g.View())

	// Install: synthetic pipeline (no real Apply).
	inst := installFrame(m, W, H)
	t.Log("\n===== INSTALL (running) =====\n" + inst.View())

	done := send(inst, installDoneMsg{})
	t.Log("\n===== INSTALL (done) =====\n" + done.View())
}

// installFrame sets up an install-screen model and feeds synthetic progress
// events without running the real install engine.
func installFrame(m Model, _, _ int) Model {
	m.screen = scrInstall
	m.installing = true
	m.plan = installer.InstallPlan{
		Formulae:   []string{"node", "go", "ripgrep", "fd", "bat"},
		Casks:      []string{"visual-studio-code", "warp"},
		Npm:        []string{"typescript", "eslint"},
		MacOSPrefs: make([]macos.Preference, 61),
	}
	m.plan.InstallOhMyZsh = true
	m.plan.DotfilesURL = "https://github.com/x/dotfiles"
	m.phases = buildPhases(m.plan, m.installed)
	m.installTick = m.ticks

	m = send(m, evMsg{ev: progress.Event{Phase: progress.PhaseHomebrew, Name: "node", Status: progress.StepStart, Command: "brew install node"}})
	m = send(m, evMsg{ev: progress.Event{Phase: progress.PhaseHomebrew, Name: "node", Status: progress.StepOK, Detail: "2.1s"}})
	m = send(m, evMsg{ev: progress.Event{Phase: progress.PhaseHomebrew, Name: "go", Status: progress.StepStart, Command: "brew install go"}})
	m = send(m, evMsg{ev: progress.Event{Phase: progress.PhaseHomebrew, Name: "go", Status: progress.StepOK, Detail: "3.4s"}})
	m = send(m, evMsg{ev: progress.Event{Phase: progress.PhaseHomebrew, Name: "ripgrep", Status: progress.StepStart, Command: "brew install ripgrep"}})
	return m
}
