# Fix: Preserve workflow identity after shared option refactor

**Date:** 2026-10-14

## Summary

Keep the resolved workflow identity when dispatching extended steps.

## Context

Main consolidated extended-step options while the publishing step added resolved identity propagation.

## Changes

Set Identity on the shared option struct so all dispatch paths retain the resolved identity alongside toolchain PATH and authentication. Preserve script metadata and retry behavior.

## Validation

Focused publish/authentication and resolved-identity workflow regressions pass with Go 1.26.9 and the race detector.

## Follow-ups

Verify final published CI after the lower-layer conflict repairs propagate.
