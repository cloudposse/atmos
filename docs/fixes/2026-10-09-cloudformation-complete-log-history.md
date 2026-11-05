# Fix: Include every page of CloudFormation log history

**Date:** 2026-10-09

## Summary

One-shot CloudFormation logs and charts include every page of events for the root and nested stacks.

## Context

Logs reused the single-page watch poller, silently omitting older events and undercounting `event_count`.
The poller also fetched stack status even though a history listing does not need that status.

## Changes

- Add a dedicated event-history helper that follows pagination tokens, including through empty pages,
  and stops at a nil or empty terminal token.
- Preserve oldest-first per-stack order before the existing stable chronological merge.
- Suppress repeated event IDs across pages while retaining distinct events without IDs.
- Return API errors with their identities intact instead of rendering incomplete history.
- Use the helper only for one-shot logs; watch polling and later follow behavior remain unchanged.
- Update log/chart tests for pagination and remove unnecessary stack-status mock calls.

## Validation

- Multi-page merged-log and chart regressions failed before the fix.
- Targeted log, event-history, watch, and event-poll tests passed with the race detector.
- Pagination tests cover overlapping events, events without IDs, empty intermediate pages, both terminal
  token forms, and errors on the first or later page with `errors.Is`/`errors.As` identity checks.
- `go build ./...` passed.
- Patch-scoped custom golangci-lint for the CloudFormation subtree: 0 issues.
- Fix-log validation and `git diff --check` passed.
- Website build deferred to consolidated stack validation.

## Follow-ups

None.
