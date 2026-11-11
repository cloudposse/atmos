# Fix: Quality Gate no longer blocks on unapproved `Quality Gate Signal` runs

**Date:** 2026-10-31

## Summary

The Quality Gate reported fork pull requests as not ready because its own `Quality Gate Signal`
workflow had a run waiting for maintainer approval. The reporter now treats signal runs as its
own plumbing rather than CI evidence, refreshes reports that were blocked this way once, and
describes workflow runs awaiting approval accurately instead of telling the author to open logs
that do not exist.

## Context

The repository requires maintainer approval for every Actions run on pull requests from external
contributors. `Quality Gate Signal` runs on `pull_request`, `pull_request_review` and
`pull_request_review_comment`; its only job runs `true` to wake the trusted CI Timing Summary
coordinator. Each review a bot posts on a fork PR (CodeRabbit, GitHub Advanced Security) starts a
new signal run that GitHub holds at `action_required` until a human approves it.

The reporter selects the newest run of each workflow for the PR head and treats `action_required`
as an unsuccessful terminal result. It already excluded its report, reconcile and release-milestone
runs from evidence, but not the signal workflow. A fork PR with every real check green therefore
stayed `Not ready` with the single blocker
`workflow: .github/workflows/quality-signal.yml: action_required`, and every scheduled reconcile
re-read the same held run. Approving the no-op job by hand cleared the gate until the next bot
review created another one. The accompanying guidance, "Open the logs for the unsuccessful
results", was also wrong for held runs: they have no logs.

## Changes

- `magefiles/qualitygate/quality_workflows.go`: `backgroundReconciliation` became `reporterRun`.
  It now also recognizes `.github/workflows/quality-signal.yml` at the workflow's own trigger
  events, so signal runs and their check runs never count as CI evidence. Other paths keep their
  trusted-event list. Reporter runs are skipped before the newest run per path is chosen, so an
  older run of the same path from an event outside the reporter's set stays evidence instead of
  disappearing behind a newer excluded run. A PR cannot use this to hide a failing workflow: a PR
  that could rename a workflow into the signal path could equally delete it, and required checks
  defend against that.
- `magefiles/qualitygate/quality_status_recovery.go`: the exclusion marker is now
  `<!-- atmos-quality-reporter-runs-excluded:v2 -->`, and the legacy-report detection includes the
  signal workflow. Any published report mentioning a reporter run as a blocker is re-evaluated once;
  the fresh report carries the new marker and omits the run.
- `magefiles/qualitygate/quality_checks.go`: the CI gate counts `action_required` workflow-run
  results separately. Results awaiting approval still block readiness, but the guidance now says
  that a maintainer must approve the waiting workflow runs from the Actions tab, alone or alongside
  the existing failure guidance. Check runs from other apps that conclude `action_required` keep
  the unsuccessful-result guidance, since their details page states the required action. Existing
  messages are unchanged when no workflow run is awaiting approval.

## Validation

- `TestQualitySignalRunsAreNotCIEvidence` reproduces the production case: a newer signal run at
  `action_required` with a green real workflow. It failed before the change with the exact blocker
  string observed on the affected PR and passes after. It also covers a dropped signal check run, a
  signal-path run at an untrusted event that must still count (including when a newer excluded run
  exists on the same path), and a real workflow awaiting approval that must still block with
  approval guidance.
- `TestQualitySelfBlockedReportsRefreshOnce` now covers all three reporter workflows and a report
  carrying the first marker, confirming one refresh and no recurring selection.
- `TestQualityGuidanceRespondsToActualState` gained rows for approval-only and mixed
  failure-plus-approval states, and for a third-party check run concluding `action_required`,
  which keeps the unsuccessful-result guidance.
- Ran `go vet -tags mage ./magefiles/...`, `go test -race -tags mage ./magefiles/qualitygate` and
  `atmos lint --changed`.
- The reporter runs default-branch code, so the behaviour change on open PRs takes effect after this
  merges to `main`; the next scheduled reconcile rewrites blocked reports.

## Follow-ups

None introduced by this change. Bot-triggered signal runs on fork PRs still wait for approval and
therefore do not wake the coordinator for those review events; the 15-minute reconcile already
covers them, which is its documented purpose.
