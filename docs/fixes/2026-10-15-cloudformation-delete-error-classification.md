# Fix: Preserve unrelated termination-protection restoration errors

**Date:** 2026-10-15

## Summary

Only typed AWS `ValidationError` responses describing a stack already being deleted suppress a termination-protection restoration error.

## Context

PR #3157 matched delete-state text anywhere in an error. An access denial, transport failure, or wrapper mentioning `DELETE_IN_PROGRESS` or `DELETE_COMPLETE` could therefore hide a failed attempt to restore protection after a failed deletion.

## Changes

- Unwrap `smithy.APIError` with `errors.As`, require `ValidationError`, and inspect only the API message.
- Preserve unrelated restoration errors and their identity.
- Use a typed AWS error in the existing deletion-in-progress test.
- Add caller regressions for wrapped validation errors, both deletion states, access denial, untyped errors, and misleading wrapper text.

## Validation

- Three regression cases failed before the fix because the original errors were lost.
- Go 1.26.9: `go test -race ./pkg/component/aws/cloudformation/...` passed.
- Go 1.26.9: `go build ./...` passed.
- Applicable pre-commit checks passed, including custom golangci-lint.

## Follow-ups

Verify CI and CodeRabbit on the published stack heads. This record lives in the following documentation PR so the core PR remains within CodeRabbit's reviewable-file limit.
