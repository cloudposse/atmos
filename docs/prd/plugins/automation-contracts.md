# Hook and step extension contracts

**Status:** Proposed; follow-on implementation for [Independently Versioned Atmos Plugins](overview.md). New commands, manifest fields, and SDK bindings below are proposed APIs. [All plugin PRDs](README.md).

## Two capabilities, one automation foundation

A hook plugin adds a reusable lifecycle hook kind, such as an enterprise deployment audit integration. A step plugin adds a task that can run in workflows, custom commands, scripts, and step-backed hooks. Both support Go/native protocol and Starlark authoring, OCI discovery, independent versions, and generated contract tests.

| Capability | Selected by | Author implements | Atmos owns |
|---|---|---|---|
| `hook` | `kind: acme/deployment-audit` | Configuration validation and a lifecycle callback returning artifacts, summaries, or findings. | Event matching, conditions, ordering, component context, recursion protection, failure policy, and reporting. |
| `step` | `type: acme/change-request` | Configuration validation and execution returning values, metadata, and named outputs. | Templates, scheduling, conditions, scope, output capture, cancellation, and configured retry/timeout behavior. |

Extend the existing [hook-kind registry](../../../pkg/hooks/kind.go), [step registry](../../../pkg/runner/step/registry.go), and [automation step bridge](../../../pkg/api/steps.go). Internal `ExecContext`, `WorkflowStep`, and Go callbacks remain implementation details; versioned SDK/wire records carry the required fields.

A step does not need a second implementation to run as a hook: use the existing `kind: step` bridge. A dedicated hook export is useful when lifecycle context and reporting are the integration's primary interface. A package can expose both and share its underlying implementation.

## User experience

```shell
atmos plugin search --capability hook
atmos plugin search --capability step
atmos plugin install acme/automation --as automation --version '~1.0'
atmos plugin info automation
```

Installing the package registers its available kinds/types for the project. It does not attach events, modify workflows, or call an external service. Explicit configuration activates an export.

An example component hook:

```yaml
components:
  terraform:
    vpc:
      hooks:
        audit-deployment:
          events: [after.terraform.apply]
          kind: acme/deployment-audit
          when: always
          on_failure: fail
          with:
            endpoint: https://audit.acme.example/deployments
```

`with` supplies plugin-owned options, validated by its packaged schema. Atmos supplies the resolved component, stack, triggering event, and original operation outcome separately. An audit callback can distinguish a failed apply from its own reporting failure. A failing after-hook reports failure but cannot undo an apply that already happened.

An example workflow file, `workflows/release.yaml`:

```yaml
workflows:
  prepare:
    steps:
      - name: change
        type: acme/change-request
        with:
          service: payments
          environment: production
      - name: show-change
        type: shell
        command: 'echo "{{ .steps.change.value }}"'
```

```shell
atmos workflow prepare -f release.yaml
```

Configure `workflows.base_path: workflows` in `atmos.yaml`. The plugin returns the created change ID as its value and may return named outputs, such as a URL. The same namespaced type works in custom-command step lists and the automation step module's generic dispatcher; it must not depend on a name being a valid Starlark attribute.

Reuse that step in a component hook:

```yaml
hooks:
  open-change:
    events: [before.terraform.apply]
    kind: step
    type: acme/change-request
    on_failure: fail
    with:
      service: payments
      environment: production
```

The bridge preserves its existing `with` translation; the plugin receives its schema-validated options identically through workflow and hook paths. No extra plugin installation or hook-specific handler is required.

## Authoring

```shell
atmos plugin create acme-audit --capability hook --runtime go --kind acme/deployment-audit
atmos plugin create acme-changes --capability step --runtime starlark --kind acme/change-request
```

Each scaffold includes a manifest, option/result schemas, handler, mock service, contract fixtures, example workflow/hook configuration, README, and release workflow. Go scaffolds build precompiled platform binaries; Starlark scaffolds package local modules. Authors use the shared [validate, test, pack, and publish workflow](authoring.md#try-locally-then-publish).

A package exporting both capabilities declares them statically:

```yaml
apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/automation
  version: 1.0.0
spec:
  runtime:
    type: starlark
    api: 1
    entrypoint: plugin.star
  capabilities:
    hooks:
      acme/deployment-audit:
        schema: schemas/audit.json
    steps:
      acme/change-request:
        schema: schemas/change.json
        result_schema: schemas/change-result.json
        category: command
        requires_tty: false
```

The proposed SDK provides hook `Run(context.Context, hooks.Request, api.Services)` and step `Execute(context.Context, steps.Request, api.Services)` handlers with typed result/error returns, plus configuration-validation callbacks. Starlark exposes `plugin.register_hook` and `plugin.register_step` bindings for the same contracts. Atmos checks runtime registrations against the verified manifest. Discovery and option-schema inspection never execute a plugin.

Step results preserve existing value, values, metadata, outputs, and skipped semantics. Output/result schemas are validated before publishing values to later steps. Hook results carry artifacts and structured summaries; finding adapters map into shared diagnostics and CI reporting while retaining scanner severity where applicable. Neither callback receives a Cobra command or mutable Atmos manager.

## Execution requirements

- Atmos owns event dispatch and scheduling. Hook plugins cannot subscribe globally just by being installed; consumers select events explicitly. Component plugins may publish lifecycle events through Atmos contract.
- Existing `when`, `on_failure`, skip behavior, ordering, and recursion guards remain authoritative. Required before-hooks prevent the operation on failure; after-hook errors preserve the original operation's outcome in diagnostics.
- Plugin option schemas extend `with` without adding each plugin's fields to Atmos structs. Static metadata must participate in workflow, custom-command, and hook validation, including namespaced step types.
- Reject collisions with built-in kinds, step names, aliases, or another plugin before mutating a registry. The current step registry allows replacement; the external adapter must not inherit that behavior.
- Invocation context includes scoped working directory, declared environment/credentials, streams, deadlines, and an invocation ID. Forked/parallel steps retain existing output and environment isolation. Starlark invocations use isolated evaluation state; native plugins must honor concurrent invocation isolation.
- Atmos handles prompts; `requires_tty` is checked before execution. Noninteractive invocations fail clearly when required input cannot be provided.
- No automatic retry follows a crash or timeout. Explicit workflow/hook retries rerun the operation, so authors document side effects and any supported idempotency keys. A stable logical invocation ID and attempt number are exposed for explicit retries; exactly-once external execution is not promised.
- Cancellation stops plugin work and owned subprocesses. Cancellation is not a skipped/successful result. Logs and failures identify plugin version, hook/step instance, and event where applicable; secret masking applies throughout.
- Package lifecycle operations do not run these hooks. Installing, updating, removing, or discovering a plugin must not trigger project automation.

## Tests and acceptance criteria

Use the common [plugin contract runner](testing.md) with fake lifecycle events, deterministic invocation IDs, and mocked HTTP/API services. Generated Go and Starlark implementations pass the same applicable scenarios.

- A published custom hook kind runs only for configured events and conditions, with separate original-operation and hook outcomes.
- A custom step works through a workflow, custom command, Starlark generic step dispatch, and `kind: step`, returning identical values and named outputs.
- Positive/negative schema fixtures prove plugin-specific options are accepted and invalid options fail before execution.
- Tests exercise fail/warn/ignore behavior, required before-hook failure, after-hook failure after a successful operation, skipped hooks, recursion, and explicit retry side effects.
- Parallel invocations cannot leak values, environment, or credentials into each other; cancellation/crash/timeout tests verify cleanup and non-success outcomes.
- Collisions, incompatible contracts, undeclared runtime exports, and malformed results fail clearly. Inspection/help and projects without these plugins launch no plugin subprocesses.
- Updating or rolling back either capability changes its behavior without rebuilding or replacing Atmos.
