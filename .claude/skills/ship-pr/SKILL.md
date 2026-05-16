---
name: ship-pr
description: Use when the user wants to open a pull request for the current branch — phrases like "open a PR", "ship this", "submit PR", "let's send it", "提 PR", "提个 MR". Walks through the canonical post-edit flow: push → open PR → wait for CI → review the diff → triage findings (self-fix small issues, escalate decisions to the user, merge directly when clean) → local cleanup. Trigger any time the user signals they're done editing and want the change on its way to main; do NOT trigger for `gh pr view` / status checks on existing PRs.
---

# Ship a PR for openboot

This is the canonical way to move a finished change from a feature branch
into `main`. The branch protection rules defined in [docs/MERGE_POLICY.md](../../../docs/MERGE_POLICY.md)
define the **mechanical** gate (6 required checks). This skill adds the
**inferential** gate on top: after CI is green, the diff gets reviewed,
findings are surfaced, and merge only happens after the user confirms.

**Auto-merge is intentionally not used.** `gh pr merge --auto` skips the
review step entirely — that defeats the purpose of having a review gate.
The merge command is run from this session, after the user OKs it.

## When to use

Trigger when the user says they're done with a change and want a PR. **Do
not** use this for:

- Draft PRs / WIP work the user wants reviewed but not merged. Use plain
  `gh pr create --draft` and stop.
- Changes to `.github/workflows/` or branch protection rules. Those need
  a human to also poke the GitHub UI.
- Release tags (`v*.*.*`). Releases follow `make test-vm-release` first.

## The flow

### Step 1 — Confirm the branch is shippable

```bash
git status -sb                       # clean except expected diff?
git rev-parse --abbrev-ref HEAD      # not on main
git log --oneline main..HEAD         # commits actually exist
```

If the branch is `main`, stop — make a feature branch first. If there are
no commits ahead of `main`, stop — nothing to ship.

### Step 2 — Push

```bash
git push -u origin "$(git rev-parse --abbrev-ref HEAD)"
```

The `pre-push` hook (installed via `make install-hooks`) runs L1 here
automatically — no need to run `make test-unit` separately. If the hook
is not installed, that's the user's choice; CI will still gate.

### Step 3 — Open the PR

```bash
gh pr create --title "<conventional commit subject>" --body "$(cat <<'EOF'
## Summary

- <bullet 1>
- <bullet 2>

## Test plan

- [ ] <how you verified>
EOF
)"
```

Title rules (from CLAUDE.md):
- Conventional Commits prefix: `feat:` / `fix:` / `docs:` / `refactor:` /
  `test:` / `chore:` / `ci:` / `perf:`.
- Keep under 70 chars. Detail goes in the body.

### Step 4 — Wait for CI

```bash
gh pr checks --watch
```

This blocks until every check finishes. Typical wall time is 3–10 min
depending on which jobs run. The required checks per
[docs/MERGE_POLICY.md](../../../docs/MERGE_POLICY.md):

- `lint`, `unit (L1)`, `integration (L2)`, `contract schema (L3)`,
  `curl|bash smoke`, `old-cli compat`

If any required check fails:
- **Stop. Do not proceed to review or merge.**
- Read the failure (`gh run view --log-failed`), explain it to the user,
  and ask whether to fix it now.
- Drift sensors (`govulncheck`, `deadcode`, etc.) failing is
  informational — flag them but do not block.

### Step 5 — Review the diff

Once CI is green, invoke the
[`architecture-review`](../architecture-review/SKILL.md) skill on the
**full PR diff** (`git diff main...HEAD`). CI catches mechanical
violations; this catches the rest — behaviour, design, test coverage,
risk, rollback.

### Step 6 — Triage the findings

Every finding goes into one of two buckets. Get this right or the gate
fails open.

**Self-fixable — fix in this session, then loop back to Step 4.**
Anything where the correction is mechanical and clearly inside the
PR's stated scope:
- typos, formatting, doc rewording for clarity,
- dead code that this PR introduced,
- missing imports, missing test cases for branches this PR adds,
- bug fixes that don't change observable behaviour,
- following through on a rule the user has already stated in this
  session (this is the recursive case).

Fix it, push to the same branch, return to Step 4 to wait for CI again,
then come back to Step 5. Do **not** prompt the user. The point of this
branch is to keep the human's attention budget for decisions that
actually need it.

**Needs user judgment — surface and stop.**
Anything with a real choice or scope question:
- design decisions, API shape, behaviour changes,
- anything that touches a deliberate prior decision (e.g. an explicit
  user instruction or an existing convention in CLAUDE.md),
- anything that would expand the PR beyond its stated scope,
- anything you'd ask a teammate about before pushing.

Surface the finding with the question made explicit; stop the flow.

**Clean — proceed straight to merge.**
If CI is green AND the diff review found nothing self-fixable AND
nothing that needs user judgment, skip the "ask the user" step and
merge directly. Asking when there is nothing to decide just burns the
user's attention.

**Rule of thumb:** would a thoughtful junior engineer file this as a
question, push a follow-up commit, or just merge it? Escalate / fix /
merge accordingly.

### Step 7 — Merge

Reached when Step 6 ended in "clean", OR when the user has explicitly
approved a merge after an escalation.

```bash
gh pr merge --squash --delete-branch
```

No `--auto`. No `--admin`. The branch-protection rules still apply — if
something flipped red between Step 4 and now, GitHub will refuse and
we'll loop back to Step 4.

Report the merge to the user as a one-liner ("PR #N merged, branch
deleted, local cleaned up"). Do not ask for confirmation **before**
merging on a clean review — the harness loop is supposed to close
itself when there is nothing to decide.

### Step 8 — Local cleanup

After merge succeeds, the remote branch was deleted by `--delete-branch`.
Bring local in sync:

```bash
git checkout main && git pull --quiet
git branch -d "$(git symbolic-ref --quiet --short @{-1} 2>/dev/null)"
```

(`@{-1}` refers to the previously checked-out branch — the one we just
merged.)

If the user closes the session before reaching Step 8, the
[`session-start.sh`](../../hooks/session-start.sh) stale-branch sensor
catches it on the next session and prints the same cleanup hint.

## What NOT to do

- **Do not use `--auto`.** It skips Step 5 (review) entirely, which is
  the whole reason this skill exists.
- **Do not use `--admin`.** That bypasses branch protection.
- **Do not merge before CI completes.** Even if the diff looks trivial,
  let `gh pr checks --watch` finish — drift detection sometimes catches
  surprising things.
- **Do not amend / force-push** after `gh pr create` unless the user asks
  — it invalidates in-flight reviews and re-runs CI from scratch.
- **Do not push directly to `main`.** Branch protection will reject it.
- **Do not auto-fix findings that need user judgment.** Anything that
  could change behaviour, expand scope, or touches a prior deliberate
  choice gets surfaced in Step 6 — not silently committed. Self-fixable
  items (typos, missing tests, doc tweaks following a stated rule) are
  the exception, not the default.

## Why this is encoded as a skill

Per [docs/HARNESS.md](../../../docs/HARNESS.md), repeated guidance becomes
a control. The merge gate has two layers:

- **Mechanical** (branch protection / required checks) — already
  enforced by GitHub. Catches what CI knows how to check.
- **Inferential** (review of the diff) — was previously oral tradition.
  Encoding the order here ("CI green THEN review THEN ask THEN merge")
  is the harness; without it, the easy thing is auto-merge, which is
  cheap and wrong.

If the recipe changes (new required check, different review tool), this
is the one place to edit; AGENTS.md points here.
