# Fix: `LinePrefixWriter` held its shared output lock per line instead of per flush

**Date:** 2026-08-06

## Summary

`pkg/io/line_prefix_writer.go`'s `writeLine` acquired and released the shared `writeMu` once per
line instead of once per flush. When a single `Write()` call produced multiple lines (e.g. a
hook's buffered `"progress\r" + "complete\n"` update), the lock was released between those lines,
letting a concurrently running node's writer interleave its own line in between and corrupt the
expected per-node contiguous output block. `Write` and `Flush` now hold `writeMu` for their whole
flush operation, and `writeLine` is a lock-free helper that callers must call while holding it.

## Context

The macOS "Acceptance Tests" CI job (job ID 92489708710) failed with:

```
--- FAIL: TestExecuteTerraformConcurrentHooksUseNodeWriters (0.00s)
    terraform_test.go:510:
        Error: "[dev/app] hook progress\n[dev/db] hook progress\n[dev/db] hook complete\n[dev/app] hook complete\n" does not contain "[dev/app] hook progress\n[dev/app] hook complete\n"
```

Both `pkg/io/line_prefix_writer.go` and this test landed together in a prior PR (#2860, "render
concurrent carriage-return updates safely"), which correctly converts `\r` progress updates into
discrete prefixed lines but left a lock-granularity gap that let two concurrent nodes' lines
interleave mid-block. Locally the test only failed intermittently (timing-dependent), which is
why it passed in earlier local runs before reproducing it under `-race -count=N`.

## Changes

- `pkg/io/line_prefix_writer.go`: `Write()` and `Flush()` now acquire `w.writeMu` once, covering
  the entire call to `flushCompleteLinesLocked()` (and, in `Flush()`, the trailing partial-line
  write too), instead of `writeLine()` acquiring/releasing `writeMu` on every individual line.
  `writeLine()` no longer touches `writeMu` itself; its doc comment now states callers must hold
  it. Lock ordering is unchanged (per-writer `w.mu` outer, shared `writeMu` inner), so this
  doesn't introduce new deadlock risk.

## Validation

- Reproduced the race before the fix: `go test ./pkg/scheduler/adapters/... -run
  TestExecuteTerraformConcurrentHooksUseNodeWriters -race -count=200` failed intermittently
  (multiple failures across 200 iterations, each showing the same interleaved-block pattern).
- After the fix, the same command passed 200/200 under `-race`.
- Full suites for touched/adjacent packages passed under `-race -count=1`: `pkg/io`,
  `pkg/scheduler`, `pkg/scheduler/adapters`, `pkg/component/container`, `pkg/workflow`.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
- `gofmt -l pkg/io/line_prefix_writer.go` — no output (already formatted).

## Follow-ups

None.
