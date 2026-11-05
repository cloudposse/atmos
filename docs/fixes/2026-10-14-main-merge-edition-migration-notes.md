# Fix: Preserve edition migration notes during main merge

**Date:** 2026-10-14

## Summary

Resolve the editions documentation conflict while merging `origin/main` at
`b5cc3ce67f` into the declarative skill-source branch.

## Context

Main introduced a migration note for Git hook resource-usage summaries and
renumbered the toolchain changelog entry. The skill branch independently added
the Claude Code permission migration note in the same location.

## Changes

Retain both migration notes and all changelog entries. Keep main's entries at
versions 1.6 and 1.7, and assign the Claude Code permission entry version 1.8.
Implementation files merge automatically.

## Validation

- No unresolved conflicts remain; staged whitespace checks pass.
- The merged configuration JSON schema parses successfully.
- `go build ./...`: passes.
- Configuration, configuration schema, schema, editions, and AI package tests
  pass except for one initial Claude cancellation fixture-startup failure during
  the concurrent build: its child did not start before the two-second timeout.
  The complete Claude Code package passes when rerun independently without code
  changes (`go test ./pkg/ai/agent/claudecode -count=1 -timeout=3m`).
- `atmos lint --changed`: zero issues.

## Follow-ups

None.
