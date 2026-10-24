# Fix: streaming UI plan/apply failed writing the planfile when the workdir was relative

**Date:** 2026-09-01

## Summary

`atmos terraform plan --ui` (and the two-phase apply/destroy flow) failed deterministically
with `Failed to write plan file: ... open <workdir>/.atmos-plan-<ts>.tfplan: no such file or
directory` whenever the component's working directory was a relative path. Root cause: the
planfile path was built by joining the working directory onto the filename a second time, even
though the terraform subprocess's `cmd.Dir` was already that same directory — a relative
working directory doubled up when re-resolved against the subprocess's own cwd.

## Context

Reported by the user running `atmos terraform plan` against a real external stacks repo via
`--chdir`, with a JIT-provisioned component workdir. Reproduced deterministically across
repeated runs against the same stack/component — not a race or an external actor, ruling out
the initial theory in `2026-09-01-streaming-ui-diagnostic-only-error-count.md`'s follow-up
section.

`pkg/terraform/ui/executor.go`'s `runTUIProgram`/`Execute` sets `cmd.Dir = opts.WorkingDir` for
the terraform subprocess. Separately, `executePlanWithTempFile` and
`generateTwoPhasePlanFile` built the planfile's own path as
`filepath.Join(opts.WorkingDir, filename)` and passed *that string* as the `-out=`/apply-file
argument to the same subprocess. `opts.WorkingDir` isn't guaranteed absolute — it derives from
`atmosConfig.BasePath`, which (unlike the separately-maintained `atmosConfig.BasePathAbsolute`)
can remain relative, e.g. under `--chdir`. When it is relative, the subprocess's cwd is already
`opts.WorkingDir` (correct, via `cmd.Dir`), but the `-out=` argument's *string value* is still
that same relative path — the subprocess re-resolves it against its own cwd (already
`opts.WorkingDir`), doubling the segment (`<workdir>/<workdir>/.atmos-plan-....tfplan`), which
doesn't exist. Terraform's own diagnostic echoes the raw, undoubled argument string it was
given, which is why the error message looked like a normal (non-doubled) path even though the
actual open target was doubled.

Terraform itself ran successfully for several seconds before failing — it doesn't need the cwd
to physically exist beyond process start for refresh/read operations, only the final `-out=`
write triggers the doubled, nonexistent path.

Not a fix to `atmosConfig.BasePath` itself (a much wider-blast-radius change touching every
`workdir.BuildPath` call site across the provisioning subsystem) — scoped narrowly to where
`pkg/terraform/ui` builds a path it both hands to a subprocess *and* later reopens directly
(`BuildDependencyTree`), since an absolute path is unambiguous regardless of the subprocess's
cwd either way.

## Changes

- `pkg/terraform/ui/executor.go`: added `planFilePath(workingDir, filename string) string`,
  which joins and then absolutizes via `filepath.Abs` (falling back to the plain join only if
  `os.Getwd()` itself fails). `executePlanWithTempFile` and `generateTwoPhasePlanFile` now use
  it instead of a raw `filepath.Join`.
- `pkg/terraform/ui/executor_test.go`: added `TestPlanFilePath_AbsolutizesRelativeWorkingDir`,
  which chdirs into a temp dir, builds a planfile path from a relative working directory
  matching the reported hash-suffixed shape, and asserts the result is absolute with no doubled
  segment. Verified this test fails against the pre-fix behavior (reverted `planFilePath` to a
  bare `filepath.Join` locally, confirmed the test catches it, restored the fix) before treating
  it as real regression coverage.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/terraform/ui/...` — all pass, including the new regression test.
- Confirmed the regression test fails without the fix and passes with it (see above).
- `gofumpt -l pkg/terraform/ui/executor.go pkg/terraform/ui/executor_test.go` — clean.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues from this change (one unrelated
  pre-existing finding in `pkg/project/config/validation.go`, not touched by this fix).

## Follow-ups

None. `atmosConfig.BasePath` vs `BasePathAbsolute` remains inconsistently used across the wider
workdir-provisioning subsystem (`pkg/provisioner/workdir/*.go`, `pkg/component/workdir_path.go`,
`pkg/provisioner/source/*.go`, `internal/terraform_backend/*.go` all consistently use the
possibly-relative `BasePath`); those call sites weren't touched here since none of them pass a
constructed path to a subprocess whose `cmd.Dir` is already that same directory, so they don't
share this specific doubled-path failure mode.
