# Fix: Clear script runner lint failures

**Date:** 2026-10-06

## Summary

Clear two script runner lint errors found by the commit hook while repairing
the Starlark YAML CI failures.

## Context

The shared process-call structure grew to 104 bytes after timeout and retry
support. Passing it by value triggered `gocritic`'s `hugeParam` check. The
standalone source reader also retained a `gosec` suppression that the current
linter no longer uses.

## Changes

- Pass the process-call options by pointer from both callers. The callee only
  reads the options, so execution behavior stays the same.
- Remove the unused suppression while preserving the explanation that script
  paths are explicitly selected by the caller.

## Validation

- Existing Starlark process, execution, Atmos wrapper, and standalone command
  tests passed with the pointer change.
- Go formatting and `git diff --check` passed.

## Follow-ups

None.
