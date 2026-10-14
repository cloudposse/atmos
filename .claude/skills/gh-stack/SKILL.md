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
| `gh stack submit` | Pushes every branch and creates/updates every PR in the stack (4 sequential steps: push branches, create PRs, update base branches, create/update the stack object — not atomic; a mid-run failure can leave some branches pushed with no PR yet, or a partially-updated stack. Rerunning is safe — it picks up wherever it left off) |
| `gh stack view` | Shows the full stack (branches, PR numbers, merge-readiness) |
| `gh stack checkout [<stack-number> / <pr-number> / <pr-url> / <branch>]` | Resolves an identifier, fetching a remote stack when needed; no argument opens the available-stack picker |
| `gh stack switch` | Opens the branch picker for the current stack; accepts no identifier |
| `gh stack rebase` / `gh stack sync` | Fetches trunk, cascades a rebase across every branch in the stack |
| `gh stack merge` | Merges one or more ready PRs in the stack (doesn't require merging the whole stack at once) |

## Three gotchas that will bite you in this repo

### 1. Switching stack layers does NOT clear your staged changes

`gh stack checkout` resolves stack numbers, PR numbers, PR URLs, and locally tracked branch names.
It can fetch a remote stack and set up local tracking; with no argument it opens the available-stack
picker. `gh stack switch` only opens the branch picker for the current stack: it does not accept an
identifier, so use `gh stack checkout <branch>`, not `gh stack switch <branch>`. Once either command
resolves *which* branch to land on, the actual switch goes through git's own checkout, which does not
reset the index or the working tree — it only touches files that differ between the two branches. If you have
staged **or unstaged** changes for **files that are identical on both branches**, those changes
silently ride along to whatever branch you land on next.

This is exactly how a real incident happened: branch-1 changes were `git add`ed, the commit failed
(see gotcha #2), the session ran `gh stack checkout <parent-branch>` to fix something unrelated, and
those still-staged branch-1 files rode along and got swept into the parent's commit.

**Rule: before switching stack layers, get to a genuinely clean state first** — commit what you have.
`git restore --staged .` alone is not enough: it unstages, but leaves any working-tree modifications
in place, which still ride along the same way staged changes do. If some changes truly aren't ready
to commit, they still need to be fully out of the working tree before switching — but getting there
must never mean silently discarding uncommitted work. **Never discard changes (`git checkout -- .`,
`git clean -fd`, `git reset --hard`, etc.) without the user's explicit, per-change approval** — this
matches this repo's Git Safety Protocol. Preserve the work first, then switch:

- Commit it, even as a throwaway WIP commit on the current branch (`git commit -m 'WIP: <tag>'`) —
  with the user's approval, `git reset --mixed HEAD~1` restores it as unstaged changes once you're
  back on this layer. Use this only for your own unpublished WIP commit at the branch tip.
- Or copy the dirty files out to a temporary worktree, or a patch file written safely: create a
  private temp directory (`dir=$(mktemp -d)`), exclusively create the patch file inside it
  (`patch=$(mktemp "$dir/XXXXXX")` — avoids following a pre-existing symlink at a predictable
  shared `/tmp/<tag>.patch` path), and capture with `git diff HEAD > "$patch"` — `HEAD`, not a bare
  `git diff`, so both staged and unstaged tracked changes are captured (a bare `git diff` only
  compares the working tree against the index and silently drops anything already staged); untracked
  files still need separate handling (copy them by hand — don't `git stash -u`, per this repo's
  no-stash convention). Reapply after switching back, then remove the temp directory.

Only if neither is viable and the changes are genuinely disposable, ask the user for explicit approval
before discarding — never remove uncommitted work by default. Never switch layers mid-edit with dirty
state for work you haven't finished placing.

**After every stack checkout, verify before you commit:**
```bash
git log --oneline -3          # confirm you're on the branch/commit you expect
git status --short            # confirm only the files you intend to touch are dirty/staged
git diff --cached --stat      # right before committing — confirm the staged set matches your intent
```

### 2. The EditorConfig hook's affected set differs from your staged diff

`atmos-validate-editorconfig` runs on every commit (`always_run: true`), but its
`atmos validate --affected` command normally selects committed changes since the merge base,
unstaged changes, and untracked files, subject to the hook's exclusions. It does not collect the
staged diff separately: a staged-only path can be missed unless another part of that affected set
also includes it. Selected EditorConfig rule changes trigger a full scan instead.

On a stacked branch, lower-layer changes can appear in that committed range: **a commit on layer 2
can fail because of a formatting issue in a file that only layer 1 touched**, even though your
layer-2 commit never touches that file. A passing hook is not proof that every staged file was checked.

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

Once the upper layer has real commits of its own, use `gh stack rebase`/`gh stack sync` instead — but
only from a genuinely clean worktree first. `github/gh-stack` requires zero uncommitted changes,
staged or not, for these commands, and does not stash them automatically. Unstaging alone (`git
restore --staged .`) is not enough — it clears the index but leaves working-tree modifications in
place, and those still block the command. Commit what you have or safely move it out of the working
tree first (per gotcha #1), not just unstage it. Once clean, `gh stack rebase`/`gh stack sync` is the
tool built for cascading a real rebase across every branch in the stack.

### 3. Compare GitHub's current PR head and configured base

GitHub computes a pull request's mergeability against its **configured base branch**. For a
stacked PR, that is the preceding branch; only the bottom PR normally targets `main`. GitHub does
not simulate rebasing the lower layers onto current `main` before checking an upper PR.

When GitHub reports `CONFLICTING`, inspect the PR's current base and head with
`gh pr view <num> --json baseRefName,baseRefOid,headRefName,headRefOid,mergeable`, then fetch those
branches before comparing them locally. A pairwise `git merge-tree --write-tree` only answers
whether the two supplied commits merge cleanly. Stale remote-tracking refs, a different configured
base, or a GitHub result still being recomputed can explain a disagreement. Confirm the exact
commit IDs on both sides and recheck GitHub's result; do not dismiss a conflict based on a local
comparison of different commits. `UNKNOWN` means GitHub has not finished computing mergeability.

**Stack freshness is a separate check.** After `git fetch origin`, use
`git merge-base --is-ancestor origin/main origin/<bottom-layer-branch>` to check whether the stack
includes current `main`. `gh stack rebase` cascades a rebase onto the updated trunk and each updated
parent. It can surface conflicts that do not exist between the currently published PR heads and
bases. A clean pairwise merge does not prove that this future cascade will be conflict-free, and a
conflict during that cascade does not explain GitHub's status for a different pair of commits.

Use `gh stack rebase` when the stack needs that cascade, then resolve and validate the resulting
conflicts locally. **Do not use `gh stack sync` as a read-only probe**: per its `--help`, it fetches,
reconciles the remote stack with the local one, cascade-rebases, and then **pushes every branch
atomically** (`--force-with-lease --atomic`) and syncs PR state on GitHub. Run it only when those
remote changes are intended and permitted by the publication schedule. `gh stack rebase` does not
push. After publishing, verify each PR's final head, actual base, and GitHub mergeability again.

When resolving the conflicts `gh stack rebase` surfaces, first check whether the two sides are
actually independent, then combine them — don't reflexively keep both. In this repo, stacked layers
usually add independent, coexisting things to the same shared file (e.g. two different component
types each adding their own `case` in the same switch statement, or their own field in the same
struct); for those, prefer **keeping both sides** over picking one — a conflict here is usually not
“which change is correct,” it's “both changes are correct and need to be merged.” But when the two
sides touch the same case, field, or logic in incompatible ways, keeping both blindly produces
duplicate cases or contradictory logic — resolve the conflict semantically instead, picking the
correct combined behavior rather than concatenating both raw hunks, and re-run the affected checks
afterward.

## PR labeling across a stack

Per the `pull-request` skill's semver-label rule: **the label is per-PR, not per-feature.** If a
five-layer stack adds one user-visible feature, only the final layer that wires it up gets
`minor`/`major` — the foundation/plumbing layers underneath it get `no-release`. Don't label every
layer in a stack the same way just because they're part of one larger effort.

`no-release` classifies non-user-visible work for the PR policy and release-documentation checks;
it does not suppress a version bump. The release-drafter resolver in `.github/auto-release.yml`
currently maps this label to `patch`. Keep using the policy's label for foundation PRs; changing
release publication or version resolution is a separate policy change.
