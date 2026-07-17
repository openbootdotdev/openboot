package wizard

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

// Regressions from the v0.63 review that still have a home: the install-screen
// ones (npm skip dedup, elapsed freeze) went with the screen itself when the
// apply moved back to the scrollback.

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
