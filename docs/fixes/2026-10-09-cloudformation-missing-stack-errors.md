# Fix: Classify CloudFormation missing-stack errors without hiding other failures

**Date:** 2026-10-09

## Summary

Only typed CloudFormation validation errors with a stack-specific missing-stack message are treated as
an absent stack. Missing roles, S3 objects, or stack resources keep their original errors.

## Context

The previous substring match accepted any error containing `does not exist`. It could select a CREATE
changeset for an existing stack, falsely report deletion complete, or attach a misleading creation hint.
AWS documents [ValidationError for absent stacks](https://docs.aws.amazon.com/AWSCloudFormation/latest/APIReference/API_DescribeStacks.html)
and the [Stack with id message variant](https://docs.aws.amazon.com/parallelcluster/latest/ug/troubleshooting-v3-terraform.html).

## Changes

- Unwrap `smithy.APIError` with `errors.As`, require `ValidationError`, and match the complete API message.
- Recognize bracketed stack names and stack IDs/ARNs, including the deleted-stack message variant.
- Reject untyped errors, unrelated API codes/messages, and misleading wrapper text without a text fallback.
- Update existing missing-stack test fixtures to model typed AWS errors.
- Add classification and caller regressions covering changeset selection, both delete-poll paths,
  error hints, and preservation of error identity.

## Validation

- Four caller regressions failed before the fix: unexpected CREATE, two false deletion completions,
  and an incorrect missing-stack hint.
- `go test -race ./pkg/component/aws/cloudformation/...` passed.
- `go build ./...` passed.
- Patch-scoped custom golangci-lint for the CloudFormation subtree: 0 issues.
- Fix-log validation and `git diff --check` passed.
- Website build deferred to consolidated validation with the other #3157 fixes.

## Follow-ups

None.
