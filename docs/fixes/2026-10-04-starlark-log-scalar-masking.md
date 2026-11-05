# Fix: Mask scalar secrets before structured script logging

**Date:** 2026-10-04

## Summary

Starlark log fields redact registered scalar secrets before passing values to
the logger, while ordinary numbers and booleans retain their native types.

## Context

The log binding masked strings and composite values, but returned native
integers, floats, and booleans directly. A registered numeric secret could
therefore appear in structured output. Automatic step and task fields also
bypassed the masker.

## Changes

- Check native scalar rendering against the shared masker. Return a masked string
  only when redaction changes it; otherwise preserve the original scalar.
- Mask automatic step and task context values before sending them to any sink.
- Keep the existing masking opt-out and handling of strings, composite values,
  large integers, and non-finite floats.

## Validation

- `go test -count=1 ./pkg/script/starlark/stdlib/log -run
  TestNativeScalarSecretsAreMaskedBeforeLogging` failed in two independent runs
  before the fix, exposing registered scalar and context secrets.
- After the fix, `go test -race -count=3 -coverprofile=.context/log-scalar-runtime.cover
  ./pkg/script/... ./pkg/flags` passed. The log binding reached 100% statement
  coverage. Tests verify text and JSON sinks, preserved native types, and the
  masking opt-out.
- `./custom-gcl run --timeout=10m --new-from-rev=HEAD
  ./pkg/script/starlark/...` reported zero issues.

## Follow-ups

None.
