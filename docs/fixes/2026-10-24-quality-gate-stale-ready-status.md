# Fix: Quality Gate re-checks `quality:ready` PRs whose head status fails

**Date:** 2026-10-24

## Summary

A PR labeled `quality:ready` now returns to the scheduled reconcile when its head's combined commit status becomes `failure` or `error`, so a status that turns red after CI finishes no longer leaves a stale ready label.

## Context

- Four open PRs showed `quality:ready` and a green Quality Gate comment while the `Release milestone` commit status was failing on their heads. The gate comment had been correct when published.
- Commit statuses change without any event the Quality Gate workflow subscribes to. The scheduled recovery skips a labeled PR with a report for its current head and base once the head commit is older than 24 hours. The active-workflow check added in PR #3476 reads workflow runs only, so it does not see a status that regresses after all workflows complete.
- The immediate `Release milestone` failure (a stacked PR merged into its parent branch) was fixed separately in PR #3536.

## Changes

- `magefiles/qualitygate/quality_select.go`: after the stale-PR early returns, a `quality:ready` PR gets one combined-status read of its head. `failure` or `error` selects it for reconciliation (`qualityReasonRegressed`). A combined `pending` state is ignored because it also describes a commit with no statuses. `quality:not-ready` PRs cost no extra read.
- `magefiles/README.md`: documents the scheduled re-check.

## Validation

- `go test -tags mage ./magefiles/qualitygate/... ./magefiles/release/...` passes on the branch merged with `main`.
- With the selection change reverted, the new and updated `TestQualityScheduledSelection` cases fail, then pass with it restored.
- Negative paths: a ready PR with a healthy status is not selected, and a not-ready PR with a failing status is neither selected nor read.

## Follow-ups

- Combined commit status does not include Actions check runs. A check run that regresses on a stale ready PR is still picked up only when a head, review, or workflow event reaches the gate.
