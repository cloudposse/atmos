# Hook extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. [All extension types](README.md).

A hook extension registers a namespaced kind selected by project/component configuration, such as `kind: acme/deployment-audit`. Installing a package does not subscribe to events.

```shell
atmos plugin create acme-audit --capability hook --runtime go --kind acme/deployment-audit
atmos plugin search --capability hook
```

The [automation contract](../automation-contracts.md) supplies the complete lifecycle example, manifest, callback, and test requirements.

The author receives validated options, the event, resolved component context, and original operation outcome. Results may include artifacts, summaries, and findings. Atmos owns event matching, ordering, conditions, failure policy, recursion guards, reporting, and cancellation.

Required before-hook failures prevent execution. After-hook failures cannot undo completed infrastructure operations and must retain the original outcome in diagnostics. Explicit retries preserve their existing semantics; crashes never trigger automatic retries.

## Integration

| Aspect | Today |
|---|---|
| Registry | [`hooks.RegisterKind(k *Kind) error`](../../../../pkg/hooks/kind.go) with `GetKind` and `ListKinds`. Built-ins: command, store, step, steps, and the [`kinds/`](../../../../pkg/hooks/kinds) set (infracost, kics, git, tfmigrate, checkov, tflint, trivy). |
| Duplicate policy | Errors on duplicate, nil, empty name, or nil engine. |
| Context on the interface | No. `ExecContext` has no `context.Context`. |
| Reachable from outside the module | Yes. |
| Closed lists to open | None. [`RegisterFormatHandler`](../../../../pkg/hooks/format.go) also errors on duplicates. |
| Boundary leaks | `ExecContext` carries `Cmd *cobra.Command`, `AtmosConfig`, and `Info *schema.ConfigAndStacksInfo`. |

`hooks.Finding{Path, Line, Severity, RuleID, Message}` is separate from [`validation.Diagnostic`](../../../../pkg/validation/diagnostic.go); a shared adapter maps one onto the other. The step bridge in [`step_engine.go`](../../../../pkg/hooks/step_engine.go) registers `step` and `steps` with `OnFailureWarn`, resolves `hook.Type` through the step registry, and builds a `schema.WorkflowStep` from `type:` and `with:`.

**Adapter:** `pkg/plugin/adapter/hook` wraps the plugin in a `hooks.Engine` and `hooks.Kind` and calls the `Hook` wire service. It consumes the `api.Process`, `api.Toolchain`, `api.Diagnostics`, and `api.FS` API services.

**Injection points:**
- `hooks.GetKind`: a miss consults plugin hook kinds.
- Hook option validation: reads the plugin options schema from the manifest.
- `ResultHandler` output: maps through the shared finding adapter to `validation.Diagnostic`.
- Step-backed hooks: the step bridge resolves plugin step types; see [step extensions](steps.md).

**Prerequisites:** P0.4 Context plumbing (`hooks.ExecContext.Context`); Phase 3 work package WP3.3 (steps and hooks). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `ExecContext.Cmd` (`*cobra.Command`): replaced by the command name and arguments.
- `AtmosConfig` and `Info` (`*schema.AtmosConfiguration`, `*schema.ConfigAndStacksInfo`): replaced by resolved component values, stack coordinates, and the lifecycle outcome.
- `ExecContext.ToolchainPATH`: applied Atmos-side by `api.Process`, not handed to the plugin.

Acceptance includes event/condition filtering, fail/warn/ignore behavior, recursion protection, isolated concurrent invocations, cancellation, truthful reporting after a completed operation, and Go/Starlark parity. These are project lifecycle hooks, not package install/uninstall scripts.
