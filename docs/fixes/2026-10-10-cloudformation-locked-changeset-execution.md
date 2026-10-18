# Fix: Respect component locks when executing named changesets

**Date:** 2026-10-10

## Summary

CloudFormation named changeset execution now rejects locked components before authentication or execution side effects.

## Context

The component lock guard covered apply and delete but omitted `changeset-execute`, which can update stack policy and deploy an existing changeset. Explicit approval and dry-run must not bypass component locks.

## Changes

- Classify named changeset execution as a stack mutation in the existing metadata guard.
- Exercise locked and unlocked execution with normal, dry-run, auto-approve, and combined flags through `executeSingle`.
- Preserve locked changeset create/list/delete, diff previews, and drift/get diagnostics, with authentication stubbed to prevent network access.

## Validation

Before the fix, the four locked named-execution cases failed: normal and auto-approve reached authentication, while dry-run and combined dry-run/auto-approve returned success.

With `GOTOOLCHAIN=go1.26.9`:

- `go test -race ./pkg/component/aws/cloudformation -count=1` passed all package tests and the 72-case metadata matrix. Package statement coverage was 97.7%; `executeSingle` and `validateComponentOperation` both reached 100%.
- `go build ./...` passed.
- Custom golangci-lint with `--new-from-rev=HEAD` reported zero issues for the CloudFormation package.
- Formatting, `git diff --check`, and the fix-record validator passed.

The production change adds a map entry, so it adds no executable coverage blocks. The regression matrix verifies the new classification through the execution boundary.

## Follow-ups

None.
