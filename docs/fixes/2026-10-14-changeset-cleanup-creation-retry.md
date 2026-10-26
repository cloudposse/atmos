# Fix: Retry preview cleanup while a change set is being created

**Date:** 2026-10-14

## Summary

Preview cleanup waits for a change set to finish being created when AWS rejects
its deletion with `InvalidChangeSetStatus`, then retries deletion within the
existing 30-second cleanup deadline.

## Context

A failed or cancelled preview poll could leave its change set in
`CREATE_IN_PROGRESS`. The immediate cleanup attempt then failed and left the
change set and any owned preview stack behind. AWS documents this temporary
rejection in the [DeleteChangeSet API reference](https://docs.aws.amazon.com/AWSCloudFormation/latest/APIReference/API_DeleteChangeSet.html).

## Changes

- Poll `CREATE_PENDING` and `CREATE_IN_PROGRESS` until `CREATE_COMPLETE` or
  `FAILED`, then retry deletion.
- Keep all polling and retries inside the existing cleanup deadline, including
  repeated status rejections after creation has completed.
- Preserve other API errors and the original operation error. Delete an owned
  empty stack only after its change set is successfully deleted.
- Keep cleanup independent of cancellation of the preview's context.

## Validation

- `GOTOOLCHAIN=go1.26.9 go build ./pkg/component/aws/cloudformation` passed.
- `GOTOOLCHAIN=go1.26.9 go test -race ./pkg/component/aws/cloudformation -count=1`
  passed, including creation-status retries, cleanup after cancellation, terminal
  failures, and virtual-clock deadline exhaustion.
- The repository's custom golangci-lint reported zero patch issues.
- The fix-log validator and `git diff --check` passed.

## Follow-ups

None.
