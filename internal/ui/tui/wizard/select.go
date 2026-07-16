package wizard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/search"
)

// searchOnline is a seam so tests can stub the openboot.dev search client.
var searchOnline = search.SearchOnline

const (
	// onlineCatName is the synthetic sidebar category that holds packages
	// picked from openboot.dev search results (they aren't in the local
	// catalog, so without a home they'd vanish when the filter clears).
	onlineCatName  = "online"
	searchDebounce = 450 * time.Millisecond
	searchMinRunes = 2 // don't hit the network for 1-char queries
	onlineMaxHits  = 20
)

// searchTickMsg fires after the debounce delay; stale generations are dropped.
type searchTickMsg struct {
	seq   int
	query string
}

// searchDoneMsg carries the online results for generation seq.
type searchDoneMsg struct {
	seq     int
	results []config.Package
}

// categoriesFromConfig maps a remote config's package lists onto sidebar
// categories, so the select screen browses a config the same way it browses
// the catalog. Empty lists produce no category.
func categoriesFromConfig(rc *config.RemoteConfig) []config.Category {
	var cats []config.Category
	add := func(name string, entries config.PackageEntryList, cask, npm bool) {
		if len(entries) == 0 {
			return
		}
		pkgs := make([]config.Package, 0, len(entries))
		for _, e := range entries {
			pkgs = append(pkgs, config.Package{Name: e.Name, Description: e.Desc, IsCask: cask, IsNpm: npm})
		}
		cats = append(cats, config.Category{Name: name, Packages: pkgs})
	}
	add("cli tools", rc.Packages, false, false)
	add("apps", rc.Casks, true, false)
	add("npm", rc.Npm, false, true)
	return cats
}

// ── selection helpers ──

func (m Model) isInstalled(name string) bool { return m.installed[name] }

func (m *Model) toggle(name string) { m.selected[name] = !m.selected[name] }

// selCount is the number of packages the user has selected.
func (m Model) selCount() int {
	n := 0
	for _, v := range m.selected {
		if v {
			n++
		}
	}
	return n
}

// toInstallCount is the number of selected packages not already installed.
func (m Model) toInstallCount() int {
	n := 0
	for name, v := range m.selected {
		if v && !m.installed[name] {
			n++
		}
	}
	return n
}

// skippedCount is the number of selected packages already present.
func (m Model) skippedCount() int { return m.selCount() - m.toInstallCount() }

func (m Model) estMin() int { return estMinutes(m.toInstallCount()) }

// pool returns the packages shown in the right pane: the active category, or —
// when a query is present — a substring match across the whole catalog plus
// any online hits for the query (already deduped against the catalog).
func (m Model) pool() []config.Package {
	q := strings.TrimSpace(strings.ToLower(m.query))
	if q != "" {
		var out []config.Package
		for _, c := range m.cats {
			for _, p := range c.Packages {
				if strings.Contains(strings.ToLower(p.Name+" "+p.Description), q) {
					out = append(out, p)
				}
			}
		}
		return append(out, m.onlineResults...)
	}
	if m.catCur >= 0 && m.catCur < len(m.cats) {
		return m.cats[m.catCur].Packages
	}
	return nil
}

// ── online search (debounced openboot.dev lookup while filtering) ──

// queryChanged resets list position and (re)arms the search debounce for the
// current query. Every call bumps the generation, so in-flight lookups for a
// stale query can never land.
func (m *Model) queryChanged() tea.Cmd {
	m.rowCur, m.scroll = 0, 0
	m.onlineResults, m.onlineBusy = nil, false
	m.searchSeq++
	q := strings.TrimSpace(m.query)
	if len([]rune(q)) < searchMinRunes {
		return nil
	}
	seq := m.searchSeq
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchTickMsg{seq: seq, query: q}
	})
}

// clearOnline drops transient search state and invalidates in-flight lookups.
func (m *Model) clearOnline() {
	m.onlineResults, m.onlineBusy = nil, false
	m.searchSeq++
}

func (m Model) onSearchTick(msg searchTickMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.searchSeq || !m.typing || strings.TrimSpace(m.query) != msg.query {
		return m, nil // typed past it — a newer generation is armed
	}
	m.onlineBusy = true
	seq := msg.seq
	return m, func() tea.Msg {
		res, err := searchOnline(msg.query)
		if err != nil {
			res = nil // offline / API down: quietly show local hits only
		}
		return searchDoneMsg{seq: seq, results: res}
	}
}

func (m Model) onSearchDone(msg searchDoneMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.searchSeq {
		return m, nil
	}
	m.onlineBusy = false
	m.onlineResults = m.filterOnline(msg.results)
	for _, p := range m.onlineResults {
		m.onlineKnown[p.Name] = true
	}
	return m, nil
}

// filterOnline dedupes online hits against the catalog (including earlier
// online picks) and against themselves, and caps the list.
func (m Model) filterOnline(in []config.Package) []config.Package {
	seen := map[string]bool{}
	for _, c := range m.cats {
		for _, p := range c.Packages {
			seen[p.Name] = true
		}
	}
	var out []config.Package
	for _, p := range in {
		if p.Name == "" || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		out = append(out, p)
		if len(out) >= onlineMaxHits {
			break
		}
	}
	return out
}

// togglePkg flips selection for p. Online picks are materialised into the
// synthetic "online" sidebar category so they stay visible (and deselectable)
// after the filter clears — they have no home in the local catalog.
func (m *Model) togglePkg(p config.Package) {
	if m.isInstalled(p.Name) {
		return
	}
	m.toggle(p.Name)
	if !m.onlineKnown[p.Name] {
		return
	}
	if m.selected[p.Name] {
		m.addOnlinePick(p)
	} else {
		m.removeOnlinePick(p.Name)
	}
}

func (m *Model) addOnlinePick(p config.Package) {
	for i := range m.cats {
		if m.cats[i].Name != onlineCatName {
			continue
		}
		for _, q := range m.cats[i].Packages {
			if q.Name == p.Name {
				return
			}
		}
		m.cats[i].Packages = append(m.cats[i].Packages, p)
		return
	}
	m.cats = append(m.cats, config.Category{Name: onlineCatName, Packages: []config.Package{p}})
}

func (m *Model) removeOnlinePick(name string) {
	for i := range m.cats {
		if m.cats[i].Name != onlineCatName {
			continue
		}
		kept := m.cats[i].Packages[:0]
		for _, q := range m.cats[i].Packages {
			if q.Name != name {
				kept = append(kept, q)
			}
		}
		m.cats[i].Packages = kept
		if len(kept) == 0 {
			m.cats = append(m.cats[:i], m.cats[i+1:]...)
			if m.catCur >= len(m.cats) {
				m.catCur = len(m.cats) - 1
			}
		}
		return
	}
}

// selectedOnlinePkgs returns the online picks that are currently selected —
// the extra packages the plan needs beyond the catalog selection.
func (m Model) selectedOnlinePkgs() []config.Package {
	var out []config.Package
	for _, c := range m.cats {
		if c.Name != onlineCatName {
			continue
		}
		for _, p := range c.Packages {
			if m.selected[p.Name] {
				out = append(out, p)
			}
		}
	}
	return out
}

// selVisible is the number of package rows the list area can show. It must match
// selectList's own count (body height − search row − blank) exactly, or the
// scroll clamp and the render disagree: leaving blank rows at the bottom and
// rows the keyboard cursor can never reach. body height = m.height − title −
// status = m.height − 2; the list then spends 2 rows on the search + blank.
func (m Model) selVisible() int {
	v := m.height - 4
	if v < 3 {
		v = 3
	}
	return v
}

func (m Model) clampSelScroll() Model {
	visible := m.selVisible()
	if m.rowCur < m.scroll {
		m.scroll = m.rowCur
	}
	if m.rowCur >= m.scroll+visible {
		m.scroll = m.rowCur - visible + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	return m
}

// ── key handling ──

func (m Model) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocyclo // a keyboard dispatch table; splitting obscures the flow
	pool := m.pool()
	last := len(pool) - 1
	s := msg.String()

	if m.typing {
		var cmd tea.Cmd
		switch s {
		case "esc":
			m.query, m.typing, m.rowCur, m.scroll = "", false, 0, 0
			m.clearOnline()
		case "enter":
			if strings.TrimSpace(m.query) != "" && len(pool) > 0 {
				m.togglePkg(pool[clamp(m.rowCur, 0, last)])
				m.query, m.typing, m.rowCur, m.scroll = "", false, 0, 0
				m.clearOnline()
			} else {
				return m.tryInstall()
			}
		case "backspace":
			if len(m.query) > 0 {
				m.query = trimLast(m.query)
				cmd = m.queryChanged()
			}
		case "up":
			if m.rowCur > 0 {
				m.rowCur--
			}
		case "down":
			if m.rowCur < last {
				m.rowCur++
			}
		default:
			// KeyRunes covers both single keystrokes and multi-rune input
			// (bubbletea coalesces pasted/fast text into one message).
			if msg.Type == tea.KeyRunes || s == " " {
				m.query += s
				cmd = m.queryChanged()
			}
		}
		return m.clampSelScroll(), cmd
	}

	switch s {
	case "q":
		m.quit = true
		return m, tea.Quit
	case "/":
		// Filtering searches across categories, so it's a list operation —
		// pull focus to the list so ↑↓ behaves predictably after it clears.
		m.typing, m.query, m.rowCur, m.scroll, m.selFocus = true, "", 0, 0, focusList
		m.clearOnline()
	case "esc":
		m.query, m.rowCur, m.scroll = "", 0, 0
		m.clearOnline()
	case "left", "h":
		m.selFocus = focusCats
	case "right", "l":
		m.selFocus = focusList
	case "tab", "shift+tab":
		if m.selFocus == focusCats {
			m.selFocus = focusList
		} else {
			m.selFocus = focusCats
		}
	case "up", "k":
		m = m.selMoveUp()
	case "down", "j":
		m = m.selMoveDown()
	case " ":
		if len(pool) > 0 {
			m.togglePkg(pool[clamp(m.rowCur, 0, last)])
		}
	case "a":
		for _, p := range pool {
			if !m.isInstalled(p.Name) {
				m.selected[p.Name] = true
			}
		}
	case "x":
		m.selected = map[string]bool{}
	case "enter":
		return m.tryInstall()
	}
	return m.clampSelScroll(), nil
}

// selMoveUp/selMoveDown move the cursor within the focused pane: the category
// sidebar when it has focus (right pane live-previews the new category), the
// package list otherwise.
func (m Model) selMoveUp() Model {
	if m.selFocus == focusCats && m.query == "" {
		if m.catCur > 0 {
			m.catCur--
			m.rowCur, m.scroll = 0, 0
		}
		return m
	}
	if m.rowCur > 0 {
		m.rowCur--
	}
	return m
}

func (m Model) selMoveDown() Model {
	if m.selFocus == focusCats && m.query == "" {
		if m.catCur < len(m.cats)-1 {
			m.catCur++
			m.rowCur, m.scroll = 0, 0
		}
		return m
	}
	if m.rowCur < len(m.pool())-1 {
		m.rowCur++
	}
	return m
}

// ── mouse ──

type selHit int

const (
	hitNone selHit = iota
	hitCat
	hitPkg
)

// updateSelectMouse handles mouse events on the select screen: click a
// category to switch to it, click a package to toggle it, wheel to scroll the
// list, and hover to highlight the row under the cursor (no click needed).
func (m Model) updateSelectMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Hover tracking — fires on every mouse move even without a button pressed
	// (WithMouseAllMotion). Only highlight package rows, not categories or chrome.
	if msg.Action == tea.MouseActionMotion {
		kind, idx := m.selectHitTest(msg.X, msg.Y)
		if kind == hitPkg {
			m.hoverRow = idx
		} else {
			m.hoverRow = -1
		}
		return m.clampSelScroll(), nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.selFocus = focusList
		if m.rowCur > 0 {
			m.rowCur--
		}
	case tea.MouseButtonWheelDown:
		m.selFocus = focusList
		if m.rowCur < len(m.pool())-1 {
			m.rowCur++
		}
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		switch kind, idx := m.selectHitTest(msg.X, msg.Y); kind {
		case hitCat:
			m.catCur, m.query, m.typing = idx, "", false
			m.selFocus = focusCats
			m.rowCur, m.scroll = 0, 0
			m.clearOnline()
		case hitPkg:
			m.rowCur = idx
			m.selFocus = focusList
			pool := m.pool()
			if idx < len(pool) {
				m.togglePkg(pool[idx])
			}
		case hitNone:
			// click landed on chrome or blank space — nothing to do
		}
	default:
		// other buttons (middle, right, drag motion) aren't actionable here
		return m, nil
	}
	return m.clampSelScroll(), nil
}

// selectHitTest maps a screen coordinate to a category or package row. The
// geometry mirrors View + selectBody exactly: screen row 0 is the title bar so
// body row = y-1; the sidebar lists categories from body row 3, and the package
// list starts at body row 2 offset by the scroll. Kept a pure, testable
// function so the mapping is verified rather than eyeballed.
func (m Model) selectHitTest(x, y int) (selHit, int) {
	if !m.inBody(y) {
		return hitNone, -1
	}
	bodyRow := y - 1
	if x < sidebarW {
		if ci := bodyRow - 3; ci >= 0 && ci < len(m.cats) {
			return hitCat, ci
		}
		return hitNone, -1
	}
	if x == sidebarW {
		return hitNone, -1 // the vertical divider column — neither pane
	}
	if bodyRow < 2 {
		return hitNone, -1
	}
	pool := m.pool()
	if pj := m.scroll + bodyRow - 2; pj >= 0 && pj < len(pool) {
		return hitPkg, pj
	}
	return hitNone, -1
}

func (m Model) tryInstall() (tea.Model, tea.Cmd) {
	// Gate on a selection, not on new packages: an all-installed loadout still
	// carries git/shell/dotfiles/macOS steps worth reviewing and applying, so
	// zero *new* packages must not trap the user on the select screen.
	if m.selCount() == 0 {
		return m, nil // nothing selected — stay put
	}
	// Capture a git identity first when none is configured (fresh Mac), then
	// review the full plan before anything runs.
	if need, name, email := m.needsGitCapture(); need {
		m.gitName, m.gitEmail, m.gitField = name, email, 0
		m.screen, m.hoverRow = scrGit, -1
		return m, nil
	}
	return m.enterConfirm()
}

// ── rendering ──

const sidebarW = 22

func (m Model) selectBody(w, h int) string {
	left := m.selectSidebar(h)
	right := m.selectList(w-sidebarW-1, h)

	var lines []string
	for i := 0; i < h; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines = append(lines, padTo(truncCell(l, sidebarW), sidebarW)+fg(cBorder).Render("│")+r)
	}
	return strings.Join(lines, "\n")
}

func (m Model) selectSidebar(h int) []string {
	rows := make([]string, 0, h)
	rows = append(rows, "")
	rows = append(rows, " "+fg(cDim4).Render("CATALOG"))
	rows = append(rows, "")

	q := strings.TrimSpace(m.query)
	for i, c := range m.cats {
		selN, total := 0, len(c.Packages)
		for _, p := range c.Packages {
			if m.selected[p.Name] && !m.installed[p.Name] {
				selN++
			}
		}
		active := i == m.catCur && q == ""
		edge := " "
		nameStyle := fg(cDim)
		countStyle := fg(cDim4)
		if active {
			// A pointer arrow when the sidebar holds focus vs a plain bar when
			// it doesn't — a structural cue, so the focused pane reads without
			// relying on colour (piped output, colourblind users).
			if m.selFocus == focusCats {
				edge = fg(cAccent).Render("▸")
				nameStyle = fg(cTextHi)
			} else {
				edge = fg(cDim2).Render("▎")
				nameStyle = fg(cText)
			}
		}
		if selN > 0 {
			countStyle = fg(cAccentHi)
		}
		count := fmt.Sprintf("%d", total)
		if selN > 0 {
			count = fmt.Sprintf("%d/%d", selN, total)
		}
		name := nameStyle.Render(strings.ToLower(c.Name))
		rows = append(rows, bar(edge+" "+name, countStyle.Render(count)+" ", sidebarW))
	}

	// Footer pinned to the bottom of the sidebar.
	footer := []string{
		truncCell(fg(cAccent).Bold(true).Render(fmt.Sprintf("%d", m.selCount()))+
			fg(cDim).Render(fmt.Sprintf(" selected · ~%d min", m.estMin())), sidebarW),
	}
	if sk := m.skippedCount(); sk > 0 {
		footer = append(footer, truncCell(fg(cDim4).Render(fmt.Sprintf("%d already installed", sk)), sidebarW))
	}
	for len(rows) < h-len(footer)-1 {
		rows = append(rows, "")
	}
	rows = append(rows, "")
	rows = append(rows, footer...)
	return rows
}

func (m Model) selectList(w, h int) []string {
	rows := make([]string, 0, h)

	// Search / filter row.
	icon := fg(cDim3).Render("/")
	if strings.TrimSpace(m.query) != "" {
		icon = fg(cWarn).Bold(true).Render("/")
	}
	pool := m.pool()
	queryText := m.query
	if m.typing {
		queryText += "▌"
	}
	if queryText == "" {
		queryText = fg(cDim4).Render("type to filter — enter toggles the top hit")
	} else {
		queryText = fg(cTextHi).Render(queryText)
	}
	match := ""
	if strings.TrimSpace(m.query) != "" {
		hits := fmt.Sprintf("%d hits", len(pool))
		switch {
		case m.onlineBusy:
			hits += " · " + m.spinner() + " openboot.dev"
		case len(m.onlineResults) > 0:
			hits += fmt.Sprintf(" · %d from openboot.dev", len(m.onlineResults))
		}
		match = fg(cDim3).Render(hits)
	}
	rows = append(rows, bar("  "+icon+"  "+queryText, match+" ", w))
	rows = append(rows, "")

	// Package rows.
	visible := h - len(rows)
	m2 := m.clampSelScroll()
	start := m2.scroll
	end := start + visible
	if end > len(pool) {
		end = len(pool)
	}
	for i := start; i < end; i++ {
		line := m.renderRow(pool[i], i == m2.rowCur, w)
		// Subtle background highlight on the row the mouse is hovering over, so the
		// user sees where a click would land — including when it's also the
		// keyboard cursor, so the "clickable" affordance is never dropped.
		if i == m.hoverRow {
			line = hoverBg(line)
		}
		rows = append(rows, line)
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return rows
}

func (m Model) renderRow(p config.Package, cursor bool, w int) string {
	installed := m.installed[p.Name]
	on := m.selected[p.Name]

	box := fg(cDim3).Render("◯")
	switch {
	case installed:
		box = fg(cInstalled).Render("✓")
	case on:
		box = fg(cAccent).Render("◉")
	}

	listFocused := m.selFocus == focusList
	nameStyle := fg(cMuted)
	switch {
	case installed:
		nameStyle = fg(cDim2)
	case cursor && listFocused:
		nameStyle = fg(cWhite).Bold(true)
	case cursor, on:
		nameStyle = fg(cText)
	}

	tail := fg(cDim3).Render(pkgType(p))
	switch {
	case installed:
		tail = fg(cInstalled).Render("installed")
	case m.onlineKnown[p.Name]:
		// Cyan type badge marks a row sourced from openboot.dev search rather
		// than the local catalog.
		tail = fg(cInfo).Render(pkgType(p))
	}

	name := padTo(nameStyle.Render(p.Name), 20)
	rowPrefix := "  "
	if cursor && listFocused {
		// The cursor arrow shows only when the list holds focus, so exactly one
		// pane displays a pointer at a time — the focus cue, without colour.
		rowPrefix = fg(cAccent).Render("› ")
	}
	left := rowPrefix + box + "  " + name + " " + fg(cDim).Render(p.Description)
	return bar(truncCell(left, w-12), tail+" ", w)
}

func pkgType(p config.Package) string {
	switch {
	case p.IsNpm:
		return "npm"
	case p.IsCask:
		return "cask"
	default:
		return "brew"
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
