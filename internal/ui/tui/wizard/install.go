package wizard

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/installer"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/progress"
)

// phaseState is one row of the install pipeline sidebar.
type phaseState struct {
	name     string
	total    int
	done     int
	pkg      bool // true for package phases (brew/cask/npm) that count per-item
	active   bool
	finished bool
}

// logLine is one rendered line of the live install log.
type logLine struct {
	mark      string
	markColor lipgloss.Color
	text      string
	color     lipgloss.Color
}

// reporterKind classifies an installer.Reporter call.
type reporterKind int

const (
	rHeader reporterKind = iota
	rInfo
	rSuccess
	rWarn
	rError
	rMuted
)

type evMsg struct{ ev progress.Event }
type reporterMsg struct {
	kind reporterKind
	text string
}
type installDoneMsg struct{ err error }

// chanReporter forwards installer.Reporter calls onto the wizard event channel.
type chanReporter struct{ ch chan tea.Msg }

func (r chanReporter) Header(s string)  { r.ch <- reporterMsg{kind: rHeader, text: s} }
func (r chanReporter) Info(s string)    { r.ch <- reporterMsg{kind: rInfo, text: s} }
func (r chanReporter) Success(s string) { r.ch <- reporterMsg{kind: rSuccess, text: s} }
func (r chanReporter) Warn(s string)    { r.ch <- reporterMsg{kind: rWarn, text: s} }
func (r chanReporter) Error(s string)   { r.ch <- reporterMsg{kind: rError, text: s} }
func (r chanReporter) Muted(s string)   { r.ch <- reporterMsg{kind: rMuted, text: s} }

// ── starting the install ──

func (m Model) startInstall() (tea.Model, tea.Cmd) {
	plan := installer.PlanFromSelection(m.opts, m.selected)
	// Force non-interactive Apply: guarantees no huh prompt (git/npm-retry/
	// screen-recording reminder) fires mid-alt-screen. All decisions are
	// already resolved in the plan.
	plan.Silent = true

	m.plan = plan
	m.phases = buildPhases(plan)
	m.logs = nil
	m.screen = scrInstall
	m.installing = true
	m.done = false
	m.confirmed = true
	m.installTick = m.ticks

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	return m, tea.Batch(m.spawnInstall(ctx, plan), waitForEvent(m.events))
}

// spawnInstall runs the install engine on a background goroutine, streaming
// progress onto the event channel. It returns immediately.
func (m Model) spawnInstall(ctx context.Context, plan installer.InstallPlan) tea.Cmd {
	ch := m.events
	return func() tea.Msg {
		go func() {
			sink := func(ev progress.Event) { ch <- evMsg{ev: ev} }
			restoreBrew := brew.SetProgressSink(sink)
			restoreNpm := npm.SetProgressSink(sink)
			err := installer.ApplyContext(ctx, plan, chanReporter{ch: ch})
			restoreBrew()
			restoreNpm()
			ch <- installDoneMsg{err: err}
		}()
		return nil
	}
}

func buildPhases(plan installer.InstallPlan) []phaseState {
	var ps []phaseState
	add := func(name string, total int, pkg, present bool) {
		if present {
			ps = append(ps, phaseState{name: name, total: total, pkg: pkg})
		}
	}
	add("Git identity", 1, false, !plan.PackagesOnly && !plan.SkipGit)
	add(progress.PhaseHomebrew, len(plan.Formulae), true, len(plan.Formulae) > 0)
	add(progress.PhaseApplications, len(plan.Casks), true, len(plan.Casks) > 0)
	add(progress.PhaseNpm, len(plan.Npm), true, len(plan.Npm) > 0)
	add("Shell", 1, false, !plan.PackagesOnly && plan.InstallOhMyZsh)
	add("Dotfiles", 1, false, !plan.PackagesOnly && plan.DotfilesURL != "")
	add("macOS prefs", 1, false, !plan.PackagesOnly && len(plan.MacOSPrefs) > 0)
	return ps
}

// ── streaming event handling ──

func (m Model) onInstallEvent(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t := msg.(type) {
	case evMsg:
		m.applyProgressEvent(t.ev)
		return m, waitForEvent(m.events)

	case reporterMsg:
		if ph := headerPhase(t.text); ph != "" {
			m.activatePhase(ph)
		}
		if line, ok := reporterLogLine(t); ok {
			m.appendLog(line)
		}
		return m, waitForEvent(m.events)

	case installDoneMsg:
		m.installing = false
		m.done = true
		m.installErr = t.err
		for i := range m.phases {
			m.phases[i].active = false
			m.phases[i].finished = true
		}
		if m.cancel != nil {
			m.cancel()
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) applyProgressEvent(ev progress.Event) {
	m.activatePhase(ev.Phase)
	switch ev.Status {
	case progress.StepStart:
		if ev.Command != "" {
			m.appendLog(logLine{mark: "$", markColor: cDim4, text: ev.Command, color: cDim})
		}
		if ev.Name != "" {
			m.curStep = ev.Name
		}
	case progress.StepOK:
		m.incPhase(ev.Phase)
		text := ev.Name
		if ev.Detail != "" {
			text += " — " + ev.Detail
		}
		m.appendLog(logLine{mark: "✓", markColor: cAccent, text: text, color: cMuted})
	case progress.StepFail:
		m.incPhase(ev.Phase)
		m.appendLog(logLine{mark: "✗", markColor: cDanger, text: ev.Name + " (" + ev.Detail + ")", color: cDanger})
	}
}

// activatePhase marks phase name active and everything before it finished.
func (m *Model) activatePhase(name string) {
	if name == "" {
		return
	}
	idx := -1
	for i := range m.phases {
		if m.phases[i].name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	for i := 0; i < idx; i++ {
		if !m.phases[i].finished {
			m.phases[i].active = false
			m.phases[i].finished = true
		}
	}
	if !m.phases[idx].finished {
		m.phases[idx].active = true
	}
}

func (m *Model) incPhase(name string) {
	for i := range m.phases {
		if m.phases[i].name == name {
			m.phases[i].done++
			if m.phases[i].pkg && m.phases[i].done >= m.phases[i].total {
				m.phases[i].active = false
				m.phases[i].finished = true
			}
			return
		}
	}
}

func (m *Model) appendLog(l logLine) {
	m.logs = append(m.logs, l)
	if len(m.logs) > 500 {
		m.logs = m.logs[len(m.logs)-500:]
	}
}

// headerPhase maps an installer Header string onto a pipeline phase name.
func headerPhase(text string) string {
	switch {
	case strings.Contains(text, "Git Config"):
		return "Git identity"
	case strings.Contains(text, "Shell Config"):
		return "Shell"
	case strings.Contains(text, "Dotfiles"):
		return "Dotfiles"
	case strings.Contains(text, "macOS Prefer"):
		return "macOS prefs"
	case strings.Contains(text, "Installation"):
		return progress.PhaseHomebrew
	case strings.Contains(text, "NPM"):
		return progress.PhaseNpm
	}
	return ""
}

// reporterLogLine turns a meaningful reporter call (outcomes only) into a log
// line. Headers/info/muted are dropped to keep the log focused, matching the
// design's $cmd / ✓result cadence.
func reporterLogLine(t reporterMsg) (logLine, bool) {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t.text), "✓"))
	switch t.kind {
	case rSuccess:
		return logLine{mark: "✓", markColor: cAccent, text: text, color: cMuted}, true
	case rWarn:
		return logLine{mark: "!", markColor: cWarn, text: text, color: cWarn}, true
	case rError:
		return logLine{mark: "✗", markColor: cDanger, text: text, color: cDanger}, true
	}
	return logLine{}, false
}

// ── counters used by the status bar ──

func (m Model) totalSteps() int {
	n := 0
	for _, p := range m.phases {
		n += p.total
	}
	return n
}

func (m Model) completedSteps() int {
	n := 0
	for _, p := range m.phases {
		if p.pkg {
			n += min(p.done, p.total)
		} else if p.finished {
			n += p.total
		}
	}
	return n
}

func (m Model) elapsed() int {
	return (m.ticks - m.installTick) * int(tickInterval.Milliseconds()) / 1000
}

// ── key handling ──

func (m Model) updateInstall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.done {
		switch msg.String() {
		case "r":
			return m.replay()
		case "q", "enter", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) replay() (tea.Model, tea.Cmd) {
	nm := New(m.version, m.opts)
	nm.width, nm.height = m.width, m.height
	nm.ticks = m.ticks
	return nm, nm.runProbe(0)
}

// ── rendering ──

const pipelineW = 22

func (m Model) installBody(w, h int) string {
	// Reserve the bottom rows for the progress bar / completion summary.
	footer := m.installFooter(w)
	footerH := len(footer)
	paneH := h - footerH - 1
	if paneH < 1 {
		paneH = 1
	}

	sidebar := m.pipelineSidebar(paneH)
	logPane := m.logView(w-pipelineW-1, paneH)

	var lines []string
	for i := 0; i < paneH; i++ {
		l, r := "", ""
		if i < len(sidebar) {
			l = sidebar[i]
		}
		if i < len(logPane) {
			r = logPane[i]
		}
		lines = append(lines, padTo(l, pipelineW)+fg(cBorder).Render("│")+r)
	}
	lines = append(lines, fg(cBorder).Render(strings.Repeat("─", w)))
	lines = append(lines, footer...)
	return strings.Join(lines, "\n")
}

func (m Model) pipelineSidebar(h int) []string {
	rows := make([]string, 0, h)
	rows = append(rows, "")
	rows = append(rows, " "+fg(cDim4).Render("PIPELINE"))
	rows = append(rows, "")
	for _, p := range m.phases {
		icon := fg(cFaint).Render("○")
		nameStyle := fg(cDim3)
		switch {
		case p.finished:
			icon = fg(cAccent).Render("✓")
			nameStyle = fg(cMuted2)
		case p.active:
			icon = fg(cTextHi).Render(m.spinner())
			nameStyle = fg(cTextHi)
		}
		meta := fg(cDim4).Render(fmt.Sprintf("%d/%d", min(p.done, p.total), p.total))
		if !p.pkg {
			done := 0
			if p.finished {
				done = 1
			}
			meta = fg(cDim4).Render(fmt.Sprintf("%d/%d", done, p.total))
		}
		left := " " + icon + " " + nameStyle.Render(p.name)
		rows = append(rows, bar(left, meta+" ", pipelineW))
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return rows
}

// logView renders the tail of the log, bottom-aligned within h rows.
func (m Model) logView(w, h int) []string {
	rows := make([]string, 0, h)
	start := 0
	if len(m.logs) > h {
		start = len(m.logs) - h
	}
	// top padding so the log sits at the bottom of the pane
	for i := 0; i < h-(len(m.logs)-start); i++ {
		rows = append(rows, "")
	}
	for _, l := range m.logs[start:] {
		line := " " + fg(l.markColor).Render(l.mark) + " " + fg(l.color).Render(l.text)
		rows = append(rows, truncCell(line, w))
	}
	return rows
}

func (m Model) installFooter(w int) []string {
	if m.done {
		pkgN := len(m.plan.Formulae) + len(m.plan.Casks) + len(m.plan.Npm)
		head := fg(cAccent).Render("✓") + " " + fg(cTextHi).Bold(true).Render("This Mac is dev-ready.") +
			"  " + fg(cDim3).Render(fmt.Sprintf("%d packages · %ds", pkgN, m.elapsed()))
		if m.installErr != nil {
			head = fg(cWarn).Render("!") + " " + fg(cTextHi).Bold(true).Render("Finished with some errors.") +
				"  " + fg(cDim3).Render("see log above")
		}
		next := fg(cDim2).Render("next → ") + fg(cAccentHi).Render("openboot snapshot publish") +
			fg(cDim3).Render(" keeps this setup synced to your account")
		return []string{truncCell(head, w), truncCell(next, w)}
	}

	// Active install bar: spinner, current step, progress bar, percent.
	total := m.totalSteps()
	completed := m.completedSteps()
	pct := 0
	if total > 0 {
		pct = completed * 100 / total
	}
	cells := 24
	fill := 0
	if total > 0 {
		fill = completed * cells / total
	}
	barStr := fg(cAccent).Render(strings.Repeat("▰", fill)) + fg(cFaint).Render(strings.Repeat("▰", cells-fill))
	step := m.curStep
	if step == "" {
		step = "starting…"
	}
	left := fg(cAccent).Render(m.spinner()) + "  " + fg(cMuted).Bold(true).Render(padTo(truncCell(step, 18), 18)) + "  " + barStr
	right := fg(cMuted3).Render(fmt.Sprintf("%d%%", pct))
	return []string{bar(left, right, w)}
}
