# UI PACKAGE

TUI components built on Charmbracelet (bubbletea + lipgloss + huh). 5 files, 2,206 lines.

## FILES

| File | Lines | Purpose |
|------|-------|---------|
| `selector.go` | 986 | Package selector: tabs, fuzzy search, online search, multi-select |
| `snapshot_editor.go` | 518 | Snapshot editing: diff view, toggle packages, confirm |
| `progress.go` | 289 | StickyProgress: per-package timing, succeeded/failed/skipped counters |
| `scanprogress.go` | 228 | Scan progress: step timing, overall counter `[3/8]` |
| `ui.go` | 185 | Base styles, form helpers (SelectPreset, InputGitConfig, Confirm) |

## PATTERNS

- **bubbletea Model**: `selector.go` and `snapshot_editor.go` implement `tea.Model` (Init/Update/View)
- **Sticky output**: `progress.go` writes directly to stderr via ANSI escape codes, not bubbletea
- **Styles**: All lipgloss styles defined as package-level vars at top of file
- **Color palette**: Primary=#22c55e, Subtle=#666, Warning=#eab308, Danger=#ef4444
- **Width adaptation**: Components read `tea.WindowSizeMsg` and adapt layout

## SELECTOR ARCHITECTURE (selector.go)

Two render modes: normal (tab navigation) and search (`/` key).

- **Tab bar**: Sliding window — shows active tab + neighbors + `N/M` position. Adapts to terminal width.
- **Search**: Fuzzy local match (sahilm/fuzzy) + debounced online search (500ms). Animated spinner during fetch.
- **Toast**: 1.5s auto-fade notifications on toggle (`+ Added node` / `- Removed node`).
- **Scroll**: `scrollOffset` + `getVisibleItems()` for height-adaptive list (5-20 items).
- **Alt screen**: Uses `tea.WithAltScreen()` for full-screen mode.

## PROGRESS ARCHITECTURE (progress.go)

NOT a bubbletea model. Direct stderr writer for use during brew install.

- `IncrementWithStatus(success bool)` — tracks succeeded/failed count
- `SetSkipped(count int)` — for already-installed packages
- `Finish()` — prints summary line: `✔ 28 installed  ○ 2 skipped  ✗ 1 failed (1m23s)`
- Thread-safe: called from 4 parallel brew workers via mutex

## WHEN ADDING NEW UI

1. Interactive full-screen → bubbletea Model in new file
2. Progress/status during install → extend StickyProgress or ScanProgress
3. Simple form input → use helpers in ui.go (huh-based)
4. Styled text output → use ui.Header/Success/Error/Info/Warn/Muted
