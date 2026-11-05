# Fix: Retain decoded auth identities when reconstruction fails

**Date:** 2026-10-09

## Summary

Preserve an already decoded identity and emit a warning when reconstruction from
raw configuration sources fails. Addresses review feedback on #3339.

## Context

Reconstruction replaces the identity map when any identity decodes successfully.
A failed identity was skipped with only a trace message, losing its existing
Viper-decoded value. Existing import tests exercised successful reconstruction,
but did not cover a raw value that differs from the effective Viper value.

## Changes

- Keep the existing entry for the same lookup key after a decode failure and log
  the identity name and error at warning level; successful reconstruction still
  takes precedence.
- Cover failure with and without a fallback, successful reconstruction, and
  preservation of an unrelated dotted identity.
- Document source-tracking helpers and regression tests flagged by review.
- Update the trace snapshot to count repeated source merges, fixing a CI failure.
- Cover the source-merge helpers' unusable-input paths (invalid YAML, a
  non-mapping identity, an unsupported YAML function) and the main-config
  fallback used when a load has no tracker, so one bad entry is shown not to
  discard its siblings.

## Validation

The new regression test failed before the fix. Config and adapter short tests,
focused config race tests, the previously failing
`TestCLICommands/Valid_Log_Level_in_Config_File` acceptance test, patch-scoped
custom lint, and the affected-file schema and EditorConfig validation checks
passed.

## Follow-ups

None.
