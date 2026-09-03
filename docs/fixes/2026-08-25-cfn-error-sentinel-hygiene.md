# Fix: `aws/cloudformation` error sentinels no longer conflate unrelated failure classes

**Date:** 2026-08-25

## Summary

`errUtils.ErrAwsCloudFormationChangeSetFailed` and `ErrAwsCloudFormationDriftDetected` were being
used across `pkg/component/aws/cloudformation` as generic catch-all sentinels for failures that had
nothing to do with a changeset or with confirmed drift — most seriously, `drift detect`/`describe`
wrapped *any* API failure (including "this emulator doesn't implement this action") with the
drift-detected sentinel, whose message literally reads "aws/cloudformation stack has drifted,"
actively misleading a user into thinking their infrastructure changed. Two new sentinels
(`ErrAwsCloudFormationAPICallFailed`, `ErrAwsCloudFormationOperationFailed`) now separate "a plain
AWS API call failed" and "an async operation reached a failed/timeout terminal status" from genuine
changeset/drift outcomes.

## Context

Found during a field-test pass on `atmos aws cloudformation`: `drift detect` against both Floci and
MiniStack (neither implements `DetectStackDrift`) returned `Error: aws/cloudformation stack has
drifted: ... UnknownAction`/`InvalidAction` — confirmed live, not just by reading code. The same
sentinel-reuse pattern turned out to be far more widespread than the original 4 files scoped
(`output.go`, `get.go`, `list.go`, `delete.go`) — a full sweep of the package found it repeated in
`drift.go`, `changeset_verbs.go`, `events.go`, `stackset.go`, `provision.go`, `validate.go`, and
`executor.go`.

## Changes

Added two sentinels to `errors/errors.go`:
- `ErrAwsCloudFormationAPICallFailed` — a plain AWS API call failed, no changeset/drift semantics.
- `ErrAwsCloudFormationOperationFailed` — an async operation (stack update, StackSet operation,
  watched event stream) reached a failed/stopped terminal status or timed out, but the API calls to
  invoke/poll it succeeded.

Reclassified every misuse site by actually reading each call site's context (not a blanket
find-replace):
- **`ErrAwsCloudFormationAPICallFailed`**: `drift.go` (5 of 6 sites — kept the one genuine
  `--fail-on-drift` case unchanged), `output.go`, `get.go` (both), `list.go`, `delete.go` (3 of 5
  sites — kept 2 genuine local-validation-error reuses unchanged, out of scope), `stackset.go` (7 of
  9 sites), `observability.go` (`listAllStackResources`), `validate.go` (`setStackPolicy`).
- **`ErrAwsCloudFormationOperationFailed`**: `stackset.go` (the 2 "operation ended in status"/timeout
  sites), `changeset_verbs.go` (`runChangesetExecute`'s post-execution stack-status check),
  `events.go` (`streamStackEvents`'s timeout), `provision.go` (`deployDirect`'s post-execution
  check), `observability.go` (`runWatch`'s failed-status check), `executor.go` (`runDelete`'s
  post-delete failed-status check — found during this work, not in the original 4-file scope).
- Left `changeset.go` and 3 of `changeset_verbs.go`'s sites unchanged — those genuinely wrap
  changeset-computation failures (`CreateChangeSet`/`DescribeChangeSet`/`ExecuteChangeSet`/
  `ListChangeSets`/`DeleteChangeSet` failing, or a changeset reaching `FAILED` status), which is what
  the sentinel's name actually means.

Every existing test asserting the old sentinel at a reclassified call site was updated to assert the
new one (not just made to compile) — `drift_test.go`, `delete_test.go`, `get_test.go`, `list_test.go`,
`changeset_verbs_test.go`, `observability_test.go`, `provision_test.go`, `stackset_test.go`,
`executor_test.go` (the last also tightened from a loose `require.Error` to `assert.ErrorIs`, itself
a field-test finding per this repo's testing conventions).

## Validation

- `go test ./pkg/component/aws/cloudformation/... ./errors/...` — all pass, including every
  reclassified assertion.
- `go build ./... && go vet ./...` — clean.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding in `cmd/terraform/utils.go`
  remains, confirmed via `git log` to predate this session).
- Live, against Floci and MiniStack: `drift detect`/`describe` now correctly report
  `ErrAwsCloudFormationAPICallFailed` (not "stack has drifted") when the emulator doesn't implement
  the action.

## Follow-ups

`delete.go`'s 2 remaining local-validation-error sites (termination-protection gate,
`--retain-resources` requires `DELETE_FAILED`) still reuse `ErrAwsCloudFormationChangeSetFailed` —
flagged during this work as a separate, lower-priority cleanup (a third "local validation" sentinel
class), not fixed here to keep this change scoped to the API-call/operation-outcome distinction.
