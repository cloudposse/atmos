# Fix: Preserve deferred evaluation scope and target credentials

**Date:** 2026-09-25

## Summary

Custom template delimiters now participate in dependency analysis without widening
inventory-only requests to all values. Deferred Terraform output lookups no longer
reuse the calling component's credentials when the target has none.

## Context

Three review findings identified inconsistent credential selection and delimiter
handling. Stack listings discarded field requirements whenever custom delimiters
were configured, even when no values were requested. Component processing instead
analyzed custom-delimiter expressions as literals and omitted their dependencies.

The proposed change to always restore the original section filter was not safe:
a regression demonstrates that a dynamic reference can require cross-section
template settings on a later rendering pass. With the proposed fallback, the
displayed value remains an unresolved template instead of `platform`.

## Changes

- Shared analysis in `pkg/template` and `pkg/deferred` accepts the configured
  delimiters, including component overrides and explicit resets to the defaults.
- Both processing paths use that analysis. Empty requirements stay empty; genuine
  dynamic expressions still require conservative evaluation.
- The Terraform output getter uses the existing target-context resolver, matching
  state and component lookups. Missing target credentials cannot inherit caller
  credentials, including the masked-output path.
- Tests use generated mocks to reject unexpected authentication or backend calls.

## Validation

- Reproduced unexpected authentication in custom-delimiter inventory listings and
  unresolved dependencies in component processing before updating the callers.
- Four output-getter cases failed before the credential fix; all eight cases
  (including successfully resolved target credentials) pass afterward.
- A negative-control run of the proposed section-filter fallback failed the
  dynamic cross-section regression; the preserved conservative behavior passes.
- Focused evaluation, delimiter, and deferred-authentication tests passed.
- Race tests for `pkg/deferred` and `pkg/template` passed, with local package
  statement coverage of 99.6% and 91.3%, respectively. The new processing and
  output-getter regressions also passed under the race detector.
- `go build ./...`, focused `atmos test`, and the short `cmd/list` and `pkg/list`
  suites passed. `atmos fix lint` reported zero issues.

## Follow-ups

None.
