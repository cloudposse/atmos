# Fix: Extract changeset identifier preservation

**Date:** 2026-10-11

## Summary

Reduce function complexity through a behavior-preserving helper extraction.

## Context

Creating a changeset must retain its returned identifiers when polling fails or omits them. The creation function exceeded the updated cyclomatic complexity limit.

## Changes

Extracted the two identifier fallback checks into retainChangeSetIdentifiers without changing stack ownership, cancellation, or cleanup. The owning-layer creation function now meets the limit of ten.

## Validation

Using Go 1.26.9 at this owning layer:

- `go test -race ./pkg/component/aws/cloudformation -count=1`: passed (3.584s). The coverage-enabled run reported 97.6% package statement coverage.
- `go build ./pkg/component/aws/cloudformation/...`: passed.
- Custom golangci-lint with `--new-from-rev=HEAD ./pkg/component/aws/cloudformation/...`: zero issues.
- Formatting, `git diff --check`, and fix-record validation passed.

## Follow-ups

None.
