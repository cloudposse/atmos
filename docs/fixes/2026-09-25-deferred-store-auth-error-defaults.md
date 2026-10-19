# Fix: Store defaults preserve identity and authorization errors

**Date:** 2026-09-25

## Summary

Deferred `!store` and `!store.get` reads no longer substitute a configured default
for authentication, identity-context, or permission-denied errors.

## Context

Authentication setup failures already bypassed value defaults, but errors returned
by the backend read reached a separate fallback handler. That handler protected
only `ErrAuthenticationUnavailable`, allowing other identity and authorization
failures to be hidden by a default value.

## Changes

The store-owned result handler now also propagates `ErrAuthenticationFailed`,
`ErrAuthContextNotAvailable`, `ErrIdentityNotConfigured`, and `ErrPermissionDenied`.
It uses the existing provider-neutral errors and preserves their error chains.
Other read errors, null defaults, and query handling retain their existing behavior.

## Validation

- The regression test failed before the fix for all four newly protected errors
  through both YAML store functions.
- Direct and wrapped errors pass through both functions after the fix.
- `go test ./pkg/store/deferred ./pkg/secrets/deferred -race -cover -count=1` passed;
  local package statement coverage was 100% and 97.7%, respectively.
- `TEST='./pkg/store/deferred ./pkg/secrets/deferred' atmos test` passed, including
  existing ordinary read-error and null-value fallback tests.
- `go build ./...` passed; `atmos fix lint` reported zero issues.

## Follow-ups

None.
