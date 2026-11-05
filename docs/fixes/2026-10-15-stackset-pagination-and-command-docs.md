# Fix: Stop StackSet pagination on empty tokens and clarify command docs

**Date:** 2026-10-15

## Summary

Address three CodeRabbit findings in the Phase 3 CloudFormation stack layer.

## Context

StackSet instance listing stopped only for a nil token, so an empty final token could restart pagination. Two command pages also overstated StackSet membership restrictions and when the stack selector is required.

## Changes

- Stop instance pagination when the continuation token is nil or empty, matching stack event pagination.
- Extend pagination regressions to cover single and multiple pages ending in an empty token, retaining nil-token cases and verifying every account returned.
- Clarify that Atmos cannot add instances to an existing StackSet, while the AWS CloudFormation console and AWS CLI can.
- Describe `watch --stack` as required for a single component and optional as a bulk-selection filter.

- Align the corrected wording with the later documentation refresh, preserving both changes without rewriting descendant heads.

## Validation

- The two empty-token regression cases failed before the fix by detecting an unexpected extra API call.
- `GOTOOLCHAIN=go1.26.9 go test -race ./pkg/component/aws/cloudformation -count=1` passed.
- `GOTOOLCHAIN=go1.26.9 go build ./...` passed.
- Applicable pre-commit checks, including the custom Go lint gate, passed.
- The production website build passed; generated pages contain both corrected descriptions.

## Follow-ups

Verify CI and CodeRabbit review on the published commit.
