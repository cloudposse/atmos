# Fix: defer builtin for cleanup in the Automation Language

**Date:** 2026-10-07

## Summary

Scripts can register cleanup with `defer(fn, *args, **kwargs)`. Deferred calls run last in first out when the enclosing script or `steps.parallel` task finishes, after a normal return, after `fail()`, and after cancellation. The language has no `try`/`finally`, and the YAML `when: always` step cannot reach cleanup that belongs inside one script or one task.

## Context

A Python pipeline harness wrapped emulator startup, stack deploys, and teardown in `try`/`finally`. Mapping it onto the automation language left no way to run teardown after `fail()`: Starlark has no exceptions, so code after a failing call never runs. Signal traps were considered and rejected because Atmos already turns SIGINT and SIGTERM into context cancellation, and destructors were rejected because Starlark has no object lifetime hooks and collection timing would be nondeterministic. Go's `defer` has the semantics that fit: scoped, ordered, and independent of how the scope ended.

## Changes

- `pkg/script/starlark/defer.go`: the `defer` builtin stores a per-thread stack of calls with their captured arguments. `runDeferred` unwinds the stack after the script body (`Engine.Execute`) and after each task attempt (`session.attempt`), so retries unwind per attempt and task output keeps its prefix.
- Deferred calls run on the registering thread while the script's context is live. After cancellation that thread refuses calls, so a fresh thread with a 30 second grace context (`deferGracePeriod`) runs them, sharing the original output sink, step library, and stack. Calls registered while unwinding run too.
- Error policy: the body error stays primary and deferred failures are appended to its detail; when the body succeeded, deferred failures fail the script; remaining deferred calls still run either way.
- Documented at `website/docs/functions/automation/defer.mdx` and listed in the function index, the built-in reference, the language overview, the Python differences table, the script step table, the steps `when` reference, the agent skill, and the Starlark PRD mapping table.

## Validation

- `go build ./...`
- `go test ./pkg/script/starlark/... -count=1`
- `./custom-gcl run --new-from-rev=HEAD ./pkg/script/starlark/...`
- Tests cover order and captured arguments, running after `fail()`, a deferred failure with and without a body error, registration while unwinding, task scoping with prefixed output, cancellation through a blocked process, and argument validation.
- `cd website && npm run build`

## Follow-ups

None.
