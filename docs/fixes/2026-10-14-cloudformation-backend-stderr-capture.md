# Fix: Restore CloudFormation backend test stderr after callback failures

**Date:** 2026-10-14

## Summary

Make backend dry-run stderr capture restore global state and close its file when the callback panics or calls `FailNow`, and allow large writes without blocking.

## Context

CodeRabbit identified that the helper restored stderr only after the callback returned and drained its pipe only after writing finished. A stopped callback leaked the global stream and file descriptors; output exceeding pipe capacity could deadlock.

## Changes

- Capture into a temporary file and defer stderr restoration and file closure before invoking the callback.
- Preserve the existing ANSI stripping and whitespace normalization of captured output.
- Add regressions for panic cleanup, actual `FailNow` cleanup in a subprocess, and output exceeding pipe capacity. Verify the capture file closes on each path.
- Correct the malformed `--labels` validation comment to name `validateFlagSelection`.

## Validation

- The panic and `FailNow` regressions failed against the original helper, demonstrating the missing restoration and closure.
- Patch-scoped golangci-lint passed with zero issues.
- `GOTOOLCHAIN=go1.26.9 go test -race ./cmd/aws/cloudformation/backend ./cmd/aws/cloudformation -count=1` passed.
- `GOTOOLCHAIN=go1.26.9 go build ./cmd/aws/cloudformation/...` passed.

## Follow-ups

None.
