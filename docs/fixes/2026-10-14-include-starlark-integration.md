# Fix: Preserve evaluated include scope and data context

**Date:** 2026-10-14

## Summary

Keep include evaluation and script provenance compatible with main’s YAML walker.

## Context

The merged include walker could lose the data-section context, and path canonicalization dropped the eval option.

## Changes

Carry child context through included-file walks. Preserve the eval option when resolving include paths. Cover both ordinary data sections and actual script steps with provenance assertions.

## Validation

Focused include, configuration, scope, rendering, and Starlark regressions pass with Go 1.26.9 and the race detector. Combined lint passes. The website build and 63 navigation/example tests pass.

## Follow-ups

Verify final published CI after the lower-layer conflict repairs propagate.
