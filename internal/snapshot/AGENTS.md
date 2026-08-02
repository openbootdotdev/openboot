# SNAPSHOT PACKAGE

Environment capture, matching, and restoration. 6 source files + tests.

## FILES

| File | Lines | Purpose |
|------|-------|---------|
| `capture.go` | 562 | Capture pipeline: formulae/casks/taps/npm/bun/prefs/dock/login items/git/dotfiles/devtools/shell |
| `dock.go` | 268 | Dock persistent-apps capture (`com.apple.dock` plist parsing) |
| `loginitems.go` | 60 | Login items capture via `osascript` |
| `match.go` | 112 | Match captured packages against catalog, Jaccard similarity for preset detection |
| `local.go` | 74 | Read/write snapshots to `~/.openboot/snapshot.json`; `LoadFile`/`ParseBytes` |
| `snapshot.go` | 202 | Data structures + `PackageSnapshot` JSON codec accepting legacy shapes |

## CAPTURE PIPELINE

`CaptureWithProgress()` runs 12 sequential steps (`captureSteps` in capture.go), each reporting via callback:

1. Homebrew Formulae → `brew leaves` (top-level only, excludes dependencies)
2. Homebrew Casks → `brew list --cask`
3. Homebrew Taps → `brew tap`
4. NPM Global Packages → `npm list -g --json`
5. Bun Global Packages
6. macOS Preferences → reads known defaults keys
7. Dock Apps → `com.apple.dock` persistent-apps
8. Login Items → `osascript`
9. Git Configuration → user.name, user.email, core.editor, etc.
10. Dotfiles → repo URL of `~/.dotfiles`
11. Dev Tools → version detection for node, go, python, rust, docker, etc.
12. Shell Config → detects shell, oh-my-zsh, plugins, aliases

Each step is independent. Failures are non-fatal (recorded in `failed_steps`, snapshot marked `partial`).

## MATCHING LOGIC (match.go)

- `MatchPackages()`: Maps captured package names → catalog entries. Returns matched + unmatched lists.
- `DetectBestPreset()`: Jaccard similarity between snapshot packages and each preset. Threshold: 0.3.
- Package names matched case-insensitively against `config.Categories`.

## SNAPSHOT FORMAT

```json
{
  "version": 1,
  "captured_at": "2026-01-15T10:30:00Z",
  "hostname": "macbook",
  "packages": {
    "formulae": ["curl", "wget"],
    "casks": ["visual-studio-code"],
    "taps": ["homebrew/core"],
    "npm": ["typescript"],
    "bun": []
  },
  "macos_prefs": [ ... ],
  "shell": { ... },
  "git": { ... },
  "dotfiles": { ... },
  "dev_tools": [ ... ],
  "matched_preset": "developer",
  "catalog_match": { ... },
  "dock_apps": ["/Applications/Safari.app"],
  "login_items": [{ "name": "Raycast", "path": "...", "hidden": false }],
  "health": { "failed_steps": [], "partial": false }
}
```

## RESTORE PIPELINE

`snapshot --import` restores:
1. Taps → `brew tap`
2. Formulae/Casks → `brew install` (via installer.RunFromSnapshot)
3. NPM packages → `npm install -g`
4. Git config → `git config --global user.name/email` (skips if already set)
5. Shell → Installs Oh-My-Zsh, sets ZSH_THEME and plugins in .zshrc
6. macOS preferences → `defaults write`

Snapshot data is mapped to `config.SnapshotGitConfig` and `config.SnapshotShellConfig` in `cli/snapshot.go`, then consumed by `installer.stepRestoreGit` and `installer.stepRestoreShell`.

## WHEN MODIFYING

- Adding capture step: Add an entry to `captureSteps` in `capture.go`, add to `Snapshot` struct
- Adding restore step: Add to `installer.RunFromSnapshot()`, create `config.Snapshot*Config` type, wire in `cli/snapshot.go`
- Adding preset detection: Modify `DetectBestPreset()` scoring in `match.go`
- Tests: Table-driven with testify. `capture_test.go` mocks command output. `match_test.go` tests Jaccard scoring.
