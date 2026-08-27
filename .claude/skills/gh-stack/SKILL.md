---
name: gh-stack
description: "Split large work (e.g. a multi-phase PRD) into a sequence of reviewable, dependent PRs using GitHub's gh stack CLI (github/gh-stack) in this repo: gh stack init/add/submit/checkout/rebase/merge, plus three repo-specific gotchas that cause commits to land on the wrong branch or a GitHub-reported conflict to get wrongly dismissed as a false positive. Invoke before starting stacked-branch work, or when a gh stack command fails unexpectedly (e.g. an unrelated file failing pre-commit), or when GitHub reports a PR conflict."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# GitHub PR Stacks (`gh stack`)

Not to be confused with **`atmos-stacks`** (Atmos's own stack-config subsystem) or **`atmos-git`**
(Atmos's git integration feature). This skill is about splitting *your own implementation work* —
e.g. a large PRD with multiple rollout phases — into a chain of small, sequentially-based PRs using
GitHub's [`gh stack`](https://github.com/github/gh-stack) CLI extension
(`gh extension install github/gh-stack`; Conductor's cloud workspaces have it preinstalled, local
workspaces need the extension installed once).

Use this when a task is too large for one PR — each layer becomes its own branch, based on the layer
below it, each with its own PR, reviewed and mergeable independently while still expressing "this
depends on that."

## Core commands

| Command | What it does |
|---|---|
| `gh stack init` | Initializes a stack with the current branch as layer 1 (based on the trunk, usually `main`) |
| `gh stack add <branch>` | Creates and checks out a new branch on top of the current top of the stack |
| `gh stack submit` | Pushes every branch and creates/updates every PR in the stack at once |
| `gh stack view` | Shows the full stack (branches, PR numbers, merge-readiness) |
| `gh stack checkout` / `gh stack switch` | Moves between branches in the stack |
| `gh stack rebase` / `gh stack sync` | Fetches trunk, cascades a rebase across every branch in the stack |
| `gh stack merge` | Merges one or more ready PRs in the stack (doesn't require merging the whole stack at once) |

## Three gotchas that will bite you in this repo

### 1. Switching stack layers does NOT clear your staged changes

`gh stack checkout`/`switch` is a thin wrapper over `git checkout`. Git does not reset the index on
checkout — it only touches files that differ between the two branches. If you have staged (or
unstaged) changes for **files that are identical on both branches**, those changes silently ride
along to whatever branch you land on next.

This is exactly how a real incident happened: branch-1 changes were `git add`ed, the commit failed
(see gotcha #2), the session ran `gh stack checkout <parent-branch>` to fix something unrelated, and
those still-staged branch-1 files rode along and got swept into the parent's commit.

**Rule: before switching stack layers, get to a clean state first** — either commit what you have, or
`git restore --staged .` to unstage everything. Never switch layers mid-edit with a dirty index for
work you haven't finished placing.

**After every stack checkout, verify before you commit:**
```bash
git log --oneline -3          # confirm you're on the branch/commit you expect
git status --short            # confirm only the files you intend to touch are dirty/staged
git diff --cached --stat      # right before committing — confirm the staged set matches your intent
```

### 2. The `atmos-validate-editorconfig` pre-commit hook scans the whole tree, not your diff

Most of this repo's pre-commit hooks are diff-aware (lint, go-fumpt, etc.), but
`atmos-validate-editorconfig` validates every file in the working tree regardless of what's staged.
On a stacked branch, this means: **a commit on layer 2 can fail because of a pre-existing formatting
issue in a file that only layer 1 touched** — even though your layer-2 commit never touches that file.

If a commit fails on a file you didn't stage and don't recognize as part of your change, check
whether it's inherited from a lower stack layer before assuming your own change broke something.
Fix it **on the layer that owns the file** (usually the lowest layer that introduced or touched it),
commit it there, then bring the fix forward to upper layers (see the fast-forward trick below) —
don't let an upper layer's commit accidentally absorb an unrelated lower-layer fix (see gotcha #1).

## Bringing a lower-layer fix forward without disturbing a dirty upper layer

If the upper layer has **zero commits of its own yet** (fresh from `gh stack add`, still just staged/
unstaged changes in the working tree) and the layer below it gets a new commit, you don't need
`gh stack rebase` (which requires a clean index) and you don't need to stash (a stray stash is easy to
forget and this repo's convention is to avoid `git stash` — commit or fast-forward instead):

```bash
git merge --ff-only <parent-layer-latest-commit>
```

This fast-forwards the branch pointer and merges in the parent's new commit's file changes without
touching your working tree's dirty files, as long as those dirty files don't conflict with what the
fast-forward brings in (they won't, if the parent commit only touched files you haven't touched).

Once the upper layer has real commits of its own, use `gh stack rebase`/`gh stack sync` instead (after
committing or unstaging first, per gotcha #1) — that's the tool built for cascading a real rebase
across every branch in the stack.

### 3. Trust GitHub's `mergeable`/conflict report over a local pairwise branch comparison

When GitHub shows a PR as having conflicts (`gh pr view <num> --json mergeable` returns
`CONFLICTING`, or the PR page shows "This branch has conflicts that must be resolved" with a
specific file list), **that is ground truth — do not "disprove" it with local git commands and
conclude it's a stale/false positive.** A real incident: `git merge-tree --write-tree
origin/<parent> origin/<child>` succeeded cleanly (exit 0, no conflicts) and `git merge-base
--is-ancestor` confirmed a strict fast-forward relationship between the two branch tips — which
led to concluding GitHub's report was wrong. It wasn't. `gh stack sync`/`gh stack rebase`
immediately reproduced the exact same conflict, in the exact same files GitHub had listed.

**Why the local check was misleading, not GitHub:** a pairwise `merge-tree` between two branches'
*current* committed tips only proves those two exact commits merge cleanly *as they are right now*.
It says nothing about what happens once the stack's lower layers get rebased onto the current tip
of `main` — which is exactly what GitHub's own mergeability check (and `gh stack sync`/`rebase`)
account for, and what actually determines whether the stack can merge. If `main` has moved forward
since the stack was built, a real conflict can exist between "phase N rebased onto phase N-1
rebased onto current main" even though "phase N vs phase N-1's original commits" shows none. Check
whether `main` has advanced past the stack's base with `git merge-base --is-ancestor origin/main
origin/<bottom-layer-branch>` before trusting a clean pairwise diff as proof of anything — if it's
not an ancestor, `main` has moved and the pairwise check is not asking the right question.

**Rule:** the moment GitHub reports a conflict, go straight to `gh stack sync` (to reproduce and
confirm which layer conflicts) then `gh stack rebase` (to resolve it interactively) — don't spend
time trying to locally falsify the report first. If `gh stack sync`/`rebase` also reports zero
conflicts, only then is it reasonable to treat GitHub's cached state as stale and worth a recheck
after a short wait (GitHub's own mergeability computation can lag a `gh stack submit` push by a few
seconds, but it resolves in well under a minute — if it's still wrong after that, resolve via
`gh stack rebase` rather than continuing to wait).

When resolving the conflicts `gh stack rebase` surfaces, prefer **keeping both sides** over picking
one — in this repo, stacked layers usually add independent, coexisting things to the same shared
file (e.g. two different component types each adding their own `case` in the same switch
statement, or their own field in the same struct). A conflict here is almost never "which change is
correct," it's "both changes are correct and need to be merged together textually."

## PR labeling across a stack

Per the `pull-request` skill's semver-label rule: **the label is per-PR, not per-feature.** If a
five-layer stack adds one user-visible feature, only the final layer that wires it up gets
`minor`/`major` — the foundation/plumbing layers underneath it get `no-release`. Don't label every
layer in a stack the same way just because they're part of one larger effort.
