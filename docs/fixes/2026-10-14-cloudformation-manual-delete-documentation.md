# Fix: Explain manual change-set placeholder cleanup

**Date:** 2026-10-14

## Summary

Document how to remove an abandoned stack placeholder after deleting its last unexecuted CREATE change set.

## Context

`changeset delete` deliberately deletes only a named change set. Its command reference did not explain that an unexecuted stack placeholder remains and consumes stack quota.

## Changes

Explain the retained placeholder and link the explicit stack-delete command. Distinguish manual stack ownership from automatic cleanup of a placeholder created by the current preview.

## Validation

- Focused `TestRunChangesetDelete` and `TestStackExists` regression tests passed against the prepared implementation.
- Patch-scoped lint and the complete combined website build passed, including 40 prebuild tests.
- Verified the generated command page contains the new guidance and stack-delete link. Warning sources match the previous build.

## Follow-ups

None.
