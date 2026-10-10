# Fix: Validate cast recordings with embedded Starlark

**Date:** 2026-10-04

## Summary

Cast authoring skills now recommend embedded Starlark for new validators and when
updating Python-based assertions.

## Context

The Starlark integration provides file reading, JSON decoding, regular expressions,
and assertions without requiring a separate Python process for cast validation.
Recording events can split words and secrets across writes, so validators must
reconstruct output before checking content.

## Changes

- Added repository-specific guidance using the shared `cast_checks.star` module.
- Added a portable, executable validator example to the public authoring reference.
- Linked cast-generation guidance to that example and required validation after
  recording and cleanup, while retaining visual review for pacing and layout.
- Documented source-relative imports, working-directory-relative data paths, ANSI
  normalization, expected failures, and checks for secrets and local paths.

## Validation

- All three affected skills passed the skill creator's structure validator.
- The repository-specific example passed against the release-plan recording.
- The portable example passed against a synthetic recording whose expected text
  spans two output events.
- The portable example rejected a recording missing the expected output with the
  documented diagnostic and a nonzero exit status.
- Both examples ran with the freshly built Atmos binary and embedded Starlark.

## Follow-ups

Existing Python validators can migrate as their casts are updated; this change
does not replace unrelated fixture setup scripts.
