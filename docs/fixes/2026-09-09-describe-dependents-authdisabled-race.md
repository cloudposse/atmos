# Fix: `TestDescribeDependentsExec_Execute_ForwardsAuthDisabled` data race

**Date:** 2026-09-09

## Summary

The `[race] non-acceptance test suite` CI job failed on `TestDescribeDependentsExec_Execute_ForwardsAuthDisabled`
with a `WARNING: DATA RACE` in `pkg/io.(*context).Write()`, cascading into ~150 unrelated `--- FAIL` lines
once the race detector aborted the test binary. Removed the test's unnecessary `t.Parallel()` calls.

## Context

The test's two subtests (`AuthDisabled=true propagates` / `AuthDisabled=false propagates`) both called
`t.Parallel()` and both exercise `describeDependentsExec.Execute()`, which — via `viewWithScroll()` →
`printOrWriteToFile()` → `utils.PrintAsJSONSimple()` → `data.Writeln()` — writes JSON output through
`pkg/data`'s package-level `globalIOContext` singleton (set once per test binary by `data.InitWriter()` in
`internal/exec/testmain_test.go`'s `TestMain`). That singleton's underlying writer is not safe for concurrent
use, so running the two subtests in parallel raced on it under `go test -race`.

The sibling tests exercising the same `Execute()` path in `internal/exec/describe_dependents_test.go`
(`TestDescribeDependentsExec_Execute_Success_NoQuery` and others) do not use `t.Parallel()`, for the same
reason — this test diverged from that established (if unstated) convention when it was added for PR #2471.

## Changes

- `internal/exec/describe_dependents_authdisabled_test.go`: removed `t.Parallel()` from the parent test and
  its two subtests; added a comment explaining why, pointing at the shared global writer and the sibling
  tests' precedent.

## Validation

- `go test ./internal/exec/... -run TestDescribeDependentsExec_Execute_ForwardsAuthDisabled -race -v -count=5`:
  passes cleanly, 5/5 runs, no data race reported.
- `go test ./internal/exec/... -race -short` (two full-package runs, one with `-timeout=25m`): no `--- FAIL`
  entries and no `WARNING: DATA RACE` in either run. Both runs eventually hit a `go test` timeout on an
  unrelated slow real-subprocess test in this local sandbox (not present in the CI failure log, and not
  something this change touches) — not a regression from this fix.
- `go build ./...`: clean.

## Follow-ups

None.
