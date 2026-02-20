# SNAPSHOT PACKAGE

Environment capture, matching, and restoration. 8 files (4 source + 4 test), 1,783 lines.

## FILES

| File | Lines | Purpose |
|------|-------|---------|
| `capture.go` | 528 | Capture formulae/casks/taps/npm/prefs/shell/git/devtools |
| `match.go` | 112 | Match captured packages against catalog, Jaccard similarity for preset detection |
| `local.go` | 62 | Read/write snapshots to `~/.openboot/snapshot.json` |
| `snapshot.go` | 61 | Data structures: Snapshot, PackageSnapshot, MacOSPrefs, ShellConfig |

## CAPTURE PIPELINE

`CaptureWithProgress()` runs 8 sequential steps, each reporting via callback:

1. Homebrew Formulae → `brew leaves` (top-level only, excludes dependencies)
2. Homebrew Casks → `brew list --cask`
3. Homebrew Taps → `brew tap`
4. npm Packages → `npm list -g --json`
5. macOS Preferences → reads known defaults keys
6. Shell Config → detects shell, oh-my-zsh, plugins, aliases
7. Git Config → user.name, user.email, core.editor, etc.
8. Dev Tools → version detection for node, go, python, rust, docker, etc.

Each step is independent. Failures are non-fatal (captured as empty).

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
    "npm": ["typescript"]
  },
  "macos_prefs": { ... },
  "shell_config": { ... },
  "git_config": { ... },
  "dev_tools": { ... }
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

- Adding capture step: Add to `CaptureWithProgress()`, update `totalSteps`, add to `Snapshot` struct
- Adding restore step: Add to `installer.RunFromSnapshot()`, create `config.Snapshot*Config` type, wire in `cli/snapshot.go`
- Adding preset detection: Modify `DetectBestPreset()` scoring in `match.go`
- Tests: Table-driven with testify. `capture_test.go` mocks command output. `match_test.go` tests Jaccard scoring.
