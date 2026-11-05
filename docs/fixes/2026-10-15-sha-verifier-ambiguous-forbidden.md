# Fix: Reject ambiguous forbidden responses during SHA verification

**Date:** 2026-10-15

## Summary

The action SHA verifier only applies a reviewed repository exception when GitHub
explicitly identifies that repository owner's organization IP allow list as the
reason access was denied.

## Context

An HTTP 403 without quota headers could previously be treated as an access block.
GitHub secondary rate limits can omit those headers, allowing a listed repository
to pass without SHA verification. See [GitHub's rate-limit documentation](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api).

## Changes

- Recognize secondary limits from the API response body and retain bounded backoff.
- Require the explicit organization IP-policy denial before applying an exception.
- Fail verification for ambiguous 403 responses, even when primary quota remains.
- Keep rate-limit signals authoritative over a conflicting access-block message.
- Exercise the embedded verifier with realistic API error bodies and update its guide.

## Validation

The new ambiguous-403 and headerless-secondary-limit regressions failed before the
fix. The verifier, retry, and offline Node test suites pass after the fix.

## Follow-ups

None.
