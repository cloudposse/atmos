# Plugin platform decisions

**Status:** Proposed; part of the [plugin platform](../overview.md). [Platform workstream](README.md).

Numbered decision records for the platform. Sibling documents cite them by number (for example "Decision 7"). Each record states what was chosen and what it costs. The native pilot implements a subset; its [checkpoint](../first-release.md#native-pilot-checkpoint) is authoritative for current status. Open items that need an owner and a date are at the end.

## Decision 1: Native transport is hashicorp/go-plugin in gRPC mode; Atmos owns the proto

**Status:** Proposed.

**Context.** Native plugins need a process boundary, a versioned wire contract, lifecycle management, and cancellation. Comparable Go tools use several mechanisms:

| Mechanism | Used by | Fit for Atmos |
|---|---|---|
| `hashicorp/go-plugin` (subprocess, gRPC, handshake, mTLS) | Terraform/OpenTofu providers, Vault, Nomad, Packer, TFLint, Argo Rollouts, Velero, Grafana, Mattermost, Steampipe, Jaeger (legacy storage plugins) | Chosen. Proven for bidirectional calls, health checks, graceful kill, version negotiation. |
| Plain exec with argv/env | `gh` extensions, `kubectl` plugins, Helm subprocess plugins, Docker CLI plugins | An alternative for opaque commands only; not selected for native command extensions, which use `Commands.Run` and API services. |
| Stdio protobuf/JSON, one-shot or long-lived | `protoc`/Buf plugins (one-shot), Nushell plugins, Kong external plugin servers | Simple, but each project re-invents handshake, versioning, cancellation, and callbacks. |
| JSON-RPC over stdio | Language Server Protocol | Good for editors; weak typing and no streaming conventions we would own. |
| Wasm (Extism, wazero) | Helm 4, Envoy | Declared follow-on runtime ([Decision 2](#decision-2-wasm-extism-is-the-declared-follow-on-runtime-not-initial)). Sandbox is strong but Extism host-function surface and toolchain are younger. |
| Yaegi (interpreted Go) | Traefik | Source-distributed; incompatible with the [precompiled distribution guarantee](../distribution.md#consumer-guarantee). |
| In-process scripting | Atmos Starlark today | Kept as the Starlark runtime; not a native-plugin transport. |
| Go `plugin` shared objects | golangci-lint go-plugins | Excluded: requires matching toolchain and dependency versions ([reference models](../reference-models.md#a-consumer-experience-to-exclude-custom-atmos-builds)). |

`hashicorp/go-plugin` remains MPL-2.0; the license change HashiCorp applied to Terraform and other products did not extend to it. It is absent from `go.mod`; `google.golang.org/grpc` is already a direct dependency.

**Decision.** Use `hashicorp/go-plugin` in gRPC-only mode with a magic-cookie handshake, automatic mTLS, `VersionedPlugins` for future protocol majors, and a health service. Atmos owns the `atmos.plugin.v1` proto package; it is not derived from Terraform's protocol. Starlark remains a proposed in-process runtime. Native commands use `Commands.Run` for both shortcuts and qualified invocation, streaming stdout, stderr, and a terminal exit event; the current contract has no stdin stream or TTY attachment. The [protocol document](protocol.md) specifies services and messages.

**Consequences.**
- One new direct dependency and a generated-code step (`buf generate`).
- Plugins in any language implement the open protocol; the Go SDK is a convenience layer.
- Bidirectional API services use the go-plugin broker, so scoping and masking live in one place.
- Windows transport details need hardening (see [open questions](#open-questions)).

**Alternatives considered.**
- Custom stdio protocol: no dependency, but re-implements handshake, mTLS, version negotiation, and kill semantics.
- Plain exec for everything: cannot carry API services or typed errors.
- Wasm first: stronger sandbox, but excludes arbitrary tools and subprocess-heavy integrations such as Terraform wrappers.

## Decision 2: Wasm (Extism) is the declared follow-on runtime; not initial

**Status:** Proposed.

**Context.** Helm 4 offers Wasm plugins alongside subprocess plugins. `extism/go-sdk` and `tetratelabs/wazero` are already indirect dependencies of the module, so the cost of exploring is low.

**Decision.** Reserve `wasm` as a runtime name in the runtime registry and manifest schema, but ship `native` first; `starlark` and `data` are follow-on runtimes under Decision 19. A Wasm runtime would expose Extism host functions that mirror the broker services (`api.HTTP`, `api.Secrets`, and the rest), so extensions written against the service contracts port without redesign.

**Consequences.**
- Manifests may name `runtime: wasm` only after a follow-on PRD enables it; until then it is rejected with `ErrPluginIncompatible`.
- No Wasm dependency becomes direct, and no Wasm code lands in the initial release.

**Alternatives considered.**
- Ship Wasm in the initial release: doubles Atmos-function and conformance surface before the native contracts are proven.
- Never support Wasm: forecloses a sandboxed option that a peer project already ships.

## Decision 3: OCI client stays on go-containerregistry; adopt ORAS only for the referrers API

**Status:** Proposed.

**Context.** [`pkg/oci`](../../../../pkg/oci) is built on `google/go-containerregistry` (direct) and already provides `ResolveImage`, `PinDigest`, and GHCR authentication. `oras.land/oras-go/v2` is an indirect dependency. Plugin packages are non-image artifacts in an OCI index with immutable child digests.

**Decision.** Extend shared `pkg/oci` on go-containerregistry for descriptor/blob operations, artifact push and pull, index creation, tag listing, manifest deletion, and OCI layouts. Keep plugin media types and release assembly in `pkg/plugin/oci`; migrate the version resolver's direct registry calls to shared operations. Apply the [OCI reuse inventory](oci-reuse.md) and Decision 20. Promote ORAS to a direct dependency only if signature storage requires the OCI referrers API and go-containerregistry cannot provide it with acceptable effort. Trigger: [Decision 4](#decision-4-verification-is-digest-first-with-optional-signature-policy) reaches implementation and needs referrers listing.

**Consequences.**
- One OCI client, one authentication path, one set of mocks.
- Registries without referrers support need the fallback tag scheme; see [open questions](#open-questions).

**Alternatives considered.**
- ORAS now: purpose-built for artifacts, but a second client and auth stack beside `pkg/oci`.
- Shelling out to `oras` or `crane`: violates the no-extra-binaries consumer guarantee.

## Decision 4: Verification is digest-first with optional signature policy

**Status:** Implemented for the native pilot; private trust roots and legacy signature envelopes remain outside its supported policy.

**Context.** The [distribution contract](../distribution.md) requires immutable digests. Enterprises also need to restrict who published a plugin.

**Decision.** Digest verification is always on: every artifact is checked against the lockfile and the OCI index. Signature verification uses a sigstore bundle (cosign-compatible) and is enabled by `plugins.policy.require_signature` with `trusted_publishers`. The native pilot implements this with sigstore-go, authenticating certificate identity, issuer, SCT, transparency inclusion and signing time. OCI referrers carry standard bundles; cached evidence is reverified against current policy before activation. Mandatory constraints accumulated from loaded Atmos configuration files cannot be weakened by a later project layer. Dedicated user/machine plugin policy files remain proposed; protected CEL/Starlark governing rules use the existing policy directory.

**Consequences.**
- `plugin sign` and `plugin publish --sign` use a managed Cosign executable and publish standard bundle referrers through the shared OCI client. Consumer verification uses the Go library and public Sigstore TUF trust material.
- All loaded configuration files retain mandatory signer sets: OR within a layer, AND across layers against the same signature. Offline verification requires cached, unexpired TUF metadata and bundled evidence; a package with `path` has no signature, so required signatures govern released packages only and policy rules over `origin: development` and `input.path` gate local builds.
- Integrity (digest) and publisher identity (signature) stay distinguishable in `plugin info`.
- A failure raises `ErrOCIIntegrity` or `ErrPluginSignatureInvalid` ([errors](errors.md)).

**Alternatives considered.**
- Trust-on-first-use only: no publisher assurance.
- Mandatory signatures from day one: blocks private registries that do not sign.

## Decision 5: Lockfile `.atmos.plugins.lock.yaml` beside the root atmos.yaml

**Status:** Implemented for project plugin releases; tool dependency references remain planned.

**Context.** Toolchain pinning has `toolchain.lock.yaml`. Plugin locks must record index and per-platform digests, contract versions, and references to required tools.

**Decision.** A separate `.atmos.plugins.lock.yaml` (kind `AtmosPluginLock`) lives beside the root `atmos.yaml` and pins release/index/platform selections. The planned tool-dependency integration references exact `toolchain.lock.yaml` entries for `dependencies.tools` instead of duplicating them; those references and coordinated lock transactions remain unimplemented. The [configuration document](configuration.md) distinguishes the current schema from that target.

**Consequences.**
- Plugin and toolchain lock lifecycles stay independent, matching [version-management subsystem boundaries](../toolchain-dependencies.md).
- When tool dependency integration lands, both locks must be committed and frozen restoration must verify their consistency. Today frozen plugin restoration verifies the plugin declaration/lock pair.

**Alternatives considered.**
- Extend `toolchain.lock.yaml`: couples two subsystems with different resolution and verification rules.
- Lock inside `atmos.yaml`: mixes generated and hand-edited content.

## Decision 6: Install directory under XDG data; discovery cache under XDG cache

**Status:** Shared payload storage and discovery caching implemented in the native pilot; user-scope configuration remains planned.

**Context.** [`pkg/xdg`](../../../../pkg/xdg) provides `GetXDGDataDir`, `GetXDGCacheDir`, and `GetXDGConfigDir` with Atmos overrides.

**Decision.** Installed artifacts live at `<xdg-data>/atmos/plugins/releases/<index-digest-hex>/<os>/<arch>/`, containing an OCI layout and materialized payload. The root comes from `pkg/xdg`, with an explicit `plugins.cache_dir` override. Discovery metadata lives under XDG cache `atmos/plugins/discovery`, or `<cache_dir>/discovery` with an override. Source reference/kind, auth configuration, effective realm, and host identity bindings scope the cache; it stores no tokens. Adding another repository retains existing references' caches. Logout does not purge advisory discovery metadata. User-scope configuration under XDG config remains planned.

**Consequences.**
- The digest segment makes installs content-addressed and lets two projects share one verified copy.
- Cache can be deleted without losing installs; installs are restorable from the lockfile.
- Atomic install is a rename within the data directory.

**Alternatives considered.**
- A per-project `.atmos/plugins` directory: duplicates binaries and complicates cleanup.
- `~/.atmos`: bypasses existing XDG conventions.

## Decision 7: Duplicate registration is an error for every external registrant

**Status:** Proposed.

**Context.** Component, command, step, CI provider, backend, manifest, auth registry, auth integration, and target registries replace silently on duplicates ([gap analysis](gap-analysis.md#registries-to-change)).

**Decision.** Any duplicate registration by an external registrant returns an error that names both owners. Built-ins keep their deterministic `init()` order. An explicit override is available only through a named API where a custom component type needs it (`custom/provider.go`); it is never implicit.

**Consequences.**
- Existing last-writer-wins behavior is removed in Phase 0 with tests that pin built-in order.
- Plugins cannot shadow built-ins or each other; collisions surface as `ErrPluginCollision`.

**Alternatives considered.**
- Keep replacement and warn: makes behavior depend on load order.
- Priority numbers: more configuration for a problem that should fail loudly.

## Decision 8: Command collisions make the shortcut unavailable; `plugin run` always works

**Status:** Proposed.

**Context.** [`extensions/commands.md`](../extensions/commands.md) says plugins cannot replace built-ins or YAML commands, but nothing enforces it. YAML commands are processed after `RegisterAll` and can override built-ins today.

**Decision.** A plugin command that collides with a built-in, YAML command, or another plugin is marked unavailable as a top-level shortcut and diagnosed. `atmos plugin run <alias> -- …` always dispatches to the plugin. Plugins never replace built-ins or YAML commands.

**Consequences.**
- Installing a plugin cannot change the behavior of an existing command.
- Users get a deterministic escape hatch for scripting.

**Alternatives considered.**
- Last registered wins: unpredictable.
- Refuse to install on collision: blocks a package whose other extensions are valid.

## Decision 9: `CommandProvider` stays in `cmd/internal`; `cmd/plugin` is the only bridge

**Status:** Proposed.

**Context.** [`CommandProvider`](../../../../cmd/internal/command.go) depends on Cobra and Atmos flag builders; it is Go-internal by design.

**Decision.** The interface does not move or become public. `cmd/plugin` adapts manifest metadata into commands between `RegisterAll` and `processCustomCommands`. Plugins see only command name, arguments, and flags on the wire.

**Consequences.**
- No Cobra types cross the plugin boundary.
- Adapter logic is concentrated in one package and is the only place that knows both sides.

**Alternatives considered.**
- Export `CommandProvider`: freezes Cobra and flag-builder internals as public API.

## Decision 10: Versioned credential envelope and credential-type registry

**Status:** Proposed.

**Context.** The keyring decodes credentials through a closed switch over `aws`, `gcp`, `oidc`, `atmos-pro`, `mock` ([`pkg/auth/credentials`](../../../../pkg/auth/credentials)); no versioned type exists.

**Decision.** Credentials cross the boundary as `{type, expires_at, data, sensitive_keys}` with versioned types (`aws/v1`, `gcp/v1`, `azure/v1`, `oidc/v1`, `atmos-pro/v1`). Plugin-defined types are namespaced by package (`acme/token/v1`). A credential-type registry replaces the keyring switch; adapters wrap existing `ICredentials` implementations. Chain validation checks declared accepted and produced types.

**Consequences.**
- Existing stored credentials decode unchanged through built-in adapters.
- `sensitive_keys` feeds the masker ([errors](errors.md#diagnostics-and-masking)).
- A new field in a type requires a new version suffix.

**Alternatives considered.**
- Opaque `map[string]any`: no compatibility checking or masking.
- Plugins return `ICredentials` directly: leaks internal types.

## Decision 11: Bootstrap order and zero cost without `plugins:`

**Status:** Proposed.

**Context.** Plugin metadata must be available before auth, stores, and stack processing, but plugin processes must not start for commands that do not need them.

**Decision.** Order: configuration (`atmos.yaml` and imports) → `plugins:` and lockfile → static registry (metadata only) → metadata registration (kinds, types, commands, tools, skills, checks) → command tree → auth, stores, stacks → lazy process start on first use. Static metadata registration starts no subprocess and performs no Starlark evaluation. Before registration for execution, missing declared project packages are restored through the installer; this can access the registry unless offline policy forbids it. Existing selections restore by immutable lock digest and never upgrade implicitly. Cached activation rechecks current policy. A project without `plugins:` takes none of these steps.

**Consequences.**
- Help, completion, and validation work from manifests alone.
- Startup cost for existing users is unchanged.
- Import sources that must precede bootstrap remain a follow-on ([gap analysis](gap-analysis.md#follow-on-surfaces)).

**Alternatives considered.**
- Start plugins eagerly: slow and unsafe for read-only commands.
- Discover plugins from PATH: nondeterministic and unlockable.

## Decision 12: One Starlark evaluation session per package per process

**Status:** Proposed.

**Context.** [`pkg/script/starlark`](../../../../pkg/script/starlark/engine.go) evaluates scripts with predeclared modules and cancels through `AfterFunc`. Plugin packages register several extensions from one source.

**Decision.** Evaluate each Starlark package once per process to collect registrations; run each invocation on an isolated thread. No code is fetched during evaluation; modules come from the verified artifact. Plugin-only modules (`plugin`, the extended `atmos`, and the extension modules) are predeclared for plugin sessions only.

**Consequences.**
- Registration is deterministic and testable.
- Global state in a package is shared across invocations unless the SDK forbids it; the conformance suite checks isolation.

**Alternatives considered.**
- Evaluate per invocation: repeats registration and risks divergent state.
- A session per extension: duplicates top-level evaluation in combined packages.

## Decision 13: Validate registry foundations with the pilot, then split for review

**Status:** Accepted.

**Context.** The [gap analysis](gap-analysis.md) found closed lists, silent replacement, missing context parameters, and Go-internal interfaces. These are defects and design debt independent of plugins.

**Decision.** P0.1 (auth registry), P0.2 (credential types), P0.3 manifest/command foundations, and P0.12 (API rename) were validated end-to-end with the native plugin/OIDC implementation, then split into five stacked refactor and feature PRs. Other registry refactors accompany their follow-on adapters ([Decision 19](#decision-19-first-release-is-complete-cli-go-oidc-and-extension-mechanisms)). This supersedes the original requirement to merge standalone Phase 0 PRs first.

**Consequences.**
- Each change is reviewable and revertable alone and benefits built-ins immediately.
- Plugin work starts from a registry that errors on duplicates and carries `context.Context`.

**Alternatives considered.**
- Split and merge every prerequisite before validating the pilot: rejected for the initial implementation; review separation follows a proven example.

## Decision 14: No publisher install or uninstall hooks

**Status:** Proposed.

**Context.** Helm runs install, update, and delete scripts for subprocess plugins ([reference models](../reference-models.md#helm-separate-extension-type-from-runtime)).

**Decision.** Installation and removal never execute plugin code or publisher scripts. Required tools are declared in `dependencies.tools` and restored by the [toolchain](../toolchain-dependencies.md). Removing a package never deletes provisioned infrastructure.

**Consequences.**
- Installation is safe to run offline and under least privilege.
- Plugins that need setup must perform it lazily and idempotently at first use.

**Alternatives considered.**
- Install scripts: arbitrary execution before trust policy is applied.

## Decision 15: Contract testing is `buf breaking` plus an Atmos-shipped conformance suite, not Pact

**Status:** Proposed.

**Context.** Atmos consumes plugin services from an open set of third-party providers it does not control, so consumer-driven contracts cannot be verified against them. The contract is protobuf, not HTTP. Both ends of the API services are Cloud Posse code. The repository already uses pact-go for the Atmos Pro HTTP API: tests under [`pkg/pro`](../../../../pkg/pro) carry the `pact` build tag and write [`pacts/atmos-AtmosPro.json`](../../../../pacts/atmos-AtmosPro.json).

**Decision.** Schema compatibility is enforced by `buf breaking` against the previous release's proto. Behavioral conformance is the Atmos-shipped suite described in [testing](../testing.md) plus pinned fixture plugins built against old protocol versions. The Atmos Pro Pact tests are unaffected. A future Atmos Pro-hosted catalog or marketplace HTTP API would be a Pact candidate.

**Consequences.**
- Additive-only changes within a protocol major are mechanically enforced.
- Old fixture plugins stay in the repository for compatibility tests, per the Helm lesson.

**Alternatives considered.**
- Pact for plugin services: no real provider verification and no gRPC fit.
- Hand-written golden messages only: do not catch field renumbering.

## Decision 16: The gap analysis covers all surfaces

**Status:** Proposed.

**Context.** Product PRDs cite registries as ready integration points that the code does not provide.

**Decision.** The [gap analysis](gap-analysis.md) inventories the 12 planned extension surfaces in full (existing registry, duplicate policy, context, reachability, injection points, prerequisites) and one row for each follow-on surface, plus the two closed maps found along the way (Terraform state readers, keyring credential types).

**Consequences.**
- Initial scope is checked against code, not assumed.
- Follow-on surfaces have a recorded starting point without becoming commitments.

**Alternatives considered.**
- Cover only the initial surfaces: hides cross-cutting registries such as artifact and cache backends.

## Decision 17: The Plugin SDK is a nested Go module in the Atmos repository

**Status.** Accepted.

**Context.** Plugin authors must import the Plugin SDK without inheriting Atmos's dependency graph, and Atmos and the SDK must share one proto source and one `buf breaking` baseline. A separate repository would add a second repository to bootstrap, a cross-repository PR for every contract change, and two copies of the baseline that drift. The repository already hosts nested modules under [`tools/`](../../../../tools) (`noticegen`, `gomodcheck`, `lintroller`). The SDK version number follows Atmos in lockstep.

**Decision.** `sdk/` is the nested module `github.com/cloudposse/atmos/sdk`, kept in this repository. Its release number follows Atmos: publish `vX.Y.Z` and Go's required `sdk/vX.Y.Z` tags on the same commit. Protocol and capability contract versions express compatibility independently of package release numbering. `sdk/plugin` supplies authoring helpers, `sdk/api` exposes supported API services, and `sdk/proto` contains wire bindings; the module must not import Atmos's `pkg`, `cmd`, `internal`, or `errors` packages.

**Implemented source/build layout.** One proto source under `proto/atmos/plugin/v1/` and `proto/atmos/api/v1/` generates bindings into both `pkg/plugin/proto` and `sdk/proto` using `atmos dev generate proto`. The main module uses its own generated copy and does not require an unpublished SDK tag, root replacement, or committed workspace to compile. The two generated packages must never be linked into one binary; they interoperate through the wire protocol. Parity tests guard drift. Authors can explicitly scaffold a development `go.work` pointing to the local SDK checkout; release projects depend on a real SDK version. This adopts the previously recorded dual-generation fallback.

**Consequences.** Contract changes remain atomic in one repository and have one proto baseline. Duplicated generated code is accepted to keep Atmos and the SDK independently buildable and preserve `go install` compatibility. Lockstep release tagging remains a release requirement; the native pilot does not claim that an SDK version has been published. Author projects use the module's ordinary version number (for example `go get github.com/cloudposse/atmos/sdk@vX.Y.Z`), while Git tags carry the `sdk/` prefix.

**Alternatives considered.**
- Separate repository `atmos-plugin-sdk`: rejected for cross-repository churn and a duplicated breaking-change baseline.
- Plugin SDK packages inside the main module: rejected because authors would inherit Atmos's full dependency graph.

## Decision 18: The Atmos API and Plugin SDK have distinct roles

**Status.** Accepted.

**Context.** The package formerly named `pkg/automation` (now [`pkg/api`](../../../../pkg/api)) was written so other languages could add adapters that use Atmos functionality: a step library, a process execution policy, and a filesystem view, in native Go values with no Starlark dependency. The [Atmos API PRD](../../atmos-sdk.md) describes the broader programmatic interface. Starlark is its first language adapter. Plugin authors also need tools for implementing extensions that Atmos invokes. These are distinct responsibilities.

**Decision.** Name the shared interface the **Atmos API**, with Go contracts in `pkg/api`. P0.12 renames `pkg/automation` while preserving its existing types: `StepLibrary`, `StepCall`, `StepResult`, `ExecutionPolicy`, `FileSystem`, and `LocalFileSystem`. The API grows beyond this initial execution surface as capabilities gain supported contracts. Language bindings expose it under `atmos.*`.

The **Plugin SDK** remains the nested module `github.com/cloudposse/atmos/sdk` ([Decision 17](decisions.md)). `sdk/plugin` contains authoring helpers, `sdk/proto` generated wire types, `sdk/api` plugin-facing bindings to the Atmos API, and `sdk/sdktest` test doubles. The SDK does not import `pkg/api` or other main-module packages. `pkg/plugin/bridge` adapts the in-process API to the wire representation; transport bindings preserve the supported operations without requiring native Go objects to cross the wire. Services Atmos provides use proto package `atmos.api.v1`; services a plugin implements use `atmos.plugin.v1`.

**Consequences.** The API exists independently of the plugin system. Starlark, future language bindings, and plugins can use the same underlying Atmos functionality. Native contracts can retain streams and interfaces; the wire adapter defines their serializable equivalents and parity tests. P0.12 is a package rename within the main module; WP2.6 establishes the Plugin SDK module. This naming decision does not choose an execution model or transport for future TypeScript or Python adapters.

**Alternatives considered.**
- `pkg/automation`: too narrow for the intended Atmos-wide interface.
- `pkg/host` or `pkg/server`: emphasize deployment roles rather than the functionality available to callers.
- One SDK for both concepts: conflates the shared programmatic interface with plugin authoring tools.

## Decision 19: First release is complete CLI, Go OIDC, and extension mechanisms

**Status.** Accepted.

**Decision.** Deliver the [first-release scope](../first-release.md): every `atmos plugin` command, the [working Go OIDC pilot](../oidc-pilot.md), complete documentation, and registry/adapter/schema/test mechanisms for future Atmos primitives. Auth provider/identity registry completion and credential-type registration are required foundations. Executable command support ships so `plugin run` works. Auth and command scaffolds reuse the existing `AtmosScaffoldConfig` system and generator packages.

**Consequences.** Auth and executable commands are the first production adapters. Starlark/data runtimes and the other production adapters retain their contracts and inventory but do not gate the release. Required authoring, docs, tests, and pilot work move forward from Phase 4. A test-only third extension proves extensibility without modifying package lifecycle code. Discovery distinguishes unsupported packages; reserved capability schemas do not enable execution. This decision supersedes earlier statements requiring all extension implementations and runtime parity for the initial release.

## Decision 20: Converge OCI operations below plugin and future artifact workflows

**Status.** Accepted direction; detailed interfaces proposed.

**Context.** Existing OCI support already serves vendoring, scaffolding, source provisioning, and version resolution. The saved Atmos Artifacts draft proposes a wider repository framework and image-copy implementation, which is not scheduled for this release. [OCI reuse](oci-reuse.md) records the reviewed draft and concrete code inventory.

**Decision.** Extend `pkg/oci` with reusable registry/layout/auth/descriptor/blob operations; retain existing entry points and converge direct calls from version resolution. Plugin code owns media types, release policy, catalogs, and installation. Keep `go-containerregistry` for this implementation; the artifact draft's `containers/image` choice does not require a second client stack now. Shared round-trip and existing-consumer tests prove reuse.

Registry login/logout remain in the complete plugin CLI but delegate to shared auth/credential machinery. No plugin-only credential store is introduced. Preserve bootstrap authentication before any plugin is installed. Reuse the artifact draft's digest, platform, layout, credential-separation, and offline-testing semantics without implementing `atmos artifact`, bundles, or its generic repositories.

## Decision 21: Plugin admission reuses the general policy engine

**Status.** Implemented in the native pilot.

**Decision.** Register `on: plugin` with source and inspected-package phases in `pkg/policy`; collect inputs through `pkg/plugin/policy`. CEL and restricted Starlark govern actual registry authorities, repository paths, manifest names/versions/runtimes, and capability IDs discovered from extension descriptors. Apply rules before OCI access and before installation/activation, including automatic restoration and cached use. Development and explicit layout imports have distinct origins; a claimed allowed repository cannot disguise a local import. See [Plugin policies](../policy.md).

**Consequences.** No second expression engine or plugin policy directory. Protected `policy.d/`/`ATMOS_POLICY_DIR` controls definitions. Denials/errors fail closed; general admission cannot override signature requirements. Cache provenance relies on protected locks and verified bytes, and native processes remain unsandboxed. Future extension descriptors automatically supply capability metadata without new collector switches.

## Decision 22: Reuse common UI and keep lifecycle logic in focused packages

**Status.** Implemented in the native pilot.

**Decision.** `pkg/plugin/output` uses `pkg/list` for search, list, info, and registry list. Support table, JSON, YAML, CSV, and TSV through `--format` and `ATMOS_PLUGIN_FORMAT`. Interactive installed rows use the shared green status dot with readable text fallback. Independent downloads use `pkg/ui/batch` and `plugins.max_concurrency`, default four. `cmd/plugin` is command wiring; `pkg/plugin/manage` and other focused packages own lifecycle logic. Add none to `internal/exec`.

**Consequences.** Terminal degradation, machine formats, and progress follow existing auth/toolchain conventions. Reusable OCI and signature operations stay in `pkg/oci` for existing consumers and future Atmos Artifacts; no artifact command implementation is included.

## Decision 23: Pin releases independently of compatibility and recheck admission

**Status.** Implemented for project-scoped native packages.

**Decision.** `spec.compatibility.atmos` accepts a semver range, alongside protocol and required contract versions. The optional Atmos range is author-supplied; the starter currently leaves it unset. Project version declarations accept exact pins or ranges; locks record exact release/index/platform selections. Restore and automatic installation reuse locks. `update` selects within the constraint; changing an exact pin requires an explicit declaration update. Frozen installs reject declaration drift and any active `plugins.packages.<alias>.path` and write no lock.

**Consequences.** Toolchain conventions guide reproducibility, not identical formats or exact-pin update behavior. Every activation checks current admission and required signatures; lockfile verification labels cannot authorize code. Contract support/deprecation timelines remain a release-policy decision.

## Decision 24: Authoring and demonstration workflows use Atmos custom commands

**Status.** Implemented in the native pilot.

**Decision.** Keep plugin generation on existing scaffolding. `plugin build` builds the manifest executable atomically, preserving the prior binary on failure. Use `atmos dev generate proto` with pinned tools to generate CLI/SDK bindings from one source, with parity tests; remove the separate shell-script entry point. Record the emulator-backed create/build/test/pack/publish/search/install/list/info/run/OIDC flow through `atmos ... casts generate`, validate it, and embed it in the changelog.

**Consequences.** Tool installation and workflow discovery use existing Atmos mechanisms. The cast demonstrates local generated/downloaded plugin behavior; live customer OIDC sign-off remains separate.

## Decision 25: Registry access uses Atmos identities

**Status.** Implemented in the native pilot.

**Decision.** `plugins.registries.<alias>.identity` references `auth.identities`. `atmos auth` owns login, session storage, and logout; `plugin registry login/logout` remain conveniences. Built-in `oci/user` accepts an explicitly selected environment secret or a masked prompt. Existing ECR/ACR integrations expose a reusable credential exchange returning minted credentials directly, while their normal auth commands retain Docker publication.

**Consequences.** Shared OCI transport authenticates exact normalized hosts before ambient fallback. A configured identity failure is terminal; credentials do not follow catalog references to other hosts. Every operation checks the current auth session, so logout or token rotation invalidates cached exchanges. Already cached auth plugins are verified and registered temporarily before restoring missing packages; their trust evidence must also be locally available. A plugin cannot fetch itself using its own auth provider. Generated developer workflows use an anonymous local emulator by default and document authenticated remote publishing through ordinary auth commands.

## Decision 26: Optional catalog snapshots and cached direct discovery

**Status.** Accepted for the native pilot.

**Decision.** Explicit OCI repositories and optional catalog snapshots feed one cached discovery index and search interface. Multiple sources and product catalogs coexist. Registry host/identity configuration is independent of discovery. Direct indexing reads static metadata, uses bounded parallelism, and reuses immutable digest metadata. Default freshness is fifteen minutes, with explicit refresh and offline cache reads.

**Consequences.** Small private collections maintain no catalog. Public or curated registries can distribute efficient snapshots. `plugin publish` never mutates a catalog; maintainers use explicit catalog operations in an externally serialized workflow. Revision checks remain diagnostic, not an atomic concurrency guarantee. Cached discovery is untrusted metadata and remains subject to source policy; installation independently verifies its selected release. See [Discovery](../discovery.md).

## Open questions

Each needs a decision before the named work package starts.

1. **Windows transport.** Options: named pipes, or TCP loopback with mTLS. go-plugin defaults to TCP on Windows; decide the hardening (random port, loopback binding, handshake token) and whether named pipes are worth the divergence. Owner: platform maintainers with Windows CI; decide before native runtime ships.
2. **Hosted catalog automation.** Decision 26 requires an externally serialized catalog publisher. Select the public registry inventory and CI orchestration before operating a public catalog; no hosted indexing service is part of the native pilot.
3. **Per-plugin telemetry.** What Atmos captures (plugin name, version, extension, operation, duration, error class; never arguments or output), whether the plugin identity is hashed for private registries, and the opt-out. Owner: telemetry owner; decide before the first public release.
4. **`atmos-<name>` executables on PATH.** Options: `atmos plugin run` also accepts gh-style `atmos-<name>` executables found on PATH, or only installed packages. PATH discovery bypasses locking and verification. Owner: command extension owner; decide before the command extension ships.
5. **Contract support lifetime.** `spec.compatibility.atmos` semver ranges are implemented (Decision 23). Decide the published test matrix and how long deprecated contract versions remain supported. Owner: release manager; decide before the first contract is stable.
6. **Monorepo development ergonomics.** A package declaration accepts `path`, usually set in a profile the developer chooses, so the profile deep-merge replaces the former machine-local override file. It is refused under `--frozen`, and policies can restrict it by `origin` and `input.path`. Any additional cross-project override discovery remains a separate authoring design.
7. **Plugin-provided JSON schemas for IDE completion.** Options: publish plugin option schemas to SchemaStore through the existing generated `atmos.yaml` schema, serve them from the package, or expose `atmos plugin schema`. Schema identity must track the locked version. Owner: schemas owner; decide before the first plugin with stack-level options is released.
