# Fix: Terraform component CI failures and retry cancellation

**Date:** 2026-10-27

## Summary

Correct the Terraform query test's executor binding and the stack lifecycle test's concurrent counter. Use the caller's context for subprocess retry waits.

## Context

Linux and Windows acceptance shard 6 failed because the query setup-error test patched its parent executor but called separate executors from its subtests. The race suite also found concurrent writes to a stack lifecycle test counter. Review found that retry backoff used a background context and could delay cancellation.

## Changes

- Reuse the patched executor in the query setup-error subtests. Cover describe-error propagation with a portable test that also runs on macOS ARM64.
- Synchronize the lifecycle test's invocation counter. Preserve the assertions that grouped output failures prevent execution and that ungrouped execution runs all three components.
- Pass the caller's process context to retry backoff. Check that cancellation returns the context error and prevents another attempt.

## Validation

- Reproduced the retry cancellation regression before the fix; all 10 retry tests pass afterward.
- The portable query describe-error regression and the full Terraform component package pass. The existing gomonkey setup-error test skips on macOS ARM64; Linux and Windows execution remains for CI.
- Reproduced the lifecycle counter race and lost update locally. The corrected test passes 1,000 repetitions with `-race`; the full lifecycle package passes 10 repetitions with CI's shuffle seed and parallelism.
- Patch-scoped lint passes with zero issues.
- `go build ./...` passes.
- Full tests for `pkg/component/terraform`, `internal/exec`, `pkg/component`, and `cmd/terraform` pass: 8,578 passing test/subtest events, 51 skips, and zero failures. The run instruments the three implementation packages.
- Local rename-aware patch coverage is 1,659/1,801 added executable lines (92.12%). This scoped approximation does not replace CI Codecov.

## Follow-ups

The new GitHub Actions run must confirm Linux and Windows acceptance results and the race suite. Local package coverage is scoped; the full CI Codecov upload remains authoritative.
