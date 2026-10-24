# Fix: `MergeContext.WithFile` no longer shares its `ImportChain` backing array across concurrent sibling contexts

**Date:** 2026-07-28

## Summary

`pkg/merge.(*MergeContext).WithFile` built its `ImportChain` with `append(mc.ImportChain, filePath)`. Stack
processing calls `WithFile` from one goroutine per imported file, and every sibling goroutine shares the same
parent `*MergeContext` (`internal/exec/stack_processor_utils.go`), so once the parent's `ImportChain` had
spare capacity, two siblings' concurrent appends could write their different `filePath` values into the same
backing-array slot at the same time — a real data race, caught by `go test -race`, that also silently
corrupted which file a sibling's context reported as its own. `WithFile` now always allocates a fresh backing
array, so sibling contexts never share memory.

## Context

This surfaced as a `-race` failure in `TestExecuteHelmfile_ComponentNotFound` while validating an unrelated
change (routing yq's diagnostics through the Atmos logger, see
`2026-07-28-yq-concurrent-evaluation-race.md`) — that test exercises `ExecuteHelmfile` → stack processing →
concurrent import handling, not `pkg/merge` directly, so the race was pre-existing and unrelated to that
work.

`processYAMLConfigFileWithContextInternal` spawns one goroutine per import match, passing the *same* parent
`mergeContext` pointer to every goroutine (`internal/exec/stack_processor_utils.go:1596-1634`). Each goroutine
calls `mergeContext.WithFile(relativeFilePath)` (`stack_processor_utils.go:1100`). `WithFile` read
`mc.ImportChain` (a slice header: pointer, length, capacity) and appended one element without copying first.
Go's slice growth strategy commonly leaves spare capacity after several small appends (empirically: length 3
gets capacity 4, length 5 gets capacity 8), and any multi-level import chain reaches that state before
forking into parallel sibling imports. When it does, `append(mc.ImportChain, filePath)` writes into the
shared backing array at the same index for every sibling, and two siblings writing their own (different)
`filePath` at the same address at the same time is a genuine data race — confirmed by reverting the fix and
observing both the race detector and the underlying symptom: siblings reporting each other's file names in
their own `ImportChain`.

## Changes

- `pkg/merge/merge_context.go`: `WithFile` now builds `ImportChain` with
  `append(append([]string{}, mc.ImportChain...), filePath)` instead of `append(mc.ImportChain, filePath)`.
  Starting from a zero-capacity literal forces a fresh allocation on every call (the same idiom `Clone()`
  already used), so sibling contexts built from the same parent never share backing-array memory.
- `pkg/merge/merge_context_test.go`: added
  `TestMergeContext_WithFile_ConcurrentSiblingsAreRaceFree`, which builds a parent through three sequential
  `WithFile` calls (to land on a length/capacity pair with spare capacity, matching a real multi-level import
  chain), forks 32 goroutines that all call `WithFile` on that same parent concurrently, and asserts every
  sibling's resulting `CurrentFile`/`ImportChain` reflects only its own file, plus that the parent's own chain
  is untouched.

## Validation

- Reverted the fix and confirmed `TestMergeContext_WithFile_ConcurrentSiblingsAreRaceFree` fails under
  `go test -race` — both the race detector and corrupted `ImportChain` contents (siblings observed each
  other's file names) — then reapplied the fix and confirmed it passes.
- `go build ./...` and `go vet ./...` — clean. `gofumpt -l` on both changed files — no output.
- `go test -race -run 'TestMergeContext' ./pkg/merge/...` — all pass, including 5 repeated runs
  (`-count=5`) of the new test.
- `go test -race -run 'TestExecuteHelmfile_ComponentNotFound' ./internal/exec/... -count=3` — the test that
  originally surfaced this race now passes cleanly across 3 runs.
- `go test -race ./pkg/merge/... ./pkg/utils/... ./pkg/yaml/... ./internal/yq/...` — all pass.
- Not yet run: `atmos lint --changed` / `./custom-gcl run --new-from-rev=origin/main` (no prebuilt
  `custom-gcl` binary available in this shell), the full `atmos test --full` suite, and a full
  `go test -race ./internal/exec/...` run (the last full run took ~9 minutes and had already been captured
  failing before this fix; a full re-run to confirm no other unrelated failures remain is still outstanding).

## Follow-ups

None. The new regression test directly covers the concurrent-sibling scenario that caused this race.
