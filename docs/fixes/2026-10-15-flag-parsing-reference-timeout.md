# Fix: Isolate the flag-parsing reference timeout

**Date:** 2026-10-15

## Summary

Exclude one verified, reachable article from automated link checks after it timed
out in CI.

## Context

The Markdown link-check job failed on the flag-parsing article referenced in
`docs/prd/flag-handling/unified-flag-parsing.md`. The URL returned HTTP 200 when
checked locally and its content remains available.

## Changes

Add an anchored exception for the exact article URL in `lychee.toml`, preserving
the reference and checks for other pages on the same host.

## Validation

- The article returned HTTP 200 with curl.
- Lychee passed for the affected Markdown document: four valid links and the one
  excluded article, with no errors.
- Parsed the TOML and verified the new expression matches the exact URL while
  leaving the host root and other article paths covered.
- Patch-scoped lint, fix-document validation, and `git diff --check` passed.

## Follow-ups

None.
