# Fix: Keep stack literal preservation ahead of generic tag encoding

**Date:** 2026-10-27

## Summary

Preserve the stack loader's literal envelope when template processing is enabled.

## Context

Conditional-import integration introduced a stack tag handler that encodes
`!literal` using YAML-function source metadata. The branch's stack loader uses a
separate envelope to shield literal text through templates, merging, and disabled
processing modes. A clean textual merge routed preserved literals through the
new generic handler, leaking internal markers into descriptions and templates.

## Changes

Honor `PreserveStackLiterals` before the generic template-enabled encoding path.
Keep the generic path for callers that do not request stack literal preservation.
Hide the native tag together with its scalar during raw-template shielding so
the shared source protector cannot encode a temporary placeholder again.
Cover both flags together and preserve whitespace in the regression test.

## Validation

- Existing stack literal regressions failed on the combined tree before the fix.
- The handler regression covers all four preservation/template flag combinations.
- Full regression and integration results are recorded with the PR.

## Follow-ups

None.
