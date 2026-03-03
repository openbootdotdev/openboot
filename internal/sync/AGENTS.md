# SYNC PACKAGE

Compute diff + execute plan for syncing a remote config with the local system. 3 source files + 3 test files, 1,040 lines.

## FILES

| File | Lines | Purpose |
|------|-------|---------|
| `source.go` | 95 | `SyncSource` struct + Load/Save/Delete to `~/.openboot/sync_source.json` |
| `diff.go` | 239 | `SyncDiff` struct + `ComputeDiff()` comparing remote config vs local system |
| `plan.go` | 196 | `SyncPlan` struct + `Execute()` applying selected changes via brew/npm/shell/macos |

## HOW IT WORKS

```
openboot install user/config   →   saves SyncSource to disk
                                     ↓
openboot sync                  →   loads SyncSource
                                     ↓
                               →   fetches latest RemoteConfig
                                     ↓
                               →   ComputeDiff(rc) compares remote vs local
                                     ↓
                               →   user selects changes in TUI (cli/sync.go)
                                     ↓
                               →   Execute(plan, dryRun) applies changes
```

## SOURCE PERSISTENCE (source.go)

- File: `~/.openboot/sync_source.json` (0600 perms)
- Atomic write: tmp file + `os.Rename` (matches `installer/state.go` pattern)
- `LoadSource()` returns `nil, nil` when file doesn't exist (not an error)
- Fields: `UserSlug` (raw input), `Username`/`Slug` (resolved), `SyncedAt`, `InstalledAt`

## DIFF COMPUTATION (diff.go)

`ComputeDiff(rc *config.RemoteConfig) (*SyncDiff, error)`:

1. Captures local state via `snapshot.CaptureFormulae/Casks/Taps/Npm()`
2. **Fails fast** on capture errors to prevent false positives
3. Set-based comparison via `diffLists()` → (missing, extra)
4. Shell diff: compares theme + plugins via `snapshot.CaptureShell()`
5. macOS diff: compares preferences via `snapshot.CaptureMacOSPrefs()`
6. Dotfiles diff: compares git remote URL from `~/.dotfiles`

Key types:
- `SyncDiff` — bidirectional diff (missing = in remote not local, extra = local not remote)
- `ShellDiff` — theme/plugins changes
- `MacOSPrefDiff` — per-preference domain/key/value diff

Helper: `ToSet([]string) map[string]bool` — exported for use in `cli/sync.go`

## PLAN EXECUTION (plan.go)

`Execute(plan *SyncPlan, dryRun bool) (*SyncResult, error)`:

Execution order (dependency-aware):
1. Install taps (other packages may depend on them)
2. Install formulae → casks → npm
3. Uninstall taps → formulae → casks → npm
4. Update dotfiles (clone)
5. Update shell (theme + plugins via `shell.RestoreFromSnapshot`)
6. Apply macOS preferences (via `macos.Configure`)

Error handling: Collects all errors via `errors.Join` (continues on failure).

## REUSED FUNCTIONS

| Function | Package | Used For |
|----------|---------|----------|
| `snapshot.CaptureFormulae/Casks/Taps/Npm()` | snapshot | Local state capture |
| `snapshot.CaptureShell()` | snapshot | Local shell config |
| `snapshot.CaptureMacOSPrefs()` | snapshot | Local macOS prefs |
| `brew.Install/InstallCask/InstallTaps()` | brew | Package installation |
| `brew.Uninstall/UninstallCask/Untap()` | brew | Package removal |
| `npm.Install/Uninstall()` | npm | npm package ops |
| `dotfiles.Clone()` | dotfiles | Dotfiles repo clone |
| `shell.RestoreFromSnapshot()` | shell | Shell config update |
| `macos.Configure()` | macos | macOS pref writes |

## TESTING NOTES

- Pure logic functions (diffLists, ToSet, HasChanges, Totals, TotalActions, IsEmpty) have 100% coverage
- `getLocalDotfilesURL` tested with temp git repo at 80%
- `ComputeDiff` and `Execute` depend on external commands (brew, npm, git) — not unit-testable without interface refactoring. Use `make test-integration` for these.
- Source persistence tested with `t.TempDir()` + `t.Setenv("HOME", tmpDir)` pattern

## WHEN MODIFYING

- Adding a new diff category: Add fields to `SyncDiff`, update `HasChanges/TotalMissing/TotalExtra/TotalChanged`, add capture in `ComputeDiff`, update `cli/sync.go` TUI
- Adding a new plan action: Add fields to `SyncPlan`, update `TotalActions`, add execution branch in `Execute`, update `cli/sync.go` `buildSyncPlan`
- Changing persistence format: Update `SyncSource` struct — JSON tags are the wire format
