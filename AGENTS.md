# AGENTS.md

Canonical pointer for AI coding agents working on this repo. If you're a
human, you probably want [README.md](README.md) or [CONTRIBUTING.md](CONTRIBUTING.md).

## Read first

- **[CLAUDE.md](CLAUDE.md)** — project conventions and where-to-look table.
  Treat this as authoritative; this file is a summary index for agents.
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — test layering L1–L4, Runner
  interface, hook setup.
- **[docs/HARNESS.md](docs/HARNESS.md)** — the steering meta-doc: when a
  class of issue recurs, which file do you edit to prevent it next time.

## Project invariants (the things that must not drift)

These are enforced by [`internal/archtest`](internal/archtest/README.md) as
fitness functions. New violations fail `make test-unit` (L1). If you're an
agent and a violation is intentional, the test message tells you how to
update the baseline — do not silence the rule.

| Invariant | Enforced by |
|---|---|
| `exec.Command` only in `internal/system` and documented runner packages | `internal/archtest/exec_test.go` |
| `http.NewRequest` / `http.DefaultClient` only in `internal/httputil` | `internal/archtest/http_test.go` |
| Use `os.UserHomeDir()` — never `os.Getenv("HOME")` | `internal/archtest/envhome_test.go` |
| Error wrapping with `%w`, no bare returns | reviewer + `errcheck` (golangci-lint) |
| UI output via `ui.*` helpers — never raw `fmt.Println` in user-facing paths | reviewer (planned: archtest rule) |
| Destructive ops check `cfg.DryRun` before acting | reviewer (planned: archtest rule) |

The full convention list lives in CLAUDE.md → "Project-specific conventions".

## Run before committing

```bash
go vet ./...                 # cheap, ~1s
go test ./internal/...       # L1, ~15s, includes archtest
```

Both are wired into `scripts/hooks/pre-commit` (vet only) and
`scripts/hooks/pre-push` (full L1). Install once with `make install-hooks`.

## When archtest fails

The failure message tells you exactly what to do. Two cases:

1. **You added a new violation by accident** — fix the code (move the call
   into the allowed package, use the wrapper, etc.).
2. **The violation is intentional** — append the new `file:line` to
   `internal/archtest/baseline/<rule>.txt` via:

   ```bash
   ARCHTEST_UPDATE_BASELINE=1 go test ./internal/archtest/...
   git add internal/archtest/baseline/
   ```

   The diff to the baseline file IS the audit trail — explain in your
   commit message why the new call site is justified.

## Skills

Project-specific Claude skills live under [`.claude/skills/`](.claude/skills/):

- `bootstrap-feature` — how to add a CLI command end-to-end.
- `architecture-review` — what to check when reviewing a PR against
  project invariants.
- `ship-pr` — canonical post-edit flow: push → `gh pr create` → wait
  for CI → review the diff → triage (self-fix small stuff, escalate
  decisions, merge directly when clean) → `gh pr merge --squash` →
  local cleanup. Use this instead of calling `gh pr create` /
  `gh pr merge` directly. **Do not use `--auto`** — it skips the
  review gate, which is the point of having this skill on top of the
  branch-protection rules in [`docs/MERGE_POLICY.md`](docs/MERGE_POLICY.md).
  **Do not ask the user to confirm a clean merge** — the loop closes
  itself when there is nothing to decide.

These are loaded automatically when Claude runs in this repo.

## Tools you may NOT use (without asking)

- `git push --force` against `main` or release tags.
- `git commit --amend` on commits already pushed.
- `git reset --hard` discarding uncommitted work.
- Running `make test-vm-inner` (or `test-vm-inner-run`) outside a throwaway
  machine — these install real packages onto the current host.
- Anything that modifies the user's `~/.zshrc`, Homebrew install, or
  macOS `defaults`.

Everything else (Edit/Write/Read in repo, `make test-unit`, `go vet`, etc.)
is safe to run without confirmation.
