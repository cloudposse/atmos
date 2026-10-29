# Fix: Check unresolved stack-name inputs before template execution

**Date:** 2026-10-27

## Summary

Scoped auth validation reports an incomplete check when a stack-name template reads an unresolved expression, including through conditions or formatting.

## Context

Checking only the rendered name missed expressions consumed by a conditional. A template comparing `vars.stage` with `dev` took its other branch when `stage` contained an unevaluated `!env` function. That name excluded a potentially matching component and incorrectly reported valid authentication configuration.

## Changes

Inspect template reads against the original source values before rendering. Cover fields, root variables, aliases, pipelines, conditionals, ranges, nested contexts, indexing, and named templates. Reading a whole map conservatively checks its children. Direct reads of known fields still work when unrelated fields contain expressions.

Preserve literal values and restore them only for rendering, without mutating source maps. Honor configured manifest delimiters and edition-specific string functions when identifying unresolved inputs. Keep pure Go naming templates on the runtime's default delimiters.

## Validation

A CLI regression reproduced the false success before the fix. Race-enabled auth, config, and static-name tests pass afterward, including conditional and truncated names, literal controls, unused dynamic fields, custom delimiters, and legacy versus explicit-tag editions. Diagnostics identify the manifest without exposing expression values.

## Follow-ups

None.
