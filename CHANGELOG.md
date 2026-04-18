# Changelog

## v1.0 (unreleased)

OpenBoot v1.0 narrows the product to two verbs: **`install`** (add things to your Mac) and **`snapshot`** (save your current state somewhere). Everything else is either cloud-config CRUD or independent tooling. See [`docs/SPEC.md`](docs/SPEC.md) for the full spec.

### Breaking changes

Six commands are removed outright. Each prints an error with a migration hint when invoked:

| Removed | Use instead |
|---------|-------------|
| `openboot pull` | `openboot install` (no args) |
| `openboot push` | `openboot snapshot --publish` |
| `openboot diff` | `openboot install --dry-run` |
| `openboot clean` | **no replacement** — OpenBoot no longer manages package removal |
| `openboot log` | **no replacement** — version history is dropped |
| `openboot restore` | **no replacement** — version history is dropped |
| `openboot init` | **no replacement** — use your project's own tooling (npm/pip/go/cargo) |
| `openboot setup-agent` | **no replacement** — existed only to service `openboot init` |
| `openboot doctor` | **no replacement** — use `brew doctor` and `git config --list` directly |
| `openboot update` | **no replacement** — use `brew upgrade` directly; OpenBoot self-updates on launch |

Three flat commands move under a `config` namespace:

| Before | After |
|--------|-------|
| `openboot list` | `openboot config list` |
| `openboot edit` | `openboot config edit` |
| `openboot delete` | `openboot config delete` |

No aliases are kept — silent aliasing would regress behavior invisibly (the old `pull` did uninstalls, the new `install` does not).

### New & changed

- **`install` auto-resumes**: `openboot install` with no args now reads the saved sync source and applies only the additions — previously it went straight to the interactive wizard.
- **Smart source resolution**: the position argument in `install [source]` is identified by pattern: `./path` or `*.json` → local file, `user/slug` → cloud config, known preset name → preset, otherwise → cloud alias. Explicit `--from` / `--user` / `-p` still override.
- **Sync header**: install now shows `→ Syncing with @user/slug (last synced X ago)`. Warns with `⚠` when > 90 days stale. The final confirm repeats the config name to prevent skim-through.
- **`snapshot --publish`**: direct non-interactive cloud upload. Respects the sync source (updates it) or creates a new config with a prompt. Does not ask for name/desc/visibility when updating existing.
- **`snapshot` in pipe**: piping `openboot snapshot` to another command now emits JSON to stdout automatically (TTY detection).
- **Shell capture**: snapshots now include the Oh-My-Zsh state, theme, and plugins (previously the field was defined but never populated — publishing silently dropped shell data).

### Philosophy

- **Never uninstall.** `install` adds things; it never removes them. Use `brew uninstall` or Time Machine for rollbacks.
- **No version management.** Configs don't have a timeline. `snapshot --publish` overwrites; there's no history to walk backwards through.
- **Machine is the unit.** Each machine has one cloud association (the sync source) and reconciles forward to it. No conflict resolution, no merging, no pull/push dance.

### Versioning policy change

The previous `CLAUDE.md` rule "CLI changes must maintain backward compatibility" has been relaxed:

- Breaking CLI changes are allowed, but require a **major version bump** and a migration entry here.
- Same-name commands **must not** silently change behavior — either preserve semantics or remove the command.
- Future breaks should use alias + deprecation warning for 1–2 minor versions before removal; this v1.0 cleanup was a one-time exception because the surface change was too broad for aliases to be safe.
