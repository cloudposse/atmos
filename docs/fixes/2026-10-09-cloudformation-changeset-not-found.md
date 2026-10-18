# Fix: Use CloudFormation's typed changeset-not-found error

**Date:** 2026-10-09

## Summary

Named changeset lookup recognizes the CloudFormation `ChangeSetNotFound` API code and preserves the
original AWS error when reporting the changeset-not-found sentinel.

## Context

The changeset lookup reused the stack-not-found classifier. Tightening that classifier correctly
excluded changeset messages, exposing the wrong-domain lookup. AWS documents
[ChangeSetNotFound for DescribeChangeSet](https://docs.aws.amazon.com/AWSCloudFormation/latest/APIReference/API_DescribeChangeSet.html).
The previous not-found wrapper also discarded the original error.

## Changes

- Classify typed `smithy.APIError` with code `ChangeSetNotFound`, independent of message wording.
- Retain strict missing-stack classification when the containing stack is absent.
- Preserve sentinel and original errors through `errors.Is` and `errors.As`.
- Add wrapped SDK and generic-code regressions, plus missing-role/S3, wrong-code, and plain-text controls.
- Verify the remaining upper-layer stack classifier consumers operate on stacks.

## Validation

- Typed changeset-not-found and error-identity regressions failed before the fix.
- `go test -race ./pkg/component/aws/cloudformation/...` passed.
- `go build ./...` passed.
- Patch-scoped custom golangci-lint for the CloudFormation subtree: 0 issues.
- Fix-log validation and `git diff --check` passed.
- Website build deferred to consolidated stack validation.

## Follow-ups

None.
