# Fix: `resolvePRBase` no longer returns a degenerate base for merged PRs

**Date:** 2026-08-06

## Summary

For merged pull requests, `atmos describe affected --upload` running in GitHub
Actions with `ci.enabled: true` could resolve a self-referential diff base when
the workflow's checkout wasn't pinned to `pull_request.head.sha`. This caused
every commit that landed on the target branch between when the PR branch was
cut and when it merged to be misreported as part of that PR — up to 510
false-positive "affected" components in one confirmed production incident.

## Context

`resolvePRBase` in `pkg/ci/providers/github/base.go` auto-resolves the diff
base for `pull_request` events via a fallback chain (merge-base, then
`HEAD~1`, then the payload's `base.sha`, then a ref to the target branch tip).
For a merged PR (`action == "closed"`, `pull_request.merged == true`), if
`HEAD` isn't pinned to the PR's own head SHA, it can end up on or past the
target branch's post-merge tip. In that state, the existing tiers can produce
a base that is effectively `HEAD` itself (in one incident, the resolved
`--base` exactly equaled `merge_commit_sha`), instead of the PR's true fork
point. Diffing that degenerate base against the PR's actual head SHA then
surfaces every unrelated commit that landed on the target branch in the
interim as "affected."

## Changes

- `pkg/ci/providers/github/base.go`: added a new tier 0 to `resolvePRBase`,
  gated on `action == "closed" && pull_request.merged == true`, that resolves
  `pull_request.merge_commit_sha`'s first parent via git (fetching the commit
  from `origin` first if it isn't present locally) and returns it as the base.
  This is derived from the merge commit GitHub itself created, so it's correct
  regardless of what the workflow checked out. Any failure (field missing,
  fetch failure, no parent) falls through unchanged to the existing 4-tier
  chain — no behavior change for any other event/action.
- Added a secondary guard in the existing tier 1 (merge-base): for closed PRs,
  if the resolved merge-base equals the currently checked-out `HEAD`, it's now
  treated as a failed/degenerate resolution and falls through to the rest of
  the chain, instead of being accepted as a valid "success." This closes the
  same failure class even when `merge_commit_sha` is unavailable in the
  payload.
- `pkg/ci/providers/github/base_test.go`: added
  `TestResolveBase_PullRequest_Merged_UsesMergeCommitParent` (deterministic
  git fixture reproducing the incident: a real merge commit with two parents,
  checkout left at the merge commit, asserting the resolved base is the merge
  commit's parent and not `HEAD` itself),
  `TestResolveBase_PullRequest_Merged_NoMergeCommitSHA_FallsThroughUnchanged`
  (regression guard: identical behavior to the pre-fix code when
  `merge_commit_sha` is absent from the payload), and
  `TestResolveBase_PullRequest_OpenedOrSynchronize_Unaffected` (regression
  guard: the new tier never fires for non-`closed` actions, even if a payload
  carries `merged: true` and a `merge_commit_sha`).

## Validation

- `go build ./...` — clean.
- `go test ./pkg/ci/providers/github/... -run TestResolveBase -v` — all tests
  pass, including the three new ones and every pre-existing test in the file
  (unmodified).
- `go vet ./pkg/ci/...` and `go test ./pkg/ci/...` — all packages pass.
- `atmos lint --changed` — 0 issues (after extracting tier 0 into
  `resolveMergedPRTier` to satisfy `nestif`, and introducing the
  `payloadKeySHA` constant to satisfy `revive`'s `add-constant` check on the
  repeated `"sha"` literal).
- `atmos test` (short suite): `tests/testhelpers` and
  `tests/testhelpers/httpmock` pass; `tests` (the `TestCLICommands` golden-
  snapshot suite) times out locally in this environment. Confirmed
  pre-existing and unrelated to this change: at the time of this run, this
  branch's `HEAD` was identical to `origin/main` aside from the two edited
  files and this doc, and the touched code (`pkg/ci/providers/github`) is
  nowhere near the CLI/emulator-preflight code path that hangs.

## Follow-ups

None.
