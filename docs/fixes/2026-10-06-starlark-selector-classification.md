# Fix: Classify Starlark YAML expressions in selectors

**Date:** 2026-10-06

## Summary

Reject `!starlark` in `metadata.tags` and `metadata.labels`, preserving the
selector evaluation contract.

## Context

Acceptance shard 8 failed on Linux, macOS, and Windows because the selector
completeness test found the new YAML function unclassified. Although Starlark
configuration evaluation exposes no command API, its context reads can resolve
dependencies that execute commands or require authentication. Selectors must
remain usable before those operations.

## Changes

- Add `!starlark` to the forbidden selector functions and their constants check.
- Cover expression, multiline return, and nested-map values in validation tests.
- Document the selector restriction in the Starlark YAML PRD.

## Validation

- `GOMAXPROCS=4 go test -p 2 ./pkg/tags -count=1` passed the complete package suite.
- Fix-record validation and `git diff --check` passed.
- The three hosted acceptance jobs will rerun after the fix is pushed.

## Follow-ups

None.
