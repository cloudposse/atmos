# Fix: Automation documentation and skill corrections from the field test

**Date:** 2026-10-07

## Summary

A manual field test of the Starlark automation features found documentation and agent-skill text that contradicted the shipped or intended behavior. Correct those pages, and make `atmos stack get --file` fail loudly when the named manifest cannot be read instead of silently reporting the merged value.

## Context

The field test ran each documented example against a real build. Several pages claimed features were unavailable that exist (`fs.glob`, `steps.input`, `steps.run`), omitted behavior users hit (`ctx.args` in Git hooks, `--skip=starlark`, closed `metadata` schema, ambiguous short tool names, `!literal` for templates inside `script:`), or described flags and errors inaccurately (`--file` help, `retry.delay`, standalone global flag forms, `validate` exit codes). Where another work package changes the code, the text describes the intended behavior.

## Changes

- Update the `ctx.args`, `ctx`, `exec.run`, `component.exec`, `steps.run`, `atmos.toolchain`, `dependencies.tools`, `cli.command`, and `cli.arg` function pages.
- Correct the `!starlark` page: valid `metadata` example, `--skip=starlark`, quoting rules for single-line bodies, stack identity fields, and context details.
- Correct the script step page, the step `output` and `retry` pages, and the execution model page (`None` prints nothing).
- Update the Git hook command pages and the Git configuration page (repository-root execution, uninstall scope, install validation, `type: env` semantics).
- Document `--file` on `atmos stack get` and `atmos stack config get`, usage rendering of custom command arguments, unknown flag types, and the standalone global-flag rules.
- Correct the `atmos-starlark` agent skill and its references.
- In `cmd/stack/operations.go`, change the shared `--file` help text to "Read or edit" and return a wrapped `ErrFileNotFound` error when `--file` names a manifest that cannot be read during a read-only get.

## Validation

- `go test ./cmd/stack/ -count=1` passes, including the new `TestRunStackGet_ExplicitFile_Unreadable`.
- `cd website && npm run build` completes without broken links.

## Follow-ups

None.
