package wizard

import (
	"context"
	"errors"
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

// buildPlan resolves the current selection into an install plan — from the
// remote config in config mode, from the catalog selection otherwise.
func (m Model) buildPlan() installer.InstallPlan {
	if m.rc != nil {
		return installer.PlanForRemoteSelection(m.opts, m.rc, m.selected, m.selectedOnlinePkgs())
	}
	return installer.PlanFromSelection(m.opts, m.selected, m.selectedOnlinePkgs())
}

func (m Model) startInstall() (tea.Model, tea.Cmd) {
	plan := m.buildPlan()
	// Apply a git identity captured on the git screen (fresh Mac). When git is
	// already configured, these stay empty and PlanFromSelection's existing
	// config is used instead.
	if strings.TrimSpace(m.gitName) != "" && strings.TrimSpace(m.gitEmail) != "" {
		plan.GitName = m.gitName
		plan.GitEmail = m.gitEmail
		plan.SkipGit = false
	}
	// Honor the confirm screen's toggles — a step switched off there must not
	// run.
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
	// Force non-interactive Apply: guarantees no huh prompt (git/npm-retry/
	// screen-recording reminder) fires mid-alt-screen. All decisions are
	// already resolved in the plan.
	plan.Silent = true

	m.plan = plan
	// The alt-screen can't host the post-install script's confirm prompt; the
	// CLI runs it after teardown from the returned plan (m.plan keeps it) —
	// strip it from the streamed apply so ApplyContext doesn't execute it here.
	streamed := plan
	streamed.PostInstall = nil
	m.phases = buildPhases(streamed)
	m.logs = nil
	m.skippedPkgs = 0
	m.aborting = false
	m.screen = scrInstall
	m.installing = true
	m.done = false
	m.confirmed = true
	m.installTick = m.ticks

	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is stored on the model and called on ctrl+c and install completion
	m.cancel = cancel

	return m, tea.Batch(m.spawnInstall(ctx, streamed), waitForEvent(m.events))
}

// spawnInstall runs the install engine on a background goroutine, streaming
// progress onto the event channel. It returns immediately.
func (m Model) spawnInstall(ctx context.Context, plan installer.InstallPlan) tea.Cmd {
	ch, done := m.events, m.installDone
	return func() tea.Msg {
		go func() {
			sink := func(ev progress.Event) { ch <- evMsg{ev: ev} }
			restoreBrew := brew.SetProgressSink(sink)
			restoreNpm := npm.SetProgressSink(sink)
			err := installer.ApplyContext(ctx, plan, chanReporter{ch: ch})
			restoreBrew()
			restoreNpm()
			// Signal that the engine has stopped touching os.Stdout (ApplyContext
			// returned) BEFORE the channel send, so Run can restore stdout without
			// racing us — even on a force-quit where nothing drains the channel and
			// the send below could block on a full buffer.
			close(done)
			ch <- installDoneMsg{err: err}
		}()
		return nil
	}
}

// PipelinePhase describes one row of the install pipeline for RunPipeline. Use
// the progress.Phase* names for package phases so streamed events line up.
type PipelinePhase struct {
	Name  string
	Total int
	Pkg   bool // true for per-item package phases (brew/cask/npm)
}

func toPhaseStates(ps []PipelinePhase) []phaseState {
	out := make([]phaseState, len(ps))
	for i, p := range ps {
		out[i] = phaseState{name: p.Name, total: p.Total, pkg: p.Pkg}
	}
	return out
}

// startPipeline runs an externally-supplied plan (RunPipeline / sync-source
// path) on a goroutine, wiring the same brew/npm sinks as spawnInstall so
// package progress streams into the shared install screen.
func (m Model) startPipeline() tea.Cmd {
	ch, done, run, ctx := m.events, m.installDone, m.pipelineRun, m.pipelineCtx
	return func() tea.Msg {
		go func() {
			sink := func(ev progress.Event) { ch <- evMsg{ev: ev} }
			restoreBrew := brew.SetProgressSink(sink)
			restoreNpm := npm.SetProgressSink(sink)
			// chanReporter feeds section headers (→ phase activation) and outcome
			// log lines onto the same channel, so an ApplyContext-based run
			// streams exactly like the wizard's own install.
			err := run(ctx, chanReporter{ch: ch})
			restoreBrew()
			restoreNpm()
			// See spawnInstall: close before the send so Run can join safely.
			close(done)
			ch <- installDoneMsg{err: err}
		}()
		return nil
	}
}

// PhasesForPlan derives pipeline phases from a resolved installer plan, so a
// RemoteConfig/preset install (install <slug>) can drive RunPipeline with the
// same sidebar the wizard builds.
func PhasesForPlan(plan installer.InstallPlan) []PipelinePhase {
	ps := buildPhases(plan)
	out := make([]PipelinePhase, len(ps))
	for i, p := range ps {
		out[i] = PipelinePhase{Name: p.name, Total: p.total, Pkg: p.pkg}
	}
	return out
}

// buildPhases derives the pipeline sidebar from the plan. Package-phase totals
// count every planned package: the engine's streaming invariant is that each
// one produces exactly one terminal event (installed, failed, or
// already-installed skip), so totals and the event stream reconcile by
// construction — including alias-resolved and state-file skips.
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

// pkgCount is the number of packages that will actually be installed, derived
// from the package-phase totals.
func (m Model) pkgCount() int {
	n := 0
	for _, p := range m.phases {
		if p.pkg {
			n += p.total
		}
	}
	return n
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
		if m.aborting {
			// A ctrl+c cancel SIGKILLs the in-flight brew/npm subprocess, so t.err
			// is usually non-nil ("signal: killed"). The old `&& t.err == nil` guard
			// therefore left ErrAborted unset for the common case (abort during the
			// package phase), and the CLI misreported the abort as an install
			// failure. Join so errors.Is(err, ErrAborted) holds while the underlying
			// cause stays in the chain for the log.
			m.installErr = errors.Join(ErrAborted, t.err)
		}
		if m.cancel != nil {
			m.cancel()
		}
		// Only a clean run gets every phase check-marked; on error or abort,
		// phases that never completed stay visibly unfinished.
		for i := range m.phases {
			m.phases[i].active = false
			if m.installErr == nil {
				m.phases[i].finished = true
			}
		}
		if m.aborting {
			// The user asked to leave; the engine has now stopped — quit and
			// let the CLI report the abort on a normal terminal.
			m.quit = true
			return m, tea.Quit
		}
		m.appendSummary()
		return m, nil
	}
	return m, nil
}

// appendSummary writes the run's outcome into the log tail — the counts, and
// crucially the names of any failed packages, which would otherwise have
// scrolled out of the visible log by the time the user reads the DONE screen.
func (m *Model) appendSummary() {
	installed := m.pkgCount() - m.skippedPkgs - len(m.failedPkgs)
	if installed < 0 {
		installed = 0
	}
	text := fmt.Sprintf("%d installed · %d already present · %s",
		installed, m.skippedPkgs, fmtElapsed(m.elapsed()))
	m.appendLog(logLine{}) // spacer
	if len(m.failedPkgs) == 0 && m.installErr == nil {
		m.appendLog(logLine{mark: "✓", markColor: cAccent, text: text, color: cTextHi})
		return
	}
	m.appendLog(logLine{mark: "!", markColor: cWarn, text: text, color: cTextHi})
	if len(m.failedPkgs) > 0 {
		m.appendLog(logLine{mark: "✗", markColor: cDanger,
			text: fmt.Sprintf("%d failed: %s", len(m.failedPkgs), strings.Join(m.failedPkgs, ", ")), color: cDanger})
	}
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
		// The npm outer retry (applyNpm) re-runs the install and re-emits an
		// "already installed" skip for every package a prior attempt installed —
		// breaking the one-terminal-event-per-package invariant. Ignore a skip for
		// a package that already had a terminal event, so skippedPkgs (which the
		// completion footer subtracts from the package total) can't be inflated
		// past the total and render a negative "N packages".
		if ev.Detail == progress.SkipDetail && m.terminalSeen[termKey(ev)] {
			return
		}
		m.markTerminal(ev)
		m.incPhase(ev.Phase)
		text := ev.Name
		if ev.Detail != "" {
			text += " — " + ev.Detail
		}
		if ev.Detail == progress.SkipDetail {
			m.skippedPkgs++
			m.appendLog(logLine{mark: "○", markColor: cDim3, text: text, color: cDim})
		} else {
			m.appendLog(logLine{mark: "✓", markColor: cAccent, text: text, color: cMuted})
		}
	case progress.StepFail:
		m.markTerminal(ev)
		m.incPhase(ev.Phase)
		if ev.Name != "" {
			m.failedPkgs = append(m.failedPkgs, ev.Name)
		}
		m.appendLog(logLine{mark: "✗", markColor: cDanger, text: ev.Name + " (" + ev.Detail + ")", color: cDanger})
	}
}

// termKey identifies a package's terminal event by phase + name. Empty for
// unnamed events (e.g. the batch StepStart), which are never deduped.
func termKey(ev progress.Event) string {
	if ev.Name == "" {
		return ""
	}
	return ev.Phase + "/" + ev.Name
}

// markTerminal records that a package produced a terminal event, so a later
// re-emitted skip for it (npm retry) can be recognised as a duplicate.
func (m *Model) markTerminal(ev progress.Event) {
	if k := termKey(ev); k != "" {
		m.terminalSeen[k] = true
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
			// Clamp: a retry pass emits a second terminal event for the same
			// package; don't let done overrun the total.
			if m.phases[i].done < m.phases[i].total {
				m.phases[i].done++
			}
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
	case rHeader, rInfo, rMuted:
		// Dropped intentionally — headers drive phase activation (see
		// headerPhase) and info/muted narration would drown the log.
		return logLine{}, false
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

// fmtElapsed renders whole seconds as "42s" or "3m24s" — a raw "7204s" after a
// long cask install reads as noise.
func fmtElapsed(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

// ── key handling ──

func (m Model) updateInstall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.done {
		switch msg.String() {
		case "r":
			if m.pipelineRun == nil { // replay is a wizard-only action
				return m.replay()
			}
		case "q", "enter", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

// updateInstallMouse handles mouse events on the install screen.
// During install: no action (user is watching progress).
// DONE screen: a click anywhere quits (same as enter/esc/q).
func (m Model) updateInstallMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.done && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		return m, tea.Quit
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
		// Pending phases keep a readable label: the glyph (○ / spinner / ✓)
		// and its hue carry the state. Fading the label toward the background
		// instead reads as "this step is missing", and vanishes outright on a
		// terminal whose background isn't the one the palette assumed.
		icon := fg(cMuted3).Render("○")
		nameStyle := fg(cMuted)
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
		pkgN := m.pkgCount() - m.skippedPkgs
		head := fg(cAccent).Render("✓") + " " + fg(cTextHi).Bold(true).Render("This Mac is dev-ready.") +
			"  " + fg(cDim3).Render(fmt.Sprintf("%d packages · %s", pkgN, fmtElapsed(m.elapsed())))
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
