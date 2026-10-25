# Fix: Add process input, file writes, and environment access to scripts

**Date:** 2026-10-24

## Summary

Add `stdin` to process calls and Atmos wrappers, `fs.write_file` for local output, and an `os` module for
effective environment access, platform information, and invocation-local environment/directory changes.

## Context

Issue #3511 reports that scripts had to pass secrets as process arguments and launch helper programs to write
files or access inherited environment values. Existing `env` contains explicitly declared script inputs only.

## Changes

- Accept string input on `exec.run` and `component.exec`, preserving bytes and replaying input for retries.
- Accept `stdin` on every `atmos.*` wrapper without adding CLI flags automatically.
- Expose `os.platform`, `os.arch`, `os.getcwd`, and frozen `os.environ` snapshots.
- Restrict `os.setenv` and `os.chdir` to the main thread; parallel tasks inherit prior changes, with per-call overrides.
- Keep host state and declared `env` unchanged; apply environment mutations to existing component handles.
- Read the effective subprocess environment with `os.getenv` without changing the explicit `env` dictionary.
- Create files with owner read/write permissions by default; require explicit overwrite and preserve existing modes.
- Document API behavior, standalone use, file path resolution, and migration from helper programs.
- Wrap canceled file writes with `ErrWriteFile` while preserving `context.Canceled` classification.
- Keep documented subprocess signatures in binding order, with `stdin` after `retry`.

## Validation

- `go test ./pkg/script/... ./pkg/filesystem/... -count=1` passed, including cancellation sentinel classification after review.
- Focused script I/O tests passed under the race detector.
- The script-step environment/file integration test passed.
- The built CLI passed standalone environment/file and real subprocess stdin checks without a project.
- Patch-scoped lint reported zero issues; the CLI build and website build passed (82 prebuild tests), including reruns after the review corrections.

## Follow-ups

None.
