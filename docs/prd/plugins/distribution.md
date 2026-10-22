# Precompiled plugin distribution

**Status:** Native OCI distribution implemented in the working branch. The full platform/release matrix remains an acceptance target; see the [first-release checkpoint](first-release.md#native-pilot-checkpoint). [All plugin PRDs](README.md).

## Consumer guarantee

Customers install precompiled native plugins using the standard released Atmos binary. Installing, updating, restoring, or removing a plugin never builds Atmos, replaces it with a custom binary, resolves Go modules, or compiles downloaded source. Consumers do not need Go, Git, or a compiler for plugin installation. Tools that a plugin invokes are separate declared dependencies.

Native authors compile their plugin once per supported operating system/architecture in their own release pipeline. Consumers receive the matching compiled artifact. Go is a supported authoring language, not an installation requirement; another language can implement the open protocol. Local source builds remain explicit author development steps.

OCI transport and credential handling extend the existing shared implementation; see [Shared OCI foundation](platform/oci-reuse.md). Plugin media types and release assembly stay in the plugin layer. Starlark/data examples below describe follow-on runtimes.

## One release, multiple platform artifacts

An example release supports `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`. These are illustrative; each publisher declares and tests its actual support matrix. Platform variants and runtime/ABI requirements must also be declared where relevant. Do not assume that a matching CPU architecture guarantees executable compatibility.

```text
oci://ghcr.io/acme/atmos-plugins/auth:1.2.0
  OCI index: sha256:<release-digest>
    linux/amd64   -> manifest + compiled executable + schemas
    linux/arm64   -> manifest + compiled executable + schemas
    darwin/amd64  -> manifest + compiled executable + schemas
    darwin/arm64  -> manifest + compiled executable + schemas
    windows/amd64 -> manifest + compiled executable + schemas
```

Native packages are executable artifacts stored in OCI, not containers that require a container runtime to run. Use Atmos-specific artifact/media types, an OCI index for platform selection, and immutable child manifest/layer digests. Platform payloads must agree on plugin identity, release version, capability contracts, and configuration schemas. `pack` rejects a mixed or inconsistent release.

The existing author command packs already-built outputs according to the manifest's platform-to-payload mapping:

```shell
atmos plugin pack . --output dist/acme-auth
atmos plugin publish dist/acme-auth --to oci://ghcr.io/acme/atmos-plugins/auth
```

The generated release workflow builds the declared matrix, tests binaries on corresponding runners, collects outputs into the package layout, and publishes the complete release only after all declared platforms pass. Cross-compilation alone is not a runtime smoke test. Signing/attestation helpers cover the release index and its referenced digests under the configured trust policy.

## Resolution, locking, and offline restore

1. Explicit installation resolves a version, validates the release metadata and supported contracts, and resolves the OCI index to an immutable digest.
2. The lockfile records that release index digest and the platform manifest digests advertised by the verified index. Lock creation may fetch metadata for all platforms; it downloads only the selected platform's executable payload by default.
3. Atmos selects the platform of its running process, reports that selection, verifies the child manifest and layers, and installs atomically. Existing selections remain usable after interrupted installation.
4. A matching `install --frozen` on a different supported platform restores the child artifact recorded for that platform without editing the lockfile or resolving the tag again.
5. Unsupported OS, architecture, variant, or declared runtime requirement fails before execution with the available platforms and required compatibility. Installation never falls back to compiling source or silently choosing an emulated binary.
6. Offline restore uses supplied OCI layouts or verified cached artifacts containing the required index, manifest, layers, and verification material. Missing platform payloads produce an explicit offline-artifact error.

A moved tag cannot change a frozen install. Adding or changing any platform payload changes the release index and requires a new immutable plugin version. Two machines on the same platform restore identical bytes; machines on different platforms restore their respective locked binaries from the same release.

Starlark and data packages use portable payloads with runtime/API or engine requirements instead of native binary selection. Their invoked tools may still have platform requirements. Package-local modules and rules are included in the verified artifact; evaluation does not download code.

## Signing, policy, and shared storage

`atmos plugin publish <layout> --to <repository> --sign` publishes an index and signs its immutable digest; `atmos plugin sign <repository>@sha256:<digest>` signs an existing release. Signing uses managed Cosign and publishes standard Sigstore bundles as OCI subject/referrer artifacts. Consumer verification uses `pkg/oci/signature`, checks the exact index digest and authenticated identity/issuer, and revalidates cached evidence against current requirements. Bare public keys, legacy envelopes, and custom trust roots are outside the pilot. A signing failure after publication must report the already-published artifact; artifact and signature writes are separate operations.

General [plugin policy](policy.md) checks the actual destination before OCI access and statically inspected metadata before installation, publication, signing, or activation. An approved catalog cannot authorize a package on a prohibited registry. Local layouts are marked `origin: layout`, because their declared source alone does not prove where the bytes originated. Cached activation uses the protected project lock and rechecks policy and integrity. Registry allowlists and publisher verification solve different requirements and may be combined.

Verified payloads are shared across projects in XDG data storage at `atmos/plugins/releases/<index-digest-hex>/<os>/<arch>[/<variant>]/`, with an OCI layout and materialized `payload/` beneath each platform directory. `plugins.cache_dir` can override the root. Projects own declarations and locks, not duplicate payloads; local removal does not delete shared releases. Independent downloads use `plugins.max_concurrency` (default four) and the shared toolchain batch UI. Configuration and lock changes commit only after the full requested installation succeeds.

The OCI model resembles Helm in using versioned artifacts and digest-based retrieval from existing registries, but the formats are distinct. Atmos uses a release index with native platform children. Discovery optionally reads published catalog snapshots or builds a cached index from an explicit list of OCI repositories. Both supply the same search interface without fetching executable payloads or enumerating the whole registry. Release publication does not update a shared catalog; catalog maintainers publish snapshots separately through an externally serialized workflow. See [Discovery](discovery.md). Catalog generation validates releases using known extension descriptors; an older Atmos cannot add an unknown future capability.

## Acceptance criteria

- A clean consumer environment without Go, Git, or a compiler installs a compiled plugin and uses it through the unchanged standard Atmos executable.
- Native plugin releases are tested across their declared platform matrix. At least two OS/architecture combinations restore their correct artifacts from one committed lockfile.
- Tests compare the Atmos binary before and after installation/update/rollback and confirm no Atmos rebuild or replacement occurred.
- A frozen restore rejects moved tags, corrupt indexes/layers, unsupported platforms, and missing offline payloads without building source or changing the lockfile.
- An interrupted install leaves the prior usable installation intact; retry verifies artifacts before completing.
- Plugin authors can release a compatible version using an independent dependency graph without matching Atmos's Go compiler or library versions.
