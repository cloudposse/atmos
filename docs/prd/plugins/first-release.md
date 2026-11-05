# Plugin first release

**Status:** Accepted scope; native Go pilot implemented, with full-release acceptance still in progress. [Overview](overview.md) · [Delivery plan](platform/delivery-plan.md).

The first release delivers four outcomes: the complete `atmos plugin` command family, a working Go OIDC plugin, documentation for authors and consumers, and reusable mechanisms for adding the remaining Atmos extension types. This document defines the release boundary; the broader extension PRDs retain their designs and follow-on acceptance criteria.

## Required foundations

- **Auth registry refactor (P0.1):** all built-in provider and identity constructors use the constructor registry. Remove the fallback kind switches, reject duplicate registrations, expose supported registration entry points, and preserve built-in configuration and behavior. External auth uses the same dispatch path.
- **Credential-type registry (P0.2):** replace keyring encode/decode switches with registered codecs, preserve legacy sessions, and support versioned native and plugin-owned envelopes. Auth chain validation resolves accepted and produced types before execution.
- **Registration safety (P0.3):** harden manifest and command registration for the first release. The remaining registries are hardened with their adapters.
- **Atmos API (P0.12):** rename the shared contract package to `pkg/api`; keep Plugin SDK authoring helpers and wire bindings distinct.
- **Shared OCI operations:** extend existing `pkg/oci` as described in [OCI reuse](platform/oci-reuse.md). Plugin publishing must not create a parallel registry client or credential store.

P0.1 and P0.2 are release gates, not optional cleanup. Existing auth regression tests and a plugin-to-built-in identity chain must pass before the pilot is accepted.

Discovery supports optional catalog snapshots and explicit OCI repository lists through one locally cached index. Multiple sources can coexist, including catalogs per product. Publishing a release never updates a shared catalog; snapshot publication is a separate maintainer operation with externally serialized writers. See [Discovery](discovery.md).

Registry access uses the ordinary Atmos auth lifecycle through `plugins.registries.<alias>.identity`, including a built-in `oci/user` identity and existing ECR/ACR integrations. Convenience registry login/logout commands delegate to auth. Exact-host matching, no ambient fallback for configured identities, shared-session logout, and bootstrap failure diagnostics are part of acceptance.

## Complete command inventory

Every command below must work end-to-end in the first release. Help-only commands and successful no-ops do not meet acceptance.

| Area | Commands under `atmos plugin` | Required proof |
|---|---|---|
| Sources and registry auth | `registry add`, `registry list`, `registry remove`, `registry login`, `registry logout` | Public/private sources; auth delegates to shared machinery and works before any plugin is installed. |
| Discovery | `search`, `info` | Optional catalogs and explicit repository indexing, filters, versions, source errors, and cached offline metadata without plugin execution. Remote `info` remains a broader acceptance target. |
| Installation | `install`, `list`, `remove`, `update` | Project/user precedence, local `path` packages, digest verification, atomic install, frozen/offline restore, update and rollback. |
| Invocation | `run` | Execute an installed command with arguments, streams, cancellation, and exit status preserved. An executable command adapter is required even though auth is the primary use case. |
| Authoring | `create`, `build`, `validate`, `test` | Generate a Go plugin through existing scaffolding; compile to its manifest's executable path; validate statically; run meaningful positive/negative contract tests and opt-in integrations. |
| Release lifecycle | `pack`, `publish`, `sign`, `delete` | Multi-platform releases, immutable versions, private auth, keyless Sigstore signing of immutable index digests, explicit remote deletion, permissions and partial-failure reporting. |
| Catalog lifecycle | `catalog build`, `catalog publish`, `catalog add`, `catalog remove` | Publish and maintain discovery metadata; unlisting and artifact deletion have distinct effects. |

Installation downloads independent packages concurrently, bounded by `plugins.max_concurrency` (default four). It uses the shared `pkg/ui/batch` toolchain presentation. Registry resolution, verification, cancellation, and cache reuse preserve the project transaction: no partial lockfile or declaration update after a failed batch. Verified payloads live in shared XDG data storage keyed by digest and platform.

Package metadata follows the shared manifest convention: `metadata.name` is the stable package identifier; optional `display_name`, `description`, `author`, and `repository` describe it, and `version` identifies the release. Repository provenance is author-configured and preserved in each platform payload. Download provenance remains the OCI source plus verified digests in the project lock.

Rollback retains the documented workflow: restore the previous declaration/lockfile and run `install --frozen`. It does not introduce an additional command. `create` advertises only implemented capabilities and runtimes; requesting a reserved follow-on capability fails clearly.

## Policy, compatibility, and presentation gates

- Existing CEL/Starlark policy evaluation governs plugins via `on: plugin`; source checks gate registry access, and inspected-package checks gate installation and activation. Both explicit and automatic installation, cache reuse, local `path` packages, local layouts, conformance tests, publication, and signing are covered. Company-only registry policies must not be bypassed by direct OCI references or catalog redirects. See [Plugin policies](policy.md).
- Digest verification is mandatory. Trusted-publisher requirements verify Sigstore evidence and compose restrictively across loaded configuration files. A passing general policy cannot weaken them. `plugin sign` accepts immutable digest references; `publish --sign` signs the published index.
- Authors can set `spec.compatibility.atmos` to a semver range alongside protocol and contract requirements. Unsupported required compatibility fails before installation commits or code runs. The generated starter currently sets protocol/contracts but leaves the optional Atmos range to its author.
- Project declarations accept exact versions and semver constraints; `.atmos.plugins.lock.yaml` pins the selected release and platform digests. Ordinary restoration does not upgrade a lock. `update` stays within the declaration; an exact pin advances only after an explicit constraint change. Frozen mode rejects drift, refuses while the active configuration sets `path`, and writes no lock.
- `search`, `list`, `info`, and `registry list` share `pkg/list` formatting, with table, JSON, YAML, CSV, and TSV formats. Installed entries use the green status dot in interactive views, while text statuses and clean machine output survive non-TTY/color-disabled use.
- The changelog includes a reproducible cast recorded through Atmos custom commands against the OCI emulator. It demonstrates create/build/test, pack/publish, registry listing, search, install/list/info, an installed command, and OIDC conformance through the downloaded binary. It is a local acceptance demonstration, not live customer IdP sign-off.

## Working Go OIDC plugin and documentation

The [OIDC pilot](oidc-pilot.md) is the authentication acceptance test: browser login with PKCE, token validation, downstream credential exchange, cached reuse, refresh, and logout. The package also exports a small executable command so one install proves both auth and `plugin run`. Consumers use a stock compatible Atmos binary and precompiled plugin artifacts.

`plugin create` reuses `AtmosScaffoldConfig` and the existing scaffold template loader, generator engine, prompts/defaults, conflict handling, and provenance. Add plugin templates to that system; share command-neutral orchestration where needed instead of invoking a Cobra command or building another renderer. The Go OIDC starter and its runnable example include the manifest, configuration schema, handlers, mock issuer/broker fixtures, contract suite, sample project, README, and release workflow.

Documentation must cover every command above, the complete scaffold-to-private-publication and install-to-login walkthroughs, issuer/client registration and callback configuration, supported token exchange, refresh/logout behavior, noninteractive behavior, troubleshooting, and compatibility. Include developer instructions for adding an extension type using the registration mechanism below. Auth/command docs and examples ship with the first release; documentation for future adapters follows those implementations.

## Mechanisms for subsequent extension types

The first release must provide more than an auth-specific loader:

1. Versioned package, manifest, lock, protocol, credential, and error contracts shared by all extension types.
2. A registry of extension descriptors keyed by capability selector. Each descriptor supplies static schema/metadata validation, supported contract versions, scaffold/test hooks, and an adapter factory. Startup registers descriptors explicitly; installer, catalog, and lifecycle code do not contain per-extension dispatch switches.
3. An adapter seam connecting a runtime handle to an existing Atmos registry, with explicit invocation scope, cancellation, duplicate policy, and lazy activation. Auth and executable commands are the first production consumers.
4. Common SDK transport, fixtures, and conformance runner hooks. A test-only third descriptor proves registration, static validation, discovery, and dispatch without editing installer, OCI, lockfile, or catalog code; it is not advertised as a supported production extension.
5. The full [gap inventory](platform/gap-analysis.md), planned contracts, and per-surface injection points remain documented. Adding a new adapter can require an Atmos release and local registry hardening; these mechanisms do not claim every internal API is already open.

Unsupported required runtime/capability contracts fail before installation is committed or code runs. Discovery may display a package as incompatible. Reserved schemas do not imply runtime support, and mixed packages with unsupported required exports fail as a whole.

## Follow-on delivery

Component engines, CI providers, hooks, steps, validation, stores, secret providers, backend provisioning, MCP tools, and skills retain their PRDs but their production adapters do not gate this release. Starlark/data/Wasm plugin runtimes, cross-runtime parity, and built-in extraction are also follow-ons. Existing Starlark automation is unaffected.

The Atmos Artifacts commands, generic repository framework, bundles, and a wholesale OCI-library migration are outside this implementation. Shared OCI work must leave a reusable foundation for that future feature.

## Native pilot checkpoint

Validate the native implementation and required refactors in one working change before splitting it into reviewable PRs. Local checkpoints are allowed; publishing remains a separate user decision.

The current native pilot supplies working implementations of every command name in the inventory, project-local installation and lockfiles, the generated OIDC example, auth/command adapters, shared OCI operations, and author/consumer documentation. It does not yet satisfy the full release matrix. Remaining release work includes user-wide installation precedence and shortcuts, remote `info`, catalog-origin-aware updates, plugin tool-dependency references/coordinated lock transactions, and the remaining negative and live-integration cases in the OIDC pilot. The pilot now implements trusted-publisher verification, signing commands, standard OCI bundle referrers, cached/offline evidence, and restrictive policy composition across loaded configuration files. It also implements general CEL/Starlark plugin admission, normalized capability metadata, toolchain-style parallel downloads, common formatted views, installed status dots, and the emulator-backed lifecycle cast. These are implementations in this branch, not a declaration that the full acceptance matrix has passed. The current CLI documents its implemented behavior and rejects remaining reserved policy settings instead of silently treating them as supported. The broader requirements above remain acceptance criteria for the first release.
