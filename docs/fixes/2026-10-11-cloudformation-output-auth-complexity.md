# Fix: Share CloudFormation output authentication opt-out checks

**Date:** 2026-10-11

## Summary

Reduce function complexity through a behavior-preserving helper extraction.

## Context

CloudFormation output evaluation and identity resolution duplicated the configuration and caller authentication opt-out predicate. The evaluator exceeded the updated complexity limit.

## Changes

Extracted cloudFormationOutputAuthDisabled and reused it at both existing decision points. Identity resolution, caller contexts, and output evaluation ordering are unchanged.

## Validation

Using Go 1.26.9 at this owning layer:

- `go test -race ./internal/exec -run 'CloudFormation|Cloudformation' -count=1`: passed (32.347s). The coverage-enabled run covers the extracted predicate completely.
- `go build ./internal/exec/...`: passed.
- Custom golangci-lint with `--new-from-rev=HEAD ./internal/exec/...`: zero issues.
- Formatting, `git diff --check`, and fix-record validation passed.

## Follow-ups

None.
