# Fix: Quality Gate now publishes for fork PRs whose benchmark run is recovered by the reconciler

**Date:** 2026-10-16

## Summary

PR #3397, a draft from a fork, never received a Quality Gate comment or readiness label. The scheduled
reconciler took the benchmark-recovery path, which found no associated PR for the fork-only head commit,
published nothing, and reported success on every 15-minute run. The recovery path now resolves fork PRs
through their head branch and can no longer finish silently without publishing. The gate also gains two
visible, blocking checks: "Ready for review" and "No merge conflicts".

## Context

For a fork-only commit, GitHub returns an empty list from `commits/{sha}/pulls`, and the workflow run's
`pull_requests` array is empty. The earlier fork fix (#3383) resolved PRs by `owner:branch` in the event
selector and the timing coordinator, but `bench.ReportRun` still associated a benchmark run with PRs only
through `commits/{sha}/pulls`. When the reconciler found a completed benchmark run without a recorded
report, `recoverBenchmarkReport` called `ReportRun`, which iterated zero PRs and returned `nil`, so
`refresh` was never reached. The existing "fork" test mocked the commit endpoint as returning the PR and
hid the gap.

Draft status was not the cause, but it was only a hidden override (`ready := !pr.Draft`) with a footer line,
not a listed gate. Mergeability was not checked at all.

## Changes

- `magefiles/internal/githubapi`: shared `ListOpenMainPRs` and `OpenPRsForHead` helpers (head-branch lookup,
  head SHA verified). `PR` gains `Mergeable` and `MergeableState`.
- `magefiles/bench`: `benchAssociatedPRs` falls back to `OpenPRsForHead` using the run's head repository
  owner and branch when the commit endpoint yields no matching PR. `PRMatches` still validates every result.
- `magefiles/qualitygate`: `prNumbers` and `unlistedPRNumbers` reuse the shared helpers.
  `recoverBenchmarkReport` counts publishes; if `ReportRun` succeeds without publishing for the PR, it emits
  a workflow warning and publishes the gate with performance pending.
- New blocking gates, first in the registry: `ready-for-review` (passes when the PR is not a draft) and
  `merge-conflicts` (fails on `mergeable=false` or state `dirty`; an uncomputed `mergeable` is reported as
  unavailable evidence so the published report is kept and retried). The duplicate draft handling in
  `qualityMarkdown` and `qualityGuidance` is removed.

## Validation

- `go vet -tags mage ./magefiles/bench/... ./magefiles/qualitygate/... ./magefiles/internal/...`: clean.
- `go test -tags mage ./magefiles/bench/... ./magefiles/qualitygate/... ./magefiles/internal/...`: pass.
- `gofumpt -l magefiles`: no files listed.
- New tests cover a fork-only commit (commit endpoint empty, head-branch lookup returns the PR) with
  negative cases for an already-listed PR, a moved head, a non-main base, and a closed PR; the reconcile
  fallback with and without an associated PR; and the new gates, including a conflicting non-draft PR with
  every other gate passing being not ready.
- Live-API check that `commits/{sha}/pulls` is empty for #3397's head and that the benchmark run has no
  `pull_requests` was done by hand with `gh api`. The end-to-end result on #3397 can only be confirmed after
  this lands on `main`, because the reporter checks out the default branch.

## Follow-ups

- After merge, dispatch `quality-report.yml` and confirm #3397 receives the gate with "Ready for review"
  pending while it is a draft.
- The scheduled reconciler only revisits PRs with no report for their head, an unavailable report, or a head
  commit under 24 hours old. A published older PR that starts conflicting only because `main` moved is not
  re-evaluated until one of those conditions holds. Widening the selection costs extra API reads per run
  across all open PRs and is tracked here rather than in an issue.
