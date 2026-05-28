# archtest

Architecture fitness functions for openboot. Each `*_test.go` file enforces one
project-level invariant documented in [CLAUDE.md](../../CLAUDE.md#project-specific-conventions).

This is the **Architecture Fitness Harness** described in Martin Fowler's
[Harness Engineering for Coding Agents](https://martinfowler.com/articles/harness-engineering.html):
cheap, fast, deterministic computational sensors that block architectural
drift introduced by either humans or AI agents.

## How it works

Each rule has two pieces:

1. A test in `*_test.go` that walks the AST of every file under `internal/`
   and `cmd/` and collects call sites matching a forbidden pattern.
2. A baseline file under `baseline/<rule-name>.txt` listing existing
   violations as `<file>:<line>`. The test fails only on **new** violations
   — entries in the baseline pass silently.

Stale baseline entries (the code was fixed but the line was not removed
from the baseline) are logged but do not fail the test.

## Current rules

| Rule | Test | Baseline | Source convention |
|---|---|---|---|
| `no-direct-exec` | `exec_test.go` | yes | "Do not call `exec.Command` directly from feature code" |
| `no-raw-http` | `http_test.go` | yes | "Use `httputil.Do()` — handles 429 + Retry-After" |
| `no-os-getenv-home` | `envhome_test.go` | no (hard rule) | "Use `os.UserHomeDir()` — never hardcode `~` or `/Users/...`" |
| `dryrun` | `dryrun_test.go` | yes | "Destructive ops: check `cfg.DryRun` first. Always." |

## Workflow

```bash
# Normal run (CI, pre-push, local).
go test ./internal/archtest/...

# After an intentional change that adds a violation, regenerate baseline:
ARCHTEST_UPDATE_BASELINE=1 go test ./internal/archtest/...
git add internal/archtest/baseline/
```

When the regeneration adds entries, the commit message should explain why
the new call site is justified — those baseline diffs are the audit trail.

## Adding a new rule

1. Add `<name>_test.go` with a `Test<Name>` function.
2. Use `walkProductionFiles` + `findCalls` / `findIdentUses` (or hand-roll
   an `ast.Inspect`) to collect violations.
3. Decide:
   - **Hard rule** (currently zero violations and should stay that way) —
     fail immediately on any hit. See `envhome_test.go`.
   - **Soft rule** (baseline existing call sites) — call `enforce(t, rule, found)`.
     Generate the baseline once with `ARCHTEST_UPDATE_BASELINE=1`.
4. Document the rule in CLAUDE.md "Project-specific conventions" so agents
   read it before writing code.

## Why not just use `golangci-lint`?

Lint config can express "no calls to X". It cannot express "no calls to X
**except in these packages, with this allowlist of existing call sites**"
without a custom analyzer. `forbidigo` comes close but is global. archtest
is project-scoped and grows with conventions specific to this codebase.
