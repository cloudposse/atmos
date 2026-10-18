# Fix: Remove temporary CloudFormation diff resources

**Date:** 2026-10-10

## Summary

CloudFormation diff now deletes its preview changeset and any new empty stack it created. Cleanup failures are returned alongside preview failures.

## Context

Diff created changesets without removing them, consuming the account quota. CREATE previews also left empty `REVIEW_IN_PROGRESS` stacks. A polling failure could discard the identity needed to clean up those resources.

## Changes

- Preserve changeset identity and stack ownership after successful creation, even when polling fails or is cancelled. Shared creation performs no cleanup, preserving the apply lifecycle.
- Clean up after successful previews, no-op previews, polling failures, and output failures. Combine cleanup errors with the original error so neither is hidden.
- Preserve the original error object when cleanup succeeds so existing diagnostic hints remain accessible.
- Detach cleanup from caller cancellation and bound it to 30 seconds.
- Delete only a newly created stack that is still `REVIEW_IN_PROGRESS`, preferring its immutable stack ID. Preexisting stacks, including preexisting empty stacks, are preserved.
- Extend the CloudFormation client interface and regenerate its mock for `DeleteChangeSet`.

## Validation

All 12 new cleanup cases failed before the fix. The cases cover existing/new stacks, preexisting empty stacks, a stack whose deployment started, an already removed stub, no-op previews, failed/cancelled polling, and changeset/inspection/stack cleanup failures, including combined errors.

With `GOTOOLCHAIN=go1.26.9`:

- Full CloudFormation package race tests passed in 3.998 seconds, including existing apply and output-failure regressions; package coverage was 97.6%.
- `createChangeSet`, `describeStackState`, `discardChangeSet`, `removeStubStack`, `joinCleanupError`, and `runDiff` each reached 100% function coverage. Local profile blocks cover all 76 changed executable production lines; final Codecov remains authoritative for PR coverage.
- `go build ./...` passed.
- Custom golangci-lint reported zero issues on changed CloudFormation code.
- Formatting, `git diff --check`, and the fix-record validator passed.

## Follow-ups

None.
