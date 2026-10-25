# Fix: Quality Gate no longer leaves `quality:ready` on PRs with a failing `Release milestone`

**Date:** 2026-10-24

## Summary

Four open 1.234.0 PRs showed `quality:ready` and a green Quality Gate comment while GitHub showed a failing `Release milestone` status. The scheduled reconciler now re-evaluates a `quality:ready` PR whose head has a failing commit status, and the milestone history check now accepts a stacked PR whose merge commit is the commit that landed on `main`.

## Context

- Commit statuses (such as `Release milestone`) change without any event the Quality Gate workflow subscribes to. The scheduled recovery deliberately skipped labeled PRs with a report for their current head and base once the head commit was older than 24 hours, so a gate that was correct when published stayed `quality:ready` after a status turned red.
- `Release milestone` failed on every open PR whose release range included `ecb502afb6` (#3467). That PR merged into its parent stack branch rather than `main`, so the history walk skipped it (`pr.Base.Ref != branch`) and reported "no eligible merged PR" even though its merge commit is the commit on `main`.

## Changes

- `magefiles/qualitygate/quality_select.go`: after the existing stale-PR early returns, a `quality:ready` PR gets one combined-status read of its head. A `failure` or `error` state selects it for reconciliation (`qualityReasonRegressed`). A combined `pending` state is ignored because it also describes a commit with no statuses. `quality:not-ready` PRs cost no extra read.
- `magefiles/release/milestone/milestone_history.go`: a merged PR with a non-default base is eligible when its `merge_commit_sha` equals the commit being checked. A PR merged elsewhere that merely shares history stays ineligible, and eligible PRs still go through `CheckIncludedPR` for label and milestone.
- `magefiles/README.md`: documents the scheduled re-check.

## Validation

- `go test -tags mage ./magefiles/qualitygate/... ./magefiles/release/milestone/...` passes.
- With each source change reverted, the new cases fail (8 selection cases in `TestQualityScheduledSelection`, and `stack landed` in `TestCheckReleaseHistory`), then pass again with the change restored.
- Negative paths covered: a ready PR with a healthy status is not selected, a not-ready PR with a failing status is not selected or read, and a stacked PR with an unrelated merge commit is still rejected.

## Follow-ups

- Combined commit status does not include Actions check runs. A check run that regresses on a stale ready PR is still only picked up when a head, review, or workflow event reaches the gate.
