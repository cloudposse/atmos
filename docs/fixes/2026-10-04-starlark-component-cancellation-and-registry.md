# Fix: Starlark component wait cancellation and registry ownership

**Date:** 2026-10-04

## Summary

Component lookups waiting for another resolver now honor their own cancellation.
The runtime owns the command catalog and independent standard-library bindings,
so its PR no longer introduces a hardcoded production command list.

## Context

Starlark's attribute interface does not receive a thread. Consequently, lazy
`ctx.component` attribute resolution uses the invocation context, including when
called by a parallel task. The thread-aware `components.get` builtin already
passes task cancellation to the resolver, but waiting for another task's memoized
lookup used an unconditional mutex and could delay that cancellation.

## Changes

- Replace the component entry mutex with a cancellable acquisition while retaining
  serialized resolution and caching only successful values.
- Document the invocation deadline used by lazy attributes and the explicit
  `components.get` pattern for task-specific resolution deadlines. The attribute
  limitation remains intentional; no evaluator rewriting or goroutine-local
  context is introduced.
- Move the previously validated command catalog, `stdlib/atmos`, `stdlib/log`,
  `stdlib/regex`, and shared conversion helpers into the runtime layer. The upper
  integration PR supplies the completed CLI registry through `WithAtmosCommands`.
- Enable automatic CodeRabbit reviews against the execution-support base while
  retaining reviews against `main`.

## Validation

- `go test -race -count=3 -timeout=5m ./pkg/script/starlark/... ./pkg/flags` passed.
- `go test -count=1 -timeout=5m -coverprofile=runtime-review-full.cover ./pkg/script/... ./pkg/flags` passed.
- Local changed-line coverage against `origin/osterman/starlark-execution-support`
  was **93.62%** (1,144 of 1,222 instrumented changed lines).
- Patch-scoped custom golangci-lint reported `0 issues`.
- Regression tests verify a canceled waiter returns while the original resolver
  remains blocked and that `components.get` propagates a task deadline to its
  resolver before the invocation deadline.

## Follow-ups

None.
