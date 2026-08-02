# Merge policy

This document codifies which CI checks must be green before a PR can be
merged into `main`. It is the human-readable counterpart to the GitHub
branch protection rules configured at:

> Settings → Branches → Branch protection rules → `main`

If the two ever drift, **GitHub is authoritative** — file a PR against
this doc to bring it back in sync.

## Required checks (block merge)

All six run on every PR. There is no post-merge-only tier: every job in
`.github/workflows/test.yml` fires on `push` (to `main`/`master`),
`pull_request`, `repository_dispatch` (`contract-updated`), and
`workflow_dispatch` alike, with no job-level `if:` gating.

`vm-e2e` is the exception in the other direction — it is **PR-only**. Its
workflow's `push` trigger is scoped to the `test/vm-e2e-speed` spike
branch, so it does not run on `main` at all.

| Check | Workflow | Why required |
|---|---|---|
| `lint` | Test | Catches gofmt / gosec / staticcheck issues that block release builds. |
| `unit (L1)` | Test | Unit + integration + contract: faked-runner Go tests *and* real `brew` / `git` / `npm` against temp dirs. Includes `internal/archtest` fitness rules. |
| `contract schema (L2)` | Test | Validates remote-config / snapshot JSON against the `openboot-contract` schemas, and asserts the CLI decoders consume the canonical fixtures losslessly. |
| `curl\|bash smoke` | Test | Builds the binary, starts `scripts/mock-server.py`, and pipes a served install script into `bash`, driving `openboot install -s -u <slug>` end to end against a mock API. Despite the name it does **not** exercise `scripts/install.sh` — that is covered in L1 by `test/integration/install_script_test.go`. |
| `old-cli compat` | Test | Runs the previous release binary against the current mock server. Catches server-side changes that would break already-shipped CLIs. |
| `vm-e2e` | vm-e2e-spike | Exercises the destructive install paths and TUI choreography on a fresh Apple Silicon macOS VM. |

### Not required (and why)

| Check | Status | Reason |
|---|---|---|
| Harness drift sensors (`govulncheck`, `deadcode`, `mod-tidy diff`, `archtest stale baseline`) | `continue-on-error: true` | Informational by design. Failures surface as annotations and, on `main`, open tracking issues via `drift-to-issue.yml`. |
| `codecov/patch` | informational | Coverage threshold is a guideline, not a gate. Hard coverage gates push toward test-shaped code without raising actual quality. |

## Operating principles

- **Floor, not ceiling.** Branch protection enforces the floor every PR
  must clear. Human review still raises the ceiling — *form*-level checks
  cannot replace a reviewer reading the diff for behaviour or design.
- **No admin bypass for routine work.** Admins can override in genuine
  emergencies but should not make a habit of it. Bypass usage is visible
  in the GitHub audit log.
- **Required ≠ blocking forever.** If a check is broken upstream (e.g.
  GitHub Actions outage), document the bypass in the merge PR description.
- **New checks are NOT auto-required.** Adding a workflow job does not
  add it to this list. Promote a check to required by editing this doc
  and updating branch protection in the same PR.

## Why these six

Each required check covers a class of regression that has shipped to
users in past commits:

- `lint` blocks PRs that fail `golangci-lint` (would block release build).
- `unit (L1)` is the broadest behaviour check — covers both faked-runner
  unit logic and real-subprocess integration drift (brew flag changes,
  `git` exit-code shifts between macOS versions).
- `contract schema (L2)` catches CLI ↔ server wire drift. Tolerant
  decoders like `UnmarshalRemoteConfigFlexible` will silently repair,
  move, or drop fields; only a canonical-fixture comparison notices.
- `curl|bash smoke` is the only check that drives a real built binary
  through a config install against a live HTTP API, catching wiring
  breakage that faked-runner tests structurally cannot reach.
- `old-cli compat` catches server-side changes that break CLIs already
  on users' machines — the one regression class the current binary's
  own tests structurally cannot see.
- `vm-e2e` (L4) covers the destructive and terminal-dependent paths that
  cannot safely run inside L1, including real Homebrew installs and the
  install-wizard choreography on a fresh macOS VM.

### The cost of requiring the two network-dependent checks

`contract schema (L2)` checks out `openboot-contract@main` and
`old-cli compat` downloads the previous GitHub release, so requiring
both ties every merge to state outside this repo. They fail in opposite
directions, and the difference is worth knowing before you trust either
badge:

- `contract schema (L2)` **blocks**. A red or mid-edit `openboot-contract`
  main stops every PR, including ones that touch neither the contract nor
  the decoders. That is the deliberate trade: tolerant decoders like
  `UnmarshalRemoteConfigFlexible` hide wire drift from every other check,
  so the alternative to a blocked PR is a user bug report.
- `old-cli compat` **passes silently**. Its release lookup ends in
  `|| true` and writes a possibly-empty `version=`, and every step after
  it is gated on `steps.prev.outputs.version != ''`. A GitHub API blip,
  or no stable release still carrying the arch asset, yields a green
  check that ran no compat test at all. It cannot block a PR — but a
  green tick is therefore not evidence that compat was verified.

If external flakiness does start blocking unrelated work, the escape
hatch is the documented bypass under *Operating principles*, not quietly
dropping the contexts from protection while this doc still lists them —
that is the exact drift this file exists to prevent.

## How to change this policy

The required-checks list has an in-repo source of truth:
[`.github/required-checks.txt`](../.github/required-checks.txt). The
`required-checks alignment (drift)` sensor in
[`.github/workflows/harness.yml`](../.github/workflows/harness.yml)
flags PRs that desync it from the workflow `name:` values. It is
`continue-on-error: true`, so it annotates rather than blocks.

It also has two blind spots. It never reads live branch protection, so a
context added or removed in the GitHub UI alone drifts silently; and it
only checks one direction — every line in the file must map to a job,
but a required context missing from the file is not flagged. Step 3
below is the only thing that catches either.

1. Open a PR that edits this file **and** `.github/required-checks.txt`
   with the proposed change.
2. In the same PR, update live branch protection via the GitHub UI **or**
   include the `gh api` command in the PR description so the reviewer can
   reproduce it. Example:

   ```bash
   gh api -X PUT repos/openbootdotdev/openboot/branches/main/protection \
     --input docs/_protection.json
   ```

3. After merge, verify with `gh api repos/openbootdotdev/openboot/branches/main/protection`.
