# Fix: Vendoring detects changed source query parameters

**Date:** 2026-10-22

## Summary

Vendor pulls fetch a changed Git ref instead of incorrectly reporting the installed files unchanged.

## Context

Lock receipts removed URL query strings before comparing declared sources. Two URLs differing only in `?ref=` were therefore indistinguishable, including after `vendor update --pull`.

## Changes

Store an optional SHA-256 query digest before redacting the source URL and compare it during materialization checks. Raw query values and credentials remain absent from the lock. Existing queried receipts need a one-time refresh under the selected lock enforcement policy: warn/silent refresh during pull, while strict requires `--refresh-lock`. Queryless receipts remain compatible.

## Validation

The regression fails twice before the change for changed refs, removed queries, changed options, and legacy queried receipts. The full lockfile and install package short tests pass after the fix, including unchanged-source and redaction controls.

## Follow-ups

None.
