# Fix: Merge main's configuration tracking with skill sources

**Date:** 2026-10-10

## Summary

Resolve the log-level snapshot conflict when merging `origin/main` at
`5093a11e25` into the declarative skill source branch.

## Context

The skill branch added `ai.skills` to case-sensitive configuration paths.
Main changed configuration source tracking, increasing this fixture's
`files_processed` count from one to two. Both changed the same trace lines.

## Changes

Regenerate the conflicting stderr snapshot from the merged code with the
acceptance harness. The result retains `ai.skills` and records two processed
sources. Configuration implementation changes merge without conflicts.

## Validation

- `go build ./...`: passes.
- Configuration, skill-source, and skill CLI package tests: pass.
- Targeted log-level acceptance snapshot regeneration and subsequent test run
  without regeneration: pass.
- `atmos lint --changed`: zero issues.
- `git diff --cached --check`: passes; no unresolved conflicts remain.

## Follow-ups

None.
