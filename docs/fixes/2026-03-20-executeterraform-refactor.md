# `ExecuteTerraform` Refactor Audit — Findings and Fixes

**Date:** 2026-03-20

**Related PR:** #2226 — `refactor(terraform): reduce ExecuteTerraform cyclomatic complexity 160→26 with 100+ unit tests`

**Severity:** Low–Medium — test quality improvements, no production bugs found

---

## What PR #2226 Did

PR #2226 refactored `ExecuteTerraform` — formerly a ~900-line monolith with cyclomatic
complexity 160 — into a clean pipeline of 26 focused helper functions across 4 files:

- **`terraform.go`** (185 lines) — orchestrator: `ExecuteTerraform` → `prepareComponentExecution`
  → `executeCommandPipeline` → `cleanupTerraformFiles`.
- **`terraform_execute_helpers.go`** (512 lines) — auth, env vars, init, validation helpers.
- **`terraform_execute_helpers_args.go`** (156 lines) — argument builders for plan/apply/init/workspace.
- **`terraform_execute_helpers_exec.go`** (322 lines) — execution pipeline, workspace setup, TTY, cleanup.

All files are within the 600-line limit. 100+ unit tests were added across 6 test files.

### Key design decisions

- Injectable test vars (`defaultMergedAuthConfigGetter`, `defaultComponentConfigFetcher`,
  `defaultAuthManagerCreator`) enable isolated unit testing of auth paths without real
  infrastructure.
- Mutual exclusion between `executeTerraformInitPhase` and `buildInitSubcommandArgs` is
  well-documented (they share `prepareInitExecution` but are guarded by
  `SubCommand == "init"` branching).

---

## Audit Findings

### Finding 1: Duplicate `resolveExitCode` tests across files

**Severity:** Low (test duplication)

`resolveExitCode` was tested in both `terraform_execute_helpers_test.go` (5 test functions
including `os/exec.ExitError`) and `terraform_execute_helpers_pipeline_test.go` (4 test
functions, subset). The `_test.go` version is a superset.

**Fix:** Removed the 4 duplicate tests from `_pipeline_test.go`. The 5 canonical tests in
`_test.go` remain.

### Finding 2: Tautological cleanup test

**Severity:** Low (misleading test)

`TestCleanupTerraformFiles_ApplyRemovesVarFile` in `_args_test.go` created a file but only
asserted `NotPanics` — never verifying the file was removed. A proper file-removal test already
exists in `_coverage_test.go` (`TestCleanupTerraformFiles_ApplyRemovesVarfileForReal`).

**Fix:** Clarified the test's purpose in its comment — it tests graceful handling of
mismatched path layouts, not actual removal. The real removal test is in `_coverage_test.go`.

### Finding 3: No tests for `resolveAndProvisionComponentPath`

**Severity:** High (coverage gap)

This ~55-line function handles auto-generate files, JIT source provisioning, workdir path
override, and re-checking directory existence. It has zero dedicated unit tests. Testing
requires mocking `generateConfigFiles`, `PrepareComponentSourceForJITProvisioning`, and
filesystem operations.

**Status:** Documented. Requires significant mocking infrastructure to test properly —
deferred to a follow-up PR.

### Finding 4: No tests for `generateConfigFiles`, `executeTerraformInitPhase`, `handleVersionSubcommand`

**Severity:** Medium (coverage gaps)

These orchestration functions combine sub-calls but have no tests verifying error propagation
through the combined flow.

**Status:** Documented. These are orchestration functions that delegate to individually-tested
helpers; the risk is lower than Finding 3.

### Finding 5: `TestRunWorkspaceSetup_OutputSubcommand_DryRun_RedirectsToStderr` is misleading

**Severity:** Low (misleading test name)

The test claims to exercise the `wsOpts` stderr-redirect branch for "output"/"show"
subcommands, but because `DryRun=true`, `ExecuteShellCommand` returns nil without ever using
the options. The test only verifies the function does not error.

**Status:** Documented. The test name is misleading about what it covers, but the behavior
is correct.

### Finding 6: Hardcoded `/dev/stdout` in `runWorkspaceSetup`

**Severity:** Low (pre-existing, cross-platform)

`terraform_execute_helpers_exec.go` line 167 uses `"/dev/stdout"` which doesn't exist on
Windows. This is a pre-existing pattern (not introduced by this PR), and `ExecuteShellCommand`
already handles Windows conversion for `/dev/null`.

**Status:** Pre-existing. Not a regression.

---

## Changes Made

### `internal/exec/terraform_execute_helpers_pipeline_test.go`

- Removed 4 duplicate `resolveExitCode` tests (superset in `_test.go`).
- Removed unused `errors` and `fmt` imports.
- Updated file header comment to reflect remaining test functions.

### `internal/exec/terraform_execute_helpers_args_test.go`

- Clarified `TestCleanupTerraformFiles_ApplyRemovesVarFile` comment to explain what the test
  actually covers (graceful handling of missing files, not actual removal).
- Removed the unused file creation that was never verified.

---

## References

- PR #2226: `refactor(terraform): reduce ExecuteTerraform cyclomatic complexity 160→26`
- `internal/exec/terraform.go` — refactored orchestrator
- `internal/exec/terraform_execute_helpers.go` — auth/env/init/validation helpers
- `internal/exec/terraform_execute_helpers_args.go` — argument builders
- `internal/exec/terraform_execute_helpers_exec.go` — execution pipeline
- `internal/exec/terraform_execute_helpers_*_test.go` — 6 test files
