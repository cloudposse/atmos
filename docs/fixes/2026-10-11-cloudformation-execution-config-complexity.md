# Fix: Isolate CloudFormation execution configuration setup

**Date:** 2026-10-11

## Summary

Reduce function complexity through a behavior-preserving helper extraction.

## Context

The single-component executor exceeded the updated complexity limit after introducing lazy authentication on a copied configuration.

## Changes

Extracted configForComponentExecution without moving authentication across stack evaluation or operation guards. Deferred authentication still references the copied configuration, preserving isolation from shared configuration.

## Validation

Using Go 1.26.9 at this owning layer:

- `go test -race ./pkg/component/aws/cloudformation -run '^TestExecuteSingle|^TestSourceAWSAuth' -count=1`: passed (3.444s). The coverage-enabled run covers the extracted helper completely.
- `go build ./pkg/component/aws/cloudformation/... ./cmd/aws/cloudformation/backend/...`: passed.
- Custom golangci-lint with `--new-from-rev=HEAD` for both affected package trees: zero issues.
- Validation included the backend argument-pointer fix and the inherited #3250 test-helper signature cleanup. The latter was temporarily overlaid locally for the run, then restored to let the stack cascade supply the owning-layer change. Without that inherited cleanup, the updated test callers cannot compile.
- Formatting, `git diff --check`, and fix-record validation passed.

## Follow-ups

None.
