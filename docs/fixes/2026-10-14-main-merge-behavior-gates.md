# Fix: Reconcile edition changelog with behavior gates

**Date:** 2026-10-14

## Summary

Resolve the editions PRD conflict while merging `origin/main` at `4d982b2bb8`
into the declarative skill-source branch.

## Context

Both branches assigned changelog version 1.8 to separate changes. Main now
implements behavior gates, while historical migrations remain ungated.

## Changes

Retain the Claude permission entry as version 1.8 and the later behavior-gate
implementation entry as version 1.9. Clarify that the Git hook metrics behavior
shipped before behavior gates were implemented, removing the stale claim that
gates are currently unavailable. No additional implementation changes.

## Validation

- Configuration, configuration schema, schema, editions, skill-source, skill CLI,
  and permission tests pass with package concurrency limited to two.
- The merged configuration JSON schema parses successfully.
- No unresolved conflicts remain; staged whitespace checks pass.
- `go build -p 4 ./...`: passes.
- `atmos lint --changed`: zero issues.

## Follow-ups

None.
