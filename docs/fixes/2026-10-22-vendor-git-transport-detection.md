# Fix: Recognize supported Git transports during vendor update and diff

**Date:** 2026-10-22

## Summary

Vendor update and diff now recognize `git://` and `ssh://` repository sources.

## Context

Pull already accepted these transports, but version detection only recognized forced Git sources and selected hosting providers. A local Git daemon reproduced update silently skipping a valid source and diff rejecting it, twice each.

## Changes

Treat explicit Git and SSH transports as Git repositories. Preserve forced `git::` sources, existing hosting-provider detection, and ordinary local-file source behavior.

## Validation

`TestGitSourceSupportedTransports` failed twice before the fix and passed twice afterward. Focused version tests and scoped changed-line lint passed. An actual local Git daemon fixture verified read-only update checks, semver constraints, imported manifest pin updates with comments preserved, filtered diffs, latest-version diffs, and invalid-ref errors. The audit separately found stale installed files after pin changes. With the linked lock-receipt fix, the full live lifecycle also verifies changed file contents and unchanged-pull skipping.

## Follow-ups

The separate lock-receipt pin-change defect is addressed in [vendor source query identity](2026-10-22-vendor-source-query-identity.md).
