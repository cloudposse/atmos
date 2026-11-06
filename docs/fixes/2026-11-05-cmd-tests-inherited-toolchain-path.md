# Fix: `cmd` tests keep toolchain tools inherited from `atmos test`

**Date:** 2026-11-05

## Summary

Under `atmos test`, the `cmd` Packer command tests failed with `"packer": executable file not found in $PATH`, and two toolchain environment tests failed on unexpected `PATH` changes. The tests now treat toolchain directories inherited from the outer Atmos run as user-supplied `PATH` entries.

## Context

`atmos test` runs `go test` as a custom-command step. Atmos adds the repository's `.tool-versions` tools to that step's `PATH` and records those directories in `_ATMOS_TOOLCHAIN_PATH`, and the test process inherits both.

The Packer tests checked `exec.LookPath("packer")`, found the toolchain copy, and did not skip. They then ran `atmos packer` in-process against `tests/fixtures/scenarios/packer`. Command startup (`applyInstalledProjectTools`) strips the recorded directories when the project selects different tools, as a fixture without a `.tool-versions` does, so `packer` disappeared before the command ran it.

The failure only showed for Packer on a machine where `helmfile`, `terraform`, and `tofu` also come from Homebrew, because Atmos keeps user-supplied `PATH` entries. CI was unaffected because it puts tools on `PATH` without the marker. `TestRootCommandContinuesWithoutUsableManifest` and `TestInstalledProjectToolsLeavesPathWithoutDefaults` assumed no inherited marker and would fail the same way under `atmos test`. The `cmd` package timeout hid them in the run that surfaced this.

## Changes

- `cmd/tool_preconditions_test.go`: `requirePacker` and `requireHelmfile` call the shared `tests.RequirePacker` / `tests.RequireHelmfile` preconditions, which also find tools in the Atmos toolchain cache, then clear the inherited marker for the test with `t.Setenv`. `keepInheritedToolchainPath` is the shared marker reset.
- `cmd/packer_*_test.go`, `cmd/helmfile_path_resolution_test.go`: use the new helpers.
- `cmd/cmd_utils_test.go`: removed `skipIfPackerNotInstalled` and `skipIfHelmfileNotInstalled`.
- `cmd/toolchain_environment_test.go`: the two tests above call `keepInheritedToolchainPath`.
- Clearing the marker in `cmd`'s `TestMain` was rejected: `TestInstalledProjectToolsReplacesParentSelections` re-runs the test binary as a child that must inherit the marker.

## Validation

- Reproduced the failure by setting `PATH` and `_ATMOS_TOOLCHAIN_PATH` to the toolchain Packer directory: `TestPackerVersionCmd` failed with the same error.
- `go test -short ./cmd` passes with that marker set (as under `atmos test`) and without it (as in CI).
- `atmos lint --changed`: 0 issues.

## Follow-ups

None.
