# Fix: Clarify the optional stack filter for CloudFormation bulk commands

**Date:** 2026-10-09

## Summary

The logs, tree, and StackSet command pages now explain that `--stack` is required for a single
component but optional when selecting components in bulk.

## Context

Six pages marked `--stack` unconditionally required while showing `--affected` examples without
it. The examples match the implementation: bulk selection uses the stack as an optional filter
and can select components across stacks. Adding a stack to those examples would unnecessarily
narrow their scope.

## Changes

- Correct the stack flag descriptions for logs, tree, and StackSet create, delete, instances,
  and update.
- Explain that `--all`, `--affected`, `--tags`, and `--labels` allow an optional stack filter.
- Preserve the existing examples and command behavior.

## Validation

- Parsed each of the six commands with `--affected --base origin/main` and no `--stack`;
  Cobra required-flag and argument validation passed for every command without executing AWS operations.
- Existing CLI argument/affected-flag tests and component bulk/affected-selection tests passed.
- `git diff --check` passed.
- The website build is handled on the owning stack branch after the documentation fixes are combined.

## Follow-ups

None.
