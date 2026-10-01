# Fix: `type: store` step `with:` blocks were silently misrouted or dropped

**Date:** 2026-08-19

## Summary

A documented `type: store, action: write` step failed before ever reaching execution — in
workflows, custom commands, and via the `kind: step` component-hook bridge — because its `with:`
configuration never reached the store handler. Two independent decode paths shared the same class
of bug: one misrouted the step into the container decoder and errored outright, the other silently
dropped the configuration and produced a confusing generic error.

## Context

Reported bug: a custom-command step shaped like

```yaml
- type: store
  action: write
  with:
    store: image-metadata
    key: image-dev
    value: "some-value"
    stack: dev
    component: app
```

failed immediately with:

```text
invalid workflow control step: container `action: write` does not accept a `with:` block
```

Investigating the root cause (`decodeStepWith` in `pkg/schema/workflow.go`) surfaced that the same
shared decode path is used by workflow files, custom commands, *and* archive-style steps, and that
a second, independent bug existed in the `kind: step` component-hook bridge
(`pkg/hooks/step_engine.go`) for any step type whose configuration lives only in the generic
`With` map (`store`, `tflint`) rather than flat `WorkflowStep` fields.

## Changes

### Root cause 1 — `decodeStepWith` container misroute

`decodeStepWith` routed *any* step with a non-empty `action:` into the container `with:` decoder,
regardless of `type:`:

```go
if strings.TrimSpace(stepType) == "container" || strings.TrimSpace(action) != "" {
    return decodeContainerWith(node, action, t.container)
}
```

`Action` is a shared field reused by container (`build`/`push`/`run`/`inspect`), store (`write`),
and archive (`create`/`extract`/`update`/`replace`) steps, so any non-container type that also set
`action:` fell into `decodeContainerWith`'s `default` case and errored. Fixed by requiring
`stepType == containerStepType` before routing to the container decoder
(`pkg/schema/workflow.go`). This is the single shared decode path for both the workflow-file path
(`WorkflowStep.UnmarshalYAML`) and the custom-command/Viper path (`TasksDecodeHook` →
`decodeTaskFromMap`), so both were broken and both are now fixed.

### Root cause 2 — `kind: step` hook bridge drops generic `With`

`StepFromHook` and `workflowStepFromHookPayload` (`pkg/hooks/step_engine.go`) round-trip the
hook's `with:` payload directly into `WorkflowStep`'s top-level fields — correct for step types
with flat fields (`archive`, `say`), but step types like `store`/`tflint` have no flat fields to
receive `store`/`key`/`value`, so that config was silently dropped and `StoreHandler.Validate`
failed with a generic "store is required" error that never named the store the user configured.
Fixed with a new `preserveGenericWith` helper that backfills `ws.With` from the hook payload
whenever the normal decode leaves it `nil`, applied at both the static (`StepFromHook`, used by
preflight) and runtime (`workflowStepFromHookPayload`, used by `stepEngine.Run` and
`stepsEngine.Run`) decode points.

### Code

| File                          | Change                                                                 |
|--------------------------------|-------------------------------------------------------------------------|
| `pkg/schema/workflow.go`       | `decodeStepWith` only routes to the container decoder when `type: container` |
| `pkg/hooks/step_engine.go`     | New `preserveGenericWith` helper, called from `StepFromHook` and `workflowStepFromHookPayload` |

### Test Coverage Added

- `pkg/schema/task_test.go`: `TestStoreStepWithBlock_WorkflowAndCustomCommandDecodeIdentically`
  (both decode paths preserve the generic `With` map for `type: store, action: write`); corrected
  `TestDecodeTaskFromMap_InvalidWithBlock`, which had unknowingly been asserting the *buggy*
  behavior (`type: shell` + arbitrary `action:` + `with:` only errored because of this bug) —
  retargeted to `type: container` with a genuinely unsupported action so it tests what its
  docstring claims.
- `pkg/hooks/step_engine_test.go`: `TestStepFromHookPreservesGenericWithForStoreType` and
  `TestStepFromHookWithVariablesPreservesGenericWithForStoreType` cover the static and runtime
  hook decode paths respectively.

## Validation

- `go build ./...`
- `go test ./pkg/schema/... ./pkg/runner/step/... ./pkg/hooks/... ./pkg/config/...` — all pass.
- Live end-to-end verification with a built `atmos` binary: a real custom command and a real
  workflow file, each with a `type: store, action: write` step, both now decode successfully and
  fail only on the expected downstream error (`store not configured: <name>`) instead of the
  reported container-decode error.
- Regression-checked live: `type: container, action: build` still builds correctly (real Docker
  build, no leaked images); `type: container` with a genuinely unsupported action still produces
  the container-specific error.
- Local `atmos test` (full short-suite) hit an unrelated, pre-existing environment issue (a
  Podman/vfkit VM auto-boot hang in the sanitized test HOME on this machine, already documented in
  a separate memory record) — confirmed independent of this change and not a new regression.

## Follow-ups

None.
