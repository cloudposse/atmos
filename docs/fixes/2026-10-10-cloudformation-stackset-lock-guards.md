# Fix: Enforce component locks for StackSet mutations

**Date:** 2026-10-10

## Summary

CloudFormation StackSet create, update, and delete now reject components with `metadata.locked: true` before authentication or execution side effects.

## Context

The component lock guard covered stack apply and delete, but the later StackSet operations were missing from its mutation policy. StackSet creation can deploy instances, updates propagate to existing instances, and deletion removes instances with `RetainStacks: false`.

## Changes

- Classify StackSet create, update, and delete as stack mutations.
- Keep StackSet instance listing available on locked components.
- Exercise the actual single-component execution path with resolved metadata, locked and unlocked components, dry runs, and automatic confirmation enabled or disabled.

## Validation

- Before the fix, locked StackSet mutations reached the authentication seam, while their dry runs incorrectly succeeded.

- Go 1.26.9: `go test -race ./pkg/component/aws/cloudformation/... -count=1` passed, including all 32 execution-policy cases.
- Go 1.26.9: `go build ./...` passed.
- Custom GolangCI lint scoped to changes since `6d7dcb5028a223a7afdacf08dc869174ba0abede` passed with zero issues.
- Fix-log validation and `git diff --check` passed.

## Follow-ups

None.
