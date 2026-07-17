// Package wizard implements the OpenBoot install planner: a full-screen
// bubbletea program that flows boot-probe → two-pane select → git → review,
// under a persistent title bar and status bar.
//
// It replaces the interactive planning prompts (preset select, package
// selector, per-step confirms) that used to flip between full-screen and inline
// several times per run. It deliberately stops at the plan: the apply runs
// after the alt-screen is gone, streaming into the scrollback, because an
// alt-screen install takes its own output with it when it exits.
//
// Preset installs (-p) enter with the loadout preselected; remote-config
// installs (slug, -u, --from, alias) enter config mode via RunForConfig, with
// the config's own packages on the select screen. Non-interactive paths
// (--silent, --dry-run, --update, no TTY) never reach the wizard.
package wizard

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/installer"
)

type screen int

const (
	scrBoot screen = iota
	scrSelect
	scrGit
	scrConfirm
)

// focusPane is which of the two select-screen columns holds keyboard focus.
// ← → (and tab) move focus between them; ↑ ↓ act on the focused one.
type focusPane int

const (
	focusList focusPane = iota // package list (right column) — the default
	focusCats                  // category sidebar (left column)
)

// tickInterval drives the boot-probe spinner.
const tickInterval = 120 * time.Millisecond

type tickMsg struct{}

// Model is the unified install-wizard state.
type Model struct {
	version string
	opts    *config.InstallOptions

	// rc, when non-nil, puts the wizard in config mode (install <slug> / -u /
	// --from / alias): the select screen shows the config's own packages
	// instead of the catalog, probing auto-advances past the loadout question,
	// and the plan is built from the filtered config.
	rc       *config.RemoteConfig
	srcLabel string // "user/slug" (or config name) shown in the status bar

	width, height int
	screen        screen
	ticks         int // monotonic, drives spinner + elapsed
	quit          bool
	confirmed     bool // true once install has been kicked off

	// ── boot ──
	probes    []probeRow
	probeIdx  int // index of the running probe; == len(probes) when done
	loadouts  []loadout
	loadCur   int
	installed map[string]bool // catalog packages already present on this Mac

	// ── select ──
	cats     []config.Category
	catCur   int
	rowCur   int
	scroll   int
	query    string
	typing   bool
	selFocus focusPane
	hoverRow int // mouse hover index in pool(), -1 when not on a package row
	selected map[string]bool

	// ── select: online search (packages beyond the local catalog) ──
	searchSeq     int              // debounce generation; stale ticks/results are dropped
	onlineBusy    bool             // a search request is in flight
	onlineResults []config.Package // current query's online hits (deduped vs catalog)
	onlineKnown   map[string]bool  // names sourced from openboot.dev, for the row badge

	// ── git identity (captured only when none is configured) ──
	gitName  string
	gitEmail string
	gitField int // 0 = name, 1 = email

	// ── confirm (pre-install review) ──
	preview      installer.InstallPlan // what this run would do, for display + toggles
	confShell    bool
	confDotfiles bool
	confPrefs    bool
	confCur      int

	// ── result ──
	// plan is what the CLI applies once the wizard exits; confirmed says the
	// user reviewed it and pressed ↵ rather than quitting.
	plan installer.InstallPlan
}

// New builds a wizard model for the given version and resolved install options.
func New(version string, opts *config.InstallOptions) Model {
	return Model{
		version:     version,
		opts:        opts,
		screen:      scrBoot,
		probes:      newProbes(),
		loadouts:    newLoadouts(),
		installed:   map[string]bool{},
		cats:        config.GetCategories(),
		hoverRow:    -1,
		selected:    map[string]bool{},
		onlineKnown: map[string]bool{},
	}
}

// NewForConfig builds a wizard model for a remote-config install: the sidebar
// categories are the config's own package lists, everything preselected —
// review-and-prune, mirroring the config's declarative intent.
func NewForConfig(version string, opts *config.InstallOptions, rc *config.RemoteConfig) Model {
	m := New(version, opts)
	m.rc = rc
	m.srcLabel = configLabel(rc)
	m.cats = categoriesFromConfig(rc)
	for _, c := range m.cats {
		for _, p := range c.Packages {
			m.selected[p.Name] = true
		}
	}
	return m
}

// configLabel names a remote config for display: user/slug when known.
func configLabel(rc *config.RemoteConfig) string {
	if rc.Username != "" && rc.Slug != "" {
		return rc.Username + "/" + rc.Slug
	}
	if rc.Name != "" {
		return rc.Name
	}
	return "config"
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.runProbe(0))
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Re-clamp the select scroll to the new height: selectList renders from a
		// re-clamped copy, but selectHitTest reads the stored scroll, so without
		// this a click right after a resize (before any mouse motion re-clamps)
		// could toggle the wrong package. Harmless on the other screens.
		return m.clampSelScroll(), nil

	case tickMsg:
		m.ticks++
		return m, tickCmd()

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			// Nothing has been applied yet — the wizard only plans — so quitting
			// is always clean and needs no confirmation.
			m.quit = true
			return m, tea.Quit
		}
		switch m.screen {
		case scrBoot:
			return m.updateBoot(msg)
		case scrSelect:
			return m.updateSelect(msg)
		case scrGit:
			return m.updateGit(msg)
		case scrConfirm:
			return m.updateConfirm(msg)
		}

	case tea.MouseMsg:
		return m.routeMouse(msg)

	case probeDoneMsg:
		return m.onProbeDone(msg)

	case searchTickMsg:
		return m.onSearchTick(msg)

	case searchDoneMsg:
		return m.onSearchDone(msg)

	}
	return m, nil
}

// routeMouse dispatches a mouse event to the active screen's handler.
func (m Model) routeMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case scrBoot:
		return m.updateBootMouse(msg)
	case scrSelect:
		return m.updateSelectMouse(msg)
	case scrGit:
		return m.updateGitMouse(msg)
	case scrConfirm:
		return m.updateConfirmMouse(msg)
	}
	return m, nil
}

func (m Model) spinner() string {
	return spinnerFrames[m.ticks%len(spinnerFrames)]
}

// ── View / chrome ──────────────────────────────────────────────────────────

// minTermW/minTermH are the smallest terminal the layout renders legibly in:
// below this the two-pane screens truncate into garbage, so show a resize
// hint instead of a broken frame. Keys (q / ctrl+c) still work.
const (
	minTermW = 60
	minTermH = 15
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < minTermW || m.height < minTermH {
		return m.smallTermView()
	}
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}

	var body string
	switch m.screen {
	case scrBoot:
		body = m.bootBody(m.width, bodyH)
	case scrSelect:
		body = m.selectBody(m.width, bodyH)
	case scrGit:
		body = m.gitBody(m.width, bodyH)
	case scrConfirm:
		body = m.confirmBody(m.width, bodyH)
	}

	return m.titleBar() + "\n" + fitBlock(body, m.width, bodyH) + "\n" + m.statusBar()
}

// smallTermView replaces the whole frame when the terminal is smaller than
// the layout can survive. Plain short lines so it stays legible at any size;
// ctrl+c (handled globally) still quits.
func (m Model) smallTermView() string {
	lines := []string{
		"",
		" " + fg(cAccent).Render("▲") + " " + fg(cTextHi).Render("openboot"),
		"",
		" " + fg(cWarn).Render(fmt.Sprintf("terminal too small — %d×%d", m.width, m.height)),
		" " + fg(cMuted).Render(fmt.Sprintf("resize to at least %d×%d to continue", minTermW, minTermH)),
		"",
		" " + fg(cDim2).Render("ctrl+c quit"),
	}
	return fitBlock(strings.Join(lines, "\n"), m.width, m.height)
}

// inBody reports whether screen row y is inside the rendered body (rows
// 1..height-2; row 0 is the title bar and height-1 the status bar). Mouse
// hit-tests guard on it so a click on the chrome never maps to a body row —
// e.g. on a short terminal a status-bar click must not pick a loadout.
func (m Model) inBody(y int) bool {
	return y >= 1 && y <= m.height-2
}

func (m Model) crumb() string {
	switch m.screen {
	case scrBoot:
		return "setup"
	case scrSelect:
		return "select packages"
	case scrGit:
		return "git identity"
	default:
		return "review plan"
	}
}

func (m Model) titleBar() string {
	left := fg(cAccent).Render("▲") + " " +
		fg(cMuted).Render("openboot") + " " +
		fg(cDim3).Render("v"+m.version)
	right := fg(cDim3).Render(m.crumb())
	return bar(left, right, m.width)
}

func (m Model) statusBar() string {
	mode, modeColor, keys, right := m.statusContent()
	badge := lipgloss.NewStyle().
		Background(modeColor).
		Foreground(lipgloss.Color("#08080a")).
		Bold(true).
		Padding(0, 1).
		Render(mode)
	left := badge + "  " + fg(cDim2).Render(keys)
	return bar(left, fg(cMuted3).Render(right), m.width)
}

// bar lays out left- and right-aligned segments across width w, truncating the
// left segment (by visual width) if the two would collide.
func bar(left, right string, w int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > w {
		// Truncate the left content to make room for the right.
		maxLeft := w - rw - 1
		if maxLeft < 0 {
			maxLeft = 0
		}
		left = lipgloss.NewStyle().MaxWidth(maxLeft).Render(left)
		lw = lipgloss.Width(left)
	}
	gap := w - lw - rw
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// fitBlock forces s to exactly h lines of width w: pads short lines with
// spaces, truncates long ones, and pads/truncates the line count.
func fitBlock(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		var line string
		if i < len(lines) {
			line = lines[i]
		}
		if lipgloss.Width(line) > w {
			line = lipgloss.NewStyle().MaxWidth(w).Render(line)
		}
		if pad := w - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// padTo right-pads a rendered cell to visual width w (no truncation).
func padTo(s string, w int) string {
	if pad := w - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// truncCell truncates a rendered string to visual width w.
func truncCell(s string, w int) string {
	if w < 0 {
		w = 0
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// Run launches the wizard: boot probe → select → git → review. It installs
// nothing; it returns the reviewed plan and whether the user confirmed it, and
// the caller applies that plan on the normal terminal once the alt-screen is
// gone.
func Run(version string, opts *config.InstallOptions) (plan installer.InstallPlan, confirmed bool, err error) {
	return runProgram(New(version, opts))
}

// RunForConfig launches the wizard in config mode for a fetched remote config
// (install <slug> / -u / --from / alias): the select screen shows the config's
// own packages, preselected. Returns like Run.
func RunForConfig(version string, opts *config.InstallOptions, rc *config.RemoteConfig) (plan installer.InstallPlan, confirmed bool, err error) {
	return runProgram(NewForConfig(version, opts, rc))
}

func runProgram(m Model) (plan installer.InstallPlan, confirmed bool, err error) {
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithInput(os.Stdin))
	final, runErr := p.Run()
	if runErr != nil {
		return installer.InstallPlan{}, false, fmt.Errorf("run wizard: %w", runErr)
	}
	fm := final.(Model)
	return fm.plan, fm.confirmed, nil
}
