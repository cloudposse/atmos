# Fix: Capture the release plan through Starlark exec.run

**Date:** 2026-10-24

## Summary

Correct the local release command's Starlark planning call to use `exec.run(..., output="capture")` and read `result.stdout`.

## Context

The Starlark conversion incorrectly passed `output="capture"` to `steps.shell`, whose output modes are raw, log, viewport, and none. Atmos correctly rejected the configuration before planning or dispatch. The test used a step stub that bypassed real validation and therefore missed the invalid option.

## Changes

Use subprocess output capture for planning, retaining native confirmation and the interactive shell step for execution. The regression test now runs real automation step validation, exercises exec.run through a stub process runner, and checks that failed planning never confirms or dispatches a release.

## Validation

- Full release package tests passed with the pinned GoReleaser 2.18.3 rehearsal.
- Regression cases cover fresh and resumed releases with accepted, declined, empty, and failed-planning outcomes.
- `atmos release cut </dev/null` using the installed Atmos binary completed planning and stopped with `Release cancelled` at the default No confirmation; no workflow was dispatched.
- Custom Go lint, fix document validation, and diff checks passed.

## Follow-ups

None.
