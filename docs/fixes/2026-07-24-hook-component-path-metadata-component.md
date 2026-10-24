# Fix: Hooks resolve `$ATMOS_COMPONENT_PATH` and their own CWD from the resolved `metadata.component` target, not the stack-facing alias

**Date:** 2026-07-24

## Summary

Built-in hook kinds (`infracost`, `trivy`, `checkov`, `kics`, `tflint`, `kind: command`, `kind: git`,
`kind: step`/`kind: steps`) resolved `$ATMOS_COMPONENT_PATH` — and, for `kind: command`, the hook
subprocess's own working directory — using the stack-facing component name instead of the component's
resolved `metadata.component` target (the shared-module / "abstract component" pattern) or provisioned
workdir. Fixed by threading `ProcessStacks`-resolved component info through the hook execution context,
consolidating path resolution behind one exported `hooks.ComponentPath()` used by every hook kind, and
setting `cmd.Dir` on the hook subprocess itself so relative-path tools are also anchored correctly.

## Context

[Issue #2799](https://github.com/cloudposse/atmos/issues/2799): a component that aliases a real,
on-disk component via `metadata.component` (e.g. `nat-gateway-alias` with
`metadata.component: nat-gateway`, where the actual `.tf` files live under
`components/terraform/nat-gateway/`) had its hooks run against
`.../components/terraform/nat-gateway-alias` — a directory that doesn't exist — instead of the resolved
`.../components/terraform/nat-gateway`. `atmos describe component nat-gateway-alias` already resolved
this correctly (`component_path` pointed at `nat-gateway`), but hooks did not share that resolution.

For `kind: infracost` specifically this failed silently: infracost doesn't error on a missing directory,
it just logs `Could not autodetect any projects from path <dir>` and reports `$0.00` / "No priced
resources," which reads as a normal (if boring) result instead of a broken cost-estimation run.

Root cause: `prepareHookContext` (`cmd/terraform/utils.go`) called `InitCliConfig`, which resolves stack
configuration (including `metadata.component`) into its own **private copy** of
`schema.ConfigAndStacksInfo` and discards it — the `info` used to build the hook execution context never
received the resolved `FinalComponent`/`ComponentFolderPrefix`. Downstream, `pkg/hooks`'s path-resolution
helper (`componentPathFor`, unexported) was also terraform-only and duplicated ad hoc in the `git` hook
kind (which used the repo root, not the component directory, as its Git workdir) and absent entirely from
the step-hook bridge (`kind: step`/`kind: steps` never set a default `working_directory`).

## Changes

- `cmd/terraform/utils.go`: `prepareHookContext` now calls `e.ProcessStacks(&atmosConfig, info, true,
  false, false, nil, authManager)` (templates/YAML-function processing disabled — hook discovery runs
  before auth) after `InitCliConfig`, so the `info` carried into the hook context has the same resolved
  `FinalComponent`/`ComponentFolderPrefix` that `describe component` uses.
- `pkg/hooks/command_engine.go`: renamed the unexported `componentPathFor` to exported
  `ComponentPath(ctx *ExecContext) string` and extended its resolution to cover all provisionable
  component types (terraform/helmfile/packer/ansible via `component.BuildAndResolveWorkdirPath`) plus a
  new `componentBasePath()` helper that maps `ComponentType` → the matching `*DirAbsolutePath` config
  field (terraform/helmfile/packer/ansible/kubernetes/helm) for the in-repo fallback, resolved via
  `u.GetComponentPath` instead of a raw `filepath.Join`. `subprocessPrep` gained a `dir` field, and
  `runSubprocess` now sets `cmd.Dir = p.dir`, so the hook subprocess's actual working directory — not
  just the `$ATMOS_COMPONENT_PATH` env var — matches the resolved component directory.
- `pkg/hooks/kinds/git/engine.go`: `resolveCurrentTarget` now takes `ctx *hooks.ExecContext` and uses
  `hooks.ComponentPath(ctx)` as the Git workdir (previously the repository root), so an unnamed-repository
  `kind: git` hook's `commit.paths` are component-relative like every other hook kind.
- `pkg/hooks/step_engine.go`: new `setDefaultStepWorkingDirectory` sets `step.WorkingDirectory =
  ComponentPath(ctx)` when a step doesn't set one, applied in both `stepEngine.Run` (single `kind: step`)
  and `stepsEngine.Run` (each step in `kind: steps`) before execution — an explicit
  `with.working_directory` is left untouched.
- `pkg/hooks/main_test.go`: new `_ATMOS_TEST_WRITE_CWD` `TestMain` branch writes the subprocess's actual
  CWD and `$ATMOS_COMPONENT_PATH` to `ATMOS_OUTPUT_FILE`, used to assert both agree in the new
  command-engine test.
- Test additions covering each surface: `TestPrepareHookContextResolvesMetadataComponent`
  (`cmd/terraform/utils_hooks_test.go`), `TestCommandEngine_UsesComponentDirectoryForCWDAndEnv` +
  `TestRunSubprocess_FailsForMissingComponentDirectory` + a new non-terraform-component-type case in
  `TestComponentPathFor_Fallbacks` (`pkg/hooks/command_engine_test.go`),
  `TestEngineRunCurrentRepoUsesComponentDirectory` (`pkg/hooks/kinds/git/engine_test.go`), and
  `TestStepHooksDefaultToComponentWorkingDirectory` (`pkg/hooks/step_engine_test.go`). A few pre-existing
  table-driven tests in `command_engine_test.go`/`command_engine_ci_summary_test.go` were updated to
  create a real component subdirectory under their `t.TempDir()` `TerraformDirAbsolutePath`, since
  `runSubprocess` now requires `cmd.Dir` to exist.
- `docs/prd/custom-hooks.md`, `docs/prd/git-ops.md`, `docs/prd/hooks-step-types.md`: documented the new
  CWD/`metadata.component`/workdir resolution behavior for command, git, and step hooks respectively.

## Validation

- `go build ./...` — succeeds, no errors.
- `go test ./cmd/terraform/... ./pkg/hooks/...` (`-count=1`, full package set) — all packages `ok`.
  (`pkg/hooks` and several `cmd/terraform/*` subpackages print a pre-existing, unrelated
  `[no tests to run]` / `testing: warning: no tests to run` annotation even when tests pass — confirmed
  via `git stash` that this also occurs on `origin/main` with this fix's changes removed, so it is not a
  regression from this change.)
- Targeted verbose run of every new test added for this fix — all pass:
  `TestPrepareHookContextResolvesMetadataComponent`, `TestComponentPathFor_Fallbacks` (including the new
  non-terraform-component-type case), `TestCommandEngine_UsesComponentDirectoryForCWDAndEnv`,
  `TestRunSubprocess_FailsForMissingComponentDirectory`, `TestEngineRunCurrentRepoUsesComponentDirectory`,
  `TestStepHooksDefaultToComponentWorkingDirectory`.
  `TestPrepareHookContextResolvesMetadataComponent` in particular reproduces the issue's exact scenario
  (a `reported-order-alias` stack component resolving to `FinalComponent: "mock"` via
  `metadata.component`) against the `tests/fixtures/scenarios/terraform-apply-all-dependencies` fixture.
- `atmos lint --changed` was not run in this pass (not executed as part of documenting this already
  -implemented fix); `go build`/`go test` above are the validation performed.

## Follow-ups

None.
