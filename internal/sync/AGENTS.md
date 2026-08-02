# SYNC PACKAGE

Compute diff + execute plan for syncing a remote config with the local system.

## FILES

| File | Lines | Purpose |
|------|-------|---------|
| `source.go` | 102 | `SyncSource` struct + Load/Save/Delete to `~/.openboot/sync_source.json` |
| `diff.go` | 280 | `SyncDiff` struct + `ComputeDiff()` comparing remote config vs local system |
| `plan.go` | 212 | `SyncPlan` struct + `Execute()`/`ExecuteContext()` applying selected changes via brew/npm/shell/macos |

## HOW IT WORKS

```
openboot install user/config   →   saves SyncSource to disk
                                     ↓
openboot install (no args)     →   loads SyncSource (cli/install.go runSyncInstall)
                                     ↓
                               →   fetches latest RemoteConfig
                                     ↓
                               →   ComputeDiff(rc) compares remote vs local
                                     ↓
                               →   3-way prompt: install / customize / cancel (cli/sync_helpers.go)
                                     ↓
                               →   ExecuteContext(ctx, plan, dryRun) applies additions
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

Helper: `ToSet([]string) map[string]bool` — exported wrapper around the `diff` package's `ToSet`

## PLAN EXECUTION (plan.go)

`Execute(plan *SyncPlan, dryRun bool)` / `ExecuteContext(ctx, plan, dryRun) (*SyncResult, error)`:

Execution order (dependency-aware):
1. Install taps (other packages may depend on them)
2. Install formulae → casks → npm
3. Uninstall taps → formulae → casks → npm
4. Update dotfiles (clone)
5. Update shell (theme + plugins via `shell.RestoreFromSnapshot`)
6. Apply macOS preferences (via `macos.Configure`)

The uninstall branches remain implemented, but since v1.0 no CLI path
populates the `Uninstall*` fields — install is additive
(`buildInstallPlan` in `cli/sync_helpers.go` never sets them).

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
- `Execute` is unit-tested with faked `brew.Runner`/`npm.Runner` doubles (`execute_test.go`). `ComputeDiff` still captures via real commands (brew, npm, git) — exercise it via real-subprocess tests in `test/integration/` (run as part of L1, `make test-unit`).
- Source persistence tested with `t.TempDir()` + `t.Setenv("HOME", tmpDir)` pattern

## WHEN MODIFYING

- Adding a new diff category: Add fields to `SyncDiff`, update `HasChanges/TotalMissing/TotalExtra/TotalChanged`, add capture in `ComputeDiff`, update the printed diff in `cli/sync_helpers.go`
- Adding a new plan action: Add fields to `SyncPlan`, update `TotalActions`, add execution branch in `Execute`, update `buildInstallPlan` in `cli/sync_helpers.go`
- Changing persistence format: Update `SyncSource` struct — JSON tags are the wire format
