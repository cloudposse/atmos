# Fix: Preserve both edition changelog entries during main merge

**Date:** 2026-10-10

## Summary

Resolve the editions PRD changelog conflict when merging `origin/main` at
`7e015995f5` into the skill-source branch.

## Context

Main added tool-version defaults and color controls. Both branches independently
added a version 1.6 entry to the editions PRD.

## Changes

Keep main's toolchain entry as version 1.6 and retain the skill branch's Claude
Code permission entry as version 1.7. All implementation changes merge without
conflicts.

## Validation

- `go build ./...`: passes.
- Full tests for configuration, configuration schema, schema, editions, skill
  sources, skill CLI, and permissions: pass.
- Permission tests also pass with CI-style colored output enabled.
- Merged JSON configuration schema parses successfully.
- `atmos lint --changed`: zero issues.
- No unresolved conflicts; staged whitespace check passes.

## Follow-ups

None.
