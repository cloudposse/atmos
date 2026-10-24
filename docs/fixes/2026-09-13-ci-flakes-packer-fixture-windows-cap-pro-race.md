# Fix: three recurring CI flakes (packer fixture race, Windows subprocess cap, pro lock/unlock UI race)

**Date:** 2026-09-13

## Summary

Three unrelated, recurring CI flakes observed on 2026-09-12/13:

1. `cmd`'s packer command tests raced on the shared tracked fixture directory
  (`tests/fixtures/scenarios/packer`) under the race job's `-shuffle=on -parallel=4`, surfacing as
  `--- FAIL: TestPackerInitCmd` / `TestPackerInspectCmd` with `Failed to open file
  nonprod-aws-bastion.packer.vars.json`.
2. `internal/ci/acceptance`'s Windows-only subprocess concurrency cap (introduced in
  `docs/fixes/2026-09-11-windows-acceptance-subprocess-race.md`) was still too high: the same
  Windows-only GC/allocator crash ("fatal error: found pointer to free object" / "marked free object in
  span") recurred four more times on 2026-09-12, Windows shard 3, across PRs #3107 (run 34699763477) and
  #3122 (runs 34703060789 and 34726124320, twice).
3. `internal/exec`'s `TestExecuteProLock` and `TestExecuteProUnlock` (and their subtests) raced on the
  process-global UI writer under `-race`, on the race job, shard 3/4, run 34726125072.

## Root cause per item

### 1. Packer fixture race

`cmd/packer_validate_test.go` already worked around this by copying the fixture into a private
`t.TempDir()` before running real `atmos packer` commands against it — the comment there explains that
Windows acceptance shard 1 runs this test binary concurrently with `internal/exec`'s
`TestExecutePacker_Validate`, and both processes generate/clean up the same
`nonprod-aws-bastion.packer.vars.json` var-file in the tracked fixture directory. The other packer cmd
tests (`packer_init_test.go`, `packer_inspect_test.go`, `packer_build_test.go`, `packer_output_test.go`,
`packer_path_resolution_test.go`) still pointed `ATMOS_CLI_CONFIG_PATH`/`ATMOS_BASE_PATH` directly at the
tracked fixture directory, so under the race job's own `-shuffle=on -parallel=4` (multiple packer cmd
tests running concurrently in the same binary, not just across binaries), they raced on the same
generated var-file for the same reason.

### 2. Windows subprocess cap still too low

The cap of 4 concurrent subprocess launches (set in commit `72319350ae`) reduced but didn't eliminate the
crash: it recurred four times in one day across two different PRs, always on Windows shard 3, always the
same `mcache`/`mgcsweep` "found pointer to free object" signature. Four real `go build`/`go test`/`*.test.exe`
subprocesses launching at once was still enough concurrent allocation+syscall pressure to trip the
Windows-only Go runtime race (golang/go#44900, #45364, #47415, #54247).

### 3. `pro.go` lock/unlock UI race

`executeProLock`/`executeProUnlock` in `internal/exec/pro.go` write success messages through the
process-global UI writer (`ui.Writeln`/`ui.Successf` → `pkg/terminal` → `pkg/io`'s shared context
buffer). `TestExecuteProLock` and `TestExecuteProUnlock` (and every subtest) called `t.Parallel()`, so two
goroutines could call `ui.Writeln`/`ui.Successf` concurrently and race on that shared buffer. `pkg/io` and
`pkg/ui` do have a `Reset()` function, but its own doc comment states the caller must ensure no concurrent
I/O is in progress when calling it — it's a full-reset seam for sequential tests that swap `os.Stdout`
(used that way once, non-parallel, in `internal/exec/terraform_execute_helpers_test.go`), not a per-test
isolation mechanism safe to use from `t.Parallel()` subtests. No other `SetGlobal`/`WithTestContext`/
`NewTestContext`/`SetIOContext`-style per-test isolation seam exists in `pkg/io` or `pkg/ui`.

## Fix

### 1. Packer fixture race

Added `cmd/packer_fixture_test.go` with a `packerFixtureWorkDir(t *testing.T) string` helper that copies
`../tests/fixtures/scenarios/packer` into a private `filepath.Join(t.TempDir(), "packer")` directory (same
`os.CopyFS` approach `packer_validate_test.go` already used) and returns its path. Switched every packer
cmd test that executes a real `atmos packer ...` command against the fixture to call this helper instead of
using the tracked fixture path directly:

- `cmd/packer_validate_test.go` — pointed its existing private-copy logic at the new shared helper (kept
  the explanatory comment, updated to reference the helper).
- `cmd/packer_init_test.go`, `cmd/packer_inspect_test.go`, `cmd/packer_output_test.go` — one call site each.
- `cmd/packer_build_test.go` — four call sites (`TestPackerBuildCmd`, `TestPackerBuildCmdInvalidComponent`,
  `TestPackerBuildCmdMissingStack`, `TestPackerBuildCmdWithDirectoryTemplate`).
- `cmd/packer_path_resolution_test.go` — both tests (`TestPackerPathResolution`,
  `TestPackerPathResolutionWithCurrentDir`) run real `atmos packer validate` commands, so switched both;
  removed the now-redundant `os.Stat`-based skip (the tracked fixture always exists, and
  `packerFixtureWorkDir`'s `require.NoError` on the copy already fails loudly if it doesn't), which let the
  `os` import be dropped.

Left `cmd/packer_version_test.go` unchanged: `atmos packer version` never touches
`ATMOS_CLI_CONFIG_PATH`/the fixture directory at all, so there's nothing to race on.

### 2. Windows subprocess cap

Lowered `maxConcurrentSubprocesses` from 4 to 2 in
`internal/ci/acceptance/subprocess_cap_windows.go`, and extended the constant's doc comment with this
round's evidence (the four 2026-09-12 recurrences, their PR/run numbers) so the next person who considers
raising it again has the history. `subprocess_cap_other.go` (non-Windows no-op) is untouched.

### 3. `pro.go` lock/unlock UI race

No existing per-test UI/IO isolation helper fit the `t.Parallel()` case (see Root cause above), so per the
task's least-invasive-correct-option guidance: removed `t.Parallel()` from `TestExecuteProLock` and
`TestExecuteProUnlock` and every one of their subtests (6 call sites total) in `internal/exec/pro_test.go`,
with a comment on each top-level test explaining they share the global UI writer and citing the race-job
run that caught it. No other tests in the file were touched — the rest of `pro_test.go`'s `t.Parallel()`
tests (`TestShouldUploadStatus`, `TestUploadStatus`, etc.) don't call `executeProLock`/`executeProUnlock`
and don't write through the UI layer, so they're unaffected.

## Verification

- `go build ./...` — clean.
- `go vet ./cmd/... ./internal/ci/acceptance/... ./internal/exec/...` — clean.
- `gofumpt -l` on every changed file — no output.
- `go test ./cmd/... -run 'TestPacker' -count=2 -shuffle=on -parallel=4 -v` — all packer tests `SKIP`
  (`packer not installed: exec: "packer": executable file not found in $PATH` — packer is not on PATH in
  this sandbox, so the fixture-copy code path itself wasn't exercised end-to-end here, but the test file
  compiles and runs cleanly under shuffle/parallel with no failures).
- `GOOS=windows go build ./internal/ci/acceptance/...` — clean (cross-compile check only; this sandbox
  can't execute a Windows binary).
- `go test ./internal/ci/acceptance/... -count=1` — `ok`, 10.5s.
- `go test ./internal/exec/ -run 'TestExecutePro(Lock|Unlock)' -race -count=3 -shuffle=on -parallel=4 -v` —
  all three iterations `PASS`, no `DATA RACE` reported.
- `go test ./cmd/... -count=1` — all packages `ok` except a pre-existing, unrelated failure in
  `cmd/toolchain` (`TestAddCommand_RunE_DefaultFlagIgnoredForMultipleTools`) caused by this sandbox having
  no network access to `raw.githubusercontent.com`'s aqua registry; not touched by this change and not one
  of the three flakes in scope.
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml --allow-serial-runners
  --new-from-rev=origin/main ./cmd/... ./internal/ci/acceptance/... ./internal/exec/...` — `0 issues.`

## Files

- `cmd/packer_fixture_test.go` (new)
- `cmd/packer_validate_test.go`
- `cmd/packer_init_test.go`
- `cmd/packer_inspect_test.go`
- `cmd/packer_output_test.go`
- `cmd/packer_build_test.go`
- `cmd/packer_path_resolution_test.go`
- `internal/ci/acceptance/subprocess_cap_windows.go`
- `internal/exec/pro_test.go`

## Follow-ups

None.
