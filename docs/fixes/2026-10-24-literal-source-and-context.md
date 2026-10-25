# Fix: Preserve template source and restore literal naming context

**Date:** 2026-10-24

## Summary

Protecting a literal scalar no longer rewrites unrelated YAML template expressions, and literal variables no longer leak internal encoding into stack or workspace names.

## Context

Parsing and marshaling an entire document to shield one literal changed unquoted template expressions into YAML mappings. Stack naming also built its context from encoded source variables before final restoration.

## Changes

Shield literal scalar source directly without round-tripping the document through YAML. Restore a copy of variables when constructing naming context, retaining the encoded source map for later evaluation. Correct test import groups and remove a checkout-specific evidence filename from the earlier doctor fix record.

## Validation

Both regressions failed before the fixes. Focused literal, template, and field-test suites pass afterward, including four naming strategies, unchanged cached variables, raw template files, custom delimiters, inheritance, repeated passes, and generators.

## Follow-ups

None.
