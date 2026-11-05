# Fix: Permission receipt assertions with CI color output

**Date:** 2026-10-09

## Summary

Normalize ANSI styling before checking permission prompt text in terminal tests.

## Context

Linux acceptance shard 10 failed the `always_allow` and `always_deny` cases of
`TestCLIPrompterTerminalCachedChoices`. Both decisions were correctly persisted,
but CI enables colored output, which inserts ANSI sequences into the saved-receipt
text. The raw substring assertion failed despite the correct visible receipt.

## Changes

Strip ANSI sequences from captured cached and uncached prompt output before text
assertions, consistent with the existing receipt rendering tests. Preserve the
decision, persistence, scope, and receipt assertions. Include captured text in the
saved-receipt assertion's failure message. No production behavior changes.

## Validation

- Reproduced both original failures locally with `CI=true GITHUB_ACTIONS=true
  NO_COLOR= CLICOLOR=1 TERM=xterm-256color`.
- Full permission package passes with those color settings and
  `-count=1 -covermode=atomic -parallel=2 -timeout=2m`.
- Full permission package also passes with `NO_COLOR=1` and `-count=1`.
- `./custom-gcl run ./pkg/ai/tools/permission/...`: zero issues.

Local validation ran on macOS. The remote Linux acceptance job remains the
authoritative check of its runner environment.

## Follow-ups

None.
