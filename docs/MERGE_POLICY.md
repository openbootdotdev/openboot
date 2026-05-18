# Merge policy

This document codifies which CI checks must be green before a PR can be
merged into `main`. It is the human-readable counterpart to the GitHub
branch protection rules configured at:

> Settings → Branches → Branch protection rules → `main`

If the two ever drift, **GitHub is authoritative** — file a PR against
this doc to bring it back in sync.

## CI stages

CI is split into two stages to keep PR feedback fast.

### Pre-merge (required — blocks merge)

Runs on every PR. Must pass before merge.

| Check | Workflow | Why required |
|---|---|---|
| `lint` | Test | Catches gofmt / gosec / staticcheck issues that block release builds. |
| `unit (L1)` | Test | Unit + integration + contract: faked-runner Go tests *and* real `brew` / `git` / `npm` against temp dirs. Includes `internal/archtest` fitness rules. |

### Post-merge (runs on push to `main`)

Does not block merge. Catches regressions on the merged state before
the auto-release sensor can tag. Runs on `workflow_dispatch` and
`repository_dispatch` too.

| Check | Workflow | Why post-merge |
|---|---|---|
| `contract schema (L2)` | Test | Clones external repo + pip install — too slow for every PR. Validates remote-config / snapshot JSON against the `openboot-contract` schemas. |
| `curl\|bash smoke` | Test | Builds binary + starts mock server — too slow for every PR. Confirms `scripts/install.sh` still bootstraps the CLI. |
| `old-cli compat` | Test | Downloads previous release from GitHub — too slow and network-dependent for every PR. Catches server-side changes that would break already-shipped CLIs. |

### Not required (and why)

| Check | Status | Reason |
|---|---|---|
| `macos e2e (L4)` | runs only on tag pushes / manual dispatch | Slow + destructive; runs at release time, not per PR. |
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

## Why these two

Each required check covers a class of regression that has shipped to
users in past commits:

- `lint` blocks PRs that fail `golangci-lint` (would block release build).
- `unit (L1)` is the broadest behaviour check — covers both faked-runner
  unit logic and real-subprocess integration drift (brew flag changes,
  `git` exit-code shifts between macOS versions).

The three heavier checks (`contract schema (L2)`, `curl|bash smoke`,
`old-cli compat`) still run on every merge to `main` — they just don't
block PRs, because they're too slow or network-dependent to require on
every push to a feature branch.

## How to change this policy

The required-checks list has an in-repo source of truth:
[`.github/required-checks.txt`](../.github/required-checks.txt). The
`required-checks alignment (drift)` sensor in
[`.github/workflows/harness.yml`](../.github/workflows/harness.yml)
fails on PRs that desync it from the workflow `name:` values.

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
