# Design: Homebrew Auto-Upgrade

**Date:** 2026-02-27

## Problem

When openboot is installed via Homebrew, `AutoUpgrade` currently skips all upgrade logic and returns immediately. Users must manually run `brew upgrade openboot` to stay up to date.

## Goal

Automatically upgrade Homebrew-installed versions of openboot by invoking `brew upgrade openboot` as a subprocess, keeping behavior consistent with non-Homebrew installs.

## Approach

Modify `AutoUpgrade` in `internal/updater/updater.go` to call a new `homebrewAutoUpgrade(currentVersion)` function instead of returning early on Homebrew installs.

## Design

### `homebrewAutoUpgrade(currentVersion string)` (new function)

1. Load cached update state via `LoadState()`
2. If `state.UpdateAvailable` and `isNewerVersion(state.LatestVersion, currentVersion)`:
   - Run `brew upgrade openboot` with stdout/stderr discarded
   - On success: print `ui.Success("Updated to latest version via Homebrew.")`
   - On failure: print `ui.Warn(...)` and suggest manual `brew upgrade openboot`
3. Refresh state cache asynchronously via `checkForUpdatesAsync(currentVersion)`

The 24-hour `CheckInterval` cache is reused — no new network calls more than once per day.

### `AutoUpgrade` changes

```
Before:
  if IsHomebrewInstall() { return }

After:
  if IsHomebrewInstall() {
      homebrewAutoUpgrade(currentVersion)
      return
  }
```

### `runSelfUpdate` (`internal/cli/update.go`)

No change. `openboot update --self` continues to inform the user to run `brew upgrade openboot` manually, since this is an explicit user-initiated action.

### Configuration

Homebrew auto-upgrade respects `OPENBOOT_DISABLE_AUTOUPDATE=1` (checked first in `AutoUpgrade`). The `autoupdate` config field (`notify`/`true`/`false`) does **not** apply to the Homebrew path — Homebrew installs always auto-upgrade unless disabled by the env var.

## Files Changed

- `internal/updater/updater.go` — add `homebrewAutoUpgrade`, modify `AutoUpgrade`
- `internal/updater/updater_test.go` — add unit tests for `homebrewAutoUpgrade`

## Out of Scope

- Supporting `autoupdate: "notify"` mode for Homebrew installs
- Linux Homebrew (`/home/linuxbrew/`) — same logic applies automatically since `IsHomebrewInstall` already detects it
