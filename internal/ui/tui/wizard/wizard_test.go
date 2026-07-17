package wizard

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/progress"
)

func sized(w, h int) Model {
	m := New("1.4.0", &config.InstallOptions{Version: "1.4.0"})
	return send(m, tea.WindowSizeMsg{Width: w, Height: h})
}

func TestBootProbesGateInputThenPickLoadout(t *testing.T) {
	m := sized(96, 30)

	// While probing, loadout keys are ignored.
	m = send(m, key("2"))
	require.Equal(t, scrBoot, m.screen)

	m = finishProbes(m)
	require.Equal(t, len(m.probes), m.probeIdx, "all probes done")
	assert.True(t, m.installed["git"], "scan populated the installed set")

	m = send(m, key("2"))
	assert.Equal(t, scrSelect, m.screen)
	assert.Equal(t, config.GetPackagesForPreset("developer"), m.selected)
}

func TestBootHandPickStartsEmpty(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.Equal(t, scrSelect, m.screen)
	assert.Zero(t, m.selCount())
}

func TestSelectToggleSelectAllClear(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c")) // hand-pick, empty selection
	m.installed = map[string]bool{}

	require.Zero(t, m.selCount())
	m = send(m, key("space"))
	assert.Equal(t, 1, m.selCount(), "space toggles the cursor row")

	m = send(m, key("a"))
	assert.Equal(t, len(m.pool()), m.selCount(), "a selects all in the category")

	m = send(m, key("x"))
	assert.Zero(t, m.selCount(), "x clears selection")
}

func TestSelectInstalledRowNotToggleable(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	// Force the cursor's package to be "installed".
	pool := m.pool()
	require.NotEmpty(t, pool)
	m.installed = map[string]bool{pool[0].Name: true}
	m = send(m, key("space"))
	assert.Zero(t, m.selCount(), "installed rows can't be selected")
}

func TestSelectFilter(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("2")) // developer
	m = send(m, key("/"))
	require.True(t, m.typing)
	for _, r := range "docker" {
		m = send(m, key(string(r)))
	}
	pool := m.pool()
	require.NotEmpty(t, pool)
	for _, p := range pool {
		assert.Contains(t, strings.ToLower(p.Name+" "+p.Description), "docker")
	}
	// Esc exits filter.
	m = send(m, key("esc"))
	assert.False(t, m.typing)
	assert.Empty(t, m.query)
}

// Bubbletea coalesces pasted/fast text into one multi-rune KeyMsg; the filter
// and git inputs must accept it, not drop it.
func TestSelectFilterAcceptsPastedText(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	m = send(m, key("/"))
	require.True(t, m.typing)
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zoxide")})
	assert.Equal(t, "zoxide", m.query, "multi-rune input appends to the query")
}

func TestGitFieldsAcceptPastedText(t *testing.T) {
	defer stubGitConfig("", "")()
	m := finishProbes(sized(96, 30))
	m = send(m, key("2"))
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrGit, m.screen)
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("CI Bot")})
	m = send(m, key("tab"))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ci@example.com")})
	assert.Equal(t, "CI Bot", m.gitName)
	assert.Equal(t, "ci@example.com", m.gitEmail)
}

// The select screen is a two-pane focus model: ← → (and tab) move focus
// between the category sidebar and the package list; ↑ ↓ act on the focused
// pane. These tests pin that contract plus the mouse mapping.

func TestSelectFocusAndCategoryNav(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.Equal(t, focusList, m.selFocus, "the package list has focus by default")
	require.GreaterOrEqual(t, len(m.cats), 2)

	// ← focuses the sidebar; ↓ then advances the category and resets the row.
	m = send(m, key("left"))
	assert.Equal(t, focusCats, m.selFocus)
	start := m.catCur
	m = send(m, key("down"))
	assert.Equal(t, start+1, m.catCur, "↓ under sidebar focus moves to the next category")
	assert.Equal(t, 0, m.rowCur, "switching category resets the package cursor")
	require.GreaterOrEqual(t, len(m.pool()), 2, "chosen category needs rows to move through")

	// → returns focus to the list; ↓ now moves the package cursor, not category.
	m = send(m, key("right"))
	assert.Equal(t, focusList, m.selFocus)
	cat := m.catCur
	m = send(m, key("down"))
	assert.Equal(t, cat, m.catCur, "↓ under list focus leaves the category unchanged")
	assert.Equal(t, 1, m.rowCur, "↓ under list focus advances the package cursor")
}

// TestSelectFocusIsVisuallyIndicated proves the focus cue survives WITHOUT
// colour: with identical state but different focus, the sidebar and cursor row
// differ structurally (the pointer glyph moves to the focused pane), so the
// indicator holds up in piped output and for colourblind users — and needs no
// forced colour profile to test. What a test still can't judge is whether the
// cue is *clear enough* to a human; that stays taste.
func TestSelectFocusIsVisuallyIndicated(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.GreaterOrEqual(t, len(m.pool()), 1)

	catsFocus, listFocus := m, m
	catsFocus.selFocus = focusCats
	listFocus.selFocus = focusList

	assert.NotEqual(t,
		strings.Join(catsFocus.selectSidebar(28), "\n"),
		strings.Join(listFocus.selectSidebar(28), "\n"),
		"active category marker must differ by focus (structural, not just colour)")
	assert.NotEqual(t,
		catsFocus.renderRow(catsFocus.pool()[0], true, 90),
		listFocus.renderRow(listFocus.pool()[0], true, 90),
		"cursor row must differ by focus — the pointer shows only under list focus")
}

func TestSelectTabTogglesFocus(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.Equal(t, focusList, m.selFocus)
	m = send(m, key("tab"))
	assert.Equal(t, focusCats, m.selFocus, "tab toggles focus to the sidebar")
	m = send(m, key("tab"))
	assert.Equal(t, focusList, m.selFocus, "tab toggles focus back to the list")
}

func TestSelectHitTest(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.GreaterOrEqual(t, len(m.cats), 2)
	require.GreaterOrEqual(t, len(m.pool()), 1)

	// Sidebar: category i renders at screen row 4+i (title bar + 3 header rows).
	kind, idx := m.selectHitTest(2, 4)
	assert.Equal(t, hitCat, kind)
	assert.Equal(t, 0, idx)
	kind, idx = m.selectHitTest(2, 5)
	assert.Equal(t, hitCat, kind)
	assert.Equal(t, 1, idx)

	// List: first package at screen row 3 (title + search + blank), x past sidebar.
	kind, idx = m.selectHitTest(sidebarW+5, 3)
	assert.Equal(t, hitPkg, kind)
	assert.Equal(t, 0, idx)

	// Out of range → none.
	kind, _ = m.selectHitTest(2, 0) // title bar
	assert.Equal(t, hitNone, kind)
	kind, _ = m.selectHitTest(sidebarW+5, 999) // below the list
	assert.Equal(t, hitNone, kind)
}

func TestSelectMouseClickCategory(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.GreaterOrEqual(t, len(m.cats), 2)
	m = send(m, tea.MouseMsg{X: 2, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	assert.Equal(t, 1, m.catCur, "clicking a category switches to it")
	assert.Equal(t, focusCats, m.selFocus, "clicking a category focuses the sidebar")
}

func TestSelectMouseClickTogglesPackage(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	m.installed = map[string]bool{}
	pool := m.pool()
	require.NotEmpty(t, pool)
	click := tea.MouseMsg{X: sidebarW + 5, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m = send(m, click)
	assert.True(t, m.selected[pool[0].Name], "clicking a package toggles it on")
	assert.Equal(t, focusList, m.selFocus)
	m = send(m, click)
	assert.False(t, m.selected[pool[0].Name], "clicking again toggles it off")
}

func TestSelectMouseWheelScrolls(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.GreaterOrEqual(t, len(m.pool()), 2)
	require.Equal(t, 0, m.rowCur)
	m = send(m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	assert.Equal(t, 1, m.rowCur, "wheel down advances the list cursor")
	m = send(m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	assert.Equal(t, 0, m.rowCur, "wheel up moves it back")
}

func TestBootMouseClickPicksLoadout(t *testing.T) {
	m := finishProbes(sized(96, 30))
	require.Equal(t, scrBoot, m.screen)
	require.GreaterOrEqual(t, len(m.loadouts), 2)

	// Click the second loadout row. Geometry: 4 header + 4 probes + 3 footer
	// = loadouts start at body row 11 → screen row 12. Loadout 1 at row 13.
	m = send(m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 10, Y: 13})
	assert.Equal(t, scrSelect, m.screen, "clicking a loadout enters the select screen")
	assert.Equal(t, 1, m.loadCur, "click lands on loadout index 1")
}

func TestBootMouseHoverHighlightsLoadout(t *testing.T) {
	m := finishProbes(sized(96, 30))
	require.Equal(t, scrBoot, m.screen)
	require.Equal(t, -1, m.hoverRow, "starts with no hover")

	// Motion over the first loadout row (screen row 12) sets hoverRow.
	m = send(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: 10, Y: 12})
	assert.Equal(t, 0, m.hoverRow, "hover over loadout 0")

	// Motion off the loadout area clears it.
	m = send(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: 10, Y: 1})
	assert.Equal(t, -1, m.hoverRow, "hover off loadout clears indicator")
}

func TestGitMouseClickFocusesField(t *testing.T) {
	defer stubGitConfig("", "")()
	m := finishProbes(sized(96, 30))
	m = send(m, key("2"))
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrGit, m.screen)
	require.Equal(t, 0, m.gitField, "name field focused by default")

	// Click the email field row (body row 6 → screen row 7).
	m = send(m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 10, Y: 7})
	assert.Equal(t, 1, m.gitField, "click switches focus to email field")
}

func TestConfirmMouseClickTogglesRow(t *testing.T) {
	defer stubGitConfig("Jane Dev", "jane@ex.io")()
	m := finishProbes(sized(96, 30))
	m = send(m, key("2"))
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrConfirm, m.screen)
	require.True(t, m.confShell)
	require.GreaterOrEqual(t, len(m.confirmRows()), 1)

	// Click the first toggleable row (body row 8 → screen row 9).
	m = send(m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 10, Y: 9})
	assert.False(t, m.confShell, "click toggles the first confirm row off")
}

func TestSelectMouseHoverHighlightsRow(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	require.GreaterOrEqual(t, len(m.pool()), 2)
	require.Equal(t, -1, m.hoverRow, "starts with no hover")

	// Motion over the first package row sets hoverRow.
	m = send(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: sidebarW + 5, Y: 3})
	assert.Equal(t, 0, m.hoverRow, "hover over row 0")

	// Motion over chrome (sidebar or title) clears hoverRow.
	m = send(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: 2, Y: 1})
	assert.Equal(t, -1, m.hoverRow, "hover over chrome clears the indicator")
}

func TestTryInstallNoopWhenNothingToInstall(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c")) // empty selection
	m = send(m, key("enter"))
	assert.Equal(t, scrSelect, m.screen, "enter with nothing selected stays on select")
}

// An all-installed loadout has zero *new* packages but still carries config
// steps (git/shell/dotfiles/macOS); Enter must reach review, not trap the user.
func TestSelectAllInstalledLoadoutStillProceeds(t *testing.T) {
	defer stubGitConfig("Jane Dev", "jane@ex.io")() // configured → skip git capture
	m := finishProbes(sized(96, 30))
	m = send(m, key("2")) // developer loadout populates the selection
	require.Positive(t, m.selCount())

	// Mark every selected package as already present: 0 new, selection non-empty.
	m.installed = map[string]bool{}
	for name, on := range m.selected {
		if on {
			m.installed[name] = true
		}
	}
	require.Zero(t, m.toInstallCount(), "precondition: nothing new to install")

	m = send(m, key("enter"))
	assert.Equal(t, scrConfirm, m.screen, "an all-installed loadout must still reach review")
}

func TestGitCaptureWhenUnconfigured(t *testing.T) {
	defer stubGitConfig("", "")()

	m := finishProbes(sized(96, 30))
	m = send(m, key("2")) // developer — has packages to install
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrGit, m.screen, "no git config routes to the capture screen")

	for _, r := range "Jane Dev" {
		m = send(m, key(string(r)))
	}
	m = send(m, key("tab"))
	for _, r := range "jane@ex.io" {
		m = send(m, key(string(r)))
	}
	// Enter on the email field with both filled proceeds to the confirm
	// screen; a second enter starts the install.
	next, _ := m.Update(key("enter"))
	m = next.(Model)
	require.Equal(t, scrConfirm, m.screen, "git capture flows into the review screen")
	next, _ = m.Update(key("enter"))
	m = next.(Model)

	require.Equal(t, scrInstall, m.screen)
	assert.Equal(t, "Jane Dev", m.plan.GitName)
	assert.Equal(t, "jane@ex.io", m.plan.GitEmail)
	assert.False(t, m.plan.SkipGit)
}

func TestGitCaptureSkippedWhenConfigured(t *testing.T) {
	defer stubGitConfig("Ada", "ada@ex.io")()

	m := finishProbes(sized(96, 30))
	m = send(m, key("2"))
	m.installed = map[string]bool{}
	next, _ := m.Update(key("enter"))
	m = next.(Model)
	assert.Equal(t, scrConfirm, m.screen, "configured git goes straight to review")
}

// The confirm screen's toggles must gate the plan: a step switched off there
// must not reach the engine.
func TestConfirmtogglesGateThePlan(t *testing.T) {
	defer stubGitConfig("Ada", "ada@ex.io")()

	m := finishProbes(sized(96, 30))
	m = send(m, key("2"))
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrConfirm, m.screen)
	require.True(t, m.preview.InstallOhMyZsh, "preview computed on entry")

	// Toggle every row off: shell, dotfiles, prefs.
	rows := m.confirmRows()
	for range rows {
		m = send(m, key("space"))
		m = send(m, key("down"))
	}
	next, _ := m.Update(key("enter"))
	m = next.(Model)

	require.Equal(t, scrInstall, m.screen)
	assert.False(t, m.plan.InstallOhMyZsh, "shell toggled off")
	assert.Empty(t, m.plan.DotfilesURL, "dotfiles toggled off")
	assert.Empty(t, m.plan.MacOSPrefs, "prefs toggled off")
}

// ctrl+c during a running install must request an abort (stay in the TUI,
// cancel the context) and only quit once the engine reports done — with a
// non-nil ErrAborted so the CLI exits non-zero.
func TestCtrlCDuringInstallAbortsHonestly(t *testing.T) {
	plan := installer.InstallPlan{Formulae: []string{"a"}, SkipGit: true}
	m := New("1", &config.InstallOptions{})
	m.screen = scrInstall
	m.installing = true
	m.phases = buildPhases(plan)
	m.cancel = func() {}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(Model)
	assert.Nil(t, cmd, "first ctrl+c must not quit — it waits for the engine")
	require.True(t, m.aborting)

	// Cancelling the context SIGKILLs the in-flight brew/npm subprocess, so the
	// engine reports a NON-nil error, not a clean nil. The abort must still be
	// recognised as ErrAborted — the regression was that the old guard only set
	// ErrAborted when the error happened to be nil, so an abort during the
	// package phase (the common case) was misreported as an install failure.
	killErr := errors.New("signal: killed")
	next, _ = m.Update(installDoneMsg{err: killErr})
	m = next.(Model)
	assert.ErrorIs(t, m.installErr, ErrAborted, "abort with a killed subprocess is still ErrAborted")
	assert.ErrorIs(t, m.installErr, killErr, "underlying cause stays in the chain for the log")
	assert.True(t, m.quit)
	for _, p := range m.phases {
		assert.False(t, p.finished, "aborted phases must not show as finished")
	}
}

func TestGitScreenEscReturnsToSelect(t *testing.T) {
	defer stubGitConfig("", "")()
	m := finishProbes(sized(96, 30))
	m = send(m, key("2"))
	m.installed = map[string]bool{}
	m = send(m, key("enter"))
	require.Equal(t, scrGit, m.screen)
	m = send(m, key("esc"))
	assert.Equal(t, scrSelect, m.screen)
}

// Backspace must remove one rune, not one byte — a byte slice corrupts
// multi-byte input (张三, José) into invalid UTF-8 that would reach git config.
func TestTrimLastIsRuneAware(t *testing.T) {
	assert.Equal(t, "张", trimLast("张三"))
	assert.Equal(t, "Jos", trimLast("José"))
	assert.Equal(t, "", trimLast("a"))
	assert.Equal(t, "", trimLast(""))
}

// stubGitConfig swaps the git-identity lookup for tests and returns a restore.
func stubGitConfig(name, email string) func() {
	prev := gitConfigLookup
	gitConfigLookup = func() (string, string) { return name, email }
	return func() { gitConfigLookup = prev }
}

func TestBuildPhases(t *testing.T) {
	plan := installer.InstallPlan{
		Formulae:       []string{"a", "b"},
		Casks:          []string{"c"},
		Npm:            []string{"d"},
		InstallOhMyZsh: true,
		DotfilesURL:    "x",
		MacOSPrefs:     make([]macos.Preference, 1),
	}
	phases := buildPhases(plan)
	var names []string
	for _, p := range phases {
		names = append(names, p.name)
	}
	assert.Equal(t, []string{
		"Git identity", progress.PhaseHomebrew, progress.PhaseApplications,
		progress.PhaseNpm, "Shell", "Dotfiles", "macOS prefs",
	}, names)

	// PackagesOnly drops every config phase.
	po := buildPhases(installer.InstallPlan{Formulae: []string{"a"}, PackagesOnly: true})
	require.Len(t, po, 1)
	assert.Equal(t, progress.PhaseHomebrew, po[0].name)
}

// The streaming invariant: every planned package produces exactly one terminal
// event, so totals count the full plan and already-installed skips arrive as
// StepOK events with SkipDetail.
func TestSkipEventsCompletePhaseAndCountSkipped(t *testing.T) {
	plan := installer.InstallPlan{Formulae: []string{"a", "b", "c"}, SkipGit: true}
	m := New("1", &config.InstallOptions{})
	m.screen = scrInstall
	m.phases = buildPhases(plan)
	require.Equal(t, 3, m.phases[0].total, "totals count every planned package")

	feed := func(ev progress.Event) {
		next, _ := m.Update(evMsg{ev: ev})
		m = next.(Model)
	}
	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "a", Status: progress.StepOK, Detail: progress.SkipDetail})
	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "b", Status: progress.StepOK, Detail: progress.SkipDetail})
	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "c", Status: progress.StepOK, Detail: "1.2s"})

	assert.True(t, m.phases[0].finished, "skips + installs complete the phase")
	assert.Equal(t, 2, m.skippedPkgs)
	assert.Equal(t, 1, m.pkgCount()-m.skippedPkgs, "DONE footer counts actual installs")
}

// A retry pass emits a second terminal event for the same package; done must
// clamp at total instead of overrunning.
func TestIncPhaseClampsOnRetryEvents(t *testing.T) {
	plan := installer.InstallPlan{Formulae: []string{"a"}, SkipGit: true}
	m := New("1", &config.InstallOptions{})
	m.screen = scrInstall
	m.phases = buildPhases(plan)

	feed := func(ev progress.Event) {
		next, _ := m.Update(evMsg{ev: ev})
		m = next.(Model)
	}
	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "a", Status: progress.StepFail, Detail: "timeout"})
	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "a", Status: progress.StepOK, Detail: "retry succeeded"})
	assert.Equal(t, 1, m.phases[0].done, "retry event must not overrun the total")
	assert.Equal(t, 1, m.completedSteps())
}

func TestProgressEventsDrivePhasesAndLog(t *testing.T) {
	// SkipGit drops the "Git identity" phase so the package phases lead.
	plan := installer.InstallPlan{Formulae: []string{"a", "b"}, Casks: []string{"c"}, SkipGit: true}
	m := New("1", &config.InstallOptions{})
	m.screen = scrInstall
	m.phases = buildPhases(plan)

	feed := func(ev progress.Event) {
		next, _ := m.Update(evMsg{ev: ev})
		m = next.(Model)
	}
	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "a", Status: progress.StepStart, Command: "brew install a"})
	assert.True(t, phaseByName(m, progress.PhaseHomebrew).active, "homebrew active")
	assert.Equal(t, "a", m.curStep)

	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "a", Status: progress.StepOK, Detail: "1.0s"})
	feed(progress.Event{Phase: progress.PhaseHomebrew, Name: "b", Status: progress.StepOK, Detail: "2.0s"})
	assert.True(t, phaseByName(m, progress.PhaseHomebrew).finished, "homebrew finished at 2/2")
	assert.Equal(t, 2, m.completedSteps())

	feed(progress.Event{Phase: progress.PhaseApplications, Name: "c", Status: progress.StepStart, Command: "brew install --cask c"})
	assert.True(t, phaseByName(m, progress.PhaseApplications).active)

	// Log carries $cmd and ✓result lines.
	joined := strings.Join(logTexts(m.logs), "\n")
	assert.Contains(t, joined, "brew install a")
	assert.Contains(t, joined, "a — 1.0s")
}

func phaseByName(m Model, name string) phaseState {
	for _, p := range m.phases {
		if p.name == name {
			return p
		}
	}
	return phaseState{}
}

func TestReporterHeaderActivatesConfigPhase(t *testing.T) {
	plan := installer.InstallPlan{InstallOhMyZsh: true, SkipGit: true}
	m := New("1", &config.InstallOptions{})
	m.screen = scrInstall
	m.phases = buildPhases(plan)
	require.Len(t, m.phases, 1)

	next, _ := m.Update(reporterMsg{kind: rHeader, text: "Shell Configuration"})
	m = next.(Model)
	assert.True(t, m.phases[0].active, "shell header activates the Shell phase")
}

func TestInstallDoneMarksAllFinished(t *testing.T) {
	plan := installer.InstallPlan{Formulae: []string{"a"}, InstallOhMyZsh: true}
	m := New("1", &config.InstallOptions{})
	m.screen = scrInstall
	m.installing = true
	m.phases = buildPhases(plan)

	next, _ := m.Update(installDoneMsg{})
	m = next.(Model)
	assert.False(t, m.installing)
	assert.True(t, m.done)
	for _, p := range m.phases {
		assert.True(t, p.finished)
	}
}

// TestViewDimensions asserts every screen fills exactly the terminal box.
func TestViewDimensions(t *testing.T) {
	const W, H = 90, 28
	cases := map[string]Model{}
	cases["boot-probing"] = sized(W, H)
	cases["boot-loadouts"] = finishProbes(sized(W, H))
	cases["select"] = send(finishProbes(sized(W, H)), key("2"))
	gitCase := send(finishProbes(sized(W, H)), key("2"))
	gitCase.screen = scrGit
	cases["git"] = gitCase
	cases["install"] = installFrame(finishProbes(sized(W, H)), W, H)

	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(m.View(), "\n")
			assert.Len(t, lines, H, "line count == height")
			for i, ln := range lines {
				assert.LessOrEqualf(t, lipgloss.Width(ln), W, "line %d within width", i)
			}
		})
	}
}

func TestViewEmptyBeforeSize(t *testing.T) {
	m := New("1.4.0", &config.InstallOptions{})
	assert.Empty(t, m.View(), "renders nothing until sized")
}

// TestInstallGoroutineStreamsToDone exercises the real wiring end-to-end
// against a dry-run plan: spawnInstall's goroutine sets the brew/npm sinks,
// runs installer.ApplyContext with the channel Reporter, and the model drains
// the channel through Update until installDoneMsg. Dry-run means nothing is
// actually installed.
func TestInstallGoroutineStreamsToDone(t *testing.T) {
	opts := &config.InstallOptions{Version: "1", DryRun: true}
	m := New("1", opts)
	m.screen = scrInstall

	plan := installer.PlanFromSelection(opts, config.GetPackagesForPreset("minimal"), nil)
	plan.Silent = true
	m.plan = plan
	m.phases = buildPhases(plan)
	m.installing = true

	// Start the background install; the cmd returns nil and feeds m.events.
	m.spawnInstall(context.Background(), plan)()

	// Drain the channel through Update until the install reports done.
	deadline := time.After(30 * time.Second)
	for !m.done {
		select {
		case msg := <-m.events:
			next, _ := m.Update(msg)
			m = next.(Model)
		case <-deadline:
			t.Fatal("install did not complete within 30s")
		}
	}

	assert.False(t, m.installing)
	assert.NoError(t, m.installErr)
	// Every phase ends finished once done.
	for _, p := range m.phases {
		assert.Truef(t, p.finished, "phase %q finished", p.name)
	}
	// C2: the goroutine closes installDone once ApplyContext returns, so Run's
	// joinInstall can wait for it before restoring os.Stdout (no force-quit race).
	select {
	case <-m.installDone:
	default:
		t.Fatal("installDone must be closed once the install goroutine finishes")
	}
}

// joinInstall must not block when no install goroutine was ever started (e.g.
// the user quit on the boot/select screen) — otherwise Run would hang 30s.
func TestJoinInstallNoopWhenNoInstall(t *testing.T) {
	m := New("1", &config.InstallOptions{})
	returned := make(chan struct{})
	go func() { m.joinInstall(); close(returned) }()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("joinInstall must be a no-op when no install ran")
	}
}

func logTexts(ls []logLine) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.text
	}
	return out
}

// TestPipelineDrainsChannelAndCompletes runs a real (headless) program to catch
// the hang the unit tests below can't: pipeline-mode Init must arm waitForEvent
// so the goroutine's installDoneMsg is actually read. Without it, done never
// flips, the 'q' below is ignored (updateInstall quits only when done), and this
// test times out. (The send()-based tests inject installDoneMsg directly and so
// bypass — and miss — the Init→channel wiring.)
func TestPipelineDrainsChannelAndCompletes(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })

	m := New("t", &config.InstallOptions{})
	m.screen, m.installing = scrInstall, true
	m.phases = toPhaseStates([]PipelinePhase{{Name: "Homebrew", Total: 1, Pkg: true}})
	m.pipelineCtx = context.Background()
	m.pipelineRun = func(context.Context, installer.Reporter) error { return nil } // completes instantly

	p := tea.NewProgram(m, tea.WithInput(pr), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	done := make(chan tea.Model, 1)
	go func() { fm, _ := p.Run(); done <- fm }()

	time.Sleep(300 * time.Millisecond) // let the goroutine finish + drain to done
	_, _ = pw.Write([]byte("q"))       // DONE screen: q quits

	select {
	case fm := <-done:
		assert.True(t, fm.(Model).done, "pipeline reached done via the drained channel")
	case <-time.After(3 * time.Second):
		p.Kill()
		t.Fatal("pipeline hung — Init did not arm waitForEvent to drain m.events")
	}
}

// Pipeline mode (RunPipeline / sync-source path) reuses the install screen with
// externally-built phases; installDoneMsg must mark it done and, on a clean run,
// check-mark all phases.
func TestPipelineModeReachesDone(t *testing.T) {
	m := New("1.0.0", &config.InstallOptions{})
	m.screen = scrInstall
	m.installing = true
	m.phases = toPhaseStates([]PipelinePhase{
		{Name: "Homebrew", Total: 2, Pkg: true},
		{Name: "macOS prefs", Total: 1},
	})

	m = send(m, installDoneMsg{err: nil})
	assert.True(t, m.done, "installDoneMsg marks the screen done")
	assert.False(t, m.installing)
	for _, p := range m.phases {
		assert.True(t, p.finished, "a clean run check-marks every phase (incl. config steps)")
	}
}

func TestPipelineReplayDisabled(t *testing.T) {
	m := New("1.0.0", &config.InstallOptions{})
	m.screen, m.done = scrInstall, true
	m.pipelineRun = func(context.Context, installer.Reporter) error { return nil } // marks pipeline mode
	next, _ := m.Update(key("r"))
	// 'r' must NOT restart the wizard probe in pipeline mode — screen stays put.
	assert.Equal(t, scrInstall, next.(Model).screen, "replay is a no-op in pipeline mode")
}

// PhasesForPlan drives the slug/preset pipeline sidebar from an installer plan.
func TestPhasesForPlan(t *testing.T) {
	plan := installer.InstallPlan{
		Formulae:       []string{"jq", "ripgrep"},
		Casks:          []string{"orbstack"},
		InstallOhMyZsh: true,
		DotfilesURL:    "https://github.com/x/dotfiles",
	}
	ps := PhasesForPlan(plan)

	byName := map[string]PipelinePhase{}
	for _, p := range ps {
		byName[p.Name] = p
	}
	require.Contains(t, byName, "Homebrew")
	assert.Equal(t, 2, byName["Homebrew"].Total)
	assert.True(t, byName["Homebrew"].Pkg)
	require.Contains(t, byName, "Applications")
	require.Contains(t, byName, "Shell")
	assert.False(t, byName["Shell"].Pkg, "config steps are not per-item package phases")
	require.Contains(t, byName, "Dotfiles")
	assert.NotContains(t, byName, "npm globals", "no npm in this plan")
}

// ── deep-polish additions: small terminals, preset entry, online search,
// completion summary, hover colour degradation ──

func TestSmallTerminalShowsResizeHint(t *testing.T) {
	m := sized(48, 12)
	v := m.View()
	assert.Contains(t, v, "terminal too small")
	assert.Contains(t, v, "resize to at least 60×15")

	// Growing the window restores the real frame.
	m = send(m, tea.WindowSizeMsg{Width: 96, Height: 30})
	assert.NotContains(t, m.View(), "terminal too small")
}

func TestPresetOptionAutoAdvancesToSelect(t *testing.T) {
	m := New("1.4.0", &config.InstallOptions{Version: "1.4.0", Preset: "developer"})
	m = send(m, tea.WindowSizeMsg{Width: 96, Height: 30})
	m = finishProbes(m)
	require.Equal(t, scrSelect, m.screen, "-p skips the loadout question")
	assert.Equal(t, config.GetPackagesForPreset("developer"), m.selected)
}

func TestUnknownPresetStaysOnBoot(t *testing.T) {
	m := New("1.4.0", &config.InstallOptions{Version: "1.4.0", Preset: "bogus"})
	m = send(m, tea.WindowSizeMsg{Width: 96, Height: 30})
	m = finishProbes(m)
	assert.Equal(t, scrBoot, m.screen)
}

func TestOnlineSearchFindsTogglesAndSurvivesFilterClear(t *testing.T) {
	restore := searchOnline
	searchOnline = func(string) ([]config.Package, error) {
		return []config.Package{
			{Name: "web-only-tool", Description: "only on openboot.dev", IsNpm: true},
			{Name: "curl", Description: "already in the catalog"}, // must be deduped
		}, nil
	}
	defer func() { searchOnline = restore }()

	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	m = send(m, key("/"))
	for _, r := range "web-only" {
		m = send(m, key(string(r)))
	}
	seq := m.searchSeq

	// A stale generation must be ignored outright.
	m = send(m, searchDoneMsg{seq: seq - 1, results: []config.Package{{Name: "stale"}}})
	require.Empty(t, m.onlineResults)

	// The debounce tick for the live generation arms the lookup…
	next, cmd := m.Update(searchTickMsg{seq: seq, query: strings.TrimSpace(m.query)})
	m = next.(Model)
	require.True(t, m.onlineBusy)
	require.NotNil(t, cmd)
	// …whose result lands as a searchDoneMsg (stub is synchronous).
	m = send(m, cmd().(searchDoneMsg))
	require.False(t, m.onlineBusy)
	require.Len(t, m.onlineResults, 1, "catalog dupes are filtered out")

	// The online hit is in the pool; enter toggles the cursor row, clears the
	// filter, and the pick survives in the synthetic online category.
	pool := m.pool()
	idx := -1
	for i, p := range pool {
		if p.Name == "web-only-tool" {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0, "online hit joins the filtered pool")
	m.rowCur = idx
	m = send(m, key("enter"))
	assert.True(t, m.selected["web-only-tool"])
	assert.Empty(t, m.query, "enter-toggle clears the filter")

	foundCat := false
	for _, c := range m.cats {
		if c.Name == onlineCatName {
			foundCat = true
			assert.Len(t, c.Packages, 1)
		}
	}
	require.True(t, foundCat, "online pick is homed in the sidebar category")

	online := m.selectedOnlinePkgs()
	require.Len(t, online, 1)
	assert.True(t, online[0].IsNpm, "type info survives for categorization")

	// Deselecting removes the pick and the now-empty category.
	m.catCur = len(m.cats) - 1
	m.rowCur = 0
	m = send(m, key("space"))
	assert.False(t, m.selected["web-only-tool"])
	for _, c := range m.cats {
		assert.NotEqual(t, onlineCatName, c.Name, "empty online category is dropped")
	}
	assert.Less(t, m.catCur, len(m.cats), "category cursor re-clamped")
}

func TestCompletionSummaryListsFailures(t *testing.T) {
	m := installFrame(sized(96, 30), 96, 30)
	m = send(m, evMsg{ev: progress.Event{Phase: progress.PhaseHomebrew, Name: "ripgrep", Status: progress.StepFail, Detail: "build error"}})
	m = send(m, installDoneMsg{err: errors.New("1 package failed")})
	require.True(t, m.done)

	var logText strings.Builder
	for _, l := range m.logs {
		logText.WriteString(l.text + "\n")
	}
	assert.Contains(t, logText.String(), "installed · 0 already present", "summary counts land in the log")
	assert.Contains(t, logText.String(), "1 failed: ripgrep", "failed package names are restated at the end")
}

func TestCompletionSummaryCleanRun(t *testing.T) {
	m := installFrame(sized(96, 30), 96, 30)
	m = send(m, installDoneMsg{})
	var logText strings.Builder
	for _, l := range m.logs {
		logText.WriteString(l.text + "\n")
	}
	assert.Contains(t, logText.String(), "installed ·")
	assert.NotContains(t, logText.String(), "failed:")
}

// Hover must not depend on a background colour we guessed: reverse video is
// defined at every colour depth, so the marker survives themes we can't see.
func TestHoverUsesReverseVideo(t *testing.T) {
	const rev = "\x1b[7m"
	out := hoverBg("row")
	assert.True(t, strings.HasPrefix(out, rev), "row opens in reverse")
	assert.True(t, strings.HasSuffix(out, "\x1b[0m"), "row closes with a reset")

	// A styled span's own reset must not silently end the highlight: reverse is
	// re-opened after it (once to open the row, once after the inner reset —
	// the row's own closing reset is appended afterwards and must stay bare).
	styled := hoverBg("a" + "\x1b[0m" + "b")
	assert.Equal(t, 2, strings.Count(styled, rev), "reverse re-established after the inner reset")
	assert.False(t, strings.HasSuffix(styled, rev), "the closing reset is not re-opened")
}

// The text ramp must stay terminal-relative: a hex grey is a guess about the
// user's background, and the guess is what made pending rows and key hints
// invisible on a translucent terminal. Brand hues are exempt — they're bright
// enough to carry anywhere and they're the product's identity.
func TestTextRampIsTerminalRelative(t *testing.T) {
	for name, c := range map[string]lipgloss.Color{
		"cWhite": cWhite, "cTextHi": cTextHi, "cText": cText, "cMuted": cMuted,
		"cMuted2": cMuted2, "cMuted3": cMuted3, "cDim": cDim, "cDim2": cDim2,
		"cDim3": cDim3, "cDim4": cDim4, "cFaint": cFaint, "cBorder": cBorder,
		"cInstalled": cInstalled,
	} {
		assert.NotContains(t, string(c), "#", "%s must use an ANSI index, not a hex guess at the background", name)
	}
}
