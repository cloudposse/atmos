# Fix: conditional-import validation across YAML tags and native metadata

**Date:** 2026-10-25

## Summary

Align conditional-import/deferred tests with explicit YAML-tag handling, and correct metadata edge cases found while field-testing native tag support.

## Context

CI tested the PR merged with a newer `main`, whose YAML-function policy distinguishes explicit tags from quoted strings. Branch-only tests had constructed raw function strings and therefore missed that integration. The live metadata pass then found unavailable tag lists being treated as empty, literal exclamation-prefixed tags being rejected, and invalid custom-column syntax in the draft example.

## Changes

- Merge `main` and construct explicit YAML source values in deferred/CEL fixtures, retaining tests that reject quoted `!cel` strings.
- Keep unknown tag lists distinct from known empty/null lists. Conditions needing unknown tags fail evaluation instead of enabling negative membership checks.
- Preserve unknown state across append/merge operations; a known replacement or null can clear it.
- Respect literal YAML strings beginning with `!`; actual YAML functions remain unavailable during early import resolution.
- Use default `list instances` columns in the example, cast, and blog.
- Rewrite the blog around the same native metadata selecting configuration and component instances, with scope/type limitations stated explicitly.

## Validation

The original failing cases passed against merged `main`, including under the race detector. Condition, deferred, merge, import, and config package suites passed. The CI compatibility commit cleared the original test failures; two later jobs failed before tests because the StepSecurity agent exited on DNS lookup failures.

The native-metadata field test reproduced each defect twice, then verified both runtime fixes twice with the rebuilt CLI. It also verified the happy path, combined selectors, case sensitivity, list strategies, structural append, skipped imports, and documented scope/type boundaries. See `docs/field-tests/2026-10-25-native-metadata-imports.md` for exact repro commands and observed results.

## Follow-ups

None for the implemented fixes. Final cast, lint, test, and website results are recorded in the PR validation summary.
