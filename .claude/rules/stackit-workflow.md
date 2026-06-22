# Stackit Workflow Rules

This project uses [stackit](https://github.com/getstackit/stackit) for stacked
changes. AI agents should proactively work in stacks. **Prefer stackit over raw git
for branch/commit operations.**

## Forbidden → Required

| Avoid | Use Instead |
|-------|-------------|
| `git checkout -b` | `stackit create -m "..."` |
| `gh pr create` | `stackit submit` |
| `git rebase` | `stackit restack --upstack` (or `--all-stacks`) |

**Exception:** `git commit` / `git commit --amend` is fine for adding to an existing
stacked branch, though `stackit modify` is preferred (it auto-restacks descendants).

## Required Workflow

```bash
# 1. Make changes
# 2. Stage changes FIRST (required — create needs staged changes)
git add -A
# 3. Create stacked branch with commit
stackit create -m "feat: description"
# 4. VALIDATE before submitting — see Pre-Submit Gate below
mise run check
# 5. Submit only when checks are green
stackit submit
```

## Link each PR to its Sortit issue

When a branch implements a Sortit issue, carry the issue ID into the commit so a
merged PR can be reconciled back to its issue. Add a `Closes:` trailer to the stackit
commit message:

```bash
stackit create -m "feat(x): short description

Closes: 01KKTA1E6KB97DA39V606AT1JA"
```

stackit propagates the commit body into the PR description, so the ID rides along to
GitHub. **Without this link, a merged PR cannot be mapped back to its issue, so the
issue silently stays `open` after the work ships** — the exact failure mode that
motivated this rule (issues implemented and merged but never closed). The trailer is
the durable breadcrumb that makes a post-merge close possible across the submit→merge
boundary; the close itself is driven by the wrap-up checklist (`/sortit-wrap-up`) and,
once built, an automated close-on-merge reconciliation sweep.

## Pre-Submit Gate (do not skip)

**`mise run check` must pass before every `stackit submit`.** CI runs golangci-lint,
eslint, backend tests, web tests, and the web build; submitting red wastes a review
cycle and, in a stack, blocks every PR above it. See
[validation.md](./validation.md) for the full gate and the per-branch caveat (fix
issues in the branch that introduced them so they propagate up via restack).

```bash
mise run check && stackit submit   # gate, then submit
```

If CI still fails after a green local run, reproduce the exact CI command locally
(e.g. cold-cache `mise run lint:go`) before re-submitting — don't push speculative
fixes.

## Skills (Preferred over manual commands)

| Skill | Purpose |
|-------|---------|
| `/stack-create` | Create stacked branch |
| `/stack-submit` | Submit PRs for the stack |
| `/stack-status` | Check stack health |
| `/stack-fix` | Diagnose and fix issues |
| `/stack-sync` | Sync with trunk, cleanup merged branches |
| `/stack-restack` | Rebase branches (scoped / multi-stack / parallel) |
| `/stack-absorb` | Auto-route working changes into the correct commits |

Run `/stackit` for the full guide.

## Routing fixes to the right commit in a stack

A lint/test failure usually belongs to the branch that introduced the code, not the
tip. To fix it where it lives so it propagates up via restack:

- **Automatic:** stage the fix and `stackit absorb` (blames each hunk to its commit).
  Verify with `stackit absorb --dry-run` first — if a hunk routes to a commit that
  doesn't actually contain those lines (e.g. a struct added in a later phase), do it
  manually instead.
- **Manual:** `stackit checkout <branch>` → apply the fix → `stackit modify -a`
  (amends + restacks descendants) → `stackit checkout <tip>` → re-run `mise run check`.

## Common Pitfalls

| Mistake | Fix |
|---------|-----|
| Submitting before validating | Run `mise run check` first — every time |
| Forgetting to stage before `create` | Always `git add -A` before `stackit create` |
| Empty branch created | You forgot to stage; delete branch, retry with staged changes |
| Fixing a stack bug only at the tip | Fix it in the branch that introduced it (absorb / modify) |
| Manual rebase broke stack | `stackit restack --upstack` (or `--all-stacks`) |
| Stack out of sync after merge | `stackit sync` to cleanup merged branches and update trunk |
| "Check Stack Order" CI red on upper PRs | **Expected** — that guard fails any PR whose parent isn't `main`, to enforce bottom-up merge. It clears as parents merge; it is not a code failure. |
