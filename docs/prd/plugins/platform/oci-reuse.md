# Shared OCI foundation for plugins

**Status:** Required first-release design. [Delivery plan](delivery-plan.md) · [Distribution](../distribution.md).

Plugins extend the existing OCI implementation. Registry transport and authentication live below plugin lifecycle code so vendoring, scaffolding, source provisioning, version resolution, and future Atmos Artifacts can reuse them. Implementing Atmos Artifacts is not a prerequisite.

## Prior design and current code

The Atmos Artifacts draft is not present in this checkout or at the current tip of `osterman/atmos-artifacts-prd`. This review used `docs/prd/artifacts.md` from saved Git tree `2f81e11338d28a74c2a7fdf18a52d3cd5136ac4a` (checkpoint `2dfb9c31e8`). Relevant sections are **Skopeo Integration Model**, **Auth Ownership**, **Architecture**, and **Emulator Integration Tests**. This records the exact local design evidence without introducing a broken link to an unmerged file.

That draft proposes `containers/image/v5` for image copy/sync and a future generic artifact repository framework. Reuse its semantic boundaries here: immutable descriptors, preserved digest/media type/platform, explicit source and destination credentials, standard OCI layouts, centralized auth, and end-to-end offline round trips. Its library selection applies to that future image workflow; this plugin release retains the existing `go-containerregistry` implementation. A future backend change must sit behind shared interfaces and pass the same conformance suite.

| Existing code | Reuse or convergence work |
|---|---|
| [`pkg/oci/pull.go`](../../../../pkg/oci/pull.go) | `ResolveImage`, `PinDigest`, context-bounded pull, retries, and compatibility entry point `ProcessImage`. Extract descriptor/blob operations from image extraction. |
| [`pkg/oci/auth.go`](../../../../pkg/oci/auth.go) | `GHCRAuth` and current credential fallback behavior. Factor common auth resolution; retain existing precedence for callers. |
| [`pkg/oci/tar.go`](../../../../pkg/oci/tar.go), [`zip.go`](../../../../pkg/oci/zip.go) | Existing materialization and archive safety; generic OCI transfer itself must not extract or assume a Terraform component layer. |
| [`pkg/oci/progress.go`](../../../../pkg/oci/progress.go) | Shared progress and retry observers. |
| [`pkg/version/resolver/oci/oci.go`](../../../../pkg/version/resolver/oci/oci.go) | Existing tag listing, digest HEAD, and separate `remoteOptions`; route these through shared operations while retaining datasource behavior. |
| [`pkg/generator/source/resolver.go`](../../../../pkg/generator/source/resolver.go) | OCI scaffold-template resolution and digest pinning remain consumers of shared OCI. |
| [`pkg/vendoring/install/fetch.go`](../../../../pkg/vendoring/install/fetch.go), [`pkg/provisioner/source/vendor.go`](../../../../pkg/provisioner/source/vendor.go), [`pkg/downloader/artifact.go`](../../../../pkg/downloader/artifact.go) | Preserve pulls, materialization, and provenance; no plugin imports or parallel auth paths. |
| [`pkg/oci/ocitest`](../../../../pkg/oci/ocitest) | Extend the in-process registry harness for index/artifact/layout, auth, and failure tests. |

## Ownership and required operations

`pkg/oci` owns reference parsing, descriptors, raw manifests/indexes/blobs, digest checks, platform selection, tag listing, registry writes/deletes, OCI layout read/write, and signature-material transport required by policy. Common options carry context, credentials, TLS/CA/proxy policy, and progress. Source and destination options are separate. Methods return explicit unsupported-operation errors for registry capabilities that are absent.

`pkg/plugin/oci` is a thin plugin-format layer: plugin media types, release assembly, and package metadata consistency. `pkg/plugin/install`, `catalog`, `discovery`, and `lock` own installation state, compatibility, immutability policy, catalog snapshots, the local discovery index, and lockfiles. Direct discovery uses shared tag, descriptor, and metadata operations; it fetches no executable payload and introduces no registry-specific repository enumeration API. Generic OCI code imports none of these packages. Catalog schema and publication sequencing stay plugin-specific even though catalog bytes use the same OCI transport.

The shared first-release API must cover resolve/inspect, read/write manifests and blobs, read/write indexes, list tags, delete a selected manifest, and import/export OCI layouts. Transfer preserves bytes and digests; platform selection never silently rewrites a locked index. Subject/referrer retrieval needed for the selected signature format is shared, with a documented unsupported/fallback path; plugin installation cannot silently skip required verification.

Keep `ResolveImage`, `PinDigest`, and `ProcessImage` as compatibility entry points over the shared implementation. Migrate the version resolver's direct registry calls to it. This gives at least two production consumers of the common operations without implementing a general artifact manager or migrating every subsystem at once.

## Registry authentication

`atmos plugin registry login/logout` remain required commands. They delegate to shared registry authentication/credential handling, using Atmos identities and existing registry helpers/keychains where applicable; they do not introduce plugin-only credential storage. `registry add/remove` manages source configuration independently of login state.

The artifacts draft places auth ownership in `atmos auth`. Preserve that ownership while exposing the requested plugin command entry points. Initial registry auth must work using built-in identities/helpers before installing any auth plugin. A registry credential chain that requires the plugin being downloaded fails with the auth-cycle diagnostic. Source credentials must never be reused implicitly at the destination, and write/delete failures must never fall back to anonymous mutation.

## Native pilot additions

Shared registry/layout operations now serve plugin release and catalog workflows, and the version resolver uses the common client. `pkg/oci/signature` owns Sigstore verification and trust evidence; shared referrer operations preserve immutable subjects. `pkg/plugin/sign` owns plugin release signing and attachment orchestration. Consumers verify with the Go library; `plugin sign`/`publish --sign` use managed Cosign for author credentials and bundle production.

Plugin-specific admission wraps the shared client in `pkg/plugin/policy` so every actual catalog/package read or mutation is checked before credentials/network access. Generic OCI packages do not import plugin policy. Other existing consumers retain their own governing policy boundaries. This keeps the primitives reusable for future Atmos Artifacts without implementing its repository framework now.

The native pilot accepts `plugins.registries.<alias>.identity`. Shared `pkg/oci` resolution delegates identity-bound hosts to `pkg/auth/registryauth` before consulting ambient credentials. Built-in `oci/user` supplies realm-scoped username/token credentials; cloud identities use a matching ECR/ACR integration. Registry login/logout delegate to that same auth lifecycle, while unbound registries retain explicit keyring credentials and the Docker chain. Authentication is lazy, coalesced for parallel downloads, and refreshed before cached credentials expire. A configured identity failure never falls through to ambient auth. Catalog links do not convey credentials to another host; that host needs its own binding. Registry access must bootstrap without depending on the plugin being downloaded.

## Verification and delivery

WP1.4 first extracts shared operations and migrates existing consumers, then builds the thin plugin format layer. Add regression tests for existing Terraform/OpenTofu layer formats, GHCR/keychain precedence, digest pinning, cancellation, and bounded read retries. Mutating publication/deletion retries must follow explicit reconciliation rules.

Run deterministic round trips: seed a source registry, pull to an OCI layout, publish to a second registry with distinct credentials, disable the source, and restore into fresh local state. Compare index, manifest, and blob digests and execute the selected fixture binary. Cover multiple platforms, auth isolation, private CAs, offline missing blobs, bad digests, unsupported deletion/referrers, and interrupted writes. Reuse the existing registry emulator for opt-in smoke coverage; ordinary tests use `ocitest` and temporary layouts.

Do not add artifact commands, `artifact.repositories`, bundles, image mirroring/sync, generic S3/Git repositories, or SBOM generation to this release. Those future layers can consume the shared primitives established here.
