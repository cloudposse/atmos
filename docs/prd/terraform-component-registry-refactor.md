# Terraform component registry refactor

## Objective

Route Terraform and OpenTofu operations through the component registry. Move Terraform lifecycle ownership out of `internal/exec` in reviewable steps. Preserve existing commands, configuration, state addresses, execution order, and reporting.

This work supplies reusable host interfaces for the separate Terragrunt integration. It does not depend on the environment registry initiative or include the Terragrunt prototype.

## Stack sequence

1. **Provider and execution boundary.** Register a built-in Terraform provider. Make the existing execution entry point select that provider. Move execution options into `pkg/component`. Inject the legacy runtime without importing `internal/exec` from the provider. Cover single and scheduled dispatch, output capture, cancellation, and error identity.
2. **Terraform lifecycle extraction.** Move orchestration and preparation into `pkg/component/terraform`. Inject configuration loading, authentication, stack resolution, process execution, and reporting services. Remove the corresponding implementations from `internal/exec`; retain compatibility entry points only where callers still need them.
3. **Component capabilities.** Define independent discovery, preparation, state resolution, and provenance capabilities. Inventory and state reads must not run the execution lifecycle. Keep component family separate from configuration format and selected executable.
4. **External plugin adapter.** Extend the existing plugin PR stack with these capabilities. Preserve the host scheduler, cancellation, scoped credentials, streams, and structured results across the process boundary.
5. **Terragrunt integration.** Replace the prototype's injected callbacks with those capabilities. Native HCL units remain Terraform component instances and use the same execution lifecycle as native YAML instances.

Each layer must build and pass its own tests. Each layer must meet at least 90% patch coverage. CI coverage is authoritative; local coverage reports must state their package scope.

## Initial layer

`pkg/component/terraform.Provider` implements the existing component interface. The host registers it with an injected runtime. `internal/exec.ExecuteTerraform` becomes the compatibility dispatcher, so single execution, scheduler nodes, migration, and plan comparison all select the provider. The old pipeline is private and reachable through the runtime.

`component.ExecutionContext.Options` carries the existing execution options. The internal `ShellCommandOption` type is an alias, not a second options implementation. Preserve option order, writer identity, the difference between nil and empty environments, and reporting callbacks. The request context is authoritative for execution. This is an in-process host contract. External plugins must use the separate typed protocol and host services; callbacks and writers are not wire payloads.

`GenerateArtifacts` accepts an already resolved configuration, `ConfigAndStacksInfo`, and a working directory. In this layer it exposes the existing backend, generated-file, provider-override, and required-provider generation step. Varfile generation remains in the existing preparation pipeline. This method must not initialize Terraform or select a workspace. The complete prepared-component contract belongs to the lifecycle extraction layer.

The provider validates the resolved backend type's shape. Existing policy and module validation stay in the execution pipeline, where stack and authentication context are available. Command metadata must not become a passthrough allowlist.

## Extraction acceptance criteria

- The Terraform provider and its interfaces must not import `internal/exec`.
- Each extraction must remove the superseded implementation and move its behavioral tests with it. A second copy does not count as extraction.
- Preserve Terraform/OpenTofu selection, initialization, workspace selection, saved plans, dry-run behavior, hooks, retries, and policy enforcement.
- Preserve single, bulk, query, and affected dispatch, including reverse destruction order.
- Preserve stdout/stderr destination and capture, UI execution, cancellation, detailed exit codes, auth, and execution reporting.
- Preserve inventory identities used by list/describe, state functions, and Atmos Pro.
- Tests that replace the global registry must restore all providers they found.

The initial runtime adapter is temporary. Registration alone does not complete the lifecycle extraction.
