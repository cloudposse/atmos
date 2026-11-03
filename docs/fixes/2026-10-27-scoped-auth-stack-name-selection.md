# Fix: Preserve logical stack matches during scoped auth validation

**Date:** 2026-10-27

## Summary

Scoped auth validation includes every possible logical stack match when a root-level manifest shares the selector's filename. Static naming uses the same default Go template delimiters as runtime stack naming.

## Context

Selecting `dev` could choose only `dev.yaml`, omitting another manifest whose explicit name, name pattern, or name template also resolved to `dev`. An invalid component in that omitted manifest incorrectly passed validation. Custom CLI template delimiters also made the static name template disagree with runtime naming.

## Changes

- Preserve discovered candidates unless a plain explicit manifest name proves they are unrelated. Physical filename matches remain candidates.
- Keep ambiguous remote imports in scope and report that they cannot be checked without fetching them.
- Move static name rendering into configuration helpers and static import-path validation into the imports package.
- Restore literal name data without changing cached source maps. Report unresolved native expressions used in names instead of silently excluding the component.
- Use default Go delimiters for `name_template`, while manifest expressions retain their configured delimiters.

## Validation

Runtime regressions reproduced both review findings before the fixes. Additional tests reproduced omitted literal and native-expression names. Race-enabled tests cover CLI auth validation, configuration, imports, and YAML source identity. Local HTTP fixtures verify that both unrelated and ambiguous imports cause zero requests.

## Follow-ups

None.
