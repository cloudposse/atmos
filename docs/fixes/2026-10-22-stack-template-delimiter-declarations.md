# Fix: Preserve stack template delimiter declarations during rendering

**Date:** 2026-10-22

## Summary

Component and stack descriptions render per-component custom delimiters consistently, including delimiter pairs containing quote characters and repeated template evaluations.

## Context

The field fixture configured `['<<', '>>']` in component template settings. A narrow component query rendered correctly, but `describe stacks` failed with `illegal number syntax: "-"`. Full component rendering could also fail: serialized delimiter settings were themselves parsed as template expressions. Inline declarations could silently change during a second evaluation pass.

## Changes

Shield the exact YAML scalar source of delimiter declarations before each template pass, then restore it before decoding settings for the next pass. Other template source bytes are preserved. Use the effective per-component delimiter pair for delimiter-safe YAML serialization in both component and stack descriptions.

## Validation

The full-path regression failed before the fix and now passes for `<< >>`, `[[ ]]`, and quote-containing delimiters. It verifies rendered values, function strings, and unchanged settings over repeated evaluation. Existing tests for mid-evaluation delimiter changes, explicit empty overrides, literal preservation, and raw literal template protection also passed. Before/after logs are in `.context/skills-field-test/product-regressions-{before,after}.log`.

## Follow-ups

None.
