package wizard

import (
	"context"
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

func TestSelectCategoryCycle(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c"))
	start := m.catCur
	m = send(m, key("tab"))
	assert.Equal(t, (start+1)%len(m.cats), m.catCur)
}

func TestTryInstallNoopWhenNothingToInstall(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("c")) // empty selection
	m = send(m, key("enter"))
	assert.Equal(t, scrSelect, m.screen, "enter with nothing to install stays on select")
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

	next, _ = m.Update(installDoneMsg{})
	m = next.(Model)
	assert.ErrorIs(t, m.installErr, ErrAborted)
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

	plan := installer.PlanFromSelection(opts, config.GetPackagesForPreset("minimal"))
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
}

func logTexts(ls []logLine) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.text
	}
	return out
}
