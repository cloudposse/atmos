# Fix: `describe affected` reports false positives on out-of-date PRs

**Date:** 2026-04-30

## Issue

A customer running `atmos describe affected` in GitHub Actions on a
pull request that was **out of date with `main`** (i.e., `main` had
moved ahead since the PR branch was last synced) reported that
`atmos describe affected` listed many more affected components than
the PR actually modified — sometimes effectively "every component".

The same command on the same PR, when up-to-date with `main`, reported
the correct (small) set of affected components. The customer's
workflow used the new zero-config CI base detection introduced in
[#2241](https://github.com/cloudposse/atmos/pull/2241):

```yaml
- name: Describe affected
  run: atmos describe affected
```

No `--ref`, `--sha`, or `--base` flags. `ci.enabled: true` in
`atmos.yaml`.

## Root cause

`pkg/ci/providers/github/base.go:resolvePRBase` runs a fallback chain
when resolving the base commit for `pull_request` /
`pull_request_target` events:

1. `git merge-base(HEAD, origin/<target>)` — gold standard, returns
   the **fork point** of the PR branch.
2. `HEAD~1` — only for closed/merged PRs.
3. **Last resort: return the *ref* `refs/remotes/origin/<target>`.**

Step 1 is the correct strategy. It works regardless of how out of date
the PR is, because the fork point doesn't move when `main` advances.

The bug is in step 3. `MergeBase` requires the local repo to have
`origin/<target>` available *and* enough history to find a common
ancestor. In CI shallow checkouts (`actions/checkout@v4` defaults to
`fetch-depth: 1`) neither is guaranteed:

- `origin/<target>` may not be fetched at all.
- Even if fetched, the shallow boundary may be more recent than the
  fork point.

When `MergeBase` failed for either reason, Atmos fell through to step
3 and returned a *ref* to `refs/remotes/origin/<target>`. Downstream
in `internal/exec/describe_affected_helpers.go`,
`ExecuteDescribeAffectedWithTargetRefCheckout` resolved that ref to
the **current tip** of the target branch and ran a tree-to-tree diff
against the PR head:

```
Latest main:    A — B — C — D — E      (origin/main HEAD)
                      \
PR branch:             F — G            (PR HEAD; fork point is B)
```

- Correct comparison (merge-base): `diff(B, G)` → only PR's changes.
- Buggy comparison (`origin/main` tip): `diff(E, G)` → PR's changes
  plus every file modified by C, D, E that the PR hasn't pulled in.
  Those show up as "affected" because the tree diff sees an old
  version on the PR side and a newer version on the main side.

The PRD itself flagged this limitation:
`docs/prd/native-ci/framework/base-resolution.md:94`:

> Limitation: merge-base requires the target branch ref
> (`origin/<target>`) to be available locally. In shallow CI
> checkouts, this may not be fetched. The fallback chain handles this
> gracefully.

"Gracefully" was the bug — the fallback returned wrong results
without surfacing any error.

### Why a stale `pull_request.base.sha` is still better than current tip

`event.pull_request.base.sha` is the SHA of the target branch at the
time of the last PR event (open / synchronize). When the PR is out of
date, this SHA is **frozen at the last sync** — it does *not* point
to the current tip of main.

For an out-of-date PR, comparing against `pull_request.base.sha` is
still imperfect (it can include commits on main between the fork
point and the last sync), but **it cannot include commits on main
after the last sync**, which is the dominant source of false
positives in the bug report.

### Why the user's hypothesis (`pull_request.merge_commit_sha`) is correct in theory

The user asked: "is there a future merge commit that we should be
comparing against the current main instead?" That's
`refs/pull/<n>/merge` (a.k.a. `pull_request.merge_commit_sha`), a
synthetic merge commit GitHub generates with parents
`(latest_main, PR_HEAD)`. It is regenerated when main moves, so:

- `M^1` = latest main at the time `M` was generated.
- `diff(M^1, M)` = the PR's pure changes, regardless of how out of
  date the PR branch is.

This is a correct alternative. We chose merge-base + auto-fetch
instead because:

- It keeps the existing PRD architecture (merge-base as primary).
- It does not require fetching `M`'s parent separately.
- It handles `actions/checkout@v4`'s default behavior (which checks
  out the merge commit for `pull_request` events) naturally — when
  HEAD is the merge commit, `merge-base(HEAD, origin/main)` already
  resolves to the merge commit's first parent.

`merge_commit_sha` remains a viable future option if the auto-fetch
path proves problematic.

## Fix

Three changes, all in this PR:

### 1. `MergeBase` now self-heals from shallow checkouts

`pkg/git/merge_base.go` adds a new public function
`MergeBaseWithAutoFetch(repoDir, targetBranch)` that wraps `MergeBase`
with bounded recovery:

- If `MergeBase` fails because `origin/<target>` is not present
  locally, run `git fetch origin <target>` and retry.
- If `MergeBase` then returns `ErrNoCommonAncestor` (the shallow
  boundary doesn't reach the fork point), run a single
  `git fetch --deepen=200 origin <target>` and retry.
- `ErrHeadOnTargetBranch` is propagated unchanged (no fetch can fix
  it; the caller falls through to `HEAD~1`).
- Any other failure returns the original error so the caller can
  fall through.

### 2. `resolvePRBase` falls back to payload `base.sha`, not current tip

`pkg/ci/providers/github/base.go:resolvePRBase` now uses this fallback
chain for `pull_request` / `pull_request_target` events:

1. `git.MergeBaseWithAutoFetch(".", target)` — gold standard, with
   shallow-clone self-heal.
2. `HEAD~1` — closed/merged PRs only (unchanged).
3. **`event.pull_request.base.sha`** from the payload. Frozen at last
   sync; never the current tip of the target branch.
4. **Last resort:** ref `refs/remotes/origin/<target>` *with a
   `log.Warn`* explaining that the result may include unrelated
   commits from the target branch. This path is only reached when
   the payload has no `base.sha` (hand-crafted or legacy events).

### 3. Worktree creation also self-heals

`internal/exec/describe_affected_helpers.go:ExecuteDescribeAffectedWithTargetRefCheckout`
now accepts a `targetBranch` parameter. When worktree creation fails
because the resolved target commit isn't in the local object DB
(common when the base SHA came from the event payload but was never
fetched locally), Atmos runs `git fetch origin <targetBranch>` and
retries once.

### 4. Plumbing

`TargetBranch` is added to:

- `pkg/ci/internal/provider.BaseResolution` (so providers can
  surface the target branch alongside the resolved base).
- `internal/exec.DescribeAffectedCmdArgs`.
- `ExecuteDescribeAffectedWithTargetRefCheckout`'s signature, threaded
  through `internal/exec/atlantis_generate_repo_config.go`,
  `internal/exec/terraform_affected.go`,
  `internal/exec/terraform_affected_graph.go`, and
  `pkg/list/list_affected.go` (passes empty string — `list affected`
  does not yet plumb CI auto-detection; out of scope here).

## Verification

### Unit tests

- `pkg/git/merge_base_test.go`: new
  `TestMergeBaseWithAutoFetch_RecoversFromMissingRef` builds a real
  origin + clone, deletes `origin/main` to simulate the shallow case,
  and asserts the fork-point SHA is recovered after auto-fetch.
- `pkg/git/merge_base_test.go`:
  `TestMergeBaseWithAutoFetch_PropagatesHeadOnTargetBranch` and
  `TestMergeBaseWithAutoFetch_ReturnsErrorWhenFetchImpossible` for the
  non-recovery paths.
- `pkg/ci/providers/github/base_test.go`:
  `TestResolveBase_PullRequest_OutOfDate_FallsBackToPayloadSHA`
  reproduces the customer scenario at the unit-test level — payload
  has `base.sha`, merge-base fails (no real origin), and the
  resolution returns the payload SHA, **not** the buggy
  `refs/remotes/origin/main` ref.
- `pkg/ci/providers/github/base_test.go`:
  `TestResolveBase_PullRequest_NoPayloadBaseSHA_LastResortRef`
  documents that the legacy ref path still works for hand-crafted
  payloads with no `base.sha`.
- `internal/exec/describe_affected_test.go:TestResolveBaseFromCI`
  updated to require `describe.SHA` is populated and
  `describe.Ref` is empty — guards against any future regression that
  re-introduces the ref-tip fallback.

### End-to-end (manual)

In the customer's reproduction:

1. PR forked from `main@B`, main since advanced to `E` with commits
   that touched components the PR didn't.
2. Workflow uses `actions/checkout@v4` defaults (`fetch-depth: 1`,
   merge-ref checkout).
3. Before this fix: `atmos describe affected` reported every
   component as affected.
4. After this fix: only the components actually touched by the PR.

## Out of scope

- Switching to `pull_request.merge_commit_sha` as the primary
  strategy. Documented as a considered alternative in the PRD update.
- Wiring CI auto-detection into `pkg/list/list_affected.go`.
- Fetching enough history for `MergeBase` to succeed on a force-pushed
  PR with no common ancestor against current main — bounded by the
  single `--deepen=200` retry; truly orphaned histories will fall
  through to the payload `base.sha` path.

## Supersedes

This fix supersedes [#2285](https://github.com/cloudposse/atmos/pull/2285),
which proposed promoting `pull_request.base.sha` to the *primary*
strategy. We kept merge-base as primary (gold standard) and used
`base.sha` only as a fallback that replaces the buggy ref-tip path.
The fetch helpers and signature plumbing are lifted from #2285;
credit to the original author.
