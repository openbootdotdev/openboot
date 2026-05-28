# UI PACKAGE

Output helpers, progress widgets, and form wrappers. Bubbletea TUI models
live in the `tui/` subpackage.

## FILES (internal/ui/)

| File | Lines | Purpose |
|------|-------|---------|
| `ui.go` | 247 | Base styles, color helpers, output helpers (Header/Success/Error/Info/Warn/Muted/DryRun*), huh form wrappers (InputGitConfig, SelectPreset, Confirm, SelectOption, Input) |
| `progress.go` | 246 | StickyProgress: per-package timing, succeeded/failed/skipped counters |
| `scanprogress.go` | 221 | ScanProgress: step timing, overall counter `[3/8]` |
| `scrollregion.go` | 143 | ANSI scroll-region plumbing for StickyProgress |

## FILES (internal/ui/tui/)

| File | Lines | Purpose |
|------|-------|---------|
| `selector.go` + `selector_view.go` | ~1,026 | Package selector: tabs, fuzzy search, online search, multi-select |
| `snapshot_editor.go` + `snapshot_editor_search.go` | ~1,022 | Snapshot editing: diff view, toggle packages, add online, confirm |
| `config_customizer.go` | 253 | Remote-config customizer: toggle which packages to install |
| `macos_selector.go` | 435 | macOS preferences selector: category tabs, toggle, confirm |

## PATTERNS

- **bubbletea Model**: `tui/*.go` files implement `tea.Model` (Init/Update/View)
- **Sticky output**: `progress.go` writes directly to stderr via ANSI escape codes, not bubbletea
- **Styles**: All lipgloss styles defined as package-level vars at top of file
- **Color palette**: Primary=#22c55e, Subtle=#666, Warning=#eab308, Danger=#ef4444
- **Width adaptation**: Components read `tea.WindowSizeMsg` and adapt layout

## PROGRESS ARCHITECTURE (progress.go)

NOT a bubbletea model. Direct stderr writer for use during brew install.

- `IncrementWithStatus(success bool)` — tracks succeeded/failed count
- `SetSkipped(count int)` — for already-installed packages
- `Finish()` — prints summary line: `✔ 28 installed  ○ 2 skipped  ✗ 1 failed (1m23s)`
- Thread-safe: called from 4 parallel brew workers via mutex

## WHEN ADDING NEW UI

1. Interactive full-screen → bubbletea Model in `tui/` subpackage
2. Progress/status during install → extend StickyProgress or ScanProgress in `ui/`
3. Simple form input → use helpers in `ui.go` (huh-based)
4. Styled text output → use `ui.Header`/`Success`/`Error`/`Info`/`Warn`/`Muted`
