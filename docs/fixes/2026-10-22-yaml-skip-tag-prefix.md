# Fix: Accept YAML tag prefixes in function skip lists

**Date:** 2026-10-22

## Summary

`--skip '!terraform.output'` now preserves the expression just like `--skip terraform.output`. Both spellings also work for other deferred YAML functions.

## Context

The field test reproduced a real Terraform output fetch despite an explicitly skipped `!terraform.output` tag. Legacy dispatch stripped `!` only from the function being evaluated, while registry dispatch compared only bare names.

## Changes

Use one tag skip matcher in legacy and registry evaluators. It strips one optional leading `!` from either side while retaining exact, case-sensitive matching. It does not broaden skip behavior to load-time functions.

## Validation

The new `TestFieldTestSkipTaggedFunctions` failed before the fix by attempting a missing Terraform lookup. It now preserves environment, Terraform-output, and registry-backed CI-label expressions for both skip spellings. Existing skip edge-case and registry tests passed with the focused internal/exec and pkg/function suites. Before/after test logs are in `.context/skills-field-test/product-regressions-{before,after}.log`.

## Follow-ups

None.
