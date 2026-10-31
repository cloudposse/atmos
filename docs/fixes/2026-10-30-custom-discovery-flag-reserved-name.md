# Fix: Custom `--example` flags no longer break every command

**Date:** 2026-10-30

## Summary

A custom command can again declare a flag named `example` or `docs` of any type. Previously, a string, untyped, or integer `example` flag, or an integer `docs` flag, made nearly every Atmos command fail with `reserved flag name`.

## Context

The `--docs` and `--example` discovery flags were added as global flags. Custom commands are loaded on nearly every invocation, and the type check compared a custom flag against the new global flag. Only a boolean `--docs` was exempt, and a string `--docs` passed because it matches the global type. Flag registration already treats both names as owned by the custom command, so validation and registration disagreed. The regression test for the upcoming release found this: the same project works on v1.233.2.

## Changes

- `cmd/cmd_utils.go`: a custom `docs` or `example` flag of any type passes validation when the existing flag is the global discovery flag.
- `cmd/command_documentation_markdown_test.go`: extends `TestCustomDiscoveryFlagRegistration` with the failing types.

## Validation

- Extended the test first and confirmed `docs`/int and the `example` string, untyped, and int cases failed before the change.
- `TestCustomDiscoveryFlagRegistration` passes for every type, including a value passed to the custom command.
- End to end with real binaries against the `custom-command` fixture: `list stacks` and the custom command fail on the unfixed candidate and pass with this fix, for `--example=v` and `--example v`. `--example` on other commands still prints discovery output.
- Patch-scoped `custom-gcl` lint reports 0 issues.

## Follow-ups

None.
