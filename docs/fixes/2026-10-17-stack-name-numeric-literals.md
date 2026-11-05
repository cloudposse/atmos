# Fix: Preserve numeric literals in stack name templates

**Date:** 2026-10-17

## Summary

Stack name templates again accept numeric constants such as `{{ repeat 2.0 "dev" }}` for integer arguments, across every configuration edition.

## Context

The computed-identity guard wrapped function arguments in an `any`-typed function. This converted ideal numeric constants to concrete Go values before their destination function could interpret them. Existing templates started failing with `expected int; got float64`, which could also hide components from lookup.

## Changes

Leave numeric literal AST nodes unwrapped: they cannot contain computed Starlark source. Retain guards on evaluated values, string arguments, containers, and function results.

## Validation

- Five new cases failed before the fix, covering decimal/exponent notation and Sprig/Gomplate functions; the integer control passed.
- All six new cases now match ordinary template execution.
- Existing computed-identity rejection, lazy evaluation, and Spacelift identity tests pass with `go test ./internal/exec -run 'Test(StackNameTemplates|EnsureLiteralStackIdentity|ResolveSpaceliftContextPrefixRejects)' -count=1`.

## Follow-ups

None.
