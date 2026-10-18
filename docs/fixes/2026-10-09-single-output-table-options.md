# Fix: Preserve options for single-output tables

**Date:** 2026-10-09

## Summary

Single-output table rendering now honors the supplied syntax-highlighting configuration.

## Context

`FormatSingleValueWithOptions` transformed uppercase keys but dispatched table output through
a helper that supplied empty options. Bulk table output honored `AtmosConfig`, while a single
output silently lost its highlighting settings.

## Changes

Route single-value tables through the shared table renderer with the supplied options after
key transformation. Other output formats retain their existing dispatch behavior.

## Validation

- The regression failed before the fix: forced highlighting appeared in bulk output but not
  in the equivalent single-output table.
- `go test -race ./pkg/output -count=1` passed, including uppercase-key preservation and
  `NoColor` overriding forced highlighting.
- Patch-scoped custom lint reported zero issues; `git diff --check` passed.

## Follow-ups

None.
