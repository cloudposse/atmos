# Fix: reset `proexec` pending-data global before a test that asserts it's empty

**Date:** 2026-09-04

## Summary

`TestExecuteListInstancesCmd_NoUploadNoProGate_NoPendingData` failed under the
`[race] non-acceptance test suite` job with `Data` unexpectedly containing
`{"instances":[{"component":"vpc",...}],"version":1}` instead of the expected
`nil`. Root cause: `proexec.pendingAsyncData` is package-level global state,
read-and-cleared only inside `CaptureAsync`. `TestUploadInstancesWithDeps_Success`
(and its siblings that exercise `uploadInstancesWithDeps`'s success path) set it
as a side effect via `SetPendingAsyncData` but never call `CaptureAsync`
afterward -- correct for what those tests are actually checking, but it leaves
the global dangling for whichever test in the same binary calls `CaptureAsync`
next. Reset it explicitly at the start of the affected test rather than relying
on execution order.

## Context

`pkg/list/list_instances.go`'s `uploadInstancesWithDeps` calls
`proexec.SetPendingAsyncData(...)` unconditionally after a successful upload
(so the instance list rides along on the command's own exec-metadata record,
per research.md Decision 23 -- see the comment at that call site). In
production this is safe: `cmd/root.go`'s post-run hook calls `CaptureAsync`
exactly once per process, immediately consuming and clearing it. In a unit
test that calls `uploadInstancesWithDeps` directly, no such hook runs, so the
global is left set after the test function returns -- exactly the failure
mode `proexec/async.go`'s own doc comments warn about ("must be called only
immediately before the command's own CaptureAsync invocation -- never left
set across invocations").
`TestExecuteListInstancesCmd_NoUploadNoProGate_NoPendingData` calls
`proexec.CaptureAsync` directly to check whether `ExecuteListInstancesCmd`
itself left anything pending; if a sibling test's leftover data is still set
when this one runs, it reads that stale value instead of a clean `nil`.

## Changes

- `pkg/list/list_instances_coverage_test.go`: reset
  `proexec.SetPendingAsyncData(nil)` at the start of
  `TestExecuteListInstancesCmd_NoUploadNoProGate_NoPendingData`, before calling
  `ExecuteListInstancesCmd`, so the test's own assertion doesn't depend on
  what ran before it in the same test binary.

## Validation

- `go test ./pkg/list/... -race -run 'TestExecuteListInstancesCmd_NoUploadNoProGate_NoPendingData|TestUploadInstancesWithDeps' -v -count=5` -- all pass.
- `go test ./pkg/list/... -race -count=3` (full package) -- all pass.
- `atmos lint --changed` -- 0 issues.

## Follow-ups

None. The other `TestUploadInstancesWithDeps_*` tests that also set this
global without consuming it are working as designed for what they test
(`uploadInstancesWithDeps` in isolation) -- the fix belongs at the point that
actually asserts on the global's state, not at every setter.
