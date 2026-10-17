# Fix: Preserve Starlark and manifest-load template behavior

**Date:** 2026-10-14

## Summary

Combine Starlark source protection with manifest-load template deferral.

## Context

Main added source protection and identity guards while this stack added component deferral and path lookup.

## Changes

Keep source restoration, template preparation, component deferral, and identity errors. Extract template-function setup and pass template input by pointer without mutating it. Correct scalar-output assertions: YAML files quote ambiguous strings, while terminal output remains raw.

## Validation

Affected-package build and focused template/Starlark/manifest-load tests pass with Go 1.26.9. Scalar output regression tests pass with the race detector. The combined stack passes lint.

## Follow-ups

Verify final published CI after the lower-layer conflict repairs propagate.
