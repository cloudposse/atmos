# Fix: Honor Helmfile execution flags

**Date:** 2026-10-22

## Summary

Helmfile dry runs no longer execute the component command. Explicit stderr redirection writes the subprocess error stream to the selected destination. Global options reach Helmfile before its subcommand.

## Context

Helmfile allowed unknown flags but never registered `--dry-run`, so Cobra discarded it before execution. The globally registered `--redirect-stderr` flag was parsed but its value was lost when constructing the legacy execution context. Live wrapper commands reproduced both problems twice. The same missing registration discarded `--global-options`, also reproduced twice by inspecting subprocess arguments.

## Changes

Register Helmfile's dry-run and global-options flags through the standard parser and preserve their parsed values. Preserve an explicitly parsed stderr destination when building the command execution context, alongside the existing raw-argument path.

## Validation

An actual CLI/subprocess regression fails before both fixes: dry-run creates an execution marker, and redirection produces no file. After the fixes, dry-run leaves the marker absent and ordinary execution writes exactly the subprocess stderr message to the selected file. The focused command override, schema-directory, and Helmfile flag regressions pass together. The subprocess argument regression asserts that a quoted global-options value becomes separate options before `apply`; native flags after `--` remain a successful control.

## Follow-ups

None.
