# Fix: widen the Windows tofu.exe temp-dir cleanup retry budget

**Date:** 2026-09-04

## Summary

`TestYamlFuncTerraformOutput` failed on a Windows acceptance shard with `testing.go:1464:
TempDir RemoveAll cleanup: unlinkat ... tofu.exe: The process cannot access the file
because it is being used by another process` -- the test's own assertions all passed;
only `t.TempDir()`'s cleanup failed. A retry helper for exactly this race
(`removeWithRetryForTransientLock`) already existed, added in #1908 earlier the same day,
but its budget (10 attempts x 50ms = 500ms) was too short: the same failure recurred in CI
within hours of that fix landing on `main`. Widened the budget to 40 attempts x 250ms
(~10s) and added a diagnostic log on final exhaustion.

## Context

`isolateTerraformTestBinary` (`internal/exec/yaml_func_terraform_output_test.go`) copies a
`tofu`/`terraform` binary into a per-test directory so tests can run it as a subprocess
without racing other packages' toolchain installs. After the test body runs the binary and
returns, `t.TempDir()`'s own single-shot `os.RemoveAll` cleanup fires. On Windows, a
subprocess binary can keep its file handle open for a window after the process exits --
held by the OS itself or a real-time antivirus scanner -- so an immediate delete attempt
can still find the file locked.

`removeWithRetryForTransientLock`, registered via `t.Cleanup` after `t.TempDir()`'s own
cleanup (so it runs first, since `t.Cleanup` is LIFO), already existed to pre-clear this
exact lock before `t.TempDir()`'s RemoveAll runs. It was added same-day in #1908. The
failure observed here (PR #3052's CI run, `internal-exec.test.exe`, Windows shard 1/10,
2026-09-04T22:06Z) happened on a branch already synced past that commit, confirming the
500ms budget is genuinely insufficient under real CI load rather than a hypothetical
concern -- not something to just rerun past.

## Changes

- `internal/exec/yaml_func_terraform_output_test.go`: `removeWithRetryForTransientLock`'s
  budget increased from 10 attempts x 50ms (500ms total) to 40 attempts x 250ms (~10s
  total). On final exhaustion, logs via `t.Logf` instead of silently returning, so a future
  recurrence is diagnosable in the test's own output rather than only surfacing as
  `t.TempDir()`'s generic cleanup-failure message.

## Validation

- `go build ./internal/exec/...` -- clean.
- `go test ./internal/exec/... -run 'TestIsolateTerraformTestBinary|TestYamlFuncTerraformOutput' -v`
  -- both pass locally (macOS; the Windows-only lock race doesn't reproduce on this
  platform, so this confirms no regression, not a reproduction of the original failure).
- `atmos lint --changed` -- 0 issues.
- Full Windows acceptance suite was not run locally (no Windows runner available); the real
  validation is the next Windows CI run on a PR carrying this change.

## Follow-ups

None. `describe_affected_test.go`'s sibling `copyRepoWithRetry`/`isTransientRepoCopyError`
pattern uses a similarly short budget (5 attempts x 50ms) but has no confirmed CI failure
behind it -- left alone rather than widened speculatively, per the same standard applied
here: fix what's demonstrated broken, not what merely looks similar.
