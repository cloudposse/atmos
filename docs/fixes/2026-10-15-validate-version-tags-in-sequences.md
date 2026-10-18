# Fix: Validate version tags inside sequences

**Date:** 2026-10-15

## Summary

Reject `!version` tags inside configuration sequences, including nested containers
and `!append` lists.

## Context

PR #3352 review found that `devcontainer.examples: [!version {literal: main}]`
loaded successfully. Sequence mappings bypassed the node-level validation, and
nested sequences could be decoded without visiting their tags at all.

## Changes

Validate version tags recursively before sequence processing or append resolution.
No sequence descendant is a valid scalar `ai.skills.<name>.ref` location. Errors
include the offending configuration path, including sequence indices. Existing
evaluation of other YAML tags remains unchanged.

Add configuration-loading regressions for tagged mappings, empty containers,
nested sequences, scalar tags, and append lists. Verify ordinary collections,
quoted tag-like strings, and supported environment tags retain their values.

## Validation

- Seven malformed-container cases reproduced successful loading before the fix.
- Configuration, schema, skill-source, and skill CLI package tests pass, including
  all eleven sequence rejection cases and scalar-ref compatibility tests.
- `go build -p 4 ./...` passes.
- `atmos lint --changed` reports zero issues.

## Follow-ups

None.
