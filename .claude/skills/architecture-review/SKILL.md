---
name: architecture-review
description: Use when reviewing a PR, branch, or staged diff in the openboot repo. Audits the change against project invariants from CLAUDE.md — subprocess hygiene, HTTP wrapper usage, error wrapping, dry-run gating, UI helper usage, test placement. Trigger when user says "review this", "check this PR", "audit the diff", or asks about whether a change follows project conventions.
---

# Architecture review for openboot

Run this checklist on any diff before approving it. The checks map 1:1 to
the rules in CLAUDE.md → "Project-specific conventions" and the
fitness functions in `internal/archtest`.

## Step 1 — Diff scope

Confirm what's changed:

```bash
git diff --stat main...HEAD                 # files
git diff main...HEAD -- 'internal/**/*.go'  # production Go
```

If the diff spans more than ~10 files or mixes feature + refactor +
docs, call that out and suggest splitting — one thing per commit is a
project convention.

## Step 2 — Run the mechanical checks first

```bash
go vet ./...
make test-unit                          # includes archtest
golangci-lint run ./...                 # if available
```

These catch ~80% of what a human reviewer would otherwise comment on.
**If they don't pass, stop reviewing and ask the author to fix first.**

## Step 3 — Architecture invariants (manual review of the diff)

For each changed file under `internal/` or `cmd/`:

| Check | Look for | If violated |
|---|---|---|
| Subprocess | New `exec.Command` / `exec.CommandContext` call | Should be in `internal/system` or wrapped in a Runner. Reject + suggest `internal/system.RunCommand`. |
| HTTP | New `http.NewRequest`, `http.Get`, `http.DefaultClient` | Should go through `internal/httputil.Do`. Reject + suggest wrapper. |
| Home dir | `os.Getenv("HOME")`, hardcoded `/Users/...`, hardcoded `"~/"` | Use `os.UserHomeDir()`. |
| Error wrap | `return err` without context | Should be `fmt.Errorf("doing X: %w", err)` unless the error is already wrapped one frame up. |
| UI output | `fmt.Println`, `fmt.Printf` in non-UI packages | Use `ui.Info`, `ui.Success`, etc. (legacy violations are baselined — only flag NEW ones). |
| Dry run | New destructive operation (mkdir, write, exec, network mutate) | Must check `cfg.DryRun` and print `[DRY-RUN] Would X` instead. |
| Embedded data | New `data/*.yaml` or schema change | Confirm fallback path still loads. |
| State path | New file under `~/.openboot/` | Confirm the path is created via `os.UserHomeDir()`, perms are `0o600` for secrets / `0o755` for dirs. |

## Step 4 — Test layer check

For each non-trivial change:

- **Pure logic** → must have an L1 test in `internal/<pkg>/`. If missing, request it.
- **Subprocess interaction** → fake via Runner in `internal/<pkg>/` OR add a
  real-subprocess test under `test/integration/` (still L1, no build tag).
- **CLI flag parsing** → table-driven L1 test.
- **Destructive op** → must run cleanly under `--dry-run` in a test.

If the change is small enough to land without tests (e.g. comment
rewording, typo fix), say so explicitly rather than letting it pass
unmentioned.

## Step 5 — Behaviour invariants

| Check | Why it matters |
|---|---|
| Does the change break the curl\|bash install path? | `scripts/install.sh` is the primary install vector. The smoke test CI job covers this — make sure it still passes. |
| Does the change alter the contract schema (config / snapshot JSON)? | If yes, the schema needs updating in `openboot-contract` repo and bumping. Otherwise the L2 contract job will fail. |
| Does the change add or change a CLI flag? | Confirm `--help` output is updated, and old-cli compat job still passes (previous release × new mock server). |
| Does the change touch `data/presets.yaml`? | Three presets must still parse and resolve. |
| Does the change touch `~/.openboot/*` file format? | Confirm backward compat with old files (snapshot, auth, state) or write a migration. |

## Step 6 — Risk assessment

End the review with:

- **Risk:** low / medium / high (one sentence on blast radius).
- **Rollback:** how a user / operator would revert (CLI flag, file delete,
  brew downgrade).
- **Observability:** what would tell us this broke in production
  (telemetry, GitHub issue pattern, smoke job).

## What the reviewer SHOULD NOT do

- Do not approve a change that adds an archtest violation without an
  updated `baseline/` file and a justification in the commit message.
- Do not approve mixed-purpose commits — request a split.
- Do not approve changes to `.github/workflows/` without confirming the
  workflow still runs (`act` locally or trigger on a draft PR).
- Do not approve direct edits to `data/packages.yaml` without confirming
  the source-of-truth is openboot.dev (see CLAUDE.md "Where to Look").
