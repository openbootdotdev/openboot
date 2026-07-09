// Package wizard implements the OpenBoot install TUI (Redesign v5): a single
// full-screen bubbletea program that flows boot-probe → two-pane select →
// live pipeline install, under a persistent title bar and status bar.
//
// It replaces the previous interactive planning prompts (preset select, package
// selector, per-step confirms) and the linear Apply output. Non-interactive
// paths (--silent, --dry-run, presets, --from, -u, sync) never reach here.
package wizard

import (
	"context"
	"errors"
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
	scrInstall
)

// focusPane is which of the two select-screen columns holds keyboard focus.
// ← → (and tab) move focus between them; ↑ ↓ act on the focused one.
type focusPane int

const (
	focusList focusPane = iota // package list (right column) — the default
	focusCats                  // category sidebar (left column)
)

// ErrAborted is returned by Run when the user cancels a running install with
// ctrl+c. It distinguishes a deliberate abort from install failures.
var ErrAborted = errors.New("installation aborted")

// tickInterval drives the spinner and the derived elapsed clock.
const tickInterval = 120 * time.Millisecond

type tickMsg struct{}

// Model is the unified install-wizard state.
type Model struct {
	version string
	opts    *config.InstallOptions

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
	selected map[string]bool

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

	// ── install ──
	events      chan tea.Msg
	plan        installer.InstallPlan
	phases      []phaseState
	logs        []logLine
	curStep     string
	installing  bool
	aborting    bool // ctrl+c received mid-install; waiting for the engine to stop
	done        bool
	installErr  error
	skippedPkgs int // terminal events with SkipDetail (already installed)
	installTick int // ticks value when install started, for elapsed
	cancel      context.CancelFunc
}

// New builds a wizard model for the given version and resolved install options.
func New(version string, opts *config.InstallOptions) Model {
	return Model{
		version:   version,
		opts:      opts,
		screen:    scrBoot,
		probes:    newProbes(),
		loadouts:  newLoadouts(),
		installed: map[string]bool{},
		cats:      config.GetCategories(),
		selected:  map[string]bool{},
		events:    make(chan tea.Msg, 1024),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.runProbe(0))
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// waitForEvent blocks on the install event channel and delivers the next msg.
func waitForEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.ticks++
		// Keep ticking while anything is animating (probing or installing).
		return m, tickCmd()

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			// First ctrl+c during a running install requests an abort: cancel
			// the context and keep the TUI up until the engine reports back
			// (installDoneMsg), so the goroutine is joined and the abort is
			// reported honestly. A second ctrl+c force-quits.
			if m.screen == scrInstall && m.installing && !m.aborting {
				m.aborting = true
				if m.cancel != nil {
					m.cancel()
				}
				return m, nil
			}
			if m.cancel != nil {
				m.cancel()
			}
			if m.installing {
				m.installErr = ErrAborted
			}
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
		case scrInstall:
			return m.updateInstall(msg)
		}

	case tea.MouseMsg:
		if m.screen == scrSelect {
			return m.updateSelectMouse(msg)
		}
		return m, nil

	case probeDoneMsg:
		return m.onProbeDone(msg)

	case evMsg, reporterMsg, installDoneMsg:
		return m.onInstallEvent(msg)
	}
	return m, nil
}

func (m Model) spinner() string {
	return spinnerFrames[m.ticks%len(spinnerFrames)]
}

// ── View / chrome ──────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
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
	case scrInstall:
		body = m.installBody(m.width, bodyH)
	}

	return m.titleBar() + "\n" + fitBlock(body, m.width, bodyH) + "\n" + m.statusBar()
}

func (m Model) crumb() string {
	switch m.screen {
	case scrBoot:
		return "setup"
	case scrSelect:
		return "select packages"
	case scrGit:
		return "git identity"
	case scrConfirm:
		return "review plan"
	default:
		if m.done {
			return "done"
		}
		return "installing"
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

// Run launches the wizard. It returns the applied plan, whether an install was
// started (confirmed), and any error from the install run (ErrAborted when the
// user cancelled mid-install). Stray stdout from the install engine is
// redirected away from the alt-screen for the program's lifetime; the abort
// flow keeps the TUI alive until the engine goroutine reports done, so the
// redirect isn't restored under its feet.
func Run(version string, opts *config.InstallOptions) (plan installer.InstallPlan, confirmed bool, err error) {
	m := New(version, opts)

	realOut := os.Stdout
	if devnull, derr := os.OpenFile(os.DevNull, os.O_WRONLY, 0); derr == nil {
		os.Stdout = devnull
		defer func() {
			os.Stdout = realOut
			_ = devnull.Close()
		}()
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithOutput(realOut), tea.WithInput(os.Stdin))
	final, runErr := p.Run()
	if runErr != nil {
		return installer.InstallPlan{}, false, fmt.Errorf("run wizard: %w", runErr)
	}
	fm := final.(Model)
	return fm.plan, fm.confirmed, fm.installErr
}
