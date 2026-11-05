# Fix: Add CI context queries and readable metadata columns

**Date:** 2026-10-15

## Summary

Add `--query` / `-q` to `atmos ci context` and replace the CI metadata allowlist's
manually aligned text with a responsive list.

## Context

CI context documentation relied on an external `jq` process even though other
Atmos inspection commands offer built-in queries. Long environment variable
names also ran together in the documentation's fixed-width text columns.

## Changes

- Register query options through the standard flag parser, including `ATMOS_QUERY`
  and explicit flag precedence.
- Evaluate snapshot or field-inventory queries using Atmos's YQ engine with JSON
  codecs, retaining native types and large integers. Buffer results before output.
- Document JSON result streams, array collection, quoted projection keys, and
  filtering after `--paths`; replace the external query examples.
- Lay out all 24 metadata names in a responsive grid with separate columns and
  a single column on narrow screens.

## Validation

- CI command package tests passed, including projections, shorthand, null and
  scalar results, large integers, multiple/empty results, field filtering,
  environment precedence, and invalid-query errors without partial output.
- Website prebuild checks passed (14 tests).
- The CLI build and patch-scoped lint passed. Real CLI invocations with a local
  GitHub event fixture produced the documented projection and filtered path array.
- The production website build passed. Browser checks confirmed all 24 names
  remain visible, with separate desktop columns and a single mobile column,
  without page overflow at 1440, 768, 390, and 320 pixels.
- Fix-document validation and whitespace checks passed.

## Follow-ups

None.
