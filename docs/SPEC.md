# OpenBoot CLI Spec (v1.0)

> **Positioning:** `install` puts things into your Mac; `snapshot` sends your current state somewhere else. That's the whole product.

---

## Mental Model

Two directions, two verbs:

```
             INSTALL ←──────────── source (cloud | file | preset | sync source)
                │
              [Mac]
                │
           SNAPSHOT ──────────────→ target (file | cloud | stdout)
```

- **`install`** — add only. Never uninstalls.
- **`snapshot`** — output only. Captures current state.

Everything else is auth/version infrastructure.

---

## Commands

### Core

| Command | Purpose |
|---------|---------|
| `openboot install [source]` | Add packages / settings from source to this Mac. |
| `openboot snapshot` | Capture current state (local, cloud, or stdout). |

### Infrastructure

| Command | Purpose |
|---------|---------|
| `openboot login` / `logout` | Authenticate with openboot.dev. |
| `openboot version` | Print version. Self-update happens on launch automatically. |

**Total: 5 commands.** (plus `help`, `completion` from Cobra)

---

## `install [source]` — source resolution

Position argument is identified by pattern (in order):

1. Starts with `./`, `../`, `/`, or ends with `.json` → **local file**
2. Contains exactly one `/` with valid slug chars on both sides → **cloud config** (`user/slug`)
3. Plain word → try **alias** → try **preset** → error with suggestion
4. No argument → use saved **sync source** (error if none)

Explicit flags always override the positional argument:

- `--from <file>` — force local file
- `--user <u/s>` — force cloud config
- `-p <preset>` — force preset

**Examples:**

```bash
openboot install                          # sync source
openboot install alice/dev-setup          # cloud
openboot install ./backup.json            # local file
openboot install backup.json              # local file (.json ext)
openboot install developer                # alias → preset → error
openboot install -p developer             # explicit preset
```

**`--dry-run`** shows what would be added, does not show extras (install doesn't care about extras).

**No-arg behavior always shows sync source:**
```
→ Syncing with @alice/dev-setup (last synced 2 days ago)
```
If last-synced > 90 days, add warning: `⚠ last synced 6 months ago`.

The final confirm repeats the config name: `Apply 5 changes from @alice/dev-setup? (Y/n)`

---

## `snapshot` — output routing

Captures current state. Destination is chosen by flag or TTY.

| Flag | Behavior |
|------|----------|
| (none, in terminal) | Interactive menu: local / publish / both / stdout |
| (none, no TTY) | JSON to stdout |
| `--local` | Save to `~/.openboot/snapshot.json` |
| `--publish` | Upload to openboot.dev |
| `--json` | JSON to stdout |
| `--local --publish` | Both |

### `--publish` rules

| Situation | Behavior |
|-----------|----------|
| `--slug X` given and X exists | Update X (do not modify sync source) |
| `--slug X` given but X doesn't exist | Error: `No config named X. Remove --slug to create new.` |
| No `--slug`, sync source exists | Update sync source's config; show `Publishing to @alice/foo (updating)` |
| No `--slug`, sync source's slug was deleted remotely | Error with recovery suggestion |
| No `--slug`, no sync source | Prompt for name / description / visibility; create new; set as sync source |

Visibility is asked only when creating; updates preserve existing visibility.

---

## Data Model

### Snapshot (read-only observation)

Contains everything captured from the system. Full fidelity, for local backup.

```
packages (formulae, casks, taps, npm)
macos_prefs
shell (oh-my-zsh, theme, plugins)   ← must be added (currently missing)
dotfiles.repo_url
git (user, email)                   ← snapshot-only, never in Config
dev_tools (node/python/ruby versions)  ← snapshot-only
```

### Config (declarative intent)

What can be declared and shared. Edited via web UI.

```
packages (formulae, casks, taps, npm)
macos_prefs
shell (oh-my-zsh, theme, plugins)
dotfiles_repo
post_install                         ← Config-only (web-edited)
preset                               ← Config-only
```

**Intentional asymmetry:**
- `git.user` / `git.email` → Snapshot only (personal, not shared)
- `dev_tools` versions → Snapshot only (observation, not enforced)
- `post_install` scripts → Config only (edited on web, not captured locally)

---

## Breaking Changes from v0.x

**Removed commands** (each prints error + migration hint):

| Removed | Replacement |
|---------|-------------|
| `pull` | `install` (no args) |
| `push` | `snapshot --publish` |
| `diff` | `install --dry-run` |
| `clean` | **No replacement.** OpenBoot no longer manages removals. |
| `log` | **No replacement.** Version history dropped. |
| `restore` | **No replacement.** Version history dropped. |
| `list` | **No replacement.** Manage configs at openboot.dev directly. |
| `edit` | **No replacement.** Manage configs at openboot.dev directly. |
| `delete` | **No replacement.** Manage configs at openboot.dev directly. |
| `init` | **No replacement.** Project deps are each ecosystem's own job (npm/pip/go/cargo). |
| `setup-agent` | **No replacement.** Existed only to service `init`. |
| `doctor` | **No replacement.** Use `brew doctor` and `git config --list` directly. |
| `update` | **No replacement.** `brew upgrade` for packages; OpenBoot self-updates on launch. |

**Why hard-break instead of aliases:**
Old `pull` / `push` had behaviors (uninstall, revision messages) that the new equivalents don't have. Silent aliasing would make behavior regress invisibly — worse than a clear error.

---

## Versioning Policy (replaces old CLAUDE.md rule)

- Breaking changes are allowed, but require a **major version bump** + migration notes in CHANGELOG.
- Same-name commands **must not** silently change behavior — either preserve semantics or remove the command.
- Deprecation path: no intermediate aliases for this v1.0 cleanup (too many concurrent changes). Future breaks should use alias + warning for 1-2 minor versions before removal.

---

## Out of Scope (decisions we're NOT making)

- **Package uninstall** — OpenBoot never removes packages. Users rely on `brew uninstall` or Time Machine.
- **Version history / revisions** — Configs have no timeline. `snapshot --publish` overwrites.
- **Multi-device sync / conflict resolution** — This isn't a sync system. Each machine is independent; publishing means "I declare my current state as the new config".
- **Shell diff in Snapshot/Config roundtrip** — Currently broken (Snapshot struct lacks Shell field); fix is part of v1.0 scope but not a new feature.
