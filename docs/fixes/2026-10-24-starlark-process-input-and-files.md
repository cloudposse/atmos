# Fix: Add process input, file writes, and environment access to scripts

**Date:** 2026-10-24

## Summary

Add `stdin` to process calls, `exec.getenv` for effective environment access, and `fs.write_file` for local output.

## Context

Issue #3511 reports that scripts had to pass secrets as process arguments and launch helper programs to write
files or access inherited environment values. Existing `env` contains explicitly declared script inputs only.

## Changes

- Accept string input on `exec.run` and `component.exec`, preserving bytes and replaying input for retries.
- Read the effective subprocess environment with `exec.getenv` without changing the explicit `env` dictionary.
- Create files with owner read/write permissions by default; require explicit overwrite and preserve existing modes.
- Document API behavior, standalone use, file path resolution, and migration from helper programs.

## Validation

- `go test ./pkg/script/... ./pkg/filesystem/... -count=1` passed.
- Focused script I/O tests passed under the race detector.
- The script-step environment/file integration test passed.
- The built CLI passed standalone environment/file and real subprocess stdin checks without a project.
- Patch-scoped lint reported zero issues; the CLI build and website build passed (82 prebuild tests).

## Follow-ups

None.
