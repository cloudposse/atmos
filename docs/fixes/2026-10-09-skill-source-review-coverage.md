# Fix: Skill source and approval review coverage

**Date:** 2026-10-09

## Summary

Add behavioral tests for declarative skill sources and approval prompts, and correct
the plural verb in the Atmos AI skill's permission-mode guidance.

## Context

PR #3352's attached Codecov report showed 78.60% patch coverage against an 85%
target on an earlier revision. Review also identified a grammar error. The older
listing and installer-isolation findings were already addressed on this branch.

## Changes

- Exercise interrupted installation recovery, untrusted ownership records, malformed
  sources, pinned-download evidence, and configuration edits using isolated filesystems.
- Verify CLI source ambiguity, project/user selection, bulk updates, local-edit
  preservation, invalid configuration/state, and missing ad-hoc resolutions.
- Verify marketplace metadata, reference containment, project precedence, unavailable
  skills, and invalid or symlinked installation state.
- Drive all four cached permission choices and uncached allow/deny through a real
  terminal in accessible mode; reload decisions from disk and check receipts. Verify
  cancellation and timeout errors with non-rendering forms.
- Change “continues” to “continue” in the plural commands sentence.

No production behavior, coverage thresholds, or coverage exclusions changed.

## Validation

- `atmos fix coverage`: all 18 touched packages pass before and after the changes.
  The final profile covers 172 additional statements.
- Package statement coverage: skill CLI 82.90% → 86.47%; marketplace
  84.04% → 87.16%; source engine 85.65% → 90.93%; permissions 88.62% → 95.78%.
- `atmos lint --changed`: passes. Targeted lint for the four affected packages
  also reports zero issues.
- Windows cross-compilation passes for all four affected packages' test binaries.
  PTY-dependent tests explicitly skip on Windows; cancellation/timeout tests
  remain portable.

Coverage is scoped to touched packages' own tests, not full-suite
(`-coverpkg=./...`) breadth. CI's full-suite Codecov upload remains authoritative;
these package figures do not claim that CI's patch target has passed.

## Follow-ups

None.
