# Fix: Check YAML function scope in doctor

**Date:** 2026-10-22

## Summary

Doctor now diagnoses YAML functions in the context where their values are consumed, rather than treating every registered name as valid throughout a stack or vendor manifest.

## Context

Configuration, stack, workflow, scaffold, and vendor loaders have different evaluation pipelines. A valid name can remain unevaluated in a workflow command or vendor source. The previous rule also rejected valid configuration merge directives and skill source version references.

## Changes

Added a consumer capability catalog with native-tag and string-form support, accepted node shapes, and evaluation timing. Doctor applies field-specific exceptions for skill refs, interactive workflow inputs, and conditions; validates aliases when they cross consumer boundaries; preserves foreign tags and literal data; and reports incomplete coverage for dynamic discovery, unresolved imports, and unclassified stack fields. Structural component identity fields are checked before deferred value evaluation, while copied mock strings retain their literal meaning. Workflow discovery follows the configured directory, deduplicates files nested under stacks, and keeps GitHub Actions separate. Named scaffold manifests are checked with their own loader policy.

The function pages share one MDX availability component. Each page lists supported locations and evaluation timing, including the exact workflow prompt fields and step types. The plain-Markdown export expands the same component so exported documentation retains that information.

Custom-command condition examples and the scope matrix now distinguish quoted CEL strings in CLI configuration from native `!cel` tags supported in workflow/stack condition fields.

## Validation

Ran focused doctor, configuration, utility, and function-tag tests successfully. Dispatcher tests execute configuration loading, workflow interactive function resolution, scaffold tag handling, and vendor loading to compare observed results with catalog claims. Doctor tests cover path and node restrictions, native versus quoted syntax, literal and foreign tags, workflow discovery, aliases, incomplete coverage, and no function execution.

Executed a minimal custom command twice for each syntax: native `!cel` failed during configuration loading, while quoted `"!cel true"`/`"!cel false"` and plain CEL strings executed only the true branch. All six expectations passed. Run-specific fixtures and outputs remain in ignored workspace storage.

## Follow-ups

The broader skills field-test report tracks integration prerequisites and newly discovered behavior separately. Runtime function arguments and returned values remain outside doctor's static check.
