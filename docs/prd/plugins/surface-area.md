# Extension surface map

**Status:** Scope guidance for [Atmos plugins](overview.md). An internal registry is an integration point, not automatically a public contract. [Extension PRDs](extensions/README.md).

## Full platform inventory

Authentication, component engines, CI providers, commands, validation/policy/linting, hooks, steps, MCP tools, skills, stores, secret providers, and backend provisioning form the full platform inventory. The [first release](first-release.md) ships Go auth provider/identity adapters and commands; the other production adapters are follow-ons. One package may provide several supported types. Registries receive external runtime adapters; their internal Go interfaces do not become the public protocol.

## Defined follow-ons

| Surface | Existing foundation | Use and boundary |
|---|---|---|
| [Starlark functions](extensions/functions.md) | [Embedded engine](../../../pkg/script/starlark/engine.go) | Typed `alias.function(...)` calls backed by Go SDK or packaged Starlark implementations; separate from YAML/HCL bindings. |
| [Package configuration](package-configuration.md) | Existing configuration loader and plugin lifecycle | Automatic OCI-delivered commands and Git hook definitions, including packages that also provide native services. |
| Generators | [Registry](../../../pkg/generator/registry.go) | Shared generated files and organizational overlays. Component-specific generation already belongs to component extensions; generic generators need ordering and file ownership. |
| YAML functions/data lookups | [Registry](../../../pkg/function/registry.go) | Inventory and naming lookups during evaluation. Define sensitive/unknown values, caching, phases, and cycles first. Existing store/secret functions already reach plugin backends. |
| Import sources/transports | [Adapters](../../../pkg/config/import_adapter_registry.go) | Enterprise configuration catalogs. Bootstrap must locate plugins before imports need them. |
| Artifact-delivery targets | [Target registry](../../../pkg/provisioner/target/target.go) | Deliver generated artifacts to destinations such as Git/Kubernetes. State-backend provisioning belongs to the platform inventory above. |
| Tool metadata/version resolution | [Tool registries](../../../pkg/toolchain/registry/registry.go), [version resolvers](../../../pkg/version/resolver/resolver.go) | Enterprise tool distribution. Initial plugins can require existing tools without implementing a registry. |
| Git/PR providers | [Git registry](../../../pkg/git/registry.go), [PR publishers](../../../pkg/git/pull_request.go) | Source hosts independent of the CI runner; CI selection does not select a Git provider. |
| Artifact storage/caches | [Artifacts](../../../pkg/ci/artifact/registry.go), [caches](../../../pkg/ci/cache/registry.go) | Blob retention and restore semantics differ from key/value stores. |
| Script engines/emulators | [Scripts](../../../pkg/script/registry.go), [emulators](../../../pkg/emulator/driver.go) | Additional languages and environments. Current runtimes and emulator-calling steps cover initial needs. |
| AI model providers | [Registry](../../../pkg/ai/registry/registry.go) | Inference transports, streaming, and accounting; separate from MCP tools and skills. |
| Auth integration adapters | [Registry](../../../pkg/auth/integrations/registry.go) | Additional credential consumers beyond provider/identity chains. Existing environment preparation stays supported. |
| Standalone formatters | [Hook format handlers](../../../pkg/hooks/format.go) | New output formats; initial extensions use shared diagnostics/artifacts and Atmos rendering. |

These are candidates, not initial-release commitments. The [gap analysis](platform/gap-analysis.md#follow-on-surfaces) records each surface's current registry and duplicate policy.

## Composition before adding another extension type

Notifications, ticketing, and release operations can usually be commands, hooks, or steps. Checks can be validators. A new extension type is justified when Atmos must own a distinct lifecycle or typed interaction.

Bootstrap configuration and package metadata remain readable without plugin code. Plugins cannot redefine core merge semantics, replace built-in validation, or weaken Atmos trust policy. Every future contract needs activation, scope, compatibility, failure behavior, and tests.
