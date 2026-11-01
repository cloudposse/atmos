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
- Add portable coverage of authentication failure handling, default identities, Terraform RC composition, workspace failure boundaries, and init recovery with provider locking. Select these cases from the CI coverage profile.
- Document the auth-context wrapper's supported operations and output-stream forwarding. Align the PR description and fix records with the latest saved test run.

## Validation

- Reproduced the retry cancellation regression before the fix; all 10 retry tests pass afterward.
- The portable query describe-error regression and the full Terraform component package pass. The existing gomonkey setup-error test skips on macOS ARM64; the Linux and Windows acceptance runs passed.
- Reproduced the lifecycle counter race and lost update locally. The corrected test passes 1,000 repetitions with `-race`; the full lifecycle package passes 10 repetitions with CI's shuffle seed and parallelism.
- The new coverage tests also pass with CI's Go 1.26.10 toolchain. Against the downloaded CI profile, they cover 23 previously missed blocks containing 29 statements.
- Patch-scoped lint passes with zero issues.
- `go build ./...` passes.
- Full tests for `pkg/component/terraform`, `internal/exec`, `pkg/component`, and `cmd/terraform` pass: 8,604 passing test/subtest events, 51 skips, and zero failures. The run instruments the three implementation packages.
- Local rename-aware patch coverage is 1,694/1,801 added executable lines (94.06%). This scoped approximation does not replace CI Codecov.

## Follow-ups

The acceptance and race suites passed in [CI run 40594955477](https://github.com/cloudposse/atmos/actions/runs/40594955477). Its Codecov report measured 89.56% patch coverage, below the required 90%, despite the higher local approximation. The additional tests target branches missed by that CI run; their new Codecov result remains to be confirmed. The canceled release-milestone workflow passed on rerun.
