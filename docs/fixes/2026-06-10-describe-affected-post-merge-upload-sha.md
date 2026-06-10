# Fix: Native CI Post-Merge Uploads Use the PR Head SHA

**Date:** 2026-06-10

## Issue

A native CI `atmos describe affected --upload` run triggered from a raw GitHub
`pull_request.closed` event after a PR was merged could upload affected-stack
results with the pre-merge PR head SHA instead of the commit that landed on the
base branch.

In the reported run, `actions/checkout` had checked out the expected `main`
commit, but the upload payload was correlated to the PR SHA. Atmos Pro then
associated the post-merge plan with the wrong commit, which showed up as a
false-positive drift signal.

This is separate from the long-standing Atmos Pro synthetic
`settings.pro.pull_request.merged` event. That synthetic event remains the
preferred semantic event for merged PR workflows and is unchanged by this fix.

## Root Cause

`pkg/ci/providers/github/base.go:resolvePRBase` extracted
`event.pull_request.head.sha` for every `pull_request` event and exposed it as
the upload correlation SHA through `BaseResolution.HeadSHA`.

That is correct for open PR events (`opened`, `synchronize`, `reopened`) because
Atmos Pro indexes open PR checks by the PR head SHA. It is not correct for a raw
GitHub `pull_request.closed` event where `pull_request.merged == true`: after a
merge, especially a squash merge, the commit on the base branch is different
from the original PR head SHA.

GitHub exposes the landed commit in `pull_request.merge_commit_sha` for merged
PRs:

- merge commit strategy: the merge commit SHA
- squash strategy: the squashed commit SHA on the base branch
- rebase strategy: the commit the base branch was updated to

The base-diff resolution was not the problem. The existing chain still chooses
the comparison base through merge-base, `HEAD~1`, payload `base.sha`, or the
target ref fallback. The bug was only the SHA sent in the upload payload for
native CI correlation.

## Fix

The GitHub CI provider now treats `BaseResolution.HeadSHA` as the upload
correlation SHA, not always as the PR head SHA:

- open PR events continue to use `event.pull_request.head.sha`;
- raw merged `pull_request.closed` events use
  `event.pull_request.merge_commit_sha`;
- merge queue events continue to use `event.merge_group.head_sha`;
- if a raw merged PR payload is missing `merge_commit_sha`, the override is left
  empty so the upload path falls back to the checked-out local `HEAD`.

The provider also records diagnostic metadata on `BaseResolution`:

- event action;
- `pull_request.merged`;
- `pull_request.head.sha`;
- `pull_request.merge_commit_sha`;
- checked-out local `HEAD`;
- final upload correlation SHA.

`internal/exec/describe_affected.go` logs these fields when CI base detection
runs and logs the final upload `head_sha` before calling Atmos Pro. This makes
future SHA mismatches visible in GitHub Actions logs without changing the upload
wire format.

The native CI base-resolution PRD was updated to document the event-aware upload
SHA behavior and to explicitly call out that the synthetic
`settings.pro.pull_request.merged` event is not changed.

## Verification

Added tests:

- `pkg/ci/providers/github/base_test.go` verifies raw merged PR-closed events
  use `merge_commit_sha` for upload correlation.
- `pkg/ci/providers/github/base_test.go` verifies a missing `merge_commit_sha`
  leaves the override empty so local `HEAD` is used.
- `internal/exec/describe_affected_test.go` verifies CI auto-detection propagates
  the merged commit SHA into `DescribeAffectedCmdArgs.HeadSHAOverride`.

Commands run:

```shell
go test ./pkg/ci/providers/github ./internal/exec
go test ./pkg/ci/...
```
