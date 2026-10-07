# Fix: Keep unmerged implementation work in its PR stack

**Date:** 2026-10-07

## Summary

Prohibit creating public GitHub issues for implementations that have not merged into `main`.

## Context

The repository's follow-up tracking rule required issues for deferred work without distinguishing existing features from unfinished implementations. The fix-log skill repeated that instruction.

## Changes

- Require fixes for unmerged implementations in the active PR or stack, with remaining tasks in the PRD or task list.
- Apply the same rule to fix-log records.
- Delete the eight public issues created for the unmerged Starlark CI module and preserve their work items in its PRD without issue links.

## Validation

- GitHub confirmed deletion of all eight issues.

## Follow-ups

The CI module's remaining implementation work is tracked in [its PRD](../prd/native-ci/framework/script-ci-module.md#remaining-implementation-work).
