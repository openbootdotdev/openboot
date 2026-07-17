package wizard

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/macos"
	"github.com/openbootdotdev/openboot/internal/npm"
	"github.com/openbootdotdev/openboot/internal/system"
)

// probeRow is one line of the boot-time environment probe.
type probeRow struct {
	key    string // short label, e.g. "probe", "brew", "scan", "sync"
	busy   string // shown with a spinner while the probe runs
	result string // shown with a ✓ once done
	ok     bool
	done   bool
}

func newProbes() []probeRow {
	return []probeRow{
		{key: "probe", busy: "reading hardware profile…"},
		{key: "brew", busy: "checking Homebrew…"},
		{key: "scan", busy: "scanning /opt/homebrew, /Applications…"},
		{key: "sync", busy: "syncing catalog…"},
	}
}

// loadout is a starting-point preset offered on the boot screen.
type loadout struct {
	key    string
	name   string
	preset string // backing preset name in config
}

func newLoadouts() []loadout {
	return []loadout{
		{key: "1", name: "Minimal", preset: "minimal"},
		{key: "2", name: "Developer", preset: "developer"},
		{key: "3", name: "Full", preset: "full"},
	}
}

// probeDoneMsg reports the completion of probe idx.
type probeDoneMsg struct {
	idx       int
	result    string
	ok        bool
	installed map[string]bool // non-nil only for the scan probe
}

// runProbe returns a command that executes probe i off the UI goroutine.
func (m Model) runProbe(i int) tea.Cmd {
	if i >= len(m.probes) {
		return nil
	}
	cats := m.cats
	return func() tea.Msg {
		switch i {
		case 0:
			return probeDoneMsg{idx: i, result: hardwareProfile(), ok: true}
		case 1:
			res, ok := brewHealth()
			return probeDoneMsg{idx: i, result: res, ok: ok}
		case 2:
			inst := scanInstalled(cats)
			word := "tools"
			if len(inst) == 1 {
				word = "tool"
			}
			return probeDoneMsg{
				idx:       i,
				result:    fmt.Sprintf("%d %s already on this Mac — will be skipped", len(inst), word),
				ok:        true,
				installed: inst,
			}
		default:
			return probeDoneMsg{idx: i, result: catalogSummary(cats), ok: true}
		}
	}
}

func (m Model) onProbeDone(msg probeDoneMsg) (tea.Model, tea.Cmd) {
	if msg.idx >= 0 && msg.idx < len(m.probes) {
		m.probes[msg.idx].result = msg.result
		m.probes[msg.idx].ok = msg.ok
		m.probes[msg.idx].done = true
	}
	if msg.installed != nil {
		m.installed = msg.installed
	}
	m.probeIdx = msg.idx + 1
	if m.probeIdx < len(m.probes) {
		return m, m.runProbe(m.probeIdx)
	}
	// Config mode: the remote config already answers the loadout question —
	// go straight to reviewing its (preselected) packages.
	if m.rc != nil {
		return m.enterSelect(m.selected)
	}
	// A preset given on the CLI (-p / OPENBOOT_PRESET) already answers the
	// loadout question — skip straight to the select screen with it applied,
	// so the flag means "start from this loadout, review, install" instead of
	// bypassing the wizard entirely.
	if m.opts.Preset != "" {
		if _, ok := config.GetPreset(m.opts.Preset); ok {
			return m.enterSelect(config.GetPackagesForPreset(m.opts.Preset))
		}
	}
	return m, nil // probing complete; wait for a loadout choice
}

// ── probe implementations (read-only; go through system/brew/npm) ──

func hardwareProfile() string {
	var parts []string
	if ver, err := system.RunCommandSilent("sw_vers", "-productVersion"); err == nil {
		if v := strings.TrimSpace(ver); v != "" {
			parts = append(parts, "macOS "+v)
		}
	}
	if chip, err := system.RunCommandSilent("sysctl", "-n", "machdep.cpu.brand_string"); err == nil {
		if c := strings.TrimSpace(chip); c != "" {
			parts = append(parts, c)
		}
	}
	if memStr, err := system.RunCommandSilent("sysctl", "-n", "hw.memsize"); err == nil {
		if b, perr := strconv.ParseInt(strings.TrimSpace(memStr), 10, 64); perr == nil && b > 0 {
			parts = append(parts, fmt.Sprintf("%d GB", b/(1024*1024*1024)))
		}
	}
	parts = append(parts, runtime.GOARCH)
	return strings.Join(parts, " · ")
}

func brewHealth() (string, bool) {
	if !brew.IsInstalled() {
		return "Homebrew not found — packages will be skipped", false
	}
	if out, err := system.RunCommandSilent("brew", "--version"); err == nil {
		line := strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]
		if v := strings.TrimSpace(strings.TrimPrefix(line, "Homebrew")); v != "" {
			return "Homebrew " + v + " — healthy", true
		}
	}
	return "Homebrew — healthy", true
}

// scanInstalled returns the set of catalog package names already present on the
// system (brew formulae, casks, and npm globals).
func scanInstalled(cats []config.Category) map[string]bool {
	installed := map[string]bool{}
	formulae, casks, _ := brew.GetInstalledPackages()
	npmPkgs, _ := npm.GetInstalledPackages()
	for _, cat := range cats {
		for _, p := range cat.Packages {
			switch {
			case p.IsNpm:
				if npmPkgs[p.Name] {
					installed[p.Name] = true
				}
			case p.IsCask:
				if casks[p.Name] {
					installed[p.Name] = true
				}
			default:
				if formulae[p.Name] {
					installed[p.Name] = true
				}
			}
		}
	}
	return installed
}

func catalogSummary(cats []config.Category) string {
	n := 0
	for _, c := range cats {
		n += len(c.Packages)
	}
	return fmt.Sprintf("catalog ready · %d packages · %d macOS prefs", n, len(macos.DefaultPreferences))
}

// ── boot: key handling ──

func (m Model) updateBoot(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.probeIdx < len(m.probes) {
		return m, nil // still probing — ignore input
	}
	switch msg.String() {
	case "q":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if m.loadCur > 0 {
			m.loadCur--
		}
	case "down", "j":
		if m.loadCur < len(m.loadouts)-1 {
			m.loadCur++
		}
	case "1", "2", "3":
		i := int(msg.String()[0] - '1')
		if i >= 0 && i < len(m.loadouts) {
			return m.pickLoadout(i)
		}
	case "c":
		return m.enterSelect(map[string]bool{})
	case "enter":
		return m.pickLoadout(m.loadCur)
	}
	return m, nil
}

func (m Model) pickLoadout(i int) (tea.Model, tea.Cmd) {
	return m.enterSelect(config.GetPackagesForPreset(m.loadouts[i].preset))
}

func (m Model) enterSelect(sel map[string]bool) (tea.Model, tea.Cmd) {
	m.selected = sel
	m.screen = scrSelect
	m.catCur, m.rowCur, m.scroll = 0, 0, 0
	m.query, m.typing = "", false
	m.hoverRow = -1
	return m, nil
}

// ── boot: rendering ──

func (m Model) bootBody(w, _ int) string {
	const pad = "   "
	var b []string
	// No wordmark here: the title bar one row up already reads
	// "▲ openboot vX.Y.Z". Repeating the name immediately below it said the
	// same word twice and spent a row doing it. The tagline is the only part
	// that adds anything, so the body opens with that.
	b = append(b, "")
	b = append(b, pad+fg(cDim3).Render("zero → dev-ready, in one command"))
	b = append(b, "")

	upto := m.probeIdx
	if upto >= len(m.probes) {
		upto = len(m.probes) - 1
	}
	for i := 0; i <= upto && i < len(m.probes); i++ {
		p := m.probes[i]
		mark := fg(cAccent).Render("✓")
		text := fg(cMuted).Render(p.result)
		if !p.done {
			mark = fg(cInfo).Render(m.spinner())
			text = fg(cDim2).Render(p.busy)
		} else if !p.ok {
			mark = fg(cWarn).Render("!")
			text = fg(cWarn).Render(p.result)
		}
		b = append(b, pad+mark+"  "+fg(cDim2).Render(padTo(p.key, 8))+text)
	}
	b = append(b, "")

	if m.probeIdx >= len(m.probes) {
		b = append(b, pad+fg(cMuted3).Render("Choose a starting point — or press ")+
			fg(cAccent).Render("c")+fg(cMuted3).Render(" to hand-pick:"))
		b = append(b, "")
		for i, l := range m.loadouts {
			b = append(b, pad+m.renderLoadout(i, l, w-2*len(pad)))
		}
		b = append(b, "")
		b = append(b, pad+fg(cFaint).Render("Every run also restores ")+
			fg(cDim).Render("git identity · zsh + oh-my-zsh · dotfiles · macOS prefs")+
			fg(cFaint).Render(" — not just packages."))
	}
	return strings.Join(b, "\n")
}

func (m Model) renderLoadout(i int, l loadout, w int) string {
	preset, _ := config.GetPreset(l.preset)
	count := len(config.GetPackagesForPreset(l.preset))
	selected := i == m.loadCur

	cursor := "  "
	keyBox := fg(cMuted3).Render("[" + l.key + "]")
	name := fg(cText).Render(padTo(l.name, 11))
	if selected {
		cursor = fg(cAccent).Render("▸ ")
		keyBox = fg(cAccent).Render("[" + l.key + "]")
		name = fg(cTextHi).Bold(true).Render(padTo(l.name, 11))
	}

	left := cursor + keyBox + " " + name + " " + fg(cDim).Render(preset.Description)
	meta := fg(cMuted2).Render(fmt.Sprintf("%d pkgs", count)) + "  " +
		fg(cAccentHi).Render(fmt.Sprintf("~%d min", estMinutes(count)))
	line := bar(left, meta, w)
	// Highlight when the mouse hovers but this isn't the keyboard cursor — same
	// pattern as the select screen, so interactive rows read as clickable.
	if i == m.hoverRow {
		line = hoverBg(line)
	}
	return line
}

// estMinutes is a rough install-time estimate (~0.4 min/pkg), matching the
// design's back-of-envelope figure.
func estMinutes(pkgCount int) int {
	m := (pkgCount*2 + 4) / 5 // ~0.4 * count, rounded
	if m < 1 {
		return 1
	}
	return m
}

// ── boot: mouse ──

func (m Model) updateBootMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Hover tracking — highlight the loadout under the cursor.
	if msg.Action == tea.MouseActionMotion {
		m.hoverRow = m.bootHitTest(msg.X, msg.Y)
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if m.probeIdx < len(m.probes) {
		return m, nil // still probing — ignore clicks
	}
	if idx := m.bootHitTest(msg.X, msg.Y); idx >= 0 {
		m.loadCur = idx
		return m.pickLoadout(idx)
	}
	return m, nil
}

// bootHitTest maps a screen coordinate to a loadout index. The geometry mirrors
// bootBody: after probing is done the loadout list starts at a fixed position
// below the probes + "Choose a starting point" header.
func (m Model) bootHitTest(x, y int) int {
	// Not interactive while probing, and never for clicks on the chrome.
	if m.probeIdx < len(m.probes) || !m.inBody(y) {
		return -1
	}
	bodyRow := y - 1 // title bar is screen row 0
	// Header: blank + tagline + blank = 3 body rows
	// Probes: len(m.probes) rows
	// Footer before loadouts: blank + "Choose…" + blank = 3 body rows
	loadoutStart := 3 + len(m.probes) + 3
	idx := bodyRow - loadoutStart
	if idx >= 0 && idx < len(m.loadouts) {
		return idx
	}
	return -1
}
