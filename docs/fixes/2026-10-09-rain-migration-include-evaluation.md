# Fix: Evaluate opt-in includes while loading Atmos configuration

**Date:** 2026-10-09

## Summary

Atmos configuration includes using `| eval` now evaluate supported YAML functions before decoding the included data. Rain migration examples define release values explicitly, and the original migration guide URL redirects to its new location.

## Context

The configuration loader decoded included YAML without invoking its YAML-function evaluator. Local and remote includes could therefore return literal environment variable names and nested include arguments even when evaluation was requested. The Rain migration skill also used a fixed artifact key, and the renamed migration guide had no redirect.

## Changes

- Evaluate opted-in local and remote include subtrees through the configuration loader's existing YAML-function decoder.
- Preserve the included local file's directory, or the outer configuration file's directory for remote content, when resolving nested relative includes.
- Keep includes without `| eval` as data and avoid evaluating returned strings twice.
- Track active includes and bound nesting so cyclic includes fail with an error; allow repeated sibling includes.
- Define release variables in the Rain migration examples and derive the published artifact key from the release.
- Redirect `/migration/from-rain` to `/migration/rain`.

## Validation

- New local and remote include regressions reproduce the previous literal-value behavior and pass with the fix.
- Go 1.26.9 race tests pass for `pkg/config`, `pkg/utils`, and `pkg/manifest`.
- All 57 website navigation tests and 37 website prebuild tests pass.
- The complete website and CLI builds pass. Changed-line coverage for the Go fix is 36/38 executable lines (94.74%); final Codecov coverage is still required.
- Patch-scoped `atmos lint --changed` passes with zero issues.
- The generated legacy redirect and destination page are present in the built website.
- The Floci example passes initial deployment, unchanged redeployment, changed release, archive verification, HTTP 200 and response assertions, and cleanup using the rebuilt CLI.

## Follow-ups

None.
