# Step extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. [All extension types](README.md).

A step extension adds a namespaced task usable in workflows, YAML custom commands, Starlark scripts, and step-backed hooks.

```shell
atmos plugin create acme-changes --capability step --runtime starlark --kind acme/change-request
atmos plugin search --capability step
```

A script can use the existing generic dispatcher with the proposed external type:

```python
result = steps.run("acme/change-request", **{
    "with": {"service": "payments", "environment": "production"},
})
```

The generic dispatcher passes schema-defined options through `with`. Names containing slashes do not need to be valid Starlark attributes.

The [automation contract](../automation-contracts.md) defines concrete YAML workflow/hook examples, option/result schemas, and execution requirements. A plugin author implements validation and execution; Atmos owns scheduling, scopes, conditions, templates, outputs, retries, and cancellation. Values and named outputs remain compatible with later built-in steps.

A step runs through `kind: step` without also implementing a hook extension. Namespaced types must pass static validation and all existing step dispatch paths. External adapters reject built-in/alias collisions even though the current internal step registration permits replacement.

## Integration

| Aspect | Today |
|---|---|
| Registry | [`step.Register(handler StepHandler)`](../../../../pkg/runner/step/registry.go) with `Get`; about 44 handlers register in `init()`. |
| Duplicate policy | Silently replaces, aliases included. `Get` falls back to aliases. |
| Context on the interface | Yes. `Execute` takes `ctx`. |
| Reachable from outside the module | Yes. |
| Closed lists to open | Static unknown-field validation depends on the unexported optional `KnownFields()` interface. A namespaced external type needs equivalent metadata without a handler process. |
| Boundary leaks | `Execute(ctx, *schema.WorkflowStep, *Variables)` and `io.Writer` outputs. |

The Starlark bridge is [`api.StepLibrary`](../../../../pkg/api/steps.go) (`Names`, `Fork`, `Validate`, `Run`), implemented by [`NewAutomationLibrary`](../../../../pkg/runner/step/automation_library.go). `StepCall` carries `Type`, `Configuration`, `WorkingDirectory`, `ProcessEnv`, `Stdout`, `Stderr`, and `Parallel`.

**Adapter:** `pkg/plugin/adapter/step` implements `StepHandler` (with an optional `KnownFields` derived from manifest metadata) and calls the `Step` wire service. It consumes the `api.Prompt`, `api.Process`, `api.FS`, and `api.Log` API services.

**Injection points:**
- `step.Get`: a miss consults plugin step types.
- Workflow, custom-command, and hook validators: consult plugin option schemas.
- `StepLibrary.Names()`: includes plugin types so `steps.run("acme/x")` resolves.
- The hook step bridge in `pkg/hooks/step_engine.go`: resolves plugin types through `Get`.

**Prerequisites:** P0.3 Duplicate policy; P0.11 Step collision policy and static validation of namespaced types; Phase 3 work package WP3.3 (steps and hooks). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `*schema.WorkflowStep`: replaced by the validated `with` options map and step identity.
- `*Variables`: replaced by resolved template values and named outputs.
- `io.Writer` (stdout and stderr): replaced by streamed output chunks and the `api.Log` service.

Acceptance covers all four invocation surfaces, result equivalence, malformed results/options, TTY requirements, parallel isolation, retry side effects, timeouts, cancellation, and Go/Starlark parity.
