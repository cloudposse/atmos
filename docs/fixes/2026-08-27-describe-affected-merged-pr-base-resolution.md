# Fix: `describe affected` resolves a wrong base for merged multi-commit PRs

**Date:** 2026-08-27

## Issue

After merging a multi-commit pull request whose **final commit reverted an
earlier commit** (the earlier commit changed an org-wide stack default — a
backend setting inherited by every stack; the final commit reverted it and
added an unrelated CI-workflow tweak, so the PR's *net* diff was two workflow
files), `atmos describe affected --upload` on the `pull_request closed
(merged)` event reported **every component in the repository as affected**
and dispatched a wall of post-merge plan/apply runs.

The same PR's earlier `synchronize` event had correctly reported **zero**
affected components for the identical head commit. The workflow was the
documented zero-config setup: `ci.enabled: true`, no base flags, and
`actions/checkout` pinned to `github.event.pull_request.head.sha` (required
for Atmos Pro upload correlation) with `fetch-depth: 0`.

The run logs showed the difference directly:

```
# synchronize event (correct):
Auto-detected CI base ... source="merge-base(HEAD, origin/main)"
✓ Uploaded 0 affected component(s)

# closed/merged event (wrong):
Auto-detected CI base ... source="HEAD~1 (merged PR, merge-base unavailable)"
✓ Uploaded <every component in the repo> affected component(s)
```

## Root cause

Two compounding problems in `pkg/ci/providers/github/base.go:resolvePRBase`,
both instances of the same design flaw: **every strategy in the fallback
chain is only correct under an *assumed* checkout, and nothing verified which
checkout the workflow actually did.**

1. **Merge-base against `origin/<target>` degenerates after the merge.** For
   a merge-commit (or queue) merge, the PR head becomes an ancestor of the
   target branch, so `merge-base(HEAD, origin/<target>) == HEAD` —
   `ErrHeadOnTargetBranch` — and the gold-standard tier always falls through
   on exactly the events where correctness matters most.

2. **The `HEAD~1` fallback assumed the merge commit was checked out.** The
   documented workflow checks out `head.sha` instead (it must, for Atmos Pro
   upload correlation — a conflict the PRD had already noted). With the PR
   head checked out, `HEAD~1` is the PR's *own previous commit*, so the
   "diff" is the PR's **final commit alone**, not its net change. A final
   commit that reverts an earlier org-wide change therefore re-reports the
   entire reverted blast radius as affected. (The same shape can also
   under-detect: changes in earlier commits become invisible.)

Merge queues do not avoid this: GitHub still fires `pull_request closed
(merged)` after the queue lands the PR, and a queue merge commit has the PR
head as an ancestor, triggering the identical chain.

## Fix

Merged PRs (`action == "closed"` && `pull_request.merged`) no longer use the
open-PR chain. Instead the resolver **classifies what the workflow actually
checked out** by comparing the local HEAD against the event payload, and
picks the strategy that is provably correct for that checkout:

| Checkout | Base |
|---|---|
| `head.sha` | `merge-base(HEAD, merge_commit_sha^1)` — the true fork point; correct for merge, squash, and rebase strategies; cannot collapse to HEAD. The merge commit is fetched by SHA if a narrow checkout didn't bring it in. |
| `head.sha`, fast-forward merge (`merge_commit_sha == head.sha`) | `merge-base(HEAD, base.sha)` — for externally-merged/fast-forwarded PRs the merge commit IS the PR head, so anchoring on its parent would silently drop every commit but the last (under-detection, caught in an adversarial field-test pass); the payload's pre-merge `base.sha` still yields the fork point. |
| merge commit | `HEAD^1` — the pre-merge target tip (the one case the old fallback was right for). |
| synthetic `refs/pull/<n>/merge` | first parent of HEAD — the target tip the test merge was built on. |
| unknown | `event.pull_request.base.sha` with a `Warn` — never guess silently. |

This converges merged-PR resolution on the same payload-anchored semantics
the `merge_group` path already had (`merge_commit_sha^1` is the plain-merge
analog of `merge_group.base_sha`). Closed-*unmerged* PRs resolve like open
PRs. The `Auto-detected CI base` log line now includes `checkout=...` so any
future wrong-base report is diagnosable from a single line.

New helpers: `git.CommitParents`, `git.MergeBaseSHAs`, `git.FetchCommit`
(`pkg/git/commit.go`).

## Testing

`pkg/ci/providers/github/base_merged_test.go` builds a real git fixture
reproducing the incident shape (multi-commit PR whose final commit reverts an
earlier org-wide change, target branch advanced independently) and asserts,
for every checkout × strategy combination (head.sha × merge/squash, merge
commit, synthetic merge, unknown, queue-merged closed event), that the
resolved base is the fork point / pre-merge tip — and explicitly **not** the
PR's own previous commit (the pre-fix wrong answer). Closed-unmerged PRs are
verified to stay on the open-PR chain. `pkg/git/commit_test.go` covers the
new helpers, including the parentless-initial-commit case.

## Lessons

- A fallback chain where each tier is "correct under assumption X" needs to
  *verify* X, not assume it; otherwise the chain silently picks a strategy
  whose precondition doesn't hold and reports a confident wrong answer.
- Log lines should carry enough context (here: the checkout classification)
  to distinguish "which strategy won" from "was that strategy valid".
