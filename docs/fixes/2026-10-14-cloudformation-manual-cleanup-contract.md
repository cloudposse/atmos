# Fix: Clarify manual CloudFormation placeholder cleanup

**Date:** 2026-10-14

## Summary

Clarify that deleting a named change set preserves its stack and that deploy targets carry the documented packaging selector.

## Context

The PRD described automatic preview cleanup without stating the different ownership contract for manual change-set deletion. It also incorrectly implied that an existing `REVIEW_IN_PROGRESS` placeholder prevents a later CREATE operation.

## Changes

Document explicit stack deletion for abandoned placeholders, including the quota consequence and the need to preserve pending change sets. State that Atmos treats `REVIEW_IN_PROGRESS` as a CREATE target. Require the shared typed target and JSON schemas to preserve the existing string `packaging` selector.

## Validation

- `go test -race -count=1 ./pkg/component/aws/cloudformation -run 'TestRunChangesetDelete|TestStackExists'` passed against the prepared implementation.
- `atmos lint --changed` passed.
- Combined stack-tip regression tests and the full website build passed (40 prebuild tests); the rendered cleanup guidance and link were verified.

## Follow-ups

Carry the schema regression and command-reference clarification in their owning implementation and documentation layers of this stack.
