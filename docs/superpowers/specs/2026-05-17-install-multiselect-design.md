# Install-time package multi-select for remote configs

## Problem

Today, when a user runs `openboot install` against a remote/cloud config
(via `curl openboot.dev/<user> | bash`, `-u <slug>`, `--from <file>`, or
an already-synced sync source), they get a single all-or-nothing
confirmation. They cannot subset the package list at install time.

Two scenarios are blocked:

1. **End user trying someone else's config.** They `curl|bash`
   `fullstackjam`'s config, see 47 packages flash by, and want to
   skip a handful (heavy GUI apps they already have, things they
   don't use). Today they either install everything or cancel.
2. **Harness-driven e2e tests.** When the test harness drives a TARA
   VM to install from a config, it needs to install a small
   deterministic subset to keep run time reasonable. There is no
   non-interactive way to express "only install these packages from
   the config."

The preset path (`-p developer`) already has a TUI multi-select via
`ui.RunSelector` — this work brings the same capability to the
remote-config paths.

## Scope

In scope:
- **TUI multi-select** for formulae / casks / npm during install from
  remote config, sync source, or local file.
- **`--pick=name1,name2,...` flag** for scripted, non-interactive
  subsetting; works in silent mode and in interactive mode.
- **`[Y/c/n]` prompt** replacing the current `[Y/n]` confirmation in
  remote-config and sync-source install flows.

Out of scope:
- macOS prefs, post-install scripts, dotfiles repo, OhMyZsh —
  these stay all-or-nothing per the existing step-level skip flags
  (`--macos=skip`, `--post-install=skip`, `--dotfiles=skip`,
  `--shell=skip`).
- The preset path's existing TUI (`ui.RunSelector`) — unchanged.
- Snapshot restore path — unchanged.
- Persisting the user's selection as a new "subscribed subset" for
  future sync operations — out of scope for this iteration; selection
  applies to this install only.

## UX

### Default flow (interactive, no `--pick`)

After the remote config is fetched, before any install action:

```
→ Installing fullstackjam/dev-setup
  CLI tools: 23
  Apps: 12
  npm: 4

Install all 39 packages? [Y]es / [c]ustomize / [n]o:
```

- `Y` or Enter → install everything (today's behaviour)
- `c` → enter TUI; user toggles; on Enter the chosen subset installs
- `n` → cancel

### Customize TUI

A new `ui.ConfigCustomizerModel` (bubbletea), modeled on the existing
`ui.SnapshotEditorModel` but with input/output specific to
`config.RemoteConfig`. Tabs: **Formulae**, **Casks**, **NPM**. All
items pre-checked; Space toggles; Tab/←→ switches category; `a`
toggles all in current tab; `/` filters; Enter confirms; `q` /
Ctrl-C cancels.

Out of the TUI returns `(picks map[string]bool, confirmed bool)`. The
caller feeds `picks` into the same `ApplyPicks` helper used by `--pick`.

### Scripted flow (`--pick`)

```bash
openboot install -u fullstackjam --pick=git,jq,ripgrep,visual-studio-code
openboot install --silent --from ./config.json --pick=git,jq
```

- Comma-separated list of package names matching `Packages[i].Name`,
  `Casks[i].Name`, or `Npm[i].Name`.
- Resolved by `ApplyPicks(rc, picks)`.
- Unknown names → fail-fast with an error listing them.

### Mode matrix

"Silent" = `--silent` flag set, **or** dry-run without a TTY (the
existing `opts.Silent || (opts.DryRun && !system.HasTTY())` check
used throughout the installer). Everything else is "interactive".

`--dry-run` with a TTY is interactive: the prompt and TUI both run as
normal, and the chosen subset feeds into the dry-run plan that's
printed instead of executed.

| Mode | `--pick` given? | Behaviour |
|---|---|---|
| silent | no | install everything (today's behaviour, unchanged) |
| silent | yes | apply picks, install the subset |
| interactive | no | show `[Y/c/n]` prompt |
| interactive | yes | apply picks, install the subset (skip prompt — explicit intent already expressed) |

### Sync-source path

`runSyncInstall` in `internal/cli/install.go` keeps its existing diff
display, but its `Apply N change(s) from <label>?` Y/n becomes
`[Y/c/n]`. Entering `c` opens the same TUI, pre-populated with **only
the diff's additions** (not the full subscribed config). Rationale:
the user is updating an existing subscription; they should be
choosing which of the *new* items to pull, not re-confirming the
items they already accepted previously.

## Architecture

Route B from the brainstorm: a new `ConfigCustomizer` sibling to
`SnapshotEditor`. No refactor of `SnapshotEditor`; rendering
primitives may be extracted later once both editors are stable.

### Files

**New**
- `internal/ui/config_customizer.go` — bubbletea Model + `RunConfigCustomizer`.
- `internal/ui/config_customizer_test.go`
- `internal/cli/pick.go` — `ParsePicks`, `ApplyPicks` (pure functions, no I/O).
- `internal/cli/pick_test.go`

**Modified**
- `internal/cli/install.go`
  - Add `--pick` flag binding.
  - In `runInstallCmd`, after the remote config is loaded and before
    `installer.Run`, dispatch on the mode matrix above:
    - silent + pick → `ApplyPicks`
    - silent + no pick → no-op
    - interactive + pick → `ApplyPicks`
    - interactive + no pick → call `installer.PromptCustomize(rc)`
      which returns one of {acceptAll, customize, cancel}; on
      `customize`, call `ui.RunConfigCustomizer(rc)` + `ApplyPicks`.
  - In `runSyncInstall`, replace the Y/n confirmation with the same
    three-way prompt; `c` runs `ui.RunConfigCustomizer` with a
    RemoteConfig restricted to the diff additions.
- `internal/installer/installer.go`
  - Remove the existing one-off `ui.Confirm("Install these packages?")`
    in `runInstall` — the prompt now lives at the CLI layer where the
    customize dispatch happens. The installer just applies what's in
    `cfg.RemoteConfig`.

### Data flow

```
install.go runInstallCmd
  ├─ resolveInstallSource → applyInstallSource → installCfg.RemoteConfig populated
  ├─ if installCfg.RemoteConfig != nil:
  │    rc := installCfg.RemoteConfig
  │    if --pick:
  │      rc, unknown = ApplyPicks(rc, picks); fail-fast if unknown != nil
  │    else if interactive:
  │      choice = promptCustomize(rc)        // [Y/c/n]
  │        Y → continue with rc unchanged
  │        c → picks = ui.RunConfigCustomizer(rc)
  │            rc, _ = ApplyPicks(rc, picks)
  │        n → return nil (cancelled, exit 0)
  │    installCfg.RemoteConfig = rc
  └─ installer.Run(installCfg)
       └─ runInstall: no longer prompts; just applies cfg.RemoteConfig
```

`runSyncInstall` follows the same shape but `rc` starts as a
RemoteConfig restricted to the diff additions before the matrix runs.

### `pick.go` API

```go
// ParsePicks splits a comma-separated --pick value into a set.
// Whitespace around names is trimmed; empty entries are skipped.
func ParsePicks(raw string) map[string]bool

// ApplyPicks returns a copy of rc whose Packages, Casks, and Npm
// slices contain only entries whose Name appears in picks. Taps,
// dotfiles, shell, macOS prefs, post-install, and other fields are
// passed through unchanged. Any names in picks that didn't match any
// package are returned in unknown so the caller can fail fast (--pick)
// or ignore (TUI, where picks come from rc itself).
func ApplyPicks(rc *config.RemoteConfig, picks map[string]bool) (filtered *config.RemoteConfig, unknown []string)
```

### `ConfigCustomizer` shape

Smaller than `SnapshotEditor`: three tabs, no online search, no manual
add, no macOS prefs / taps tabs.

```go
type ConfigCustomizerModel struct {
    rc           *config.RemoteConfig
    tabs         []customizerTab // Formulae, Casks, NPM
    activeTab    int
    cursor       int
    scrollOffset int
    width, height int
    searchMode   bool
    searchQuery  string
    filtered     []customizerFilteredRef
    confirmed    bool
}

type customizerItem struct {
    name        string
    description string
    selected    bool
}

type customizerTab struct {
    name     string
    icon     string
    items    []customizerItem
}

func NewConfigCustomizer(rc *config.RemoteConfig) ConfigCustomizerModel
func RunConfigCustomizer(rc *config.RemoteConfig) (picks map[string]bool, confirmed bool, err error)
```

Keybindings (match `SelectorModel` / `SnapshotEditorModel` for
consistency): Tab/←→ switch tab, ↑↓ navigate, Space toggle, `a`
toggle-all-in-tab, `/` filter, Enter confirm, `q` / Ctrl-C cancel.

## Error handling

| Case | Behaviour |
|---|---|
| `--pick=foo` where foo is not in the config | Error: `unknown package(s) in --pick: foo. Run with --dry-run to see available names.` Exit non-zero. |
| `--pick=""` (empty) | Treated as not provided. |
| `--pick` with no interactive TTY and no `--silent` | Honored; same path as silent. |
| TUI confirmed with zero items selected | Proceed with empty install (already a valid state today when preset packages are all unchecked). The completion summary will show "0 packages installed." |
| TUI cancelled (q / Ctrl-C) | Return `ErrUserCancelled` from the installer — same as preset path cancel. |
| `--pick` combined with sync-source path | Same semantics: subset is applied to the diff additions, not to the full config. Names not in the diff additions → fail-fast. |
| `--pick` used with `-p preset` (no remote config) | Error: `--pick requires a remote config; use the preset selector instead.` |
| `--pick` + `--dry-run` | Picks apply normally; the dry-run summary reflects the subset. |

## Testing

L1 only — no new e2e, the existing curl|bash smoke job already
exercises the install path end-to-end and will exercise the no-pick
case unchanged.

- `internal/cli/pick_test.go` (table-driven)
  - `ParsePicks`: empty, single, multiple, whitespace, trailing comma.
  - `ApplyPicks`: filter formulae only / casks only / npm only / mixed; preserves taps and non-package fields verbatim; unknown names reported; deterministic order.
- `internal/ui/config_customizer_test.go`
  - `NewConfigCustomizer` populates 3 tabs from a fixture RemoteConfig with all items pre-selected.
  - `RunConfigCustomizer` confirmed=false path returns no picks.
  - Toggle behaviour: space deselects, `a` toggles all in tab.
  - Search filter: matches name and description case-insensitively.
- `internal/cli/install_test.go` (extend existing)
  - Mode matrix: build a fake RemoteConfig + Runner; verify each cell in the matrix above produces the expected installed set.
  - sync-source path: `--pick` restricted to diff additions; unknown name in diff additions → error.

No archtest changes needed (no new exec / http / env calls).

## Out-of-scope follow-ups (not in this spec)

- Extracting shared TUI rendering primitives between `SnapshotEditor`,
  `SelectorModel`, and `ConfigCustomizer`. Revisit once `ConfigCustomizer`
  has been in use for a release.
- Persisting the user's customize choice into the sync source so the
  next `openboot install` only offers newly-added items. Useful but
  changes the sync data model.
- `--pick` support in the preset path. The preset path already has a
  TUI; if a scripted preset subset is needed later, add it then.
