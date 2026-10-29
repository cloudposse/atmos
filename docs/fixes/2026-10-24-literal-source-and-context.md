# Fix: Preserve template source and restore literal naming context

**Date:** 2026-10-24

## Summary

Protecting a literal scalar no longer rewrites unrelated YAML template expressions, and literal variables no longer leak internal encoding into stack or workspace names or component provenance.

## Context

Parsing and marshaling an entire document to shield one literal changed unquoted template expressions into YAML mappings. Stack naming also built its context from encoded source variables before final restoration. The typed provenance maps bypassed generic restoration, exposing encoded final values and dependency values while retaining references to cached source data.

## Changes

Shield literal scalar source directly without round-tripping the document through YAML. Restore a copy of variables when constructing naming context, retaining the encoded source map for later evaluation. Restore typed provenance final values and dependency values into copied maps and slices after evaluation. Correct test import groups, normalize workflow test paths for the slash-based report format, and remove a checkout-specific evidence filename from the earlier doctor fix record.

## Validation

Regression tests failed before their respective fixes. Focused literal, template, and field-test suites pass afterward, including four naming strategies, unchanged cached variables, raw template files, custom delimiters, inheritance, repeated passes, and generators. Actual component descriptions now verify scalar, list, and nested provenance across all four processing toggle combinations and repeated calls, with no internal markers. Mutating restored provenance leaves cached stack and source maps unchanged; direct tests also verify isolation in both directions.

## Follow-ups

None.
