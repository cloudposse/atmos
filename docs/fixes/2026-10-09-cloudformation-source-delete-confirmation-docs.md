# Fix: Document CloudFormation source deletion confirmation

**Date:** 2026-10-09

## Summary

CloudFormation source deletion documentation now describes interactive confirmation and the optional
`--force` flag that skips it.

## Context

The command reference and source overview incorrectly claimed that deletion always required
`--force`. The shared implementation prompts when terminal input is available, preserves the
directory when confirmation is declined, and rejects non-interactive deletion without `--force`.
A missing directory produces a warning and succeeds before confirmation is attempted.

## Changes

- Mark `--force` optional in usage and flags, and explain that it skips confirmation.
- Replace the incorrect without-force error example with the interactive confirmation workflow.
- Correct the safety list and overview; document explicit component, stack, and force arguments
  for unattended use.

## Validation

- Shared source-delete command, flag parsing, directory deletion, and source-validation tests passed.
- Forced confirmation and non-TTY confirmation tests passed.
- Command-handler tests passed for non-TTY rejection and forced deletion.
- `git diff --check` passed.
- The website build is handled on the owning stack branch after the documentation fixes are combined.

## Follow-ups

None.
