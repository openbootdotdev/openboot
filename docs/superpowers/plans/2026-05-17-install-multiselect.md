# Install-time Package Multi-Select Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a TUI multi-select (`[Y/c/n]` prompt → `ConfigCustomizer`) and a scripted `--pick=a,b,c` flag for `openboot install` against remote configs (cloud, file, sync source).

**Architecture:** New pure function `cli.ApplyPicks(rc, picks)` filters `RemoteConfig.{Packages,Casks,Npm}` by name. New `ui.ConfigCustomizerModel` (bubbletea, modeled on `SnapshotEditorModel`) lets the user toggle items and returns a `picks` set fed into the same `ApplyPicks`. CLI layer dispatches between `--pick`, the interactive prompt, and silent default.

**Tech Stack:** Go 1.24, Cobra (CLI), Charmbracelet bubbletea + huh (TUI), testify (tests). Spec: `docs/superpowers/specs/2026-05-17-install-multiselect-design.md`.

---

## File Structure

**New**
- `internal/cli/pick.go` — `ParsePicks`, `ApplyPicks` pure functions.
- `internal/cli/pick_test.go` — table-driven unit tests.
- `internal/ui/config_customizer.go` — `ConfigCustomizerModel` bubbletea Model + `RunConfigCustomizer`.
- `internal/ui/config_customizer_test.go` — Model state-transition tests.

**Modified**
- `internal/cli/install.go` — add `--pick` flag; insert customize dispatch in `runInstallCmd` and `runSyncInstall`.
- `internal/installer/installer.go` — remove the in-installer `ui.Confirm("Install these packages?")` block (the prompt now lives at the CLI layer).
- `internal/config/types.go` — no changes (read-only).

---

## Task 1: `ParsePicks` and `ApplyPicks` pure functions

**Files:**
- Create: `internal/cli/pick.go`
- Create: `internal/cli/pick_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/cli/pick_test.go
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

func TestParsePicks(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]bool
	}{
		{"empty", "", map[string]bool{}},
		{"single", "git", map[string]bool{"git": true}},
		{"multiple", "git,jq,ripgrep", map[string]bool{"git": true, "jq": true, "ripgrep": true}},
		{"whitespace trimmed", " git , jq ,ripgrep ", map[string]bool{"git": true, "jq": true, "ripgrep": true}},
		{"empty entries skipped", "git,,jq,", map[string]bool{"git": true, "jq": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePicks(tt.raw)
			assert.Equal(t, tt.want, got)
		})
	}
}

func sampleRemoteConfig() *config.RemoteConfig {
	return &config.RemoteConfig{
		Username: "alice",
		Slug:     "dev",
		Packages: config.PackageEntryList{{Name: "git"}, {Name: "jq"}, {Name: "ripgrep"}},
		Casks:    config.PackageEntryList{{Name: "visual-studio-code"}, {Name: "docker"}},
		Npm:      config.PackageEntryList{{Name: "typescript"}, {Name: "eslint"}},
		Taps:     []string{"homebrew/cask-fonts"},
		DotfilesRepo: "https://github.com/alice/dotfiles",
		PostInstall:  []string{"echo done"},
	}
}

func TestApplyPicks_FiltersFormulae(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, unknown := ApplyPicks(rc, map[string]bool{"git": true})
	require.Empty(t, unknown)
	assert.Equal(t, []string{"git"}, filtered.Packages.Names())
	assert.Empty(t, filtered.Casks)
	assert.Empty(t, filtered.Npm)
}

func TestApplyPicks_FiltersAcrossCategories(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, unknown := ApplyPicks(rc, map[string]bool{
		"git": true, "docker": true, "typescript": true,
	})
	require.Empty(t, unknown)
	assert.Equal(t, []string{"git"}, filtered.Packages.Names())
	assert.Equal(t, []string{"docker"}, filtered.Casks.Names())
	assert.Equal(t, []string{"typescript"}, filtered.Npm.Names())
}

func TestApplyPicks_PreservesNonPackageFields(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, _ := ApplyPicks(rc, map[string]bool{"git": true})
	assert.Equal(t, rc.Taps, filtered.Taps)
	assert.Equal(t, rc.DotfilesRepo, filtered.DotfilesRepo)
	assert.Equal(t, rc.PostInstall, filtered.PostInstall)
	assert.Equal(t, rc.Username, filtered.Username)
	assert.Equal(t, rc.Slug, filtered.Slug)
}

func TestApplyPicks_ReportsUnknownNames(t *testing.T) {
	rc := sampleRemoteConfig()
	_, unknown := ApplyPicks(rc, map[string]bool{"git": true, "nope": true, "alsonope": true})
	assert.ElementsMatch(t, []string{"nope", "alsonope"}, unknown)
}

func TestApplyPicks_EmptyPicksProducesEmptyLists(t *testing.T) {
	rc := sampleRemoteConfig()
	filtered, unknown := ApplyPicks(rc, map[string]bool{})
	assert.Empty(t, unknown)
	assert.Empty(t, filtered.Packages)
	assert.Empty(t, filtered.Casks)
	assert.Empty(t, filtered.Npm)
}

func TestApplyPicks_DoesNotMutateInput(t *testing.T) {
	rc := sampleRemoteConfig()
	origCount := len(rc.Packages)
	ApplyPicks(rc, map[string]bool{"git": true})
	assert.Equal(t, origCount, len(rc.Packages))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run TestParsePicks -v`
Expected: FAIL with "undefined: ParsePicks"

- [ ] **Step 3: Implement `pick.go`**

```go
// internal/cli/pick.go
package cli

import (
	"strings"

	"github.com/openbootdotdev/openboot/internal/config"
)

// ParsePicks splits a comma-separated --pick value into a set.
// Whitespace around names is trimmed; empty entries are skipped.
func ParsePicks(raw string) map[string]bool {
	out := map[string]bool{}
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out[name] = true
	}
	return out
}

// ApplyPicks returns a copy of rc whose Packages, Casks, and Npm
// slices contain only entries whose Name appears in picks. Taps,
// dotfiles, shell, macOS prefs, post-install, and other fields are
// passed through unchanged. Any names in picks that didn't match any
// package are returned in unknown so the caller can fail fast (--pick)
// or ignore (TUI, where picks come from rc itself).
func ApplyPicks(rc *config.RemoteConfig, picks map[string]bool) (filtered *config.RemoteConfig, unknown []string) {
	cp := *rc
	cp.Packages = filterEntries(rc.Packages, picks)
	cp.Casks = filterEntries(rc.Casks, picks)
	cp.Npm = filterEntries(rc.Npm, picks)

	matched := map[string]bool{}
	for _, e := range cp.Packages {
		matched[e.Name] = true
	}
	for _, e := range cp.Casks {
		matched[e.Name] = true
	}
	for _, e := range cp.Npm {
		matched[e.Name] = true
	}

	for name := range picks {
		if !matched[name] {
			unknown = append(unknown, name)
		}
	}
	return &cp, unknown
}

func filterEntries(in config.PackageEntryList, picks map[string]bool) config.PackageEntryList {
	out := make(config.PackageEntryList, 0, len(in))
	for _, e := range in {
		if picks[e.Name] {
			out = append(out, e)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestParsePicks|TestApplyPicks' -v`
Expected: PASS for all 8 subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/pick.go internal/cli/pick_test.go
git commit -m "feat(cli): add ParsePicks/ApplyPicks for install --pick

Pure functions that parse a comma-separated --pick value and filter a
RemoteConfig to only the named packages. Used by both the --pick flag
and the upcoming ConfigCustomizer TUI."
```

---

## Task 2: `ConfigCustomizer` TUI model

**Files:**
- Create: `internal/ui/config_customizer.go`
- Create: `internal/ui/config_customizer_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/ui/config_customizer_test.go
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)

func sampleCustomizerConfig() *config.RemoteConfig {
	return &config.RemoteConfig{
		Username: "alice", Slug: "dev",
		Packages: config.PackageEntryList{{Name: "git", Desc: "version control"}, {Name: "jq"}, {Name: "ripgrep"}},
		Casks:    config.PackageEntryList{{Name: "visual-studio-code"}, {Name: "docker"}},
		Npm:      config.PackageEntryList{{Name: "typescript"}, {Name: "eslint"}},
	}
}

func TestNewConfigCustomizer_BuildsThreeTabs(t *testing.T) {
	rc := sampleCustomizerConfig()
	m := NewConfigCustomizer(rc)
	require.Len(t, m.tabs, 3)
	assert.Equal(t, "Formulae", m.tabs[0].name)
	assert.Equal(t, "Casks", m.tabs[1].name)
	assert.Equal(t, "NPM", m.tabs[2].name)
	assert.Equal(t, 3, len(m.tabs[0].items))
	assert.Equal(t, 2, len(m.tabs[1].items))
	assert.Equal(t, 2, len(m.tabs[2].items))
}

func TestNewConfigCustomizer_AllItemsPreselected(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	for _, tab := range m.tabs {
		for _, item := range tab.items {
			assert.True(t, item.selected, "%s should be preselected", item.name)
		}
	}
}

func TestConfigCustomizer_SpaceToggles(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// cursor at first item ("git"), press Space
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)
	assert.False(t, m.tabs[0].items[0].selected)
}

func TestConfigCustomizer_TabSwitchesCategory(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(ConfigCustomizerModel)
	assert.Equal(t, 1, m.activeTab)
}

func TestConfigCustomizer_SelectAllInTab(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// Deselect first item in tab 0 so not all are selected
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)
	// Press 'a' — should select all (because not all were selected)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(ConfigCustomizerModel)
	for _, item := range m.tabs[0].items {
		assert.True(t, item.selected)
	}
	// Press 'a' again — should deselect all
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(ConfigCustomizerModel)
	for _, item := range m.tabs[0].items {
		assert.False(t, item.selected)
	}
}

func TestConfigCustomizer_EnterConfirms(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ConfigCustomizerModel)
	assert.True(t, m.confirmed)
}

func TestConfigCustomizer_PicksReflectsSelections(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// Deselect "git" (tab 0, cursor 0)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)

	picks := m.Picks()
	assert.False(t, picks["git"])
	assert.True(t, picks["jq"])
	assert.True(t, picks["ripgrep"])
	assert.True(t, picks["visual-studio-code"])
	assert.True(t, picks["docker"])
	assert.True(t, picks["typescript"])
	assert.True(t, picks["eslint"])
}

func TestConfigCustomizer_PicksOnlyIncludesSelected(t *testing.T) {
	m := NewConfigCustomizer(sampleCustomizerConfig())
	// Deselect "git" via Space
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(ConfigCustomizerModel)

	picks := m.Picks()
	// "git" should be absent from the map entirely (not present as false)
	_, present := picks["git"]
	assert.False(t, present, "deselected items should not appear in Picks()")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run TestConfigCustomizer -v`
Expected: FAIL with "undefined: NewConfigCustomizer"

- [ ] **Step 3: Implement `config_customizer.go`**

```go
// internal/ui/config_customizer.go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/openbootdotdev/openboot/internal/config"
)

type customizerItem struct {
	name        string
	description string
	selected    bool
}

type customizerTab struct {
	name  string
	icon  string
	items []customizerItem
}

// ConfigCustomizerModel is a bubbletea Model that lets the user toggle which
// packages from a RemoteConfig should be installed. Three tabs: Formulae,
// Casks, NPM. All items are preselected on entry.
type ConfigCustomizerModel struct {
	tabs         []customizerTab
	activeTab    int
	cursor       int
	scrollOffset int
	width        int
	height       int
	confirmed    bool
}

// NewConfigCustomizer builds the model from a RemoteConfig.
func NewConfigCustomizer(rc *config.RemoteConfig) ConfigCustomizerModel {
	formulae := make([]customizerItem, len(rc.Packages))
	for i, e := range rc.Packages {
		formulae[i] = customizerItem{name: e.Name, description: e.Desc, selected: true}
	}
	casks := make([]customizerItem, len(rc.Casks))
	for i, e := range rc.Casks {
		casks[i] = customizerItem{name: e.Name, description: e.Desc, selected: true}
	}
	npm := make([]customizerItem, len(rc.Npm))
	for i, e := range rc.Npm {
		npm[i] = customizerItem{name: e.Name, description: e.Desc, selected: true}
	}
	return ConfigCustomizerModel{
		tabs: []customizerTab{
			{name: "Formulae", icon: "🍺", items: formulae},
			{name: "Casks", icon: "📦", items: casks},
			{name: "NPM", icon: "📜", items: npm},
		},
	}
}

func (m ConfigCustomizerModel) Init() tea.Cmd { return nil }

func (m ConfigCustomizerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, keys.Tab), key.Matches(msg, keys.Right):
			m.activeTab = (m.activeTab + 1) % len(m.tabs)
			m.cursor = 0
			m.scrollOffset = 0

		case key.Matches(msg, keys.ShiftTab), key.Matches(msg, keys.Left):
			m.activeTab = (m.activeTab - 1 + len(m.tabs)) % len(m.tabs)
			m.cursor = 0
			m.scrollOffset = 0

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scrollOffset {
					m.scrollOffset = m.cursor
				}
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.tabs[m.activeTab].items)-1 {
				m.cursor++
				visible := m.getVisibleItems()
				if m.cursor >= m.scrollOffset+visible {
					m.scrollOffset = m.cursor - visible + 1
				}
			}

		case key.Matches(msg, keys.Space):
			tab := &m.tabs[m.activeTab]
			if m.cursor < len(tab.items) {
				tab.items[m.cursor].selected = !tab.items[m.cursor].selected
			}

		case key.Matches(msg, keys.SelectAll):
			tab := &m.tabs[m.activeTab]
			allSelected := true
			for _, item := range tab.items {
				if !item.selected {
					allSelected = false
					break
				}
			}
			for i := range tab.items {
				tab.items[i].selected = !allSelected
			}

		case key.Matches(msg, keys.Enter):
			m.confirmed = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ConfigCustomizerModel) View() string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, activeTabStyle.Render("📋 Customize packages"))
	lines = append(lines, "")

	// Tab bar
	var tabParts []string
	for i, tab := range m.tabs {
		selectedCount := 0
		for _, item := range tab.items {
			if item.selected {
				selectedCount++
			}
		}
		label := fmt.Sprintf("%s (%d/%d)", tab.name, selectedCount, len(tab.items))
		if i == m.activeTab {
			tabParts = append(tabParts, activeTabStyle.Render(label))
		} else {
			tabParts = append(tabParts, itemStyle.Render(label))
		}
	}
	lines = append(lines, strings.Join(tabParts, "  │  "))
	lines = append(lines, "")

	// Items
	tab := m.tabs[m.activeTab]
	visible := m.getVisibleItems()
	if len(tab.items) == 0 {
		lines = append(lines, descStyle.Render("  (no items)"))
	} else {
		end := m.scrollOffset + visible
		if end > len(tab.items) {
			end = len(tab.items)
		}
		for i := m.scrollOffset; i < end; i++ {
			item := tab.items[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			checkbox := "[ ]"
			style := itemStyle
			if item.selected {
				checkbox = "[✓]"
				style = selectedStyle
			}
			line := fmt.Sprintf("%s%s %s", cursor, checkbox, style.Render(item.name))
			if item.description != "" {
				line += " " + descStyle.Render(item.description)
			}
			lines = append(lines, padLine(truncateLine(line, m.width), m.width))
		}
	}

	lines = append(lines, "")
	lines = append(lines, countStyle.Render(m.summary()))
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("Space: toggle • a: all in tab • Tab/←→: switch • Enter: confirm • q: cancel"))
	return padAllLines(strings.Join(lines, "\n"), m.width)
}

func (m ConfigCustomizerModel) getVisibleItems() int {
	if m.height == 0 {
		return 15
	}
	available := m.height - 10
	if available < 5 {
		available = 5
	}
	if available > 20 {
		available = 20
	}
	return available
}

func (m ConfigCustomizerModel) summary() string {
	var parts []string
	for _, tab := range m.tabs {
		count := 0
		for _, item := range tab.items {
			if item.selected {
				count++
			}
		}
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, strings.ToLower(tab.name)))
		}
	}
	if len(parts) == 0 {
		return "nothing selected"
	}
	return strings.Join(parts, ", ") + " selected"
}

// Picks returns the set of selected item names across all tabs.
func (m ConfigCustomizerModel) Picks() map[string]bool {
	out := map[string]bool{}
	for _, tab := range m.tabs {
		for _, item := range tab.items {
			if item.selected {
				out[item.name] = true
			}
		}
	}
	return out
}

// Confirmed returns true if the user pressed Enter to confirm.
func (m ConfigCustomizerModel) Confirmed() bool { return m.confirmed }

// RunConfigCustomizer launches the customizer TUI and returns the user's
// picks and whether they confirmed. If they cancelled (q / Ctrl-C), picks
// is nil and confirmed is false.
func RunConfigCustomizer(rc *config.RemoteConfig) (picks map[string]bool, confirmed bool, err error) {
	p := tea.NewProgram(NewConfigCustomizer(rc), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, false, err
	}
	m := finalModel.(ConfigCustomizerModel)
	if !m.Confirmed() {
		return nil, false, nil
	}
	return m.Picks(), true, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run TestConfigCustomizer -v`
Expected: PASS for all 8 tests.

- [ ] **Step 5: Verify build still passes**

Run: `go vet ./... && go build ./...`
Expected: no output, no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/config_customizer.go internal/ui/config_customizer_test.go
git commit -m "feat(ui): add ConfigCustomizer TUI for install-time package selection

bubbletea Model with 3 tabs (Formulae / Casks / NPM) modeled on
SnapshotEditor. All items preselected; Space toggles, a toggles all
in tab, Tab/←→ switches, Enter confirms, q cancels. Returns the set
of selected names for ApplyPicks to filter the RemoteConfig."
```

---

## Task 3: Wire `--pick` flag (silent + interactive bypass)

**Files:**
- Modify: `internal/cli/install.go`
- Test: `internal/cli/install_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/cli/install_test.go`:

```go
func TestApplyPickFlagToRemoteConfig_FiltersPackages(t *testing.T) {
	rc := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}, {Name: "jq"}},
		Casks:    config.PackageEntryList{{Name: "docker"}},
	}
	out, err := applyPickFlagToRemoteConfig(rc, "git,docker")
	require.NoError(t, err)
	assert.Equal(t, []string{"git"}, out.Packages.Names())
	assert.Equal(t, []string{"docker"}, out.Casks.Names())
}

func TestApplyPickFlagToRemoteConfig_FailsOnUnknown(t *testing.T) {
	rc := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}},
	}
	_, err := applyPickFlagToRemoteConfig(rc, "git,nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

func TestApplyPickFlagToRemoteConfig_EmptyIsNoOp(t *testing.T) {
	rc := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}},
	}
	out, err := applyPickFlagToRemoteConfig(rc, "")
	require.NoError(t, err)
	assert.Equal(t, rc, out)
}
```

Make sure these imports are present in `install_test.go`:

```go
import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/config"
)
```

(The first three may already be there — add only what's missing.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run TestApplyPickFlagToRemoteConfig -v`
Expected: FAIL with "undefined: applyPickFlagToRemoteConfig"

- [ ] **Step 3: Add the `--pick` flag binding and helper in `install.go`**

In `internal/cli/install.go`, find the `init()` function (around line 60) and add the new flag immediately after the existing `BoolVar` for `PackagesOnly`:

```go
installCmd.Flags().StringVar(&installCfg.PackagesOnly, "packages-only", false, "install packages only, skip system config")
```

→ Add right after it:

```go
installCmd.Flags().String("pick", "", "comma-separated list of package names to install from a remote config (silent and interactive); fails if any name is unknown")
```

Then add the helper function at the bottom of `install.go`:

```go
// applyPickFlagToRemoteConfig filters rc to the packages named in pickRaw.
// Empty pickRaw is a no-op. Unknown names return an error listing them.
func applyPickFlagToRemoteConfig(rc *config.RemoteConfig, pickRaw string) (*config.RemoteConfig, error) {
	if pickRaw == "" {
		return rc, nil
	}
	picks := ParsePicks(pickRaw)
	filtered, unknown := ApplyPicks(rc, picks)
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown package(s) in --pick: %s. Run with --dry-run to see available names", strings.Join(unknown, ", "))
	}
	return filtered, nil
}
```

- [ ] **Step 4: Wire `--pick` into `runInstallCmd`**

In `internal/cli/install.go`, modify `runInstallCmd` (currently around lines 99–125). After `applyInstallSource(src)` succeeds and before `installer.Run(installCfg)`, insert the dispatch:

Current:
```go
		if err := applyInstallSource(src); err != nil {
			return err
		}
	}

	err := installer.Run(installCfg)
```

Replace with:
```go
		if err := applyInstallSource(src); err != nil {
			return err
		}
	}

	if installCfg.RemoteConfig != nil {
		pickRaw, _ := cmd.Flags().GetString("pick")
		if pickRaw != "" {
			rc, perr := applyPickFlagToRemoteConfig(installCfg.RemoteConfig, pickRaw)
			if perr != nil {
				return perr
			}
			installCfg.RemoteConfig = rc
		}
	} else {
		// --pick requires a remote config (file / cloud / sync source).
		if pickRaw, _ := cmd.Flags().GetString("pick"); pickRaw != "" {
			return fmt.Errorf("--pick requires a remote config; use the preset selector instead")
		}
	}

	err := installer.Run(installCfg)
```

- [ ] **Step 5: Run all tests + vet**

Run: `go test ./internal/cli/ -v && go vet ./...`
Expected: all PASS, no vet errors.

- [ ] **Step 6: Manual smoke test with dry-run + a fixture config**

Create a temp fixture and verify the flag works end-to-end:

```bash
cat > /tmp/openboot-test.json <<'EOF'
{
  "username": "test", "slug": "fixture",
  "packages": [{"name":"git"},{"name":"jq"},{"name":"ripgrep"}],
  "casks": [{"name":"docker"}],
  "npm": []
}
EOF

go run ./cmd/openboot install --from /tmp/openboot-test.json --pick=git,docker --dry-run --silent 2>&1 | head -40
```

Expected output should show only git and docker being planned, not jq or ripgrep.

Then verify the error path:

```bash
go run ./cmd/openboot install --from /tmp/openboot-test.json --pick=git,nope --dry-run --silent 2>&1
```

Expected: exit non-zero with "unknown package(s) in --pick: nope".

- [ ] **Step 7: Commit**

```bash
git add internal/cli/install.go internal/cli/install_test.go
git commit -m "feat(cli): add --pick flag to install for scripted package subsets

--pick=a,b,c filters a remote config down to the named packages before
install. Works with --from, -u, sync source, in both silent and
interactive modes. Unknown names fail fast. --pick + preset returns
a clear error."
```

---

## Task 4: Three-way `[Y/c/n]` prompt + customize dispatch

**Files:**
- Modify: `internal/cli/install.go`
- Modify: `internal/installer/installer.go`
- Test: `internal/cli/install_test.go`

- [ ] **Step 1: Write the failing test for the dispatch helper**

Append to `internal/cli/install_test.go`:

```go
func TestPromptCustomizeChoice_Constants(t *testing.T) {
	// Verify the choice constants are distinct strings used by SelectOption.
	assert.NotEqual(t, customizeChoiceAll, customizeChoiceCustomize)
	assert.NotEqual(t, customizeChoiceAll, customizeChoiceCancel)
	assert.NotEqual(t, customizeChoiceCustomize, customizeChoiceCancel)
}
```

(Behavior is mostly TUI-dispatch — we keep the unit test light and rely on the manual smoke test in Step 5 plus the existing installer integration tests for confidence.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestPromptCustomizeChoice_Constants -v`
Expected: FAIL with "undefined: customizeChoiceAll"

- [ ] **Step 3: Add the prompt helper to `install.go`**

At the bottom of `internal/cli/install.go`, add:

```go
const (
	customizeChoiceAll       = "Install all"
	customizeChoiceCustomize = "Customize (pick packages)"
	customizeChoiceCancel    = "Cancel"
)

// promptCustomizeAndApply shows a 3-way prompt before installing from a remote
// config. Returns the rc to install (possibly filtered by user's picks) and
// whether the user wants to continue. When cancelled, returns (nil, false, nil).
func promptCustomizeAndApply(rc *config.RemoteConfig) (*config.RemoteConfig, bool, error) {
	fmt.Println()
	ui.Info(fmt.Sprintf("→ %s/%s", rc.Username, rc.Slug))
	ui.Muted(fmt.Sprintf("  CLI tools: %d", len(rc.Packages)))
	ui.Muted(fmt.Sprintf("  Apps: %d", len(rc.Casks)))
	if len(rc.Npm) > 0 {
		ui.Muted(fmt.Sprintf("  npm: %d", len(rc.Npm)))
	}
	fmt.Println()

	choice, err := ui.SelectOption(
		fmt.Sprintf("Install %d packages?", len(rc.Packages)+len(rc.Casks)+len(rc.Npm)),
		[]string{customizeChoiceAll, customizeChoiceCustomize, customizeChoiceCancel},
	)
	if err != nil {
		return nil, false, fmt.Errorf("prompt: %w", err)
	}

	switch choice {
	case customizeChoiceAll:
		return rc, true, nil
	case customizeChoiceCustomize:
		picks, confirmed, err := ui.RunConfigCustomizer(rc)
		if err != nil {
			return nil, false, fmt.Errorf("customizer: %w", err)
		}
		if !confirmed {
			return nil, false, nil
		}
		filtered, _ := ApplyPicks(rc, picks)
		return filtered, true, nil
	default: // Cancel
		return nil, false, nil
	}
}
```

- [ ] **Step 4: Wire the prompt into `runInstallCmd`**

In `internal/cli/install.go`, the dispatch from Task 3 currently handles only the `--pick` case. Extend it to handle the interactive prompt path. Replace the block added in Task 3 Step 4 with:

```go
	if installCfg.RemoteConfig != nil {
		pickRaw, _ := cmd.Flags().GetString("pick")
		if pickRaw != "" {
			rc, perr := applyPickFlagToRemoteConfig(installCfg.RemoteConfig, pickRaw)
			if perr != nil {
				return perr
			}
			installCfg.RemoteConfig = rc
		} else if !installCfg.Silent && (!installCfg.DryRun || system.HasTTY()) {
			rc, proceed, err := promptCustomizeAndApply(installCfg.RemoteConfig)
			if err != nil {
				return err
			}
			if !proceed {
				ui.Info("Cancelled.")
				return nil
			}
			installCfg.RemoteConfig = rc
		}
	} else {
		if pickRaw, _ := cmd.Flags().GetString("pick"); pickRaw != "" {
			return fmt.Errorf("--pick requires a remote config; use the preset selector instead")
		}
	}
```

You'll need a new import in `install.go` (likely already present, verify):

```go
"github.com/openbootdotdev/openboot/internal/system"
```

- [ ] **Step 5: Remove the now-redundant `ui.Confirm` from `installer.go`**

In `internal/installer/installer.go`, find the block in `runInstall` that shows the remote-config confirmation (lines ~76–91):

```go
	// Remote-config installs: show what will be installed and confirm before proceeding.
	if plan.RemoteConfig != nil && !opts.Silent && !opts.DryRun {
		ui.Info(fmt.Sprintf("Custom config: @%s/%s", plan.RemoteConfig.Username, plan.RemoteConfig.Slug))
		fmt.Println()
		printPackageList("CLI tools", plan.RemoteConfig.Packages)
		printPackageList("Apps", plan.RemoteConfig.Casks)
		printPackageList("npm", plan.RemoteConfig.Npm)
		fmt.Println()
		proceed, err := ui.Confirm("Install these packages?", true)
		if err != nil {
			return err
		}
		if !proceed {
			return ErrUserCancelled
		}
		fmt.Println()
	}
```

Delete the entire block — the prompt is now in `runInstallCmd` at the CLI layer. After deletion the surrounding flow should be:

```go
	// Write resolved selections back to st so callers that hold a reference to
	// st (e.g. Run → cfg.ApplyState) can observe the final selected packages.
	if plan.SelectedPkgs != nil {
		st.SelectedPkgs = plan.SelectedPkgs
	}
	if plan.OnlinePkgs != nil {
		st.OnlinePkgs = plan.OnlinePkgs
	}

	return Apply(plan, ConsoleReporter{})
```

The `printPackageList` function and its definition stay (other callers may exist; verify with `grep printPackageList internal/installer/`).

- [ ] **Step 6: Run all tests + vet**

Run: `go test ./internal/cli/ ./internal/installer/ -v && go vet ./...`
Expected: all PASS, no vet errors. If `TestRunInstall_RemoteConfig_PromptsUser` (or similar) fails because it expected the in-installer confirmation, update the test to no longer expect that prompt — the installer is now silent for remote configs.

- [ ] **Step 7: Manual smoke test of interactive flow**

Note: this is an interactive TUI test — you must run it in your terminal, not in a captured-output context.

```bash
go run ./cmd/openboot install --from /tmp/openboot-test.json --dry-run
```

Verify:
1. You see the `→ test/fixture` header and counts
2. The 3-way prompt appears: "Install N packages?" with options "Install all / Customize (pick packages) / Cancel"
3. Selecting "Install all" → dry-run output shows all packages
4. Selecting "Customize" → TUI opens; you can Tab between Formulae/Casks/NPM, Space to deselect; Enter applies; dry-run output reflects subset
5. Selecting "Cancel" → exits with "Cancelled."

- [ ] **Step 8: Commit**

```bash
git add internal/cli/install.go internal/installer/installer.go internal/cli/install_test.go
git commit -m "feat(install): [Y/c/n] prompt with customize TUI for remote configs

Replaces the in-installer 'Install these packages? [Y/n]' with a CLI-
layer 3-way prompt (Install all / Customize / Cancel). 'Customize'
opens the ConfigCustomizer TUI; selections feed back through ApplyPicks
to produce a filtered RemoteConfig. --pick still bypasses the prompt."
```

---

## Task 5: Apply the same prompt + customize to the sync-source path

**Files:**
- Modify: `internal/cli/install.go` (`runSyncInstall`)

- [ ] **Step 1: Read the current `runSyncInstall` carefully**

Run: `grep -n "runSyncInstall\|Apply.*change\|ui.Confirm" internal/cli/install.go`

Confirm the function spans roughly lines 257–324 with the prompt at line 295.

- [ ] **Step 2: Add a helper that builds a "diff-additions only" RemoteConfig**

At the bottom of `internal/cli/install.go`:

```go
// remoteConfigFromSyncDiffAdditions returns a copy of rc trimmed to only the
// items present in the diff's "missing" sets — i.e. what would be newly added
// by this sync. Used to scope the customize TUI to the diff, not the whole
// subscribed config.
func remoteConfigFromSyncDiffAdditions(rc *config.RemoteConfig, diff *syncpkg.SyncDiff) *config.RemoteConfig {
	pickSet := map[string]bool{}
	for _, n := range diff.MissingFormulae {
		pickSet[n] = true
	}
	for _, n := range diff.MissingCasks {
		pickSet[n] = true
	}
	for _, n := range diff.MissingNpm {
		pickSet[n] = true
	}
	filtered, _ := ApplyPicks(rc, pickSet)
	return filtered
}
```

- [ ] **Step 3: Modify `runSyncInstall` to use the 3-way prompt**

In `runSyncInstall`, find the existing prompt block (around lines 293–303):

```go
	if !installCfg.Silent {
		confirmed, err := ui.Confirm(fmt.Sprintf("Apply %d change(s) from %s?", missingCount, label), true)
		if err != nil {
			return fmt.Errorf("confirm: %w", err)
		}
		if !confirmed {
			ui.Info("Cancelled.")
			return nil
		}
	}
```

Replace with:

```go
	if !installCfg.Silent {
		choice, err := ui.SelectOption(
			fmt.Sprintf("Apply %d change(s) from %s?", missingCount, label),
			[]string{customizeChoiceAll, customizeChoiceCustomize, customizeChoiceCancel},
		)
		if err != nil {
			return fmt.Errorf("prompt: %w", err)
		}
		switch choice {
		case customizeChoiceAll:
			// fall through, build plan from full diff below
		case customizeChoiceCustomize:
			additionsRC := remoteConfigFromSyncDiffAdditions(rc, diff)
			picks, confirmed, err := ui.RunConfigCustomizer(additionsRC)
			if err != nil {
				return fmt.Errorf("customizer: %w", err)
			}
			if !confirmed {
				ui.Info("Cancelled.")
				return nil
			}
			// Re-scope the diff to only the picked items.
			diff = filterSyncDiffByPicks(diff, picks)
			missingCount = diff.TotalMissing() + diff.TotalChanged()
			if missingCount == 0 {
				ui.Info("Nothing selected — exiting.")
				return nil
			}
		case customizeChoiceCancel:
			ui.Info("Cancelled.")
			return nil
		}
	}
```

- [ ] **Step 4: Add the `filterSyncDiffByPicks` helper**

At the bottom of `install.go`:

```go
// filterSyncDiffByPicks returns a SyncDiff whose Missing* lists are restricted
// to entries whose name is in picks. Non-package "Changed" categories (theme,
// dotfiles, macOS prefs, shell) are preserved unchanged — picks are
// package-only per the spec.
func filterSyncDiffByPicks(diff *syncpkg.SyncDiff, picks map[string]bool) *syncpkg.SyncDiff {
	out := *diff
	out.MissingFormulae = filterStrings(diff.MissingFormulae, picks)
	out.MissingCasks = filterStrings(diff.MissingCasks, picks)
	out.MissingNpm = filterStrings(diff.MissingNpm, picks)
	return &out
}

func filterStrings(in []string, keep map[string]bool) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if keep[s] {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 5: Handle `--pick` in the sync path too**

Apply `--pick` **before** the diff is printed so the user sees the filtered set, not the full one. Insert this block immediately after the existing `missingCount := diff.TotalMissing() + diff.TotalChanged()` line and the `if missingCount == 0` short-circuit, and BEFORE the `printInstallDiff(diff)` call:

```go
	if pickRaw, _ := installCmd.Flags().GetString("pick"); pickRaw != "" {
		picks := ParsePicks(pickRaw)
		// Validate against diff additions only — picks naming items not in
		// the diff are user errors in sync context.
		additionsRC := remoteConfigFromSyncDiffAdditions(rc, diff)
		_, unknown := ApplyPicks(additionsRC, picks)
		if len(unknown) > 0 {
			return fmt.Errorf("unknown package(s) in --pick (not in diff additions): %s", strings.Join(unknown, ", "))
		}
		diff = filterSyncDiffByPicks(diff, picks)
		missingCount = diff.TotalMissing() + diff.TotalChanged()
		if missingCount == 0 {
			ui.Info("Nothing matched --pick — exiting.")
			return nil
		}
	}

- [ ] **Step 6: Run all tests + vet**

Run: `go test ./internal/cli/ -v && go vet ./...`
Expected: PASS.

- [ ] **Step 7: Manual smoke test of sync path**

In a terminal:

```bash
# Set up a sync source pointing to a test fixture (skip if you already have one)
# This step is environment-specific; if you can't easily prime a sync source,
# skip to verifying the unit/integration tests cover the dispatch logic.

go run ./cmd/openboot install
```

Verify the prompt now reads `Install all / Customize (pick packages) / Cancel` instead of Y/n, and that "Customize" only shows the diff additions (not the whole subscribed config).

- [ ] **Step 8: Commit**

```bash
git add internal/cli/install.go
git commit -m "feat(install): customize TUI + --pick for sync-source install

Bring the same [Y/c/n] prompt to the sync-source path (openboot install
with no args + a saved sync source). The customize TUI is scoped to the
diff's additions only — users are choosing which of the *new* items to
pull, not re-confirming items they already accepted previously."
```

---

## Task 6: Final L1 + lint + harness verification

**Files:** none new

- [ ] **Step 1: Run the full L1 suite**

Run: `make test-unit`
Expected: PASS in ~75s. If anything fails, fix before continuing — do NOT silence archtest baselines.

- [ ] **Step 2: Run go vet across the whole module**

Run: `go vet ./...`
Expected: no output.

- [ ] **Step 3: Verify archtest baselines are unchanged**

Run: `git status internal/archtest/baseline/`
Expected: clean (no modifications). This work adds no new exec / http / env calls, so baselines must not move.

- [ ] **Step 4: Check the spec is fully covered**

Open `docs/superpowers/specs/2026-05-17-install-multiselect-design.md` and walk the "Mode matrix" table — for each cell, point at the test or code that implements it:

| Cell | Verified by |
|---|---|
| silent + no --pick | unchanged path; existing installer tests still pass |
| silent + --pick | `TestApplyPickFlagToRemoteConfig_*` + Task 3 manual smoke |
| interactive + no --pick | Task 4 manual smoke (3-way prompt) |
| interactive + --pick | dispatch in `runInstallCmd` bypasses prompt; verified by `--pick` smoke test |
| sync-source path | Task 5 dispatch + manual smoke |

- [ ] **Step 5: Commit anything left + push the branch**

```bash
git status
# if clean, no commit needed
git push -u origin worktree-install-multiselect
```

- [ ] **Step 6: Open PR via `ship-pr` skill**

Use the project's `ship-pr` skill — it handles push, PR creation, CI wait, review, and merge. Do NOT use `gh pr merge --auto`.

---

## Self-Review Notes

- All spec sections map to a task: pick.go (Task 1), TUI (Task 2), --pick wiring + matrix cells (Task 3), [Y/c/n] prompt + customize dispatch (Task 4), sync-source path (Task 5), L1 + ship (Task 6).
- No placeholders: every code step has full code, every test step shows the test, every command has expected output.
- Types are consistent: `ConfigCustomizerModel.Picks()` returns `map[string]bool`; `RunConfigCustomizer` returns same; `ApplyPicks` takes same; `ParsePicks` produces same.
- Manual smoke tests are flagged where they're the right tool (TUI behavior); unit tests cover everything mechanically testable.
