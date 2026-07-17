---
name: ship-pr
description: Use when the user is done editing and wants to ship the current branch via a pull request — phrases like "open a PR", "ship it", "ship this", "submit the PR", "let's send it", "merge it", "land this", "提 PR", "提个 PR", "提个 MR". Trigger whenever the user signals a change is finished and should head to the default branch, even if they don't say the word "PR". Walks the whole branch-to-merge flow for any GitHub repo with a mandatory review gate — the steps and their rules live in the body. Do NOT trigger for `gh pr view` / status checks on an existing PR, for draft / WIP PRs the user wants opened but not merged, or for cutting a release tag.
license: MIT
metadata:
  category: git
  language: en
---

# Ship a PR

The canonical way to move a finished change from a feature branch to the
repo's default branch through a pull request — for **any** GitHub repo,
without hardcoding one project's build commands or check names.

There are two gates between your branch and the default branch:

- **Mechanical** — the repo's required CI checks. GitHub already enforces
  these via branch protection; let them run.
- **Inferential** — a human-judgment review of the diff. CI catches what
  it knows how to check; this catches behaviour, design, test coverage,
  risk, and rollback. This gate is the reason the skill exists.

**Auto-merge is intentionally not used.** `gh pr merge --auto` merges the
moment CI passes, which skips the review gate entirely — that defeats the
purpose. The merge command is run from this session, *after* the diff has
been reviewed.

## When NOT to use

- **Draft / WIP PRs** the user wants opened but not merged → not this
  flow; if it's already been invoked, just `gh pr create --draft` and stop.
- **Changes to `.github/workflows/`, branch protection, or repo settings**
  → these usually need a human in the GitHub UI too; flag it.
- **Cutting a release** (tags like `v1.2.3`) → releases follow the repo's
  own release process, not this flow.

## The flow

### Step 1 — Confirm the branch is shippable

```bash
git status -sb                                # clean except the expected diff?
git rev-parse --abbrev-ref HEAD               # the current branch
git fetch origin --quiet                      # refresh the remote base ref
git remote set-head origin --auto >/dev/null  # fetch never creates origin/HEAD; this does
BASE=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@')
BASE=${BASE:-main}                            # the repo's default branch
git log --oneline "origin/$BASE..HEAD"        # commits actually exist
```

`$BASE` is the repo's default branch — usually `main`, sometimes `master`
or `develop`. Don't assume `main`; use what was detected. Comparisons run
against the **remote** ref `origin/$BASE` (hence the `git fetch`), so they
match what GitHub will diff the PR against even when the local base branch
is missing or stale — common in worktrees and feature-only checkouts.

- The remote isn't GitHub (`git remote get-url origin`) → **stop**; this
  flow is `gh`-only, tell the user (a GitLab "MR" needs a different flow).
- On the base branch → **stop**, make a feature branch first.
- No commits ahead of `origin/$BASE` → **stop**, nothing to ship.
- A PR may already exist for this branch (`gh pr view` succeeds) → still
  push (Step 2) so the PR has the latest commits, then skip Step 3 and
  continue at Step 4.

### Step 2 — Push

```bash
git push -u origin "$(git rev-parse --abbrev-ref HEAD)"
```

If the repo installs a `pre-push` hook (Husky, `core.hooksPath`, etc.) it
runs here automatically — don't re-run the same lint/test suite by hand.
If no hook is installed, that's the user's setup; CI is still the gate.

### Step 3 — Open the PR

Check for a PR template first — using it keeps the PR consistent with the
repo's conventions instead of inventing a different shape:

```bash
ls .github/pull_request_template.md .github/PULL_REQUEST_TEMPLATE.md \
   PULL_REQUEST_TEMPLATE.md pull_request_template.md \
   docs/PULL_REQUEST_TEMPLATE.md docs/pull_request_template.md \
   .github/PULL_REQUEST_TEMPLATE/ 2>/dev/null
```

If a template exists, fill in its sections honestly — including any
cross-repo / checklist items (e.g. "needs a docs update?"). Do **not**
discard it by passing a wholly custom `--body`. `gh pr create` without
`--body` tries to open an interactive editor, so fill the template
offline and pass it back:

```bash
gh pr create --title "<conventional commit subject>" --body-file <filled-template-file>
```

If there's no template, fall back to a sensible default:

```bash
gh pr create --title "<conventional commit subject>" --body "$(cat <<'EOF'
## Summary

- <what changed and why>

## Test plan

- [ ] <how you verified>
EOF
)"
```

Title rules:

- Conventional Commits prefix: `feat:` / `fix:` / `docs:` / `refactor:` /
  `test:` / `chore:` / `ci:` / `perf:` / `style:` / `build:` / `revert:`.
- Optional scope: `fix(api):`, `feat(editor):`.
- Keep under ~70 chars. Detail goes in the body.
- Several commits on the branch → write a fresh subject that sums up the
  whole PR (squash merge makes it the final commit subject).

### Step 4 — Wait for CI

```bash
gh pr checks --watch
```

This blocks until every check finishes. (Run straight after `gh pr create`
it can error with "no checks reported" before the first check registers —
wait a few seconds and re-run; exit code 8 just means checks are still
pending.) If a **required** check fails:

- **Stop. Do not proceed to review or merge.**
- Read the failure (`gh run view --log-failed`), explain it to the user,
  and ask whether to fix it now.
- Checks that aren't required (drift sensors, optional coverage, advisory
  scans) are informational — flag them, but they don't block the merge.

If you can't tell which checks are required, the ones branch protection
enforces are; `gh pr checks` marks the rest, and the merge in Step 7 will
refuse anyway if a required one is red.

### Step 5 — Get a review

Once CI is green, the diff needs an inferential review — behaviour, design,
test coverage, risk, rollback. Prefer to have the **`@claude` GitHub bot** do
this first pass *on the PR itself*, so the review lives next to the code where
the whole team can see it. Fall back to a local review only when the bot isn't
available.

**5a — Is the Claude bot wired up for this repo?**

The bot is the Claude GitHub App driven by `anthropics/claude-code-action`. The
reliable, permission-free signal is a workflow that uses it:

```bash
grep -rilE 'anthropics/claude-code-action|@claude' .github/workflows 2>/dev/null
```

(With repo-admin scope you could also confirm via
`gh api "repos/{owner}/{repo}/installations" --jq '.[].app.slug'`, but that
403s without admin — don't depend on it.)

- Match found → the bot can review; go to **5b**.
- No match → the app isn't set up here; skip to **5d (local fallback)**.

**5b — Request the review (or pick up an automated one)**

```bash
PR=$(gh pr view --json number -q .number)
```

Some repos run `claude-code-action` automatically on every push
(`on: pull_request`), so a `claude[bot]` review for the current commit may
already be on its way — check before posting, to avoid asking twice:

```bash
gh pr view "$PR" --json comments \
  -q '.comments[] | select(.author.login|test("claude|github-actions")) | "\(.author.login)\t\(.createdAt)"'
```

(Comment objects carry `createdAt`, not `updatedAt` — and the sticky
comment is edited in place, so an old timestamp can still be a live
review. When in doubt, just mention the bot again.)

If there's no current bot comment, summon it with a mention. A focused prompt
gets a more useful review than a bare "review this":

```bash
gh pr comment "$PR" --body "@claude please review this PR — focus on correctness, design, test coverage for the branches it adds, risk, and how it rolls back."
```

(`@claude` is the default trigger; a repo can rename it via `trigger_phrase` in
its workflow. If the mention gets no response at all, check the workflow for a
custom phrase.)

**5c — Wait for the bot, then read its review**

The bot answers in a single **sticky comment it edits in place** — it shows
progress first (checkboxes like "Analyzing…") and fills in the real review when
done. Wait for it to *finish*; don't act on a half-written comment. It usually
lands in under a minute, occasionally a few minutes under load.

```bash
for i in $(seq 1 30); do
  body=$(gh pr view "$PR" --json comments \
    -q '[.comments[] | select(.author.login|test("claude|github-actions"))] | last | .body')
  if [ -n "$body" ] && ! printf '%s' "$body" | grep -qiE 'analyzing|in progress|- \[ \]'; then
    printf '%s\n' "$body" && break
  fi
  sleep 10
done
```

Re-fetch until the body reads as a *completed* review (no lingering
"Analyzing…/in progress" markers). The author is normally `claude[bot]`;
a repo using a custom `github_token` surfaces it as `github-actions[bot]`
instead — either way it's obviously a Claude review by its content. A
workflow configured to submit a formal PR review rather than a comment
shows up under `gh pr view --json reviews` — glance there before declaring
the bot unavailable. If nothing
substantive shows up after a few minutes, treat the bot as unavailable and fall
back. (If the loop times out but a bot comment *is* visible — e.g. a finished
review whose body happens to contain unchecked checkboxes — read it manually
instead of discarding it.) Carry whatever it found into Step 6.

**5d — Local fallback**

When the bot isn't wired up, or never returns a finished review, review the
**full PR diff** yourself so the gate still closes:

```bash
git diff "origin/$BASE"...HEAD
```

- If a `/code-review` command or a repo-specific review skill is available, run
  it on the full diff — it's purpose-built for this.
- Otherwise review inline: behaviour, design, test coverage for the branches
  this PR adds, risk, and how it rolls back.

Either way, the findings feed into Step 6.

### Step 6 — Triage the findings

Every finding lands in exactly one bucket. Getting this right is what
keeps the gate from failing open.

**Self-fixable → fix now, then loop back to Step 4.** Mechanical
corrections clearly inside the PR's stated scope: typos, formatting, doc
wording, dead code this PR introduced, missing imports, missing tests for
branches this PR adds, bug fixes that don't change observable behaviour,
or following through on a rule the user already stated this session. Fix
it, push to the same branch, wait for CI again, then re-request the
review: capture the current sticky-comment body first, mention `@claude`
again, and poll (as in 5c) until the body *changes* from what you captured
and reads as complete — the sticky comment is edited in place, so the
pre-fix review stays visible (and satisfies 5c's break condition) until
the new one lands. Then re-triage. Don't
prompt the user — the point is to spend their attention only on real
decisions.

**Needs user judgment → surface and stop.** Anything with a genuine
choice: design / API shape / behaviour changes, anything touching a
deliberate prior decision or an existing convention, anything that would
expand the PR beyond its stated scope — anything you'd ask a teammate
about before pushing. State the file/line, the option, and the question;
stop the flow.

**Clean → merge directly.** If CI is green AND nothing is self-fixable
AND nothing needs judgment, go straight to Step 7. Don't ask "want me to
merge?" — asking when there's nothing to decide just burns attention. The
loop is supposed to close itself.

Rule of thumb: would a thoughtful engineer file this as a question, push
a follow-up commit, or just merge it? Escalate / fix / merge accordingly.

### Step 7 — Merge

Reached on a clean review, or after the user approved a merge following an
escalation.

```bash
gh pr merge --squash --delete-branch
```

- **No `--auto`** — it skips the review gate (Step 5).
- **No `--admin`** — it bypasses branch protection.
- `--squash` keeps the default branch at one commit per PR; if the repo
  only allows merge commits or rebase, use `--merge` / `--rebase` instead
  (`gh` errors and tells you if the method isn't enabled).
- Branch protection still applies — if something flipped red since Step 4,
  GitHub refuses and you loop back to Step 4.
- Refused for missing approvals / unresolved conversations → that's an
  org-policy gate, not CI; looping back to Step 4 can never clear it.
  Surface it to the user instead.

Report the result as a one-liner ("PR #N merged, branch deleted, local
synced"). On a clean review, don't ask for confirmation *before* merging.

### Step 8 — Local cleanup

`--delete-branch` deleted the local *and* remote branch, and `gh` already
switched the checkout back to the base branch — so don't `git branch -d`
afterwards; the branch is gone and the command just errors. The one thing
`gh` doesn't do is pull:

```bash
git pull --ff-only
```

If the merge ran from somewhere unusual (a worktree, detached HEAD) and
the local branch survived, delete it manually with `git branch -d`. This
step is part of the loop, not optional: because the merge is synchronous
(no `--auto`), there's no reason to leave the checkout behind the base.

## What NOT to do

- **No `--auto` by default** — it skips the review gate, the whole reason
  for this skill. If the user explicitly asks for it: don't arm it
  silently — say what it skips, and offer to close the gate now (review
  the diff first, then arm auto-merge; at that point `--auto` only skips
  the CI wait). If they still want it after being told, do it: their
  repo, their call. Caveat: auto-merge stays armed across later pushes,
  so disarm or re-review before pushing anything else.
- **No `--admin`** — bypasses branch protection.
- **Don't merge before CI finishes** — even a trivial-looking diff; drift
  checks sometimes catch surprising things.
- **Don't amend / force-push after `gh pr create`** unless the user asks —
  it invalidates in-flight reviews and re-runs CI from scratch.
- **Don't push directly to the default branch** — branch protection
  rejects it, and it skips both gates.
- **Don't auto-fix findings that need judgment** — behaviour changes,
  scope expansion, or anything touching a prior deliberate choice gets
  surfaced in Step 6, not silently committed. What counts as self-fixable
  is exactly Step 6's first bucket — when in doubt, escalate.
