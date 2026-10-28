# Terraform component registry refactor

## Objective

Route Terraform and OpenTofu operations through the component registry. Move Terraform lifecycle ownership out of `internal/exec` in reviewable steps. Preserve existing commands, configuration, state addresses, execution order, and reporting.

This work supplies reusable host interfaces for the separate Terragrunt integration. It does not depend on the environment registry initiative or include the Terragrunt prototype.

## Delivery sequence

1. **Terraform component extraction (one PR).** Register the built-in provider. Move single, bulk, query, and affected execution, initialization, workspaces, hooks, shell sessions, policy, UI routing, artifact generation, and state/output lookup into `pkg/component/terraform`. Keep one Terraform production adapter in `internal/exec`. Inject shared Atmos services. Preserve current commands and behavior.
2. **Component capabilities.** Define independent discovery, preparation, state resolution, and provenance capabilities. Inventory and state reads must not run the execution lifecycle. Keep component family separate from configuration format and selected executable.
3. **External plugin adapter.** Extend the existing plugin PR stack with these capabilities. Preserve the host scheduler, cancellation, scoped credentials, streams, and structured results across the process boundary.
4. **Terragrunt integration.** Replace the prototype's injected callbacks with those capabilities. Native HCL units remain Terraform component instances and use the same execution lifecycle as native YAML instances.

Each layer must build and pass its own tests. Each layer must meet at least 90% patch coverage. CI coverage is authoritative; local coverage reports must state their package scope.

## Scope of the initial PR

`pkg/component/terraform.Provider` implements the existing component interface. The host registers it with an injected runtime. `internal/exec.ExecuteTerraform` becomes the compatibility dispatcher, so single execution, scheduler nodes, migration, and plan comparison all select the provider. The component-owned `Executor` implements the runtime. Its private pipeline runs through the provider.

`component.ExecutionContext.Options` carries the existing execution options. The internal `ShellCommandOption` type is an alias, not a second options implementation. Preserve option order, writer identity, the difference between nil and empty environments, and reporting callbacks. The request context is authoritative for execution. This is an in-process host contract. External plugins must use the separate typed protocol and host services; callbacks and writers are not wire payloads.

`GenerateArtifacts` accepts an already resolved configuration, `ConfigAndStacksInfo`, and a working directory. The component package owns the backend, generated-file, provider-override, and required-provider generation sequence. The explicit single and bulk varfile commands also live in this package. The component execution pipeline decides when to prepare its automatic varfile. This method must not initialize Terraform or select a workspace. A reusable prepared-component capability remains a separate requirement.

`Executor` receives `Services`, which includes the `OperationServices` interface used by `Operations`. The host supplies stack processing, templates, deferred YAML resolution, naming, generated-file processing, registry dispatch, process execution, affected analysis, authentication services, and reporting. The component owns the order in which these services run. Do not install process-global callbacks or introduce a dependency from the component package to `internal/exec`. Keep deferred-context types in `pkg/component` and internal aliases to preserve type assertions across the boundary.

Move complete source files when their dependencies permit it. Extract Terraform-only code from mixed files. Keep compatibility entry points for current callers. Remove unused private wrappers. Co-locate pure behavioral tests with the implementation; keep host integration tests in `internal/exec` and instrument both packages when measuring extraction coverage.

The provider validates the resolved backend type's shape. Existing policy and module validation stay in the execution pipeline, where stack and authentication context are available. Command metadata must not become a passthrough allowlist.

## Extraction acceptance criteria

- The Terraform provider and its interfaces must not import `internal/exec`.
- `internal/exec` must contain one Terraform production adapter file. Do not meet this requirement by renaming or combining Terraform business logic inside that package.
- Each extraction must remove the superseded implementation and move its behavioral tests with it. A second copy does not count as extraction.
- Preserve Terraform/OpenTofu selection, initialization, workspace selection, saved plans, dry-run behavior, hooks, retries, and policy enforcement.
- Preserve single, bulk, query, and affected dispatch, including reverse destruction order.
- Preserve stdout/stderr destination and capture, UI execution, cancellation, detailed exit codes, auth, and execution reporting.
- Preserve inventory identities used by list/describe, state functions, and Atmos Pro.
- Tests that replace the global registry must restore all providers they found.

The remaining host adapter composes services and preserves existing entry points. `NewTerraformExecutor` creates an independent set of service bindings without changing the registry. Component execution, artifact generation, and reference getters use those bindings. The component retains the existing process-wide state and authentication caches; preserve their keys and invalidation rules.

## Ownership boundary

| Owner | Responsibility |
| --- | --- |
| `pkg/component/terraform` | Terraform/OpenTofu lifecycle, arguments, authentication orchestration, initialization, workspaces, hooks, policy, retries, UI routing, shell sessions, generation, plan comparison, and state/output lookup. |
| `internal/exec/terraform.go` | Service composition, built-in provider registration, and compatibility entry points. This is the only Terraform production file in `internal/exec`. |
| Shared Atmos host services | Stack and template evaluation, deferred authentication, generic affected analysis, process execution, validation, and reporting. These remain reusable by other component types. |
| `pkg/component` | Execution options and shared data contracts. Internal aliases preserve existing callers and type assertions. |

The component must not import `internal/exec`. Service callbacks are in-process dependencies, not an external plugin protocol. An external plugin must use typed requests and host capabilities instead of serializing callbacks or writers.

Move unit tests with the implementation. Keep tests of real host adapters in `internal/exec`, or use an external test harness to bind an independent executor to the host. Terraform-named test files in `internal/exec` do not count as production implementation. Preserve existing assertions when moving tests.

This PR does not implement external plugin transport, Terragrunt support, or the new discovery and prepared-component capabilities.
