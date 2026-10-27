# Terraform component registry refactor

## Objective

Route Terraform and OpenTofu operations through the component registry. Move Terraform lifecycle ownership out of `internal/exec` in reviewable steps. Preserve existing commands, configuration, state addresses, execution order, and reporting.

This work supplies reusable host interfaces for the separate Terragrunt integration. It does not depend on the environment registry initiative or include the Terragrunt prototype.

## Delivery sequence

1. **Provider and implementation extraction (one PR).** Register a built-in Terraform provider. Route existing execution through that provider. Move execution options into `pkg/component`. Move Terraform generation, plan comparison, plan verification, tool detection, CLI environment parsing, auth-context handoff, variable protection, paths, and workspace cleanup into `pkg/component/terraform`. Inject shared Atmos services without importing `internal/exec`. Cover single and scheduled dispatch, output capture, cancellation, error identity, and artifact order.
2. **Remaining Terraform lifecycle extraction.** Move initialization, workspace selection, auth, process orchestration, and remaining preparation into `pkg/component/terraform`. Inject configuration loading, authentication, stack resolution, process execution, and reporting services. Remove the corresponding implementations from `internal/exec`; retain compatibility entry points only where callers still need them.
3. **Component capabilities.** Define independent discovery, preparation, state resolution, and provenance capabilities. Inventory and state reads must not run the execution lifecycle. Keep component family separate from configuration format and selected executable.
4. **External plugin adapter.** Extend the existing plugin PR stack with these capabilities. Preserve the host scheduler, cancellation, scoped credentials, streams, and structured results across the process boundary.
5. **Terragrunt integration.** Replace the prototype's injected callbacks with those capabilities. Native HCL units remain Terraform component instances and use the same execution lifecycle as native YAML instances.

Each layer must build and pass its own tests. Each layer must meet at least 90% patch coverage. CI coverage is authoritative; local coverage reports must state their package scope.

## Scope of the initial PR

`pkg/component/terraform.Provider` implements the existing component interface. The host registers it with an injected runtime. `internal/exec.ExecuteTerraform` becomes the compatibility dispatcher, so single execution, scheduler nodes, migration, and plan comparison all select the provider. The old pipeline is private and reachable through the runtime.

`component.ExecutionContext.Options` carries the existing execution options. The internal `ShellCommandOption` type is an alias, not a second options implementation. Preserve option order, writer identity, the difference between nil and empty environments, and reporting callbacks. The request context is authoritative for execution. This is an in-process host contract. External plugins must use the separate typed protocol and host services; callbacks and writers are not wire payloads.

`GenerateArtifacts` accepts an already resolved configuration, `ConfigAndStacksInfo`, and a working directory. The component package owns the backend, generated-file, provider-override, and required-provider generation sequence. The explicit single and bulk varfile commands also live in this package. The legacy execution pipeline still decides when to prepare its automatic varfile. This method must not initialize Terraform or select a workspace. The complete prepared-component contract belongs to the lifecycle extraction layer.

`Operations` receives an instance of `OperationServices`. The host supplies stack processing, templates, deferred YAML resolution, naming, generated-file processing, and registry execution. Do not install process-global callbacks or introduce a dependency from the component package to `internal/exec`. Keep deferred-context types in `pkg/component` and internal aliases to preserve type assertions across the boundary.

Move complete source files when their dependencies permit it. Extract Terraform-only code from mixed files. Keep compatibility entry points for current callers. Remove unused private wrappers. Co-locate pure behavioral tests with the implementation; keep host integration tests in `internal/exec` and instrument both packages when measuring extraction coverage.

The provider validates the resolved backend type's shape. Existing policy and module validation stay in the execution pipeline, where stack and authentication context are available. Command metadata must not become a passthrough allowlist.

## Extraction acceptance criteria

- The Terraform provider and its interfaces must not import `internal/exec`.
- Each extraction must remove the superseded implementation and move its behavioral tests with it. A second copy does not count as extraction.
- Preserve Terraform/OpenTofu selection, initialization, workspace selection, saved plans, dry-run behavior, hooks, retries, and policy enforcement.
- Preserve single, bulk, query, and affected dispatch, including reverse destruction order.
- Preserve stdout/stderr destination and capture, UI execution, cancellation, detailed exit codes, auth, and execution reporting.
- Preserve inventory identities used by list/describe, state functions, and Atmos Pro.
- Tests that replace the global registry must restore all providers they found.

The runtime adapter remains temporary. Initialization, workspace selection, hooks, policy enforcement, bulk scheduling, UI execution, and state/output resolution still use the host lifecycle. This PR does not complete that extraction or implement an external plugin protocol.


## Remaining extraction boundaries

The remaining Terraform implementation can move. Shared-service calls are dependencies to inject, not a requirement to keep the implementation in `internal/exec`.

| Implementation to move | Files in `internal/exec` | Shared services to inject or reuse |
| --- | --- | --- |
| Single-component lifecycle, argument construction, policy, and UI execution | `terraform.go`, `terraform_execute_helpers*.go`, `terraform_policy.go`, `terraform_streaming_ui.go`, `terraform_test_fixtures.go` | Stack resolution, authentication, process execution, validation, and reporting. Reuse the existing hook, provisioner, policy, UI, and execution-option packages. |
| Bulk, query, and affected execution | `terraform_all.go`, `terraform_query.go`, `terraform_affected.go` | Stack inventory, affected analysis, and dependency parsing. Keep the shared scheduler in its package; move Terraform selection and dispatch into the component package. |
| Output and state lookup | `terraform_output_getter.go`, `terraform_output_utils.go`, `terraform_state_getter.go`, `terraform_state_utils.go`, `terraform_nested_auth_helper.go`, `terraform_mocks.go` | Component description and deferred target authentication. Preserve the resolution context, credential scope, mocks, cache identity, and nested YAML-function behavior. |
| Interactive shell | `terraform_shell.go` | Stack resolution, authentication, and terminal process execution. |
| Remaining selection and lifecycle helpers | `terraform_utils.go` | Move each helper with the operation that uses it; inject shared inventory and naming services. |

`terraform_component_provider.go`, `terraform_operations.go`, `terraform_compat.go`, and `terraform_lookup_options.go` currently provide host wiring or compatibility APIs. These can remain as thin adapters while callers migrate. They must not acquire new Terraform business logic. Remove obsolete wrappers when their callers move.

Move unit tests with the implementation. Keep integration tests that verify the real host adapters in `internal/exec`. A Terraform-named test file in that package does not by itself indicate that implementation still belongs there.
