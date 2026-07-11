package wizard

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/progress"
)

func npmEvent(name string, status progress.Status, detail string) evMsg {
	return evMsg{ev: progress.Event{Phase: progress.PhaseNpm, Name: name, Status: status, Detail: detail}}
}

// M1: the npm outer retry re-runs the install and re-emits an "already
// installed" skip for every package a prior attempt installed. The renderer
// must not double-count those, or the completion footer subtracts an inflated
// skip count from the package total and renders a negative "N packages".
func TestNpmRetrySkipsDoNotInflatePackageCount(t *testing.T) {
	m := New("1", &config.InstallOptions{})
	m.screen, m.installing = scrInstall, true
	m.phases = toPhaseStates([]PipelinePhase{{Name: progress.PhaseNpm, Total: 2, Pkg: true}})

	// Attempt 1: pkgA installs, pkgB fails.
	m = send(m, npmEvent("pkgA", progress.StepOK, ""))
	m = send(m, npmEvent("pkgB", progress.StepFail, "boom"))
	// Attempt 2: the retry re-scans and re-emits pkgA as already-installed while
	// pkgB now installs.
	m = send(m, npmEvent("pkgA", progress.StepOK, progress.SkipDetail))
	m = send(m, npmEvent("pkgB", progress.StepOK, ""))

	assert.Equal(t, 0, m.skippedPkgs, "a re-emitted skip for an already-counted package must not count")
	assert.GreaterOrEqual(t, m.pkgCount()-m.skippedPkgs, 0, "footer package tally must never go negative")
}

// The dedup must not swallow a genuine already-installed skip (the common,
// non-retry case where a package really was present before this run).
func TestGenuinePreInstalledSkipStillCounts(t *testing.T) {
	m := New("1", &config.InstallOptions{})
	m.screen, m.installing = scrInstall, true
	m.phases = toPhaseStates([]PipelinePhase{{Name: progress.PhaseNpm, Total: 1, Pkg: true}})

	m = send(m, npmEvent("pkgA", progress.StepOK, progress.SkipDetail))
	assert.Equal(t, 1, m.skippedPkgs, "a genuine already-installed skip counts once")
}

// C3: the elapsed clock on the completion footer must freeze once the install
// is done, not keep counting up while the user reads it.
func TestElapsedFreezesWhenDone(t *testing.T) {
	m := New("1", &config.InstallOptions{})
	m.screen, m.installing = scrInstall, true

	for i := 0; i < 10; i++ {
		m = send(m, tickMsg{})
	}
	require.Equal(t, 10, m.ticks)

	m = send(m, installDoneMsg{})
	require.True(t, m.done)
	frozen := m.elapsed()

	next, cmd := m.Update(tickMsg{})
	m = next.(Model)
	assert.Nil(t, cmd, "the tick loop stops re-arming once done")
	assert.Equal(t, 10, m.ticks, "ticks frozen at completion")
	assert.Equal(t, frozen, m.elapsed(), "elapsed clock frozen at completion")
}

// M2: a resize must re-clamp the stored scroll, so selectHitTest (which reads
// the stored scroll) agrees with selectList (which renders from a re-clamped
// copy). Without it, a click right after a resize toggles the wrong package.
func TestWindowResizeReclampsSelectScroll(t *testing.T) {
	m := New("1", &config.InstallOptions{})
	m = send(m, tea.WindowSizeMsg{Width: 96, Height: 40})
	m.screen = scrSelect
	m.cats = []config.Category{{Name: "big", Packages: makePackages(50)}}
	m.catCur, m.selFocus = 0, focusList
	m.rowCur = 49
	m = m.clampSelScroll()
	require.Greater(t, m.scroll, 0, "cursor at the end of a long list should be scrolled at height 40")

	m = send(m, tea.WindowSizeMsg{Width: 96, Height: 14})
	assert.Equal(t, m.clampSelScroll().scroll, m.scroll,
		"resize must re-clamp the stored scroll so hit-test matches the render")
}

// The vertical divider column between the two panes is neither pane — a click
// there must hit nothing, not toggle the adjacent package.
func TestSelectHitTestDividerColumnIsNoHit(t *testing.T) {
	m := finishProbes(sized(96, 30))
	m = send(m, key("2"))
	require.Equal(t, scrSelect, m.screen)

	kind, idx := m.selectHitTest(sidebarW, 5)
	assert.Equal(t, hitNone, kind)
	assert.Equal(t, -1, idx)
}

func makePackages(n int) []config.Package {
	ps := make([]config.Package, n)
	for i := range ps {
		ps[i] = config.Package{Name: fmt.Sprintf("pkg%02d", i)}
	}
	return ps
}
