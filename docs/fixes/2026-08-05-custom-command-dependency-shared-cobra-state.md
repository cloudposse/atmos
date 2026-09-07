# Fix: Command dependencies no longer race, leak flag state, or hard-exit the process on failure

**Date:** 2026-08-05

## Summary

`pkg/taskgraph/adapters/cobra_command.go`'s `CustomCommandRunner` (the in-process runner for
`dependencies.commands` on a custom command) had three related defects, all rooted in resolving
every dependency reference to a command name down to the SAME already-registered
`*cobra.Command` object and mutating its shared, live state with no isolation between dispatches:

1. **Data race**: concurrent dispatches of differently-flagged references to the same command
    (e.g. `dependencies.commands: [{name: build, flags: {env: dev}}, {name: build, flags: {env:
    prod}}]`, taskgraph's default concurrency) raced on that one shared `PersistentFlags`/context/
    annotations with no synchronization.
2. **Deterministic flag leakage**: even fully serialized, a reference with NO flag override
    silently inherited whatever value a PRIOR dispatch (sharing the same command) had last `Set()`
    -- never reset to the flag's own declared default. Reproduced 100% of the time, in either
    dispatch order, once the race above was fixed.
3. **Hard process exit inside a single dependency's execution**: any step failure inside a
    dependency's own run called `errUtils.CheckErrorPrintAndExit` -> `os.Exit`, killing the whole
    process before control ever returned to `taskgraph.Run`, making its `fail:` mode handling
    (`wait_all`/`fail_fast`/`best_effort`) completely unreachable regardless of what was declared.

## Context

Discovered while field-testing the task-runner/dependency-graph feature
(`osterman/task-runner-first-class-support`). Investigation initially suspected only the
concurrency race (issue 1); fixing it alone did not resolve the observed symptom (a bare
`compile` dependency alongside a `compile{target: test}` dependency both logging `target=test`,
zero `target=default`), which led to discovering issue 2 as a distinct, deterministic bug. Issue 3
was discovered via a regression test for issue 3 itself: with `fail: best_effort` set, the
process should survive a dependency's failure and run the parent's own steps -- pre-fix, it hard
exited every time, confirmed by a stack trace showing the exit originating from inside
`executeCustomCommand`, called through `CustomCommandRunner`, inside a `pkg/scheduler` worker
goroutine.

Also encountered, and worth recording since it complicated verification: the durable field-test
fixture (`examples/task-runner-dependencies/`) originally named one of its commands `lint`, which
collides with this Atmos repo's own `.atmos.d/lint.yaml` dev-tooling command (merged in
unconditionally regardless of local `atmos.yaml`, an existing, unrelated behavior). That collision
produced confusing, inconsistent dependency-resolution symptoms during verification and was fixed
by renaming the fixture's command to `trd-lint` -- not a product bug, a fixture-naming hazard
specific to testing from within this repo's own worktree.

## Changes

- `pkg/taskgraph/adapters/cobra_command.go`:
  - `CustomCommandRunner` now holds one `*sync.Mutex` per resolved `*cobra.Command`, lazily
    created and scoped to one `CustomCommandRunner` closure (one `taskgraph.Run` call), serializing
    the full flags -> context -> `PreRun` -> `Run` sequence per target. Same-name dependencies now
    serialize; differently-named ones still run fully concurrently.
  - Before applying `ref.Flags`, every registered persistent flag NOT present in `ref.Flags` is
    reset to its `pflag.Flag.DefValue` (via `PersistentFlags().VisitAll`), so a dispatch's flag
    state never depends on what a prior dispatch sharing the same command left behind.
  - Added `dependencyErrorKey`/`errorSink`/`WithDependencyErrorSink`/`RecordDependencyError`,
    mirroring the existing `WithDependenciesResolved`/`DependenciesAlreadyResolved` pattern: a
    dependency invocation's context now carries a mutex-guarded sink it can report its failure
    into. `CustomCommandRunner` reads the sink after `Run()` returns and returns that as the real
    error, instead of the previous hardcoded `return nil` (itself a second, previously-masked bug
    -- masked because the process used to exit before that return value ever mattered).
- `cmd/cmd_utils.go`:
  - New `exitOrRecordStepFailure(cmd, err)`: when `cmd` is running as someone else's
    already-resolved dependency (`adapters.DependenciesAlreadyResolved`), records into the
    dependency error sink and returns instead of exiting; otherwise preserves today's exact
    `errUtils.CheckErrorPrintAndExit` behavior for a command's own top-level invocation. Wired at
    the three step-failure exit points `executeCustomCommand`'s regression tests exercise: a
    step's own command failing (the final aggregated `commandErr`), a malformed `continue:`
    condition, and a `Silent` `ExitCodeError`.
  - Added a `return` immediately after the pre-existing `taskgraph.Run` error check
    (`errUtils.CheckErrorPrintAndExit(err, "", "")` for a command's OWN dependency failure) --
    previously absent because a real `os.Exit` never returns, so it was unreachable dead code in
    production; without it, a mocked/non-terminating exit (as used in tests, and the scenario this
    whole fix is about) let execution fall through and run the command's own steps anyway.
  - **Scope boundary**: this covers the step-failure paths exercised by the regression tests
    below. Other, rarer exit points earlier in `executeCustomCommand` (flag/working-directory/
    identity resolution failures occurring before any step runs) are not covered by this seam and
    still hard-exit unconditionally even when running as a dependency -- documented in
    `exitOrRecordStepFailure`'s own doc comment.
- `examples/task-runner-dependencies/atmos.yaml`: renamed the `lint` command to `trd-lint` to
  eliminate the naming collision described above (fixture-only change, not a product fix).

## Validation

- New/updated tests in `cmd/custom_command_dependency_test.go`:
  - `TestCustomCommandIntegration_DependenciesCommandsParameterizedBothRun`: widened from 2 to 6
    differently-flagged siblings to maximize concurrent pairwise race probability under `-race`.
  - `TestCustomCommandIntegration_DependenciesCommandsMixedBareAndFlaggedBothCorrect` (new):
    proves the deterministic flag-leakage case (bare reference alongside a flagged one) --
    confirmed failing 100% of the time before the flag-reset fix, passing after.
  - `TestCustomCommandIntegration_DependenciesFailBestEffortSwallowsFailure` (new): confirmed
    failing before the fix (stack trace showed `os.Exit` reached from inside a scheduler worker
    goroutine), passing after. Uses a mutex-guarded flag on the `errUtils.OsExit` seam rather than
    panic/recover, since the exit originates in a different goroutine than the test's own --
    an unrecovered panic there would crash the whole test binary rather than being catchable.
  - `TestCustomCommandIntegration_DependenciesFailFastStillPropagates` (new): negative-path
    counterpart -- the default (`wait_all`) must still surface the failure and block the parent's
    own steps. Also caught the missing-`return`-after-exit-check bug above on its first run
    (parent's own step ran despite the dependency failing, once `OsExit` was mocked).
  - `TestCustomCommandIntegration_DependenciesCommandsDiamondDedup`: unaffected, still passes.
  - All confirmed passing together in a single `go test ./cmd -race` run. (Running the same tests
    with `-count>1` hits a separate, pre-existing limitation: `RootCmd` accumulates custom-command
    registrations across repeated invocations of the same test function within one process, unrelated
    to this fix -- not something a normal `go test` invocation, CI included, ever exercises.)
  - New lower-level test in `pkg/taskgraph/adapters/cobra_command_test.go` and the package's full
    suite pass under `-race`.
- Broader regression check: `go test ./cmd/... ./internal/exec/... ./pkg/taskgraph/...
  ./pkg/runner/freshness/...` -- 61 packages, 0 failures.
- End-to-end against `examples/task-runner-dependencies/`: `verify` (mixed bare/flagged compile
  dependency) now logs exactly one `target=default` and one `target=test` line across 5 repeated
  runs; `release-besteffort` exits 0 with its own step running despite the dependency failing;
  `release-failfast` still correctly exits 1.

## Follow-ups

None.
