# Harness engineering for openboot

This document describes the **harness** around the openboot codebase: the
set of controls that catch drift and steer both human and AI contributors
toward correct outputs. It is based on Martin Fowler's
[Harness Engineering for Coding Agents](https://martinfowler.com/articles/harness-engineering.html).

If you're trying to add a new control or reason about why an existing one
exists, this is the right place to start.

## Mental model

> Agent = Model + Harness. The harness is everything you can change.

We can't change the underlying LLM. We can change what guidance it gets
*before* writing code (**feedforward**) and what feedback it gets *after*
(**feedback**). When an issue recurs, the right reaction is not "tell the
agent again" — it's to **encode the rule into the harness** so the next
agent (or the next refactor by a human) cannot drift the same way.

Two execution flavors:

- **Computational** — deterministic, fast, free: `go vet`, `golangci-lint`,
  `make test-unit`, `internal/archtest/*`. Run on every change.
- **Inferential** — non-deterministic, slower, paid: AI code review,
  `/security-review`, `/ultrareview`. Run on integration boundaries.

Three regulation categories:

1. **Maintainability** — code style, complexity, dead code.
2. **Architecture fitness** — project-specific invariants (the "do X, not Y"
   rules in CLAUDE.md).
3. **Behaviour** — does it actually do the right thing.

## Where each control lives

| Category | Control | Trigger | File |
|---|---|---|---|
| Maint. | `gofmt` / `goimports` | save / lint | `.golangci.yml` formatters |
| Maint. | `errcheck`, `staticcheck`, `gosec`, `gocyclo`, `unused`, `ineffassign`, `misspell`, `unconvert`, `exhaustive`, `govet` | `make lint` / CI `lint` job | `.golangci.yml` |
| Maint. | `govulncheck` (drift) | informational CI | `.github/workflows/harness.yml` |
| Maint. | `deadcode` (drift) | informational CI | `.github/workflows/harness.yml` |
| Maint. | `go mod tidy -diff` | informational CI | `.github/workflows/harness.yml` |
| Maint. | `required-checks alignment (drift)` — `.github/required-checks.txt` ↔ workflow job names | informational CI | `.github/workflows/harness.yml` |
| Arch. | `no-direct-exec` | L1 (`make test-unit`) | `internal/archtest/exec_test.go` |
| Arch. | `no-raw-http` | L1 | `internal/archtest/http_test.go` |
| Arch. | `no-os-getenv-home` | L1 | `internal/archtest/envhome_test.go` |
| Arch. | `dryrun` — destructive ops must check `DryRun` | L1 | `internal/archtest/dryrun_test.go` |
| Arch. | `no-raw-fmtprint` — UI output via `ui.*` helpers, not raw `fmt.Print*` | L1 | `internal/archtest/fmtprint_test.go` |
| Behav. | L1 unit + integration + contract (faked runners *and* real brew/git/npm in temp dirs) | pre-push, CI | `make test-unit` |
| Behav. | L2 contract schema (against openboot-contract repo) | CI | `.github/workflows/test.yml` `contract` job |
| Behav. | L3 e2e binary | release | `make test-e2e` |
| Behav. | L4 VM e2e (`vm`) — full destructive suite on a clean macOS host | every PR | `.github/workflows/vm-e2e-spike.yml` (macos-14 runner, two parallel jobs) |
| Behav. | Install-wizard TUI on a real pty — L3: launch/quit smoke + full keyboard choreography (stops before confirm, installs nothing); L4: same key sequence through a real install via `expect(1)`, asserting brew/git system state | L3 at release, L4 every PR | `test/e2e/install_wizard_e2e_test.go`, `test/e2e/install_wizard_vm_test.go` |
| Behav. | curl\|bash smoke (install.sh + mock server) | every PR | `.github/workflows/test.yml` `curl-bash-smoke` job |
| Behav. | Auto-release sensor — patch fast lane (`fix:`-only) auto-tags + dispatches `release.yml`; feat threshold opens a `release-ready` issue (check L4 CI green, then tag manually) | push to `main` | `.github/workflows/auto-release.yml` |
| Behav. | Release notes — Conventional Commits since previous tag, grouped by type (Features / Bug Fixes / etc) + Full Changelog link, appended to the install-instructions template | tag push or `workflow_dispatch` | `.github/workflows/release.yml` (`Write release notes` step) |
| Behav. | Old-CLI compat (previous release × current mock server) | every PR | `.github/workflows/test.yml` `cli-compat` job |
| Feedfwd. | Agent conventions | every AI turn | `CLAUDE.md`, `AGENTS.md` |
| Feedfwd. | Skills | model-loaded | `.claude/skills/*` |
| Feedfwd. | Session-start hook (warm caches, fetch deps) | every Claude session | `.claude/hooks/session-start.sh` |
| Feedback (agent) | `go vet` on edited package | after every Edit/Write/MultiEdit | `.claude/hooks/post-tool-use.sh` |
| Feedback (agent) | `go vet ./...` + archtest | end of every Claude turn (if .go dirty) | `.claude/hooks/stop.sh` |
| Maint. | `golangci-lint` on the staged diff | local git pre-commit | `scripts/hooks/pre-commit` |
| Drift loop | Failed harness sensor → open/update GitHub issue | on main / nightly | `.github/workflows/drift-to-issue.yml` |

## The steering loop

When you observe a recurring issue, decide where to encode the fix:

| Observation | Encode it as |
|---|---|
| "Agent keeps calling `exec.Command` from a feature package." | New entry in `execAllowedPaths` *or* refactor + update baseline. The rule itself is already in `internal/archtest`. |
| "Agent doesn't know about preset X." | Update `internal/config/data/presets.yaml`. Source of truth, not docs. |
| "Agent introduced a new lint failure that golangci-lint should have caught." | Enable the relevant linter in `.golangci.yml`. |
| "Agent broke a behaviour that has no test." | Write the test at the right tier — L1 covers both faked-runner units in `internal/<pkg>/` and real-subprocess integration in `test/integration/`. |
| "Agent missed a CLAUDE.md rule we keep restating." | Make it a hard or soft archtest rule (a docs rule that doesn't fail is a docs rule that drifts). |
| "Agent did something safe but suboptimal." | Add to CLAUDE.md "Project-specific conventions" and consider whether it's encodable. |
| "Agent guessed at an API contract." | Update `openboot-contract` repo + fixtures; CI already runs schema validation. |
| "Agent's PR description was off." | Tighten `pull_request_template.md`. |
| "Fixes and features piled up on `main` because nobody told an agent to cut a release." | Already handled: `.github/workflows/auto-release.yml` auto-tags patches and opens a `release-ready` issue for feats. Tune thresholds there. |
| "PR silently blocked because branch protection required a check the workflow no longer produces (rename / removal)." | Update `.github/required-checks.txt` in the same PR — the `required-checks alignment (drift)` sensor compares it against workflow job names. Then mirror the change to live protection via `gh api -X PUT .../protection` per `docs/MERGE_POLICY.md`. |

Rule of thumb: **if you reach for a doc edit, ask first whether a test or
analyzer would catch the same drift mechanically**. Mechanical wins because
it survives doc rot.

## What's intentionally NOT in the harness

- **No coverage gate that fails PRs.** Coverage is informational
  (`codecov.yml` `informational: true`). Hard coverage gates push toward
  test-shaped code without raising actual quality.
- **No fmt.Print/Println archtest rule yet.** The convention exists in
  CLAUDE.md but the codebase has ~150 existing call sites and the rule
  would be mostly noise. Reconsider after the UI helpers cover all the
  cases currently using raw stdout.
- **No agent-driven changes to `main` without human review.** All AI
  changes go through PR review and the existing CI matrix.
- **No retroactive refactors triggered by new archtest rules.** New rules
  baseline existing code so green builds stay green. Cleanup is a separate
  decision from rule introduction.
- **No session-start stale-branch sensor.** A previous iteration of the
  `ship-pr` skill used `gh pr merge --auto`, so the merge could happen
  asynchronously after the Claude session ended — leaving a feature
  branch checked out next time. The sensor existed to nag the user to
  clean it up. Once the skill dropped `--auto` and made cleanup part of
  the in-session loop (Step 8), the sensor became dead code and was
  removed. If `--auto` ever comes back, the sensor needs to come back
  with it.
- **No archtest rule for scroll-region usage.** `internal/ui/scrollregion.go`
  detects support at runtime (`TERM`, TTY, terminal height) and falls back
  to the inline `\r\033[K` renderer when unavailable. A static rule can't
  see runtime terminal capabilities, so this stays a runtime concern. The
  fallback is covered by `TestStickyProgressFallsBackWhenScrollRegionUnsupported`.
- **L4 runs on GitHub Actions, not a self-hosted runner.** `macos-14`
  runners are Apple Silicon VMs — each job gets a fresh clean macOS
  environment, which is exactly what L4 needs. Tart is no longer required.
  The L4 workflow (`vm-e2e-spike.yml`) is not yet a hard merge gate (not in
  `required-checks.txt`); it runs on every PR. Promoting it to a required
  check is the next step once the workflow has proven stable.

## How agents should think about this file

If you are reading this as an AI agent: this file is your guide to *where
to add a control*, not a checklist to run. The actual checks fire from
test commands and CI jobs. The most useful thing you can do is, when a
review reveals a recurring issue, propose where in this table to add a
new row — that's how the harness improves over time.
