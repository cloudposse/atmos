# Fix: Include CloudFormation in canonical affected deletion detection

**Date:** 2026-10-09

## Summary

CloudFormation is part of the canonical affected component types alongside its dedicated added/modified
processor, so its deletions do not depend on provider registration.

## Context

CloudFormation's added/modified processor was present, but the canonical deletion list still contained
only the original eight types. Restricting deletion detection to supported types must retain CloudFormation.

## Changes

- Add `aws/cloudformation` to `deletableComponentTypes`.
- Cover a deleted CloudFormation component, a removed components section, and an entirely deleted stack
  without registering a provider.

## Validation

- All three regression cases failed before the list change and passed afterward.
- Targeted deletion, dependency lookup, and CloudFormation affected tests passed with the race detector.
- `go build ./...` passed.
- Patch-scoped custom golangci-lint for `internal/exec`: 0 issues.
- `git diff --check` passed.

## Follow-ups

None.
