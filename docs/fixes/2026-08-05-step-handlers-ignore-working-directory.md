# Fix: Step handlers ignored `step.WorkingDirectory` for relative filesystem paths

**Date:** 2026-08-05

## Summary

`type: archive` steps run as component lifecycle hooks ignored `step.WorkingDirectory`, resolving
relative `source`/`destination` paths against the Atmos process's own working directory instead of
the hook's configured (or defaulted) component directory. The same defect existed in four other
step handlers. All five are fixed via one shared helper.

## Context

`pkg/hooks/step_engine.go`'s `setDefaultStepWorkingDirectory` correctly computes and sets
`step.WorkingDirectory` (defaulting to the resolved component path) before dispatching a `kind:
step` hook to the step registry's `Execute`. This default-computation path was already fixed once,
for a related-but-distinct problem, in `b0dcb79a66` ("fix(hooks): resolve component path from
metadata.component target", #2802) — that PR fixed how `kind: command`/`kind: git` hooks (e.g. the
`infracost` hook) set `cmd.Dir` on their subprocess, and how `kind: step`'s `working_directory`
default is computed. It never touched `archive.go`, because that handler is a completely different
code path (`kind: step` → step-registry `Execute`, not a subprocess with `cmd.Dir`), and
`archive.go`'s `resolveArchiveOptions` never read `step.WorkingDirectory` back out once it was
correctly set — it resolved `source`/`destination` via Go-template substitution only, then handed
them to `pkg/archive.Run`, which resolves relative paths against `os.Getwd()`.

Auditing `pkg/hooks` and `pkg/runner/step` for the same defect class (grepping for `WorkingDirectory`
usage) found four more handlers with the identical bug — they resolve a relative path field via
template-only resolution and let it fall through to a filesystem call that resolves against process
cwd instead of the step's configured working directory:

- `file.go` (`resolveStartPath`, `step.Path`)
- `workdir.go` (`Execute`, `step.Path` — the target path; `step.Source` is a vendor-source spec, not
  a plain fs path, and was out of scope)
- `junit.go` (`loadReport`, each `step.Files` glob pattern)
- `container_build.go` (`buildBuildConfig`, `build.Context` and `build.Dockerfile`)

Handlers already consuming `step.WorkingDirectory` correctly (used as reference patterns for the
fix): `shell.go`, `exec.go`, `script.go`, `atmos.go`, `cast.go`, `spin.go`, `container_run.go`.

## Changes

- Added `BaseHandler.ResolveInWorkingDirectory` and a private `resolveWorkingDirectory` helper in
  `pkg/runner/step/handler_base.go`. The public helper resolves a Go-template field, then anchors a
  relative result to `step.WorkingDirectory` (itself template-resolved); an empty raw value or an
  already-absolute resolved value passes through unchanged. The private helper resolves
  `step.WorkingDirectory` to an absolute base directory, falling back to `os.Getwd()` when unset —
  preserving prior behavior for steps that never set `working_directory`.
- `pkg/runner/step/archive.go`: converted `resolveArchiveOptions` to a method on `*ArchiveHandler`
  and routed `source`/`destination` through the new helper.
- `pkg/runner/step/file.go`: `resolveStartPath` now uses the helper instead of a manual
  `filepath.Abs` call.
- `pkg/runner/step/workdir.go`: `Execute` now uses the helper for the target `step.Path` instead of
  a manual `filepath.IsAbs`/`filepath.Abs` block.
- `pkg/runner/step/junit.go`: `loadReport` resolves each `step.Files` glob pattern through the
  helper.
- `pkg/runner/step/container_build.go`: `buildBuildConfig` anchors `Context` to
  `step.WorkingDirectory` via the helper, and separately anchors `Dockerfile` to the resolved
  `Context` (not directly to `WorkingDirectory`) via `filepath.Join`, matching Docker's own
  `PATH/Dockerfile` convention — the docker/podman CLI subprocess runs with no `Dir` set, so both
  must already be absolute by the time they reach `-f <Dockerfile> <Context>`.
- Regression tests added per handler (`archive_test.go`, `file_test.go`, `workdir_test.go`,
  `junit_test.go`, `container_actions_extra_test.go`) plus one hooks-integration test,
  `TestStepEngineRunsArchiveTypeWithRelativeWorkingDirectory` in `pkg/hooks/step_engine_test.go`,
  reproducing the original bug report end-to-end (a `type: archive` hook step with relative
  `source`/`destination` and an explicit `working_directory` differing from process cwd).
- Updated two pre-existing container tests (`TestContainerHandlerActionBlocks`,
  `TestContainerHandlerExecuteBuildPassesBuildxDriverAndCacheToDocker`) that had hardcoded the old,
  buggy relative-path (`"."`/`"Dockerfile"`) expectations; they now assert the corrected absolute
  paths.
- Updated two pre-existing handler tests (`file_test.go`, `workdir_test.go`) that asserted on the
  removed `fmt.Errorf` message text (`"failed to resolve path"`) to instead assert
  `errors.Is(err, errUtils.ErrTemplateEvaluation)`, per this repo's error-handling convention.

## Validation

- Every new/changed regression test was confirmed to fail against the pre-fix code before the fix
  was applied, per the mandatory bug-fixing workflow (write test → confirm red → fix → confirm
  green), verified incrementally handler-by-handler.
- `go build ./...` — clean.
- `go test ./pkg/runner/step/... ./pkg/hooks/...` (`-count=1`, uncached) — all packages `ok`, no
  failures.
- `atmos fix lint` (patch-scoped `golangci-lint` + `lintroller` custom binary, `--new-from-rev=origin/main`)
  — 0 issues (after fixing 3 `gocritic` `filepathJoin` findings in the new
  `handler_base_test.go` assertions, which used string literals with embedded path separators as
  `filepath.Join` arguments instead of separate segments).
- `go vet ./pkg/runner/step/... ./pkg/hooks/...` — clean.

## Follow-ups

None.
