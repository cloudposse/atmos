# Official public plugin packages

**Status:** Implemented in the working branch; publication requires the protected release environment and public GHCR package provisioning. [All plugin PRDs](README.md).

## Product contract

Atmos supplies the `cloudposse` discovery alias with catalog `oci://ghcr.io/cloudposse/atmos/plugins/catalog:stable`. Search reads curated metadata; it does not enumerate `plugins/*` or download every architecture. Installation resolves an immutable release index and downloads only the matching platform payload. Discovery grants no execution trust.

`plugins.discovery.default_registry` defaults to true. False removes only the implicit alias. An explicitly configured `registries.cloudposse` replaces the complete built-in entry and remains active even when the default is disabled. `registry remove cloudposse` removes an explicit override and persists `default_registry: false`, so removal survives reload. Direct OCI installation works without the catalog.

## Independent monorepo releases

A package has a namespaced manifest identity, an independent stable semantic version, and an explicit OCI repository and shared build inputs in `plugins/releases.yaml`. The manifest version, not the Atmos version or a repository-wide tag, controls its immutable OCI version. Package and shared input changes require version increases for affected packages. Changes to a package inventory entry or its platform runner mappings affect that package; adding an unrelated package does not require existing package bumps.

The latest first-parent commit affecting a package, its declared shared inputs, or inventory identifies its exact build source. Unrelated commits preserve that source. Version comparisons use an explicit base revision; reviewed fixes within a proposed release can span commits. Immutable publication rejects changed bytes at an existing version. CI runs the complete six-platform native test matrix, produces and validates the complete OCI index, serializes package publication, signs it, and verifies anonymous reads. Matching immutable bytes are a retry; differing bytes at the same version are an error.

A separate catalog job acquires a global concurrency lock before fetching current accepted main manifests. It validates available releases by anonymous digest-checked pulls, updates existing catalog metadata while preserving earlier versions, and verifies anonymous catalog access. It does not replace current state with an older prebuilt snapshot. Missing in-flight releases can be reconciled by subsequent successful publication or manual dispatch.

## Native command ownership

`atmos plugin release` owns validation, all-platform builds, current-host tests, temporary packaging, publication, and optional signing/public verification. It does not claim that cross-built targets have passed native tests. `release plan` reads committed inventories and emits deterministic package/platform matrices with explicit runner mappings. `publish --verify-public` checks public GHCR organization visibility before upload and anonymously verifies the full OCI graph afterward. `catalog sync --inventory plugins/releases.yaml` selects versions from committed manifests, verifies the published releases and publishes one merged revision, retaining existing versions and listing/yank state.

`plugin test --build --platform <target> --coverprofile <file>` owns native target compatibility, optional compilation, Go unit coverage, and conformance. Coverage collection preserves authored steps and rejects incompatible hosts.

Workflows own runners, credentials, concurrency, caches, and artifact transfer. The reusable plugin workflow consumes the existing Tests host artifacts and builds only the additional Linux/Windows ARM64 hosts. Plugin builds have their own cache. The native commands are usable outside this repository; publisher namespaces and runner labels are inventory data. Project custom commands are optional aliases. Release functionality must not live in repository-specific Python or Starlark scripts.

## Public distribution

Public source repositories do not imply public GHCR packages. Release and catalog publication fail before upload unless GitHub reports public visibility; anonymous OCI verification follows upload. First publication requires organization administrators to provision public package namespaces and repository Actions access. The implementation does not rely on an undocumented visibility mutation API. Protected environment configuration and public package provisioning are deployment prerequisites, not completed release claims.

## First package: auth-secrets

`oci://ghcr.io/cloudposse/atmos/plugins/auth-secrets` provides both native Go SDK authentication exports:

- `secrets/store`: read explicitly configured logical credentials from Atmos secret stores through invocation-scoped `atmos.api.v1.Secrets.Read`.
- `secrets/env`: project selected `secret-values/v1` fields into validated `environment/v1` credentials, materialized only in the execution environment.

The host retains store selection, backend authentication, masking, and cycle detection. The plugin supplies credential collection and environment projection. Unknown logical names and identity invocations have no secret read grant. Unpinned stores use ambient credentials; pinned store identities are resolved independently and cannot depend on the provider being initialized. Every secret is bounded text and every output field is marked sensitive.

Five-minute envelopes bound process-local identity reuse. Authentication rereads stores; no plugin-private refresh state or cloud credential files are written. Arbitrary payload fields never enter whoami output. Native execution remains under the user's operating-system account.

## Acceptance

- Unit and real native RPC tests cover provider → secret store → identity → command environment, reuse, logout, expiry, cycles, denied names, masking, and backend failure handling.
- Added authored lines meet the 90% coverage target; generated protocol bindings are reported separately.
- Workflows validate manifest versions and shared input changes, all supported platforms, immutable retry behavior, public visibility, anonymous integrity, and serialized catalog reconciliation.
- Documentation includes initial package provisioning, consumer configuration, independent versions, and the remaining protected-environment setup.

Packaged configuration and exported Starlark functions remain separate proposed PRDs; this release does not implement those adapters.
