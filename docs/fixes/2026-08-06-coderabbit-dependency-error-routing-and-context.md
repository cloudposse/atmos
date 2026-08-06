# Fix: propagate Cobra cancellation and route every executeCustomCommand error through the dependency sink

**Date:** 2026-08-06

## Summary

Two related CodeRabbit findings on `cmd/cmd_utils.go`'s `executeCustomCommand`:

1. `dependencies.commands`/`dependencies.workflows` resolution used `context.Background()` for
   `taskgraph.Run`, discarding cancellation from the invoking Cobra command's own context -- a
   dependency graph could keep running after the user cancelled (e.g. Ctrl-C) the top-level
   command.
2. The dependency-error-sink routing added for `fail: best_effort`/`fail_fast` (see
   `2026-08-05-custom-command-dependency-shared-cobra-state.md`) only covered step-execution
   failures. Roughly two dozen other exit points earlier in the same function -- argument
   processing, dependency/tool resolution, working-directory resolution, step/exec validation,
   component_config template/lookup errors, ENV var resolution, and per-step flag/auth
   preparation -- still called `errUtils.CheckErrorPrintAndExit` unconditionally, hard-exiting
   the whole process even when this command was running as someone else's dependency.

## Context

Both were flagged in PR #2882 review threads (`discussion_r3729516359`, `discussion_r3729516344`)
on the branch that introduced `dependencies.commands`/`dependencies.workflows` and its `fail:`
mode handling. The second finding is a direct extension of the same bug class already fixed once
(`2026-08-05-custom-command-dependency-shared-cobra-state.md`, Bug B): any hard exit from inside a
dependency's own execution kills the whole process before `taskgraph.Run`'s aggregate result --
and therefore its `wait_all`/`fail_fast`/`best_effort` handling -- can ever see the failure. That
earlier fix only covered the three call sites its own regression tests happened to exercise (a
step's own command failing, a malformed `continue:`, and the final aggregated error); CodeRabbit
correctly pointed out the other ~25 sites in the same function share the identical defect.

## Changes

- `cmd/cmd_utils.go`: the dependency-resolution block now uses `cmd.Context()` (falling back to
  `context.Background()` only when nil, e.g. a command invoked directly in tests without going
  through Cobra's `Execute()`), so cancellation propagates into `taskgraph.Run`.
- `exitOrRecordStepFailure` was generalized to `exitOrRecordDependencyErr(cmd, err, title,
  suggestion)`, matching `errUtils.CheckErrorPrintAndExit`'s full signature so every call site
  converts without losing its title/suggestion text. Every `errUtils.CheckErrorPrintAndExit` call
  inside `executeCustomCommand` (~28 sites, from argument processing through the final aggregated
  step error) now routes through this helper, each followed by an explicit `return` -- a real
  top-level exit never returns either, so the code after it was already unreachable in that
  branch; the `return` only matters for the dependency-recording path.
- Two dynamic errors (`fmt.Errorf` with no static base) uncovered by this refactor's lint pass
  were wrapped with a new sentinel, `errCustomCommandInvalidComponentConfig`, matching the
  existing local-sentinel pattern already used by `errCustomCommandFlagNotRegistered` in the same
  file.
- The one exit call NOT converted is the dependency-resolution block's own `taskgraph.Run` error
  (`cmd_utils.go` ~931) -- it's already guarded by `!adapters.DependenciesAlreadyResolved(cmd)`,
  so it can only ever fire for a command's own top-level dependency resolution, never while
  running as someone else's dependency.

## Validation

- `go test ./cmd -run TestCustomCommandIntegration -race`: all 18 integration tests pass,
  including the `fail: best_effort`/`fail_fast` tests that exercise this exact routing.
- `go build ./...` and `atmos lint --changed`: clean (including the two new err113 findings the
  refactor's line-touching surfaced, fixed via the new sentinel).

## Follow-ups

None.
