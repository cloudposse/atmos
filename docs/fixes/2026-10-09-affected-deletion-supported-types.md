# Fix: Keep affected deletion detection aligned with supported component types

**Date:** 2026-10-09

## Summary

Deletion detection uses the same canonical component types supported by the added/modified affected path.

## Context

The dependency lookup search order includes registered custom providers. Reusing it for deletion detection
reported deleted custom components even though added or modified components of those types were ignored.

## Changes

- Use `deletableComponentTypes` in both component and entire-stack deletion loops.
- Extend the existing Helm/Kubernetes deletion regressions with a registered custom component, proving
  supported components are still reported while unsupported custom providers are excluded.

## Validation

- Both extended regressions failed before the fix because an extra custom component was reported.
- Targeted deletion and dependency lookup tests passed with the race detector.
- `go build ./...` passed.
- Patch-scoped custom golangci-lint for `internal/exec`: 0 issues.
- `git diff --check` passed.

## Follow-ups

None.
