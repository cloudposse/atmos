# Fix: Helmfile version honors dry-run

**Date:** 2026-10-27

## Summary

`atmos helmfile version --dry-run` previews the command without starting the selected executable.

## Context

The version shortcut passed a hard-coded false value to the shell executor, bypassing the parsed dry-run flag. This also affected executables selected with `--command`.

## Changes

Pass the parsed dry-run setting through the version shortcut. Add a cross-platform CLI regression test using the Go test executable to record subprocess invocation, with normal execution and `--dry-run=false` controls.

## Validation

The regression failed twice before the fix: dry-run created the subprocess marker, while both execution controls passed. After the fix, focused race tests passed for Helmfile execution flags, version dry-run, executable overrides, invalid component selections, stack precedence, and fixture identity isolation.

## Follow-ups

None.
