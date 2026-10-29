# Component engine extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. [All extension types](README.md).

A component extension adds an execution engine for an infrastructure tool. It is distinct from distributing modules, charts, playbooks, or other component content.

## Consumer and author experience

```shell
atmos plugin create acme-pulumi --capability component --capability ci.component --runtime go
atmos plugin install acme/pulumi --as pulumi --version '~1.0'
atmos pulumi preview app -s production
```

These are proposed plugin commands. The engine declares its component kind, command metadata, configuration schema, artifact types, and lifecycle events. Engine command shortcuts follow command collision rules; qualified package invocation remains available for declared command exports.

## Contract

Adapt the [component registry](../../../../pkg/component/registry.go) for discovery, resolved stack inputs, validation, artifact generation, execution, and lifecycle dispatch. API services provide working directories, identity environments, pinned tools, streams, deadlines, and provenance.

Dynamic component kinds must work throughout stack resolution, validation, selection, dependency graphs, and affected-component discovery, not just a command wrapper. Replace closed lists/fallbacks at these integration points with registered metadata while preserving built-in behavior.

The engine returns structured operation outcomes and artifact references. It declares command-specific success semantics, including “changes detected” statuses; Atmos cannot assume every nonzero tool exit means failure. Artifact contracts distinguish source configuration, a generated plan, and the exact artifact applied.

Component-specific CI extensions interpret operation results and emit reporting requests through the selected [CI provider](ci-providers.md). They can ship with the engine. A new engine must not need core changes for plan summaries, checks, comments, or validator findings.

## Integration

| Aspect | Today |
|---|---|
| Registry | [`component.Register(provider ComponentProvider) error`](../../../../pkg/component/registry.go) with `GetProvider`, `MustGetProvider`, `ListTypes`, and `ListAllComponents`. Ansible, aws/cloudformation, container, emulator, helm, kubernetes, and mock register in `init()`. [`custom/provider.go`](../../../../pkg/component/custom/provider.go) registers at runtime. |
| Duplicate policy | Replaces: "last registration wins, allowing plugin override". |
| Context on the interface | Yes, through `ExecutionContext.Context`. |
| Reachable from outside the module | Yes. [`ComponentProvider`](../../../../pkg/component/provider.go) is public. |
| Closed lists to open | `schema.Components.GetComponentConfig`; `autoDetectComponentTypes` in `internal/exec/describe_component.go`; `typeEntries` in `internal/exec/describe_stacks_component_processor.go`; `getComponentBasePath` in `internal/exec/describe_stacks.go`; the three type lists in `pkg/list/extract/components.go`; the hardcoded `terraform, helmfile, packer` lists in `pkg/utils/component_reverse_path_utils.go` and `cmd/list/sources.go`. The component-section whitelist in the stack processor (`vars`, `settings`, `env`, `hooks`, `secrets`, and others) also applies; custom types survive only through the raw-map fallback. |
| Boundary leaks | `Execute` and `GenerateArtifacts` take an `ExecutionContext` struct carrying `*schema.AtmosConfiguration`, `schema.ConfigAndStacksInfo`, and `Flags map[string]any`. |

Terraform, helmfile, and packer are not registered: they never call `component.Register` and run through legacy paths in `internal/exec`. The component CI plugin types (`Plugin`, `HookBinding`, `HookContext`) are Go-internal; see [CI providers](ci-providers.md).

**Adapter:** `pkg/plugin/adapter/component` implements `ComponentProvider` and calls the `Component` wire service, plus `CIComponent` for an engine's `ci.component` extension. It consumes the `api.Process`, `api.Toolchain`, `api.FS`, `api.Credentials`, and `api.Log` API services.

**Injection points:**
- `component.GetProvider`: a miss consults plugin engines.
- Every closed list above reads `component.ListTypes()` plus plugin metadata.
- Stack processing treats a registered external type like a custom type, including `metadata.inherits`.
- Dependency graphs and `describe affected` resolve types through the registry.
- `ci.GetPluginForEvent`: accepts the engine's `ci.component` extension.

**Prerequisites:** P0.3 Duplicate policy; P0.6 Component registry completeness; P0.5 Public aliases for CI component types (for `ci.component`); Phase 3 work package WP3.6 (components and `ci.component`). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `ExecutionContext` struct: replaced by resolved component values, stack coordinates, working directory, and a scoped environment.
- `*schema.AtmosConfiguration` and `schema.ConfigAndStacksInfo`: replaced by the same resolved values.
- `HookHandler func(*HookContext)`: replaced by the `CIComponent` service with a structured outcome and reporting requests.

## Acceptance

A fixture engine is discovered from stack configuration, resolves imports and dependencies, authenticates through Atmos, generates artifacts, executes pinned tools, fires lifecycle events, and reports through both built-in and external CI providers. Test cancellation, partial failure, unsupported artifacts, required plan validation, command/kind collisions, and unchanged built-ins.

The [migration direction](../builtin-migration.md) applies these contracts to future Terraform, Helm, Ansible, and Kubernetes packages.
