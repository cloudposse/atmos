# PRD: Independently Versioned Atmos Plugins

**Status:** Native Go pilot implemented in the working branch; not yet released. See the [first-release checkpoint](first-release.md#native-pilot-checkpoint) for the implemented boundary. Broader extension/runtime requirements below remain follow-on designs.

**Related:** [Component registry](../../developing-component-plugins.md), [command registry](../command-registry-pattern.md), [Starlark automation](../starlark-automation-and-command-testing.md).

**Related issue:** [#261 — Documented ADRs for Atmos](https://github.com/cloudposse/atmos/issues/261) explicitly proposes a future plugin-architecture decision record. It is architectural history, not a dedicated tracking issue for this implementation.

These documents define the target product and its acceptance criteria. The native auth/command implementation is available in this branch; it does not imply that every broader example or proposed SDK symbol is implemented. Existing `atmos auth` commands are the consumer entry point for plugin authentication.

**Reading guide:** [Package composition](package-composition.md), [package configuration](package-configuration.md), [Starlark functions](extensions/functions.md), [extension PRDs](extensions/README.md), [user experience](user-experience.md), [authoring](authoring.md), [distribution](distribution.md), [toolchain dependencies](toolchain-dependencies.md), and [testing](testing.md). The [platform workstream](platform/README.md) covers what must be built: [gap analysis](platform/gap-analysis.md), [interfaces](platform/interfaces.md), [protocol](platform/protocol.md), [configuration](platform/configuration.md), [errors](platform/errors.md), [decisions](platform/decisions.md), and the [delivery plan](platform/delivery-plan.md). See the [index](README.md) for the full document set.

## Summary and goals

Enable customers, partners, and Atmos maintainers to develop, discover, install, and release plugins independently of the Atmos CLI.

The [first release](first-release.md) delivers **every `atmos plugin` command, a working Go OIDC plugin, complete author/consumer documentation, and the mechanisms for extending the platform to the other Atmos primitives**. Authentication and executable command adapters are implemented first. The remaining extension contracts stay designed and inventoried; their adapters and Starlark/data runtimes follow later. Packages use public/private OCI registries, with optional catalogs or cached repository discovery, with all lifecycle operations available through Atmos.

The immediate customer outcome is an enterprise implementing its own Go OIDC authentication provider without forking Atmos, compiling a custom binary, or waiting for an Atmos release.

The broader outcome is an extension platform where integrations evolve on their own release schedules. Independent releases remain subject to compatibility with Atmos's supported contracts; new Atmos capabilities can still require an Atmos release.

### Initial release boundaries

- Complete the auth provider/identity registry refactor and credential-type registry before accepting external auth.
- Deliver every command in the [first-release inventory](first-release.md#complete-command-inventory), backed by working behavior.
- Ship the [Go OIDC pilot](oidc-pilot.md), executable command support, and shared extension registration/adapter mechanisms.
- Retain all contracts in the [extension index](extensions/README.md); production adapters beyond auth/commands and Starlark/data runtimes are follow-ons.
- Preserve existing built-in implementations and configuration.
- Model future component extensions as execution engines, such as an integration with a new infrastructure tool.
- Support project-managed plugins and user-installed plugins.
- Defer extraction of existing built-ins and a hosted marketplace. Define additional [follow-on surfaces](surface-area.md) separately.
- Keep infrastructure component content distribution separate from executable plugin distribution.

### Terminology

A **plugin** is an installable, independently versioned package managed by `atmos plugin`. An **extension** is functionality the package exposes to an Atmos surface. One package can provide commands, identities, MCP tools, skills, and other extensions together. `capabilities` remains the manifest field for declared extension contracts and optional features; extensions do not introduce separate packages or lock entries.

## Foundation and architectural direction

Atmos's existing registries are the integration points, but they are uneven. The [gap analysis](platform/gap-analysis.md) inventories each one: most replace silently on duplicate registration, two live in Go-internal packages, three interfaces carry no `context.Context`, and several surfaces still dispatch through closed lists rather than a registry. Terraform, Helmfile, and Packer are not registered as component engines; validation has no engine registry; credentials have no versioned type.

Auth now dispatches built-in and plugin provider and identity constructors through `pkg/auth/registry`. Preserve existing configuration and behavior as additional auth adapters are introduced.

The native pilot supplies external loading, versioned contracts, package lifecycle management, discovery, and compatibility enforcement. Validate the first-release auth, credential, manifest/command, and API foundations together with the OIDC implementation before splitting the change for review, as recorded in the [delivery plan](platform/delivery-plan.md). Other registry hardening accompanies its respective follow-on adapter.

New integration features should follow these extension contracts and registries by default. The [architecture policy](architecture-policy.md) defines design/review requirements and when a core addition is appropriate.

Distinguish three concepts throughout the product and documentation:

| Concept | Responsibility |
|---|---|
| Capability registry | Connects an implementation to Atmos behavior. |
| OCI registry and catalog | Stores packages and makes available plugins discoverable. |
| Plugin runtime | Loads an implementation and invokes its supported capabilities. |

### Reference models

Terraform demonstrates independently distributed providers communicating through a versioned protocol, with dependency locking for reproducible selection. Atmos should adopt these principles without implementing Terraform's provider protocol. [Terraform plugin protocol](https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol), [dependency locking](https://developer.hashicorp.com/terraform/language/files/dependency-lock).

GitHub CLI extensions add commands by forwarding arguments to separately installed executables. Extensions can be scripts or precompiled binaries. This is the appropriate model for standalone Atmos commands; deeper integrations need structured capability contracts. [GitHub CLI extensions](https://cli.github.com/manual/gh_extension), [extension authoring](https://docs.github.com/en/github-cli/github-cli/creating-github-cli-extensions).

The [reference-model analysis](reference-models.md) adds lessons from Helm and TFLint and explicitly excludes custom Atmos builds such as `custom-gcl`. The [future migration plan](builtin-migration.md) targets integration packages for Terraform, Helm, Ansible, and Kubernetes.

### Execution model

- Native integration plugins run as managed subprocesses using an Atmos-owned, versioned Protobuf/gRPC protocol carried by `hashicorp/go-plugin`, which supplies the handshake, protocol-version negotiation, mutual TLS, and the broker for API services. A Wasm runtime is a declared follow-on, not part of the initial release. See the [protocol](platform/protocol.md) and [Decision 1](platform/decisions.md).
- The Go Plugin SDK implements the protocol and provides authoring helpers and bindings to the [Atmos API](../atmos-sdk.md) without requiring imports of Atmos internals. It lives in this repository and versions in lockstep with Atmos.
- Starlark plugins run through the existing embedded runtime, with bindings for the same capability contracts.
- Data rule packs use registered validation engines; skill files use the existing loader. Neither requires a native plugin process.
- Standalone commands may use ordinary executable dispatch or Starlark; executable commands do not need to implement RPC.
- One package may expose multiple extensions through shared implementation code; see the [Okta example](package-composition.md).
- Native authors distribute precompiled platform artifacts. Consumers never compile plugins or rebuild/replace Atmos.
- Package installation does not execute publisher setup hooks; dependencies use the existing toolchain.
- Internal registry adapters bridge these runtimes into existing Atmos interfaces. Internal Go structs, callbacks, and managers are not the public protocol.

## Product requirements

### Extension types

| Category | Required behavior |
|---|---|
| Authentication | Register provider and identity kinds; validate configuration; authenticate; participate in identity chains; handle expiration, credential reuse, environment preparation, credential paths, identity reporting, and logout. |
| Component engines | Register component types and commands; participate in discovery, stack resolution, validation, artifact generation, execution, and lifecycle events. |
| CI providers | Supply platform detection and execution context, base resolution, outputs, summaries, checks, and comments. Additional features use explicit capability declarations. |
| Standalone commands | Add discoverable commands with help, arguments, output streams, cancellation, and exit-status propagation. |
| Policy, linting, and validation | Distribute rule packs and custom validators; integrate with validation commands and explicit lifecycle gates; report common diagnostics. |
| Hooks | Register lifecycle kinds; retain Atmos event matching, conditions, outcomes, failure policy, and reporting. |
| Steps | Register reusable tasks for workflows, commands, scripts, and step-backed hooks with shared execution semantics. |
| MCP tools | Expose schema-defined tools through Atmos's MCP server and existing permissions/executor path. |
| Skills | Package instructions/resources through the existing skill loader and explicit client distribution. |
| Stores | Register typed key/value backends with optional raw reads, deletion, existence checks, and enumeration. |
| Secret providers | Register sensitive backends with scope, masking, identity, status, and supported-operation contracts. |
| Backend provisioning | Register state-backend infrastructure operations through explicit commands and configured lifecycle execution. |

Each type has a [focused extension PRD](extensions/README.md). Auth and executable commands gate the first release; the remaining acceptance criteria gate their respective follow-on implementations.

Component-specific CI reporting is distinct from CI platform providers. Plugins must also be able to supply reporting hooks for their component engines, so adding an engine does not require core changes to report its results.

### Authentication requirements

- Atmos retains responsibility for chain orchestration, credential isolation, persistence policy, and secret masking.
- Credentials cross the plugin boundary through versioned data contracts. Existing credential formats receive adapters; plugin-specific session data remains namespaced to its owning plugin.
- Compatible plugin-produced credentials can feed built-in identities. Unsupported credential combinations fail with an actionable error.
- Interactive authentication supports Atmos-mediated prompts and browser flows. Noninteractive execution fails clearly when interaction is required.
- Provider and identity schemas are supplied by the plugin rather than added to a hardcoded list in Atmos.

### Starlark requirements (follow-on)

- All executable extension types support Starlark and native protocol implementations. Skills and rule packs also support portable data packages.
- Reuse existing cancellation, invocation isolation, output, process execution, filesystem, and tool-pinning services.
- Add provider callback bindings and API services needed for HTTP requests, authentication interaction, and credential handling.
- Version the exposed Starlark plugin API alongside the capability contracts.
- Keep package-local imports reproducible; do not fetch executable code during evaluation.

### OCI distribution and discovery

Users configure named discovery sources containing explicit OCI repositories, published catalog snapshots, or both. Multiple sources can coexist, including catalogs per product. The native pilot supports project configuration; user-wide configuration remains a follow-on.

OCI standardizes listing tags within a known repository, but does not provide a portable registry-wide plugin search API. Discovery therefore starts from either an explicit repository list or an **Atmos catalog artifact stored in OCI**, without depending on repository enumeration. [OCI distribution specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#content-discovery).

Required behavior:

- Each source may identify an optional catalog snapshot and/or explicit OCI repositories. A host-only entry provides authentication without discovery.
- Catalog entries include plugin identity, description, publisher, capabilities, runtime, package location, and release compatibility metadata.
- Support search by name or description and filters for source, capability, runtime, and compatibility. An empty query lists available plugins.
- Results display fully qualified package identity, available versions, and installed status. `list`/`info` show extensions, runtime, scope, source, digest, and verification status, distinguishing integrity from verified publisher identity.
- Use shared `pkg/list` rendering for `search`, `list`, `info`, and `registry list`, with `--format table|json|yaml|csv|tsv` and `ATMOS_PLUGIN_FORMAT`. Interactive installed rows use the same green status dot as auth/toolchain views; status words remain readable without color and machine output contains no terminal decoration.
- Fetch only metadata for discovery; never execute plugins to inspect them.
- Support authenticated sources, a local index with `plugins.discovery.cache_ttl` (default `15m`), `search --refresh`, and cached offline discovery. Reuse metadata by immutable digest and bound source concurrency. See [Discovery](discovery.md).
- Report inaccessible sources separately from empty results. A failed source fails the search with an explicit error; do not silently return partial results.
- Allow direct installation by OCI reference even when the package is absent from a catalog.
- Provide catalog generation and publication tooling. Registry owners publish catalogs from a declared package inventory through a serialized publication workflow.

Catalog inclusion and cached repository results are discovery metadata, not publisher verification. Source policy applies on cache hits; installation validates the referenced package independently.

### Atmos-managed OCI lifecycle

Atmos provides the complete plugin lifecycle without requiring users to invoke a separate OCI client. OCI artifacts contain plugin packages; installing or running a plugin does not require a container runtime.

| Operation | Required behavior |
|---|---|
| Create | Scaffold and validate a plugin, package its metadata and payload, and publish a named release to a public or private OCI repository. |
| Read | Discover available plugins, inspect release metadata and supported platforms, and retrieve packages by version or digest. |
| Update | Publish a new release independently. Direct indexes discover it on refresh; catalog maintainers add it in a separate workflow. Consumers explicitly update their selected version and lockfile. |
| Delete | Delete an explicitly selected remote release and remove its catalog entry. Keep remote deletion separate from local uninstall. |

- Publishing validates the package, declared compatibility, and artifact digests before advertising the release in a catalog. Release metadata includes its immutable digest.
- Published release versions are immutable through Atmos. Publishing different contents under an existing release version fails; authors publish a new version. An identical repeat publication is safe; catalog maintenance is separate.
- Release publication never mutates a catalog. Explicit repository indexes discover it on refresh; catalog maintenance uses separate `catalog` commands.
- Catalog edits support adding, updating, and removing entries independently of deleting their artifacts. Removing a catalog entry unlists a release; it does not delete the release.
- Remote deletion resolves and displays the exact release and digest, requires confirmation interactively, and requires an explicit confirmation flag in automation. It does not implicitly delete every release of a plugin or an entire repository.
- If registry manifest deletion also removes tags pointing to the same digest, report the affected references before deletion. If the registry forbids deletion, report that limitation; unlisting remains an explicitly separate operation. OCI permits registries to disable deletion. [OCI content management](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#content-management).
- Package and catalog writes are not assumed to be atomic. Publish the artifact before listing it; unlist a release before deleting its artifact. Report partial completion and provide a retry that reconciles the remaining operation without overwriting unrelated catalog entries.
- Catalog maintainers externally serialize the entire read/update/publish operation. Revision/digest rechecks catch observed conflicts but do not eliminate simultaneous-writer races or provide atomic compare-and-swap. Only the catalog workflow should have write credentials for its catalog tag.
- Apply lifecycle operations to the explicitly selected catalog and registry. Do not claim to update third-party catalogs that also reference the release.
- Deleting a remote release does not remove local installations or rewrite consumer lockfiles. Explain that fresh installation and rollback require the locked artifact to remain available in a registry, cache, or offline copy.
- Distinguish read, publish, and delete authorization failures. Registry access uses existing credentials or credential helpers, with no credentials embedded in packages, catalogs, or lockfiles.

### Installation and version management

Provide a single `atmos plugin` command family:

| Operation | Purpose |
|---|---|
| `registry add`, `registry list`, `registry remove` | Configure named discovery sources in project or user configuration. |
| `registry login`, `registry logout` | Manage OCI access using credentials outside project configuration. |
| `search`, `info` | Discover packages and inspect versions, capabilities, and compatibility. |
| `install`, `list`, `remove` | Manage local installations and show their effective scope and source. |
| `update` | Explicitly select and lock a newer compatible version for a consumer. |
| `run` | Invoke an installed command through an unambiguous plugin-qualified path. |
| `create`, `build`, `validate`, `pack`, `publish`, `sign` | Scaffold, compile, check, package, publish, and sign immutable plugin releases; `publish --sign` combines publication and signing. |
| `test` | Run mocked capability-contract scenarios or explicitly selected live integration tests. |
| `delete` | Delete an explicitly selected remote OCI release. |
| `catalog build`, `catalog publish` | Generate and publish discovery catalogs in OCI registries. |
| `catalog add`, `catalog remove` | Add or update a release entry, or unlist it, in a configured OCI catalog. |

- Project configuration declares plugin sources and version requirements; a dedicated plugin lockfile records exact versions and immutable artifact digests.
- User installations maintain equivalent version records outside the project.
- Project selections take precedence. A missing or incompatible project plugin never silently falls back to a user-installed version.
- Project automation activates declared plugins; unrelated user-installed integration providers do not alter project behavior.
- Installation supports public/private OCI sources, local development packages, and offline OCI layouts.
- Native packages distribute compiled artifacts via an OCI platform index as specified in [distribution](distribution.md). Starlark/data packages declare runtime/API or engine requirements.
- Ordinary commands automatically install declared project plugins when needed. First use creates missing lock entries; existing entries pin restoration to immutable digests. Cached use requires no registry access, and upgrades remain explicit.
- A frozen installation mode restores the lockfile exactly for CI.
- Rollback restores the previous declaration and lockfile, then reinstalls the recorded artifacts.
- Plugins declare required toolchain tools. Explicit installation restores them through the existing manager; plugin locks reference exact entries in the configured toolchain lockfile. See [toolchain dependencies](toolchain-dependencies.md).

### Organizational plugin policy

The existing policy engine governs plugin admission through `on: plugin` rules in protected `policy.d/` or `ATMOS_POLICY_DIR`. CEL and restricted Starlark can restrict actual registry authorities, repositories, inspected package names, versions, runtimes, and registered capability IDs. Apply source checks before access and package checks before installation or activation, including automatic downloads and cached use. Catalog membership is not authorization. Local development and layout imports have explicit origins and require policy exceptions when only company registries are permitted. General policy admission cannot override digest or trusted publisher checks. See [Plugin policies](policy.md) for the complete input, enforcement points, examples, and acceptance tests.

### Compatibility, trust, and failure behavior

- Version plugin releases, package manifests, and capability contracts independently. The Plugin SDK version number follows Atmos in lockstep.
- Negotiate compatibility before invoking an integration. Additive optional capabilities must not force unrelated plugin upgrades.
- Publish supported protocol versions and deprecation policy with each Atmos release.
- Static package metadata supplies discovery, command help, and compatibility information without starting plugins.
- Installed plugins cannot silently replace built-ins or YAML custom commands. Conflicting command shortcuts remain unavailable; qualified plugin invocation remains available.
- Duplicate extension registrations fail clearly, including provider/identity kinds, engines, validators, hook kinds, step names/aliases, tools, skills, stores, secret providers, and backend provisioners. Explicit CI provider selection wins; ambiguous automatic detection requires selection.
- Verify artifact digests on installation and cache use. Support optional signature verification and machine/CI policy requiring trusted publishers; project configuration cannot weaken that policy.
- Registry authentication must work before the requested auth plugin is installed. Detect and explain authentication dependency cycles.
- Load integrations only when needed. Propagate cancellation and deadlines, clean up subprocesses, and report plugin identity/version with failures.
- Do not automatically retry operations that may have changed infrastructure or external state. Explicit retries of package/catalog operations reconcile their reported partial state.
- Native execution and Starlark SDK operations are trusted code execution. Process separation and capability declarations are not an operating-system sandbox.

## Delivery and public interfaces

Deliver the [first-release scope](first-release.md) before the remaining adapters. Phase numbers organize work packages; they do not require completing all surfaces before authoring, documentation, and pilot validation. The [delivery plan](platform/delivery-plan.md) breaks each milestone into work packages with dependencies and done-criteria.

0. Registry hardening on `main`: complete the auth registry, add a credential-type registry, make duplicate registration an error, plumb `context.Context` through stores, secret providers, and hooks, export the Go-internal CI types, register the legacy component engines, make CI detection deterministic, and extract a validation engine registry. The auth and shared foundations gate the first release; the rest lands with the relevant adapters. Refactors preserve built-in behavior.
1. Package manifests, Atmos-managed OCI lifecycle, catalogs, discovery, local installation, lockfiles, and compatibility checks.
2. Native protocol, Go SDK, API broker, extension descriptor registry, and auth/executable-command adapters.
3. First-release scaffolding, every plugin command, complete documentation, and Go OIDC pilot validation.
4. Follow-on adapters, Starlark/data runtimes, parity tests, and their templates/documentation.

Public additions include plugin configuration and lockfiles, package/catalog manifests, capability schemas, protocol definitions, Go SDK, Starlark bindings, and the plugin CLI.

Existing built-in configuration remains valid. New external provider and component kinds become valid through registered plugin metadata. Preserve existing YAML custom-command semantics.

Ship runnable Go OIDC and executable-command examples first. Go/Starlark examples for other executable extension types and data-only examples accompany their follow-on adapters. Examples and documentation must distinguish existing compiled-in registration from externally installable plugins.

## Acceptance criteria

The first release is complete when the four outcomes and prerequisite gates in [First release](first-release.md) pass, including the [OIDC pilot](oidc-pilot.md) and [shared OCI regression/round-trip tests](platform/oci-reuse.md#verification-and-delivery). Starlark parity and the remaining production adapters do not gate it.

The following checklist retains the full-platform targets (`A1`–`A21` in the delivery plan). Items requiring future adapters or runtimes apply when those ship:

- Every [extension PRD](extensions/README.md) satisfies its acceptance criteria.
- One package supplies commands, authentication, MCP tools, and skills with a single install/version/lock entry and unified update/rollback.
- Compiled multi-platform installation works without Go, Git, or a compiler and leaves the standard Atmos binary unchanged.
- Required tools restore through the toolchain with consistent locks, scoped executable paths, offline/frozen behavior, and conflict diagnostics.

- The enterprise pilot can author, publish privately, discover, install, and use its own auth provider without changing or rebuilding Atmos.
- A new component engine works through stack resolution, execution, lifecycle events, authentication, and CI reporting.
- An external CI provider can report execution results through its declared capabilities.
- An executable command and a Starlark command appear in help and preserve arguments, output, cancellation, and exit status.
- Go and Starlark implementations pass the same applicable capability-contract tests.
- Rule packs and custom validators satisfy the [validation extension acceptance criteria](extensions/validation.md#acceptance-criteria), including common output, explicit activation, required-check failures, and positive/negative rule fixtures.
- A plugin author follows the scaffold-to-publication walkthrough in a separate repository, and a consumer follows the install-to-login walkthrough using only a released compatible Atmos binary.
- The generated test suite verifies the complete Go OIDC login/exchange/reuse/refresh/logout flow against a mock issuer and broker without live credentials; opt-in integration results remain distinguishable from default contract results.
- A plugin update and rollback work against an unchanged compatible Atmos binary.
- Fresh developer and CI environments restore identical locked artifacts; offline restoration works from supplied artifacts.
- Discovery works against an OCI registry without repository enumeration, including private access, filtering, stale caches, and partial source failures.
- Using only Atmos commands, an author can create, validate, package, publish, inspect, and delete a release in a test OCI registry; publish a second version; and add, update, or remove catalog entries.
- Consumer `update` changes the selected version and lockfile; publisher `publish` creates a release; local `remove` leaves remote artifacts intact; remote `delete` leaves local installations and lockfiles intact.
- Lifecycle tests cover immutable version conflicts, insufficient permissions, disabled registry deletion, multiple tags sharing a digest, catalog conflicts, interrupted publication/deletion, and successful reconciliation of partial operations.
- Tests cover incompatible protocols, unsupported platforms, moved tags, corrupted artifacts, signature-policy rejection, registration collisions, interrupted installs, crashes, and cancellation.
- Auth tests cover built-in/plugin identity chaining, expired credentials, separate realms, noninteractive failures, logout, and secret redaction.
- Projects without external plugins preserve existing behavior and do not incur plugin subprocess launches or registry network requests.

The initial release establishes independent delivery for new plugins. Subsequent phases add the remaining adapters and runtimes; eventual extraction follows the [built-in migration plan](builtin-migration.md); extraction order and default distribution remain follow-on decisions.
