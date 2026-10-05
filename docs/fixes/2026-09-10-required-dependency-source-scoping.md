# Fix: Preserve Required Dependency Sources in Scoped Reverse Lookups

**Date:** 2026-09-10

## Summary

`atmos describe dependents` evaluates sources for unavailable required targets only when the target identity matches the selected component's type.

## Context

Reverse scoped evaluation omitted modern required dependency sources when their unavailable targets did not create structural graph edges. Its initial name-and-stack matching could also evaluate a source that targeted a different component type with the same name.

## Changes

Direct dependent lookup now asks scoped reverse evaluation to include sources with required modern dependencies on selected root component, stack, and type identities. Root targets are derived after component, tag, and label selection. Required-source discovery honors configured template delimiters, and Phase C evaluation failures include the affected stack and component list. A `terraform` component no longer causes lookup to evaluate a source targeting an unavailable same-named `packer` component.

## Validation

- Focused scoped reverse-resolution regression.
- Focused tag and label reverse-selection regression.
- Focused resolved-stack dependent lookup regression.
- Focused type-sensitive required-source and custom-delimiter regressions.
- Focused Phase C evaluation error-context regression.
- Focused public dependent lookup regression for an unavailable same-named target of a different type.

## Follow-ups

None.
