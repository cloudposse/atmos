# Component configuration and engine plugins

**Status:** Implementation in progress. The protocol, SDK, native runtime clients, static component descriptors, explicit package loader, and configuration-service adapters exist. CLI discovery, stack adaptation, and execution integration are not implemented.

**Decision:** Separate component type, configuration provider, and execution engine. Negotiate optional capabilities independently. A configuration provider must not need an executor.

See the [Terragrunt requirements](../../terragrunt-support.md) for the first consumer and [all extension types](README.md) for the platform scope.

## Required behavior

A configuration plugin supplies ordinary component instances from another format. The host must use these instances in selection, inventory, dependencies, affected detection, state reads, and execution.

For native Terragrunt adoption, the type is `terraform`, the configuration provider is the Terragrunt plugin, and the engine is Terraform or OpenTofu. Atmos owns execution, scheduling, saved plans, UI, and native Terraform CI reporting. Executable mode is separate and uses engine `terragrunt`. Never select it as an automatic fallback.

## Reuse and ownership

Terraform is registered through [`pkg/component/terraform`](../../../../pkg/component/terraform/provider.go). Its implementation has moved out of `internal/exec`; the remaining execution adapter supplies existing host services. Reuse this lifecycle, source provisioning, backend access, scoped authentication, toolchain, dependency graph, bulk executor, and CI reporting.

Reuse the merged plugin SDK, static manifest validation, OCI releases, lock files, policy, brokered APIs, and process lifecycle. Extend the descriptor registry when its host adapter becomes available. A protobuf service alone must not advertise an installable capability.

The component registry currently allows replacement registrations. External plugins must not use that behavior to replace built-in Terraform. Add explicit ownership and collision checks at the adapter boundary. Duplicate provider identities or component coordinates must fail before any registration becomes visible.

## Independent contracts

The checked-in [protocol](../../../../proto/atmos/plugin/v1/plugin.proto) is authoritative. The first component services use contract version 1.

| Capability | Service | Responsibility |
|---|---|---|
| `component.discovery` | `ComponentDiscovery.Discover` | Resolve imports or discover logical instances, source watches, and dependency edges in the requested root/revision. |
| `component.validation` | `ComponentValidation.Validate` | Reject unsupported operations, modes, and provider options before effects. |
| `component.preparation` | `ComponentPreparation.Prepare` | Reevaluate one selected unit before its operation; return inputs, module, state identity, and literal generated files. |
| `component.state` | `ComponentState.Resolve` | Resolve backend/workspace identity without preparation or execution. |

The following contracts still require implementation:

- Static metadata for modes, commands, and typed options. Component family, provider export name, package-local settings schema, and service version are implemented.
- Optional execution with stdout, stderr, diagnostics/logs, lifecycle events, and one authoritative outcome.
- Scoped host process, filesystem, toolchain, and logging APIs. The existing broker does not provide these services yet.
- External engine CI reporting. Native Terraform adaptation retains the existing Terraform CI path.

Do not transmit Go callbacks, writers, `ExecutionContext`, `AtmosConfiguration`, or `ConfigAndStacksInfo`. Send explicit coordinates, values, paths, operation context, and granted effects.

### Values and artifacts

`ComponentValue` carries cty type JSON, cty MessagePack, and sensitivity paths. Preserve exact numbers, collection types, nulls, nested unknowns, and marks. Never use `protobuf.Struct` for infrastructure values. Reject unsupported marks and invalid sensitivity paths. Error messages must not contain payloads.

The SDK provides `component.Encode` and `component.Decode`. Translate language-specific sensitivity marks deliberately. The host must decode to its own typed values and masking rules. A byte round trip does not establish secret-safe rendering.

Generated content is evaluated bytes, not a second template. Validate destination paths, ownership policies, and write scope. Reject unknown ownership policies. Saved plans must bind exact artifact bytes, provider version, source revision, effective options, state identity, and engine. Verify these before apply; never replace reviewed inputs by reevaluating them.

### Discovery and state

Installation must not enable discovery. Disabled providers must cause no startup, downloads, or directory scans during ordinary commands.

Add discovered definitions before commands reject projects without YAML stacks. Feed imports and discovery into the same resolved stack map. Reuse naming precedence and stable logical coordinates. Cache paths must not become instance names.

Discovery defaults to no commands, network, secret decryption, writes, or state reads. Host API grants must enforce the policy. These restrictions do not sandbox trusted native code at the operating-system level. Report denied effects; never omit instances or substitute empty values.

State resolution must work before plan/apply without hooks or generated files. Return the upstream working directory because relative backend paths can refer to a location other than Atmos's execution directory.

### Flags and command ownership

Use static metadata for help without starting plugins. Use `pkg/flags.NewStandardParser` and CLI > environment > configuration > default precedence.

- Declare option types, defaults, aliases, applicable modes/commands, deprecation, and sensitivity.
- Preserve absent versus explicit false, zero, empty string, and empty list.
- Atmos owns selection, `--affected`, `--ui`, and native scheduling. Reject collisions.
- Keep provider options, Terraform argv, and explicit external-engine argv separate.
- Validate plugin semantics before discovery/preparation effects.
- Apply discovery-relevant options consistently to both revisions, preparation, and applicable state resolution.
- Include effective options in cache and saved-plan identity. Never log sensitive values.
- Reject unsupported native options. Executable passthrough must be explicit and must not create a second scheduler.

`ComponentOperation` currently transmits resolved provider options. It does not implement parsing or authorize arbitrary passthrough.

## Host integration

`pkg/plugin/adapter/component` supplies discovery, validation, preparation, and state clients. Its immutable registry is private to the invocation. It does not register or replace execution engines. Compose these providers with registered engines in the stack and execution adapters. Keep business logic out of `internal/exec`.

Open remaining closed type lists through registry metadata: `schema.Components.GetComponentConfig`, component auto-detection, stack component processing, component/reverse base paths, list extraction, `list sources`, stack-section validation/inheritance, dependency/affected dispatch, and CI dispatch.

`bootstrap.LoadComponents` accepts explicit capability-qualified provider references. It uses the existing package policy, OCI cache, lock, and restoration paths. An empty reference list does not inspect package manifests. Disabled packages cannot satisfy references. Schema validation and ownership checks precede native startup; a failed batch returns no registry. Activation does not call discovery or execute the engine.

Declare each service under `spec.capabilities.components.discovery`, `.validation`, `.preparation`, or `.state`. Each export declares `component_type` and a package-local `schema`. All capabilities for one provider must agree on package identity, executable, schema, type, and negotiated description. Keep provider settings separate from resolved component inputs in every request.

Connect this explicit loader to configuration and command contexts. Preserve lazy activation for unrelated operations. Do not add a process-global discovery callback or cache plugin results without provider/options/revision identity.

Distinguish file/directory/glob watches, output dependencies, and ordering-only edges. Compare each Git revision in its own root. Include additions, removals, and renames; evaluating only current HCL is insufficient.

Cancellation must stop invocations, processes, callbacks, and descendants. Test deadlines, caller cancellation, interrupts, crashes, and bounded output/backpressure. Never retry apply automatically after a lost connection.

## Delivery and acceptance

| Layer | Required result | State |
|---|---|---|
| Protocol and SDK | Independent services, lossless values, optional registration, native transport tests. | Implemented in this branch; not activated by installation. |
| Host activation | Static descriptors, provider settings schemas, private registry, package selection, and service adapters. | Implemented; not connected to CLI stack processing. |
| Host composition | Flags, scoped services, discovery/import and engine adapters. | Required. |
| Terragrunt plugin | Separately built SDK package runs the advanced native fixture on stock Atmos. | Required. |
| Integration | Lists, state functions, affected, dependencies, UI, saved plans, and Pro share instances. | Required. |
| Compatibility | Differential tests for supported constructs and explicit failures for unsupported behavior. | Required. |

Require at least 90% patch coverage per layer. Test failures as well as successful calls. A protocol fixture does not prove installed-plugin or Terragrunt compatibility.

The first product milestone is an installed Terragrunt plugin running the advanced native fixture through stock Atmos without snapshot patches or temporary callbacks. Port the pilot's field regressions before removing its temporary hooks.
