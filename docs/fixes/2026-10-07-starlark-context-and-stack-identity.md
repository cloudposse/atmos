# Fix: Starlark ctx correctness and computed stack identity errors

**Date:** 2026-10-07

## Summary

Four `ctx.*` fields seen by `!starlark` values were wrong or inconsistent across commands, and a stack name derived from a `!starlark` value produced stacks, map keys, and files named after the encoded program text. The `ctx` fields now match everywhere, and a computed stack identity fails with one clear error.

## Context

Field testing against `tests/fixtures/scenarios/starlark-steps` found these defects:

| Field command | Expected | Observed before fix |
| --- | --- | --- |
| `json.encode(ctx.vars)` / `dict(ctx.vars)` / `**ctx.vars` | JSON object, dictionary copy, keyword arguments | JSON list of keys; `dict()` and `**` failed |
| `ctx.component` for instance `ctxecho` (`metadata.component: mock`) | `ctxecho` | `mock` |
| `ctx.component_type` in `describe stacks` | `terraform`, as in `describe component` | empty string |
| `ctx.locals` with a stack-level `locals:` block | stack locals under component locals | stack-level locals missing |
| `vars.stage: !starlark ...` with `stacks.name_template: "{{.vars.stage}}"` | clear error | `list stacks` printed a stack named after the encoded source; `describe component` said "Could not find the component"; `terraform generate planfile` wrote files named after the base64 blob |

Root causes:

- The context mapping implemented `Mapping` and `Iterable` (keys) but not `starlark.IterableMapping`, and `lib/json` and `dict()` check for `Items()` first.
- `ctx.component` read `info.Component` (the base component). Separately, the resolver's path cache keyed `ctx.component` the same as the component section's own top-level `component` key (the base name), so even the corrected value was shadowed by the cache.
- `describe stacks` never set `ComponentType` on the per-component info.
- Stack-level locals are removed from the processed component sections. Only the raw manifest (`rawStackConfigs`) still holds them.
- The stack name is rendered from `name_template` or `name_pattern` before any `!starlark` value can run, so the encoded source became the name.

## Changes

- `pkg/script/starlark`: `configurationMap` implements `Items()` through the existing `Get`. Because `Items` cannot return an error, the first failure is recorded in a per-evaluation sink and returned once the script finishes, instead of being dropped silently. Nested mappings are covered because they are the same type.
- `ctx.component` is the instance name (`ComponentFromArg`, falling back to `Component`). `ctx.stack`, `ctx.component`, and `ctx.component_type` bypass the resolver path cache so a component's own `component`, `stack`, or `component_type` keys cannot shadow them.
- `describe stacks` sets `ComponentType`, `StackFile`, and the file-level locals on the per-component info before evaluating values. The stack-level locals (`locals:` and the type section's `locals:`, from the raw manifest through the existing `getLocalsForComponentType`/`mergeLocals`) are threaded through a new `ConfigAndStacksInfo.StackLocalsSection` field from `describe stacks`, `ProcessStacks`, and the terraform varfile and backend generators. Component locals override stack locals, matching `atmos describe locals`. New helper: `internal/exec/stack_locals_context.go`.
- New sentinel `ErrStarlarkStackIdentity`. New helper `ensureLiteralStackIdentity` in `internal/exec/stack_identity_starlark.go` returns it, with the manifest and the offending field (for example `vars.stage`) as context, an explanation, and a hint. It gates `describe stacks` (before the stack filter, which also covers `list stacks` and `list components`), `processStackContextPrefix` (so `describe component`, terraform, helmfile, and planfile commands fail clearly), the Spacelift name helpers, the Atlantis repo-config generator, and the `describe affected` Spacelift admin-stack matching. `findComponentInStacks` returns the identity error so that, when no stack matched and a stack with an uncomputable name contains the component, the user sees the identity error instead of "Could not find the component".
- The committed provisional swap in the varfile and backend generators is unchanged.
- Tests: `evaluation_context_test.go` (object encoding, `dict()`, `**` unpacking, nested mapping, error surfacing), `yaml_func_starlark_context_test.go`, `stack_identity_starlark_test.go`, `starlark_context_consistency_test.go` (a throwaway project comparing `describe stacks` with `describe component`, plus the identity gate), and CLI cases in `tests/test-cases/starlark-identity.yaml`.
- Fixture: the computed-identity manifest moved from `starlark-steps` to the new `tests/fixtures/scenarios/starlark-identity/` because one such manifest makes `list stacks` and `describe stacks` fail for every stack in its project.

## Validation

- `go test ./pkg/script/starlark/ -count=1`
- `go test ./internal/exec/ -run 'TestStarlarkContext|TestStackLocalsForComponent|TestEnsureLiteralStackIdentity|TestResolveSpaceliftContextPrefixRejects|TestStarlarkIdentityIsRejected|TestStarlarkYAML' -count=1`
- `go test ./tests -run 'TestCLICommands/starlark_identity|TestCLICommands/starlark_context' -count=1`
- `./custom-gcl run --new-from-rev=origin/main ./pkg/script/starlark/... ./errors/... ./internal/exec/...`: no findings in the changed files.

## Follow-ups

- `atmos list stacks` and `atmos list components` wrap the describe-stacks error with `fmt.Errorf` and two `%w` verbs, so the hint and explanation of the identity error are not printed there; the sentinel text is. Wrapping through the error builder in `cmd/list` would show them.
- The identity error hint cannot suggest selecting the stack by manifest path: `ProcessStacks` also gates a manifest-path selection, because workspaces and planfile names derive from the same stack name.
