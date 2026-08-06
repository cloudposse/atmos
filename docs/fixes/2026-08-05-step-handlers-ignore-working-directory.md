# Fix: Step handlers ignored `step.WorkingDirectory` for relative filesystem paths

**Date:** 2026-08-05

## Summary

`type: archive` steps that run as component lifecycle hooks ignored `step.WorkingDirectory`, resolving
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

A subsequent hands-on field test of this fix (executing every handler live against real fixtures,
real Docker, and all three entry points — workflows, custom commands, and hooks — rather than
re-reading the unit tests) found the `container_build.go` fix from the original audit above was
itself incomplete: `build.bake.file`/`build.bake.files` (Buildx Bake mode) and `build.cache.from`/
`build.cache.to` `type: local` `src`/`dest` entries share the exact same bug — template-only
resolution, no anchoring — and were reproduced live, including a silent-wrong-file variant (deleting
the correct bake file entirely and observing the build still "succeed" against a same-named file in
the wrong directory). The same pass also root-caused a separate, pre-existing defect surfaced while
verifying the fix's error messages: `internal/exec/workflow_utils.go`'s `buildWorkflowStepError`
silently dropped all hints/context from the wrapped step error (see Changes below for both).

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

### Follow-up (found via field test)

- `pkg/runner/step/container_build.go`: `resolveBuildBake` now takes the handler and step so
  `bake.File` and each entry of `bake.Files` route through `ResolveInWorkingDirectory` (new
  `resolveBakeFiles` helper for the slice case), matching how `Context`/`Dockerfile` are already
  anchored. `resolveBuildCache`/`resolveBuildCacheEntries` now take the handler and step too; for
  `type: local` cache entries only, `src`/`dest` are anchored the same way via a new
  `anchorCacheLocalPaths` helper — every other cache type (`registry`, `gha`, `s3`, `azblob`, …)
  is left untouched since those keys are refs/URLs, not filesystem paths.
- `internal/exec/workflow_utils.go`: `buildWorkflowStepError` now builds via
  `errUtils.Build(errUtils.ErrWorkflowStepFailed).WithCause(err)` instead of manually dual-wrapping
  with `fmt.Errorf("%w: %w", ErrWorkflowStepFailed, err)` before calling `errUtils.Build`. Root
  cause: the dual-`%w` produces a Go 1.20 multi-error (`Unwrap() []error`), and
  `cockroachdb/errors`' hint/safe-detail extraction (`errbase.UnwrapOnce`, used by
  `GetAllHints`/`GetAllSafeDetails`) treats a multi-error as an opaque leaf node — so any
  hints/context a step handler attached deep inside `err` (e.g. via `ResolveInWorkingDirectory`'s
  own `errUtils.Build(...).WithContext(...)`) were silently unreachable to the CLI's error renderer,
  even with `--verbose`. `WithCause` extracts hints/context from `err` eagerly, before any wrapping,
  which sidesteps the multi-error blind spot; `errors.Is()` against both the sentinel and the
  original `err` still holds, and the exit-code extraction a few lines below already read `err`
  directly, so it was unaffected either way. This is a general fix scoped to this one call site, not
  a repo-wide sweep — see Follow-ups.
- New regression tests: `TestBuildBuildConfigResolvesBakeFilesAgainstWorkingDirectory`,
  `TestResolveBuildCacheAnchorsLocalPaths` (`pkg/runner/step/container_actions_extra_test.go`), and
  `TestBuildWorkflowStepErrorPreservesInnerHintsAndContext`
  (`internal/exec/workflow_utils_test.go`).

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

### Follow-up (found via field test)

- `TestBuildBuildConfigResolvesBakeFilesAgainstWorkingDirectory` and
  `TestResolveBuildCacheAnchorsLocalPaths` confirmed failing (compile-time signature mismatch,
  since the fix changes `resolveBuildBake`/`resolveBuildCache`'s signatures) against the pre-fix
  code, then passing after.
- `TestBuildWorkflowStepErrorPreservesInnerHintsAndContext` confirmed failing against the pre-fix
  `buildWorkflowStepError` (hint and context both absent from the rendered output even with
  `Verbose: true`), then passing after.
- `go build ./...` — clean.
- `go test ./pkg/runner/step/... ./internal/exec/... ./pkg/hooks/...` (`-count=1`) — all packages
  `ok`.
- Live re-repro against the field test's `/tmp/atmos-field-test/` fixture: the `container-bake-print`
  workflow now resolves the bake file from the configured `working_directory` instead of the launch
  directory (crash case resolved, confirmed by deleting the correct bake file and observing a clean
  error naming the *anchored* path rather than a silent build against the wrong file). A workflow
  error with a `WithHint` (no `--verbose` needed — hints are unconditional) now correctly shows that
  hint end-to-end through the real CLI. The `Context` half of this fix (`field`/`step`) is verified
  only at the unit level (`TestBuildWorkflowStepErrorPreservesInnerHintsAndContext`, which
  constructs a `FormatterConfig{Verbose: true}` directly) — live CLI confirmation with `--verbose
  workflow ...` was blocked by a separate, pre-existing bug (see Follow-ups): the Context section
  doesn't render for *any* workflow-step error via `--verbose`, reproduced identically with a
  trivial, unrelated pre-existing error, so it isn't caused by this change.

## Follow-ups

A repo-wide audit of the ~180 other `fmt.Errorf("%w: %w", sentinel, err)` call sites for the same
hint/context-swallowing risk was explicitly not pursued here — most of those sites either wrap plain
errors with no hints/context to lose, or never re-enter a second `errUtils.Build()` expecting them to
resurface, so they don't exhibit this defect. `buildWorkflowStepError` was the only site found where
the pattern actually breaks. If a broader sweep is wanted, it should be a separate, deliberately
scoped follow-up.

A second, separate, pre-existing bug was found while live-verifying the fix above: `--verbose` does
not enable the `## Context` section for `atmos workflow` step-failure errors at all, even for a
trivial, unrelated pre-existing error (`workflows: {name: {steps: []}}`, which has an explicit
`WithContext("workflow", ...)` with no nesting involved). `printFormattedError`
(`errors/error_funcs.go`) correctly resolves `verbose` from `verboseFlagSet`/`viper`/config
precedence and overrides `DefaultFormatterConfig().Verbose` with it — that part traced out fine —
but the resulting `Context` section still didn't appear in a live `atmos workflow ... --verbose`
run. Root cause not fully isolated (candidates: `atmosConfig` being nil at print time for this
command, routing through `printMarkdownError`'s plain fallback instead of `printFormattedError`, or
something specific to how `cmd/workflow`'s `RunE` surfaces its returned error to the top-level
printer) — not pursued further here since it's unrelated to the `WorkingDirectory` fix and
reproduces independently of it. Flagging for separate investigation rather than silently leaving
undocumented.
