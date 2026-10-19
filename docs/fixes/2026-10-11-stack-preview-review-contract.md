# Fix: Stack ancestry and CloudFormation preview cleanup guidance

**Date:** 2026-10-11

## Summary

Refresh remote refs before checking stack ancestry, and clarify the CloudFormation
PRD's handling of unchanged previews and cleanup failures.

## Context

CodeRabbit identified three documentation gaps in the stack's foundational PR.
An ancestry check could read stale remote-tracking refs. The PRD did not define
CloudFormation's no-changes `FAILED` status and incorrectly described leftover
changesets as harmless despite their quota cost.

## Changes

- Require `git fetch origin` immediately before the remote ancestry check.
- Treat recognized no-changes failures as successful empty previews after cleanup;
  preserve errors for other failure reasons.
- Require bounded cleanup, retries for transient deletion errors, and a command
  error identifying any changeset that still needs deletion or reconciliation.
- Explain that unique names do not prevent changeset quota exhaustion.

## Validation

- `git diff --check` passed.
- The fix-log format validator and skill frontmatter, size, line-count, and
  fenced-code language checks passed.
- Pre-commit passed for all three changed Markdown files.
- Inspected the later stack's `isNoOpChangeSet`, `runDiff`, and
  `discardChangeSet` paths and their existing no-op and cleanup-error regressions.
  This fix changes documentation only.

## Follow-ups

None.
