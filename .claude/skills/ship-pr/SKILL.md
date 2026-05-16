---
name: ship-pr
description: Use when the user wants to open a pull request for the current branch — phrases like "open a PR", "ship this", "submit PR", "let's send it", "提 PR", "提个 MR". Walks through the canonical post-edit flow: local architecture review → L1 unit tests → push → gh pr create → gh pr merge --auto. Trigger any time the user signals they're done editing and want the change on its way to main; do NOT trigger for `gh pr view` / status checks on existing PRs.
---

# Ship a PR for openboot

This is the canonical way to move a finished change from a feature branch
into `main`. The branch protection rules defined in [docs/MERGE_POLICY.md](../../../docs/MERGE_POLICY.md)
do the gating — this skill makes sure the obvious local checks are done
first and that auto-merge is enabled so the change lands without a human
having to babysit CI.

## When to use

Trigger when the user says they're done with a change and want a PR. **Do
not** use this for:

- Draft PRs / WIP work the user wants reviewed but not merged. Use plain
  `gh pr create --draft` and skip auto-merge.
- Changes to `.github/workflows/` or branch protection. Those need human
  review on `main`'s ruleset interaction.
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

### Step 2 — Local review (cheap, catches most issues)

Invoke the [`architecture-review`](../architecture-review/SKILL.md) skill
on the current diff. Fix anything it flags before moving on. The point of
running it locally is to avoid burning a CI cycle on a violation that
`internal/archtest` would catch in ~15 seconds.

### Step 3 — L1 locally

```bash
make test-unit
```

This is the same suite the `pre-push` hook would run, so think of it as
"don't ship without doing what the hook does". If `make install-hooks` is
already wired, this step is redundant — but running it explicitly costs
~15s and removes any uncertainty.

### Step 4 — Push

```bash
git push -u origin "$(git rev-parse --abbrev-ref HEAD)"
```

### Step 5 — Open the PR

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

### Step 6 — Hand the PR to GitHub's auto-merge

```bash
gh pr merge --auto --squash --delete-branch
```

GitHub will hold the merge until every required check from
[docs/MERGE_POLICY.md](../../../docs/MERGE_POLICY.md) reports success —
currently:

- `lint`
- `unit (L1)`
- `integration (L2)`
- `contract schema (L3)`
- `curl|bash smoke`
- `old-cli compat`

There is no need to `gh pr checks --watch` in the session. If any check
fails, GitHub will refuse to merge and the user will see the failure on
the PR — handle it the next time the user mentions the PR.

### Step 7 — Tell the user

Return the PR URL and one sentence on what was queued. Example:

> PR opened: <url>. Auto-merge enabled — will land once the 6 required
> checks pass.

## What NOT to do

- **Do not skip step 2.** "It's a small change" is exactly when archtest
  violations sneak in.
- **Do not merge without `--auto`** (i.e. plain `gh pr merge`) unless the
  user explicitly asks for an immediate merge. Bypassing required checks
  is reserved for emergencies and must be documented.
- **Do not use `--admin`.** That bypasses branch protection entirely.
- **Do not amend / force-push** after `gh pr create` unless the user asks
  — it invalidates in-flight reviews.
- **Do not push directly to `main`.** Branch protection will reject it
  anyway, but try anyway and you'll have to clean up the failed push.

## Why this is encoded as a skill

Per [docs/HARNESS.md](../../../docs/HARNESS.md), repeated guidance
becomes a control. The post-edit flow ("review → test → push → PR →
auto-merge") was previously oral tradition; encoding it here means:

1. New agents follow the same recipe without being told.
2. If the recipe changes (e.g. a new required check), this is the one
   place to edit, and AGENTS.md points here.
3. Skipped steps become a reviewable diff against this file, not a
   judgment call buried in a session transcript.
