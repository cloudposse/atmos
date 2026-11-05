# Fix: Preserve build settings in precompiled acceptance tests

**Date:** 2026-10-15

## Summary

Run precompiled acceptance tests with the same CGO and FIPS settings used to
compile them, so nested Go builds inherit the intended CI environment.

## Context

Windows test executables were compiled with `CGO_ENABLED=0` and
`GOFIPS140=latest`, but launched with the runner defaults. Tests that compile
Atmos subprocesses therefore enabled CGO. With the temporary Windows exception
reserve overlay, those binaries failed at startup with `fatal: morestack on g0`.
The earlier runtime stress and race checks did not cover this ordinary CLI path.

## Changes

- Apply the existing Go command environment to all three precompiled launchers:
  acceptance tests, internal execution tests, and command tests.
- Preserve inherited environment values and the command coverage directory.
- Exercise each launcher with a real test executable that invokes a nested Go
  command and checks the inherited build settings.

## Validation

The regression failed for all three launchers before the change: the nested Go
command reported CGO enabled and FIPS disabled. After the change, the full
`internal/ci/acceptance` suite passed with Go 1.26.9 and the race detector.
Scoped custom golangci-lint, pre-commit checks, and the full CGO-disabled build
passed. The coverage profile reports 96.6% statement coverage for the acceptance
package and covers all three changed launcher lines.

The isolated [Windows diagnostic](https://github.com/cloudposse/atmos/actions/runs/38795542315)
passed with Go 1.26.9 and the exact runtime overlay: environment propagation,
CGO-disabled CLI startup, and the affected describe/profile integration tests.
On the same AMD EPYC runner, the CGO-enabled control reproduced
`fatal: morestack on g0` and exited 2; the corrected CLI exited 0. Runner
cleanup also completed successfully.

## Follow-ups

Require CI and coverage on every final stack head. Keep this work in the
existing open stack.
