# Fix: Release milestone gate rejects queued layers of a native PR stack

**Date:** 2026-11-21

## Summary

The `Release milestone` merge-queue status failed for every layer of a native GitHub PR stack above the
bottom PR with `queue PR #N is missing from live open PRs`. The gate now looks up open queued PRs that
target another branch and evaluates them with the same milestone and release-label policy.

## Context

`reconcileMilestoneStatuses` builds its verdicts from `OpenPRs(defaultBranch)`, which lists only open PRs
whose base is the default branch. When a native stack is enqueued, every layer joins the default branch's
merge queue, but each upper layer still targets its parent layer's branch. Those PRs were absent from the
open list, so `queuePolicy.checkEntry` had no verdict for them and failed the merge group as if the PR were
unknown.

This was observed when a nine-layer stack was enqueued: the merge group for #3571 (whose base was the
branch of #3563) received a failing `Release milestone` status even though the PR was open, labeled, and
milestoned correctly. The status is not a required check today, so the queue still merged the group, but
the failure would block merges as soon as the status is required, as `Check`'s doc comment recommends.

## Changes

- `magefiles/release/milestone/milestone_gate.go`: add `addQueuedPRs`. For each merge-queue entry whose PR
  is not in the open list and is not reported as merged, fetch `pulls/<number>`. An open PR is appended to
  the evaluated set, so it gets a normal policy verdict and its own head status. A PR that is no longer
  open stays missing, so its group still fails closed. Merged entries keep the existing landed-commit rule.
- The lookup runs after queue-group heads are recorded, so a failed lookup still revokes every known head,
  including earlier groups. On a lookup error, `addQueuedPRs` returns the PRs it already fetched, and their
  heads are recorded before the error check, so a success left by an earlier run on one of those heads is
  revoked too.
- `magefiles/release/milestone/milestone_gate_stacked_test.go` (new): `TestMilestoneGateStackedQueueEntries`
  (open stacked layer passes, a stacked layer that fails policy reports the policy reason, a closed stacked
  layer stays missing, and a lookup error revokes every known head) and
  `TestMilestoneGateStackedLookupRevokesFetchedHeads` (the first of two lookups succeeds and the second fails;
  the fetched PR's head and every queue head are revoked).
- `magefiles/release/milestone/milestone_gate_test.go`: `TestMilestoneGateMergedQueueEntry` now asserts that
  merged entries are never looked up and that a closed unmerged PR is still rejected.

## Validation

- `TestMilestoneGateStackedQueueEntries` fails on the unfixed code in all four cases, reproducing the bug.
- `TestMilestoneGateStackedLookupRevokesFetchedHeads` fails when `addQueuedPRs` discards fetched PRs on
  error, and passes with the partial-result fix.
- `go test -tags mage -count=1 -race ./magefiles/release/milestone/` passes.
- Statement coverage of `addQueuedPRs` and `reconcileMilestoneStatuses` is 100%.
- `gofumpt` and `golangci-lint` were not run directly in this session; the repository's pre-commit hooks
  run both on commit.

## Follow-ups

None.
