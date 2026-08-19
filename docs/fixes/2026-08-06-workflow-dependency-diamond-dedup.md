# Fix: workflow-depends-on-workflow no longer re-resolves and re-runs its own dependencies

**Date:** 2026-08-06

## Summary

`WorkflowRunner` (the `taskgraph.Runner` dispatching a `dependencies.workflows` entry) invoked the
general-purpose `ExecuteWorkflow` entry point to run a dependency workflow's steps -- but
`ExecuteWorkflow` unconditionally resolves and runs *its own* `dependencies.workflows`/
`dependencies.commands` at the top of every call, with no way to know it was itself invoked as a
dependency. A workflow shared by two or more parents (a "diamond": `release` depends on `[test,
lint]`, both of which depend on `build`) would have `build` run once correctly (as part of the
parent's own `taskgraph.Run` graph) and then *again* for every parent that depends on it
redundantly, via each parent's nested `ExecuteWorkflow` call.

## Context

Flagged in two related PR #2882 review threads (`discussion_r3729516420`, `discussion_r3729516435`).
The second thread additionally noted a cycle-detection gap: since each nested `ExecuteWorkflow`
call built and cycle-checked its own independent graph, a cycle only reachable through a
dependency's own nested resolution wasn't guaranteed to be caught by the parent's cycle check. In
practice, tracing `WorkflowLookup` (used for *graph-building*) shows it already recursively
expands a workflow's own dependencies into the parent's single graph -- so simple mutual cycles
were likely already caught -- but the redundant-execution defect was real and unambiguous, and
eliminating the nested `taskgraph.Run` call closes both concerns the same way: there is now only
ever one graph build (and therefore one cycle check) per top-level `ExecuteWorkflow` invocation.

## Changes

- `internal/exec/workflow_utils.go`: added `dependenciesResolved bool` to the existing
  `workflowCommandFilters` variadic-options struct (already `ExecuteWorkflow`'s established
  extension point for optional out-of-band parameters, avoiding a signature change that would
  have touched the two production call sites plus ~49 test call sites). The dependency-resolution
  block is now skipped when this flag is set.
- `internal/exec/workflow_dependency_adapter.go`: `WorkflowRunner`'s `ExecuteWorkflow` call now
  passes `workflowCommandFilters{dependenciesResolved: true}`, mirroring the command-side
  `adapters.WithDependenciesResolved`/`DependenciesAlreadyResolved` pattern already used for
  `dependencies.commands`.

## Validation

- New test `TestExecuteWorkflow_DependenciesWorkflowsDiamondDedup`
  (`internal/exec/workflow_dependency_adapter_test.go`): `release` depends on `[test, lint]`, both
  depend on `build`; asserts `build`'s step ran exactly once for the whole `release` invocation.
- Existing `TestExecuteWorkflow_DependenciesWorkflowsSameFile`/`...CrossFile` still pass (no
  regression to the non-diamond cases).
- `go test ./internal/exec/...`: full package suite passes.

## Follow-ups

None.
