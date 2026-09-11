# Fix: Keep Dependency Parsing and Lookup Identity Consistent

**Date:** 2026-09-11

## Summary

Unrendered `required` templates now retain the required default until rendering, and cached dependent lookup now honors the selected component type.

## Context

Several dependency consumers decoded an unrendered `required` template as a boolean before templates ran. Cached dependent lookup also considered an unavailable same-named component of a different type, while scoped lookup did not.

## Changes

Added a shared dependency-section preparation helper and applied it before modern dependency parsing. The dependent lookup now records the resolved root type and ignores relationships whose effective target type differs from that root.

## Validation

- Focused `internal/exec` regressions for unrendered `required` values and index/scan cross-type lookup.
- Focused custom-delimiter schema regression.
- Compile-only checks for `pkg/component`, `pkg/list/dependencies`, and `pkg/scheduler/adapters`.

## Follow-ups

None.
