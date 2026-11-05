# Fix: `terraform deploy` finds the merged pull request's stored plan after a squash or merge commit

**Date:** 2026-11-05

## Summary

A CI `atmos terraform deploy` now resolves the stored plan to verify against in a defined order:

1. The plan stored for the deploy's own commit. After a merge, this supports planning the merged commit first (for example, a plan workflow dispatched on `pull_request.merged`) and applying that plan.
2. When nothing is stored there, the plan stored for the head commit of the pull request merged as that commit, which is the plan reviewers saw.

Before this fix only step 1 existed. A deploy dispatched straight after a squash, merge, or rebase merge
found nothing, and failed verification (or applied an unverified fresh plan with `required: false`).

## Context

Stored planfiles are keyed `{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan.tar`, where `SHA` is the
commit the run checked out. A pull request's plan runs on its head commit. A post-merge deploy runs on
the commit that landed on the base branch, which differs from the head for every merge strategy except
fast-forward.

This surfaced when Atmos Pro started dispatching post-merge workflows with the landed commit instead of
the pull request head (between 2026-11-03 16:03 and 2026-11-04 05:31 UTC), so its deployment
authorization gate can verify the dispatched commit is on the base branch. Customers using
`pull_request.merged` to dispatch deploys with stored plans could no longer find their plans. The same
gap already affected the documented push-to-`main` "apply on merge" workflow.

Applying the reviewed plan stays safe because deploy reconciles rather than replays: it generates a fresh
plan of the landed commit, compares it with the stored plan, and fails on drift by default.

## Changes

- `pkg/ci/internal/provider/types.go`: new optional `MergedPullRequestResolver` capability and
  `MergedPullRequest` type.
- `pkg/ci/providers/github/merged_pr.go`: GitHub implementation using "list pull requests associated
  with a commit". It accepts only a merged pull request whose `merge_commit_sha` equals the commit,
  skips fast-forward merges (head equals the commit), and pages through results.
- `pkg/ci/plugins/terraform/merged_pr_planfile.go`: `downloadStoredPlanfile` tries the current commit's
  key first and falls back to the merged pull request's head key only on `ErrArtifactNotFound`. The
  fallback is skipped for pull request and merge-queue runs, and for providers without the capability.
  A lookup failure logs a warning that names the `pull-requests: read` permission.
- `pkg/ci/plugins/terraform/handlers.go`: `downloadPlanfileForVerification` uses the new helper.
- `pkg/ci/plugins/terraform/handlers.go`: removed `downloadPlanfile`, a second download path that no hook
  ever called (unused since #2079) and that would otherwise have needed the same fallback. Its only
  test, `TestDownloadPlanfile_NoPlanfilePath`, went with it.
- `errors/errors.go`: `ErrCIMergedPRLookupFailed`.
- Docs: new "Deploying after a pull request merges" section in `website/docs/ci/planfile-storage.mdx`
  documenting the resolution order,
  a pointer from the `terraform deploy` command page, and `pull-requests: read` added to the
  apply-on-merge workflow example.

## Validation

- `go test ./pkg/ci/... ./errors/... -count=1`: pass.
- New tests: `pkg/ci/providers/github/merged_pr_test.go` (squash resolution, filtering of unrelated or
  unmerged pull requests, fast-forward skip, pagination, API failure, incomplete input) and
  `pkg/ci/plugins/terraform/merged_pr_planfile_test.go` (end to end through
  `downloadPlanfileForVerification` with a real local artifact store: fallback used, merge-commit plan
  preferred without a lookup, and six no-fallback cases).
- Reverted the `handlers.go` call site locally and confirmed
  `TestDownloadPlanfileForVerification_FallsBackToMergedPRHead` fails without the fix.
- `atmos lint --changed`: 0 issues.
- Behavior change confirmed against live data: post-merge applies in
  `cloudposse-examples/atmos-pro-example-advanced` checked out the PR head on 2026-10-28/29 and the merge
  commit from 2026-11-04 onward.
- Not run: `atmos test --full`, the website build, or a real GitHub Actions deploy against a squash
  commit.

## Follow-ups

- Atmos Pro's apply workflow docs (atmos-pro.com, separate repository) must add `pull-requests: read`,
  or the lookup returns 403 on private repositories and the fallback cannot run.
- Atmos Pro's workflow run record still stores the pull request head in `sha` while GitHub receives the
  merge commit as `inputs.sha`; the dashboard shows the wrong commit for post-merge runs.
