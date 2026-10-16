# Fix: Separate Windows orchestration compilation from execution

**Date:** 2026-10-15

## Summary

The Windows pointer-checking diagnostic now compiles and executes in separate steps, preserving three repetitions and a five-minute test execution budget.

## Context

PR #3157's Windows shard 1 reached the five-minute combined build/test limit without reporting an assertion failure. The immediately following PR had identical orchestration sources, dependencies, workflow, and runtime-overlay action. Its successful combined check took approximately 260 seconds, including 218 seconds executing tests. Compilation and execution sharing one limit left little headroom for runner variability; the failed run's silent output did not identify the phase or active test.

## Changes

- Compile the pointer-instrumented test binary with a separate five-minute step limit.
- Execute it from its package directory, matching `go test` working-directory behavior.
- Normalize `RUNNER_TEMP` with `cygpath -u` before Git Bash executes the binary, preserving quoted paths and all test flags. [MSYS2 documents this conversion](https://www.msys2.org/docs/filesystem-paths/).
- Retain `checkptr=2`, three repetitions, the runtime overlay, and existing subprocess limits.
- Add verbose test output and a five-minute Go test timeout, with a six-minute runner limit so Go can print goroutine stacks before termination.
- Preserve the normal acceptance suite and runner security policy.

## Validation

- `actionlint .github/workflows/test.yml` passed.
- Go 1.26.9: the separated pointer-instrumented build and all three test repetitions passed locally on macOS.
- Applicable pre-commit checks passed.
- Native Windows execution will be checked on the newly published PR head; local success does not establish the Windows result.

## Follow-ups

Inspect the new native Windows compilation and test steps, and investigate any repeated failure using their verbose output and timeout diagnostics. Do not blindly retry failed jobs.
