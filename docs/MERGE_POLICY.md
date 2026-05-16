# Merge policy

This document codifies which CI checks must be green before a PR can be
merged into `main`. It is the human-readable counterpart to the GitHub
branch protection rules configured at:

> Settings → Branches → Branch protection rules → `main`

If the two ever drift, **GitHub is authoritative** — file a PR against
this doc to bring it back in sync.

## Required checks on `main`

A PR cannot be merged unless every one of these checks reports success on
the merge commit's tip:

| Check | Workflow | Why required |
|---|---|---|
| `lint` | Test | Catches gofmt / gosec / staticcheck issues that block release builds. |
| `unit (L1)` | Test | Unit + integration + contract: faked-runner Go tests *and* real `brew` / `git` / `npm` against temp dirs. Includes `internal/archtest` fitness rules. |
| `contract schema (L2)` | Test | Validates remote-config / snapshot JSON against the `openboot-contract` schemas. Breaking the contract breaks live users. |
| `curl\|bash smoke` | Test | Confirms `scripts/install.sh` still bootstraps the CLI against a mock server. |
| `old-cli compat` | Test | Runs the **previous** release binary against the **current** mock server. Catches server-side changes that would break already-shipped CLIs in the field. |

## Not required (and why)

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

## Why these five

Each required check covers a class of regression that has shipped to
users in past commits:

- `lint` blocks PRs that fail `golangci-lint` (would block release build).
- `unit (L1)` is the broadest behaviour check — covers both faked-runner
  unit logic and real-subprocess integration drift (brew flag changes,
  `git` exit-code shifts between macOS versions).
- `contract schema (L2)` is the contract with the openboot.dev API — any
  drift breaks every CLI in the field.
- `curl|bash smoke` covers the **primary install path** for new users.
  Breaking this is a silent acquisition disaster.
- `old-cli compat` is the contract with *already-installed* CLIs.
  Server-side changes that break this strand users on the version they
  have until they upgrade — which they may never do.

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
