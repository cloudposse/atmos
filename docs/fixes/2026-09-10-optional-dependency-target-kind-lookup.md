# Fix: Preserve Optional Dependencies for Available Typed Targets

**Date:** 2026-09-10

## Summary

`describe dependents` now evaluates an optional dependency target by its stack and component type. An unavailable Terraform component no longer hides an enabled Packer component with the same name.

## Context

The indexed and scan-based lookup paths used one availability result for the requested component name. Component-type precedence could select a disabled Terraform target even when the dependency explicitly referenced an enabled Packer target.

Unbounded `list dependencies` also attempted to parse an unresolved templated `required` value when template processing was disabled, rather than retaining the safe required default.

## Changes

- Resolve optional target availability from the dependency's stack and kind in both dependent-lookup paths.
- Defer unresolved `required` selectors before every list dependency graph parse.
- Add regressions for typed optional targets and unresolved required templates.

## Validation

- Targeted regression tests initially failed before the implementation and pass after it.
- `go build ./...` completed successfully before the field test.
- Live field tests confirmed bounded dependency listing avoids an unrelated template failure, forward optional edges are omitted, and unbounded listing surfaces the deliberate fixture failure.

## Follow-ups

- #3113 tracks direct `describe dependents` behavior for missing optional targets discovered during field testing.
