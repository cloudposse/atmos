# Fix: Make the loaded Starlark dialect test portable to Windows

**Date:** 2026-10-04

## Summary

The loaded-module dialect regression now uses real temporary files and relative
module paths, so it exercises the same behavior on Windows and Unix.

## Context

PR #3261 acceptance run `37205472855`, Windows shard 3, failed
`TestDialectAppliesToLoadedModules`. Its mock filesystem used `/lib/loop.star` as a
literal map key. Windows path normalization produced a different key, and the
mock silently returned an empty module with no error. Loading the expected
`values` symbol consequently failed before the dialect assertion ran.

## Changes

- Write both test modules under `t.TempDir()` using `filepath.Join` and load them
  relative to the supplied working directory through the real file reader.
- Preserve assertions for top-level loops and single-assignment diagnostics in
  loaded modules. Assert that diagnostic hints exist before indexing them.
- In the CLI acceptance layer (#3261), accept either native path separator in
  the included-file traceback assertions. Windows CI returned the correct file
  and line numbers using backslashes; the old pattern required forward slashes.

## Validation

- `go test -race -count=5 ./pkg/script/starlark -run '^TestDialect' -timeout=5m`
  passed on macOS.
- `./custom-gcl run --new-from-rev=HEAD ./pkg/script/starlark` reported zero issues.
- Inspected the Windows failure log; a Windows execution of the corrected fixture
  remains part of the next CI run because no local Windows runtime is available.
- The included-file traceback CLI case passes locally with the portable patterns;
  its file name, line numbers, and error-message assertions remain intact.

## Follow-ups

None.
