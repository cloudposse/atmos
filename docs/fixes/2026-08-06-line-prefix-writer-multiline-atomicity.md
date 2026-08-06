# Fix: `LinePrefixWriter` now flushes multi-line writes as one atomic burst

**Date:** 2026-08-06

## Summary

`pkg/io/line_prefix_writer.go`'s `LinePrefixWriter` (used to prefix and serialize concurrent
Terraform node output, e.g. `[dev/app]`/`[dev/db]`, through a shared `writeMu`) acquired and
released that shared lock **per individual line** rather than per flush. When one upstream
`Write()` call resolved into multiple complete lines (e.g. a `\r`-separated progress update
followed later by its completion), releasing the lock between those lines let a concurrently
writing sibling node's entire output interleave in the gap — corrupting what should read as one
contiguous burst from a single node. Caught by CI: `TestExecuteTerraformConcurrentHooksUseNodeWriters`
(`pkg/scheduler/adapters/terraform_test.go`) failed on the macOS acceptance-test job.

## Context

CI failure log (`Acceptance Tests (macos)`, job 92499338324) showed exactly one real failure:
```
--- FAIL: TestExecuteTerraformConcurrentHooksUseNodeWriters (0.00s)
    Error: "[dev/app] hook progress\n[dev/db] hook progress\n[dev/db] hook complete\n[dev/app] hook complete\n"
      does not contain "[dev/app] hook progress\n[dev/app] hook complete\n"
```
The test forces two nodes (`dev/app`, `dev/db`) to each write `"hook progress\r"`, block until
both have done so, then release both simultaneously to write `"hook complete\n"` -- deliberately
maximizing the chance of interleaving. Tracing `LinePrefixWriter.Write` ->
`flushCompleteLinesLocked` -> `writeLine`: `writeLine` locked/unlocked the shared `writeMu` on
each call, and one `Write()` carrying `"hook progress\rhook complete\n"` resolves into two
separate lines (the `\r` and the trailing `\n` are two distinct line endings per `lineEndIndex`).
Between writing the first line (`"[dev/app] hook progress\n"`) and the second
(`"[dev/app] hook complete\n"`), the lock was fully released -- letting `dev/db`'s goroutine,
running the identical code path concurrently, complete its own two-line write entirely in that
gap.

## Changes

- `pkg/io/line_prefix_writer.go`: `Write` and `Flush` now acquire `writeMu` ONCE, before calling
  `flushCompleteLinesLocked` (and, in `Flush`, before the trailing partial-line write too), and
  hold it for every line that single call flushes. `writeLine` no longer locks `writeMu` itself --
  it now documents that callers must already hold it, since `sync.Mutex` is non-reentrant and the
  callers acquire it first.

## Validation

- `go test ./pkg/io/... ./pkg/scheduler/adapters/... -race -run
  "TestExecuteTerraformConcurrentHooksUseNodeWriters|LinePrefix" -count=10`: 46 sub-test runs, all
  pass, no deadlocks (confirms the lock-acquisition reordering didn't introduce one -- `w.mu` is
  per-writer/per-node, `writeMu` is the one shared lock, always acquired in that order, never the
  reverse).
- `go test ./pkg/io/... ./pkg/scheduler/...`: full package suites pass.
- This was the only real test failure in the attached CI log; the earlier "registry cache
  certificate is not trusted" line in the same log is expected output from a passing test
  confirming its own negative-path check, not a failure.

## Follow-ups

None.
