# Fix: Preserve generated declarations during secret enumeration retries

**Date:** 2026-10-16

## Summary

Secret enumeration retries now include components whose `secrets.vars` declarations
must be generated during evaluation. Failed evaluation of those declarations blocks
SOPS writes because the file placement cannot be checked safely.

## Context

When one component failed the stack-wide describe pass, raw discovery omitted
components with non-map declarations such as `!template` values. Their secrets and
possible SOPS file collisions were silently skipped. The CloudFormation component
schema also contained an identical duplicate `locals` property.

## Changes

- Retain present, non-map declarations as evaluation candidates while excluding
  unresolved values from actual listing and completion entries.
- Treat failed evaluation of those candidates as potentially SOPS-backed while
  retaining the existing tolerance for failures in known store-only declarations.
- Remove the duplicate schema property without changing its definition.

## Validation

- Reproduced omitted generated declarations and a silently successful collision
  check before the fix.
- Focused regressions pass for generated SOPS collisions, distinct files, store-only
  declarations, and failed generated declarations, alongside existing isolation tests.
- The complete stack schema passes a JSON parse that rejects duplicate keys.
- Go 1.26.9: `go test -race ./cmd/secret ./pkg/datafetcher/...` passes.
- `go build ./...` and all applicable pre-commit hooks pass.

## Follow-ups

Publish through the existing PR and confirm CI and the CodeRabbit re-review.
