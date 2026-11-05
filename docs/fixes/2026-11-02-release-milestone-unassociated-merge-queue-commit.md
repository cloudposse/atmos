# Fix: Release milestone gate no longer blocks every PR when GitHub leaves a merge-queue PR unmerged

**Date:** 2026-11-02

## Summary

The required "Release milestone" check failed on every `patch`, `minor` and `major` PR with
`commit f60cceba1c… has no associated PR`. That commit was a Renovate dependency bump that the merge queue
pushed to `main`, but GitHub never marked its PR (#3608) as merged. The check now recovers the PR from the
subject of a GitHub-signed commit instead of failing the whole repository.

## Context

The check walks every commit on `main` since the last stable release and asks
`commits/{sha}/pulls` which merged PR each one belongs to. An empty answer means a direct push, so it
fails closed.

For #3608 the queue entry was created at 19:24:41Z and the queue bot pushed the squash commit to `main` at
20:22:17Z. The PR then stayed `OPEN` with `merged: false`, with no `Merged` or `RemovedFromMergeQueue`
event. The commit's tree is identical to the PR head's tree, so the change did land. GitHub's API was
returning HTTP 500 around the same time (a scheduled run died on a `commits/{sha}/status` read at 20:25Z).
Because the PR was never marked merged, `commits/{sha}/pulls` returned `[]` and kept doing so after the PR was
closed. The cause on GitHub's side is not confirmed.

A milestone and a release label belong to the PR, not the commit, so the gate cannot evaluate a commit
without first resolving its PR. Every other PR was blocked for a problem it did not cause.

## Changes

- `magefiles/release/milestone/milestone_history.go`: when `commits/{sha}/pulls` is empty,
  `signedSubjectPullRequest` resolves the PR from the commit subject (`… (#N)`), but only when GitHub signed
  the commit (`verification.verified` and committer `web-flow`) and the PR targets the default branch. The PR
  is returned even though it is not marked merged, and the normal milestone and release-label checks then
  apply to it. Direct pushes, unsigned commits, subjects without a PR number, a number found only in the
  body, a PR on another base, and an unknown PR keep the existing `has no associated PR` error.
- Release planning (`magefiles/release/plan.go`) shares this path through `ReleaseChanges`, so a release cut
  also includes such a commit in its release notes instead of failing.

## Validation

- `go test -tags mage ./magefiles/...`: passes. New `TestCheckReleaseHistoryUnassociatedCommit` covers the
  accepted cases (open `no-release` PR, open PR with a milestone) and the rejections (missing milestone,
  unsigned, not committed by GitHub, no number, number only in the body, other base, PR not found).
- The existing `direct commit` cases in `milestone_history_test.go` and `plan_test.go` now serve an unsigned
  commit and still fail with `has no associated PR`.
- `atmos lint --changed`: 0 issues.
- Not run: the live workflow against GitHub. After merge, re-run `release-milestones.yml` and check that
  `gh pr checks 3567` passes.

## Follow-ups

- The fallback trusts a GitHub-signed commit subject. A GitHub web-UI commit that a person titles with
  someone else's `(#N)` would be accepted against that PR's milestone and label. Branch protection on
  `main` is what prevents such a commit today; this change does not add a separate check.
- The shared GitHub client does not retry 5xx responses, so a single HTTP 500 still fails a run and
  revokes statuses on every open PR head. That is separate from this fix and is not changed here.
