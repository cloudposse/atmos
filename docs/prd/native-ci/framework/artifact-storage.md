# Native CI Integration - Artifact Storage

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Configuration](./configuration.md) | [Implementation Status](./implementation-status.md)

## Executive Summary

We need to have an artifact storage mechanism that would provide interface for any kind of artifacts stored by atmos. Different atmos ci plugins would store different types of artifacts. For example terraform will store `plan file + terraform lock file + optionally issue summary md + optionally drift summary md`. The interface should guarantee that each artifact related to a sha, component and stack. It should be able to list and fetch artifacts based on these fields.

Primary user and use case is running on CI. We should not store anything if we are not in CI mode. For local running, artifacts are stored only if the `--ci` flag is specified. For CI running, we use the context provided by the current provider from `pkg/ci/providers/*` to get SHA and other context fields.

Atmos should have an internal API that allows programmatically saving something from the local file system as an artifact in some storage, be able to fetch it, and list available artifacts.

## Problem Statement

### Current State

On production we use external GHA to store artifacts. On the current PR we prototyped the planfile storage that works with local file and GitHub Artifacts.

Pain points with the current planfile storage implementation:
1. Artifact storage is tightly coupled with planfile storage. It is tricky to add additional artifacts (e.g., lock file, issue and drift summaries).
2. Planfile storage poorly defines interfaces and methods that connect SHA with an artifact. There is no way to get an artifact by specific SHA or get a filtered list.
3. When running `atmos terraform planfile list` it should automatically get the current SHA and list only artifacts related to it. It should also support `--component` and `--stack` filtering. Only when the `--all` flag is specified should it show all artifacts stored for all SHAs of the current repo. This use case is blocked by the current planfile storage abstract interface definition.

Current production also stores issue and drift summaries and the Terraform lock file in addition to the planfile. In the future we will probably store artifacts for `atmos packer` or `atmos helm` commands as well.

### Desired State

Artifact storage should be a unified abstraction that planfile storage becomes a specialization of.

The system should support any type of files. The specific implementation (plugin) should care about the meaning of the file.

For now the user experience should be transparent — artifacts are automatically stored when running atmos in a CI environment or with the `--ci` flag locally.

## What This Enables

For now we care only about planfile storage as the concrete workflow. The artifact storage abstraction exists to support future use cases.

Artifact storage is a prerequisite for native apply. When running `atmos terraform apply` in a CI environment or with the `--ci` flag, it should pull the plan file for the current SHA and try to apply it.

Artifacts are scoped to a single repo, but repos can share the same storage backend. The storage implementation should guarantee that artifacts from different repos do not collide.

## Functional Requirements

**Artifact types**: Basic artifact storage should support any files as artifacts. Planfile artifact is a group of files: `planfile`, `lock file` + optionally issue summary and drift summary. In the future, other artifact storage implementations will define their own types of artifacts.

**Artifact bundling**: A planfile artifact's files are always uploaded and downloaded together as an atomic operation. The packaging format is backend-specific: GitHub uses a zip archive; S3/local use a directory with raw files. The `ArtifactStore` interface abstracts this — callers provide/receive a set of files, and the backend decides how to store them.

**CRUD operations**: All of Upload, Download, List, Delete, GetMetadata are required for all artifact types.

**Versioning**: Only one version per artifact (same component+stack+SHA). If the backend supports multiple versions, our code will deal only with the latest version.

**Retention**: Backend's responsibility, but we can pass retention settings on artifact creation if the backend supports it (e.g., GitHub Artifacts allows defining TTL on upload).

**Size limits**: No size constraints defined at the moment, neither per artifact nor per backend.

**Integrity**: Use SHA-256 for integrity checking. **IMPLEMENTED**: `BundledStore.Upload()` computes SHA256 of the tar archive and stores it in metadata. `BundledStore.Download()` verifies the checksum before extracting — returns `ErrArtifactIntegrityFailed` on mismatch. When metadata has no SHA256 (backward compat), verification is skipped.

**Concurrency**: Last-write-wins. If two CI runs upload artifacts for the same component+stack simultaneously, the last one overrides the other.

**Authentication**: Most storage backends do not need auth or have it done externally. For AWS S3, we support the atmos auth identity mechanism. The artifact storage interface should have a method to confirm auth is done, e.g., `IsAuthenticated()`.

**Encryption**: Out of scope for now. We rely on backends' native encryption mechanisms.

## Key Design Decisions

**Generalization vs. specialization**: `artifact.Store` is a generic base interface defining the minimum contract common to all artifact types. Plugin-specific stores (e.g., `planfile.Store`) extend it with domain-specific fields and behavior.

**Per-plugin storage architecture**: There is no global artifact storage available to all plugins. Each CI plugin defines its own artifact type, storage interface, registry, and priority list:

1. The **plugin** registers its store backends in its own registry (e.g., `planfile/registry.go`) via `init()` blank imports
2. The **executor** provides a lazy factory closure (`HookContext.CreatePlanfileStore`) that resolves the active store using the config priority list when the handler first needs it
3. The plugin's **handler callback** calls `ctx.CreatePlanfileStore()` to get the store on demand

For example, the terraform plugin owns `planfile.Store` (which extends `artifact.Store`), registers S3/local/GitHub backends in its own registry via blank imports, and resolves its store lazily via `HookContext.CreatePlanfileStore` inside handler callbacks. A future helmfile plugin would define its own artifact type and registry independently.

**Key schema**: Artifacts are keyed by `component/stack/sha`. Sufficient for now; we do not know additional contexts yet.

**Metadata storage**: Stored as a JSON sidecar file named `{artifact-name}.metadata.json` alongside the artifact. Each backend stores the sidecar using its native mechanism (file for local, object for S3, etc.). For GitHub, metadata is embedded in the artifact archive.

**Artifact naming**: The artifact name is defined by the storage implementation layer (e.g., PlanfileStore), not by the backend. For example, PlanfileStore generates names like `{component}-{stack}`. Backends receive this name and decide how to store it: GitHub uses it directly as the GitHub artifact name; Local/S3 backends use their `key_pattern` which can reference the artifact name.

**Backend selection**: Configured per-plugin in `atmos.yaml` (e.g., `components.terraform.planfiles.priority`). The active backend is selected based on environment availability — not artifact availability. For example, if `GITHUB_ACTIONS=true` is set, the GitHub backend is selected; if not, the next backend in the priority list is checked. Once a backend is selected, all operations use it exclusively — there is no fallback on artifact-not-found. If an artifact is missing on the selected backend, the behavior is configurable: fail or ignore. The `--store` flag allows explicitly selecting a backend, bypassing priority detection.

**Multi-backend**: Different plugins can use different backends with independent priority lists and registries. This is a natural consequence of per-plugin storage ownership.

**Registry pattern**: Each plugin has its own store registry (e.g., `pkg/ci/plugins/terraform/planfile/registry.go`). The base `pkg/ci/artifact/registry.go` provides the common registration infrastructure.

## Architecture

### Package Structure

**Location**: `pkg/ci/artifact/` for the general interface; `pkg/ci/plugins/terraform/planfile/` for the planfile implementation.

**Layering**: The planfile storage extends/implements the generic artifact storage interface. The planfile artifact carries plan-specific data (change counts, etc.). `pkg/ci/plugins/terraform` parses Terraform output and sets this data on the planfile artifact.

**Backend packages**: Each backend is its own Go package, following the same pattern as `pkg/ci/providers/`.

### Core Interfaces

**Interface**: `artifact.Store` is the base interface defining common operations (Upload, Download, Delete, List, Exists, GetMetadata) keyed by SHA/Component/Stack with query-based filtering. Each plugin extends this for its domain — `planfile.Store` bundles planfile-related files (plan, lock, summaries) into a single artifact and delegates to `artifact.Store` via the adapter pattern.

**Operations**: Upload, Download, GetMetadata, List, Delete are the right set. The interface should support a query object for filtering to implement the use cases defined in the CLI subcommands section (filter by component, stack, SHA, `--all`).

**Upload input**: `io.Reader` (streaming).

**Download output**: `io.ReadCloser` (streaming).

**Atomicity**: No special conditional operations needed, but upload, download, and delete should each be atomic at the backend implementation level.

### Storage Key Format

- **Key format**: The key format is an **internal implementation detail** of each storage backend. It is not exposed to the user or displayed in CLI output like `atmos terraform planfile list`.
  - **Local/S3/Azure/GCS**: Use a configurable `key_pattern` option (e.g., `{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan`) for file/object paths. Listing and filtering use glob/prefix operations native to the backend.
  - **GitHub Artifacts**: Uses the artifact name provided by the implementation layer (e.g., PlanfileStore provides `{component}-{stack}`). SHA is derived from the workflow run's associated commit, not from the artifact name. Listing works by graph traversal: **SHA -> commit statuses -> workflow runs -> artifacts**, then in-memory filtering by component/stack parsed from artifact names.
- **Artifact type in key**: No automatic prefix based on artifact type. The key format depends entirely on the backend implementation.
- **Timestamp/run ID in key**: No. These should be stored in metadata instead.
- **Key collisions**: Last-write-wins — the artifact is simply overridden.

### Eliminate DynamoDB Dependency

Current GitHub Actions use DynamoDB for planfile metadata. The native implementation stores metadata directly in the same storage backend as the artifact, using JSON sidecar files:

```
.atmos/planfiles/plat-ue2-dev/vpc/abc123.tfplan/              # Artifact directory
.atmos/planfiles/plat-ue2-dev/vpc/abc123.tfplan/plan.tfplan   # Plan file
.atmos/planfiles/plat-ue2-dev/vpc/abc123.tfplan.metadata.json # Metadata sidecar
```

The architecture uses two layers: a generic `artifact.Store` interface (`pkg/ci/artifact/`) and a planfile-specific adapter (`pkg/ci/plugins/terraform/planfile/adapter/`) that wraps artifact.Store, translating between single-file planfile operations and multi-file artifact bundles. See [Planfile Storage](../terraform-plugin/planfile-storage.md) for full details.

### Support All Plan-Storage Backends

Implement a registry pattern (following `pkg/store/`) for artifact storage backends, with planfile storage as an adapter on top:

- **S3** - AWS S3 bucket with metadata sidecar (planfile-level)
- **Azure Blob** - Azure Blob Storage container (deferred)
- **GCS** - Google Cloud Storage bucket (deferred)
- **GitHub Artifacts** - GitHub Artifacts API v4 (IMPLEMENTED)
- **Local** - Local filesystem (both artifact and planfile levels)

## Storage Backend Requirements

### Amazon S3

- **S3 features**: Out of scope. Versioning, lifecycle policies, and encryption depend on the S3 bucket settings, which are external to atmos.
- **S3 metadata**: Use a sidecar file for metadata, not S3 object metadata.
- **IAM permissions**: External to atmos implementation.
- **Cross-account access**: Use atmos auth identities for S3 authentication. Cross-account is handled externally via identity configuration.

### GitHub Artifacts

- **Retention limit**: No effect on design. If an artifact is destroyed by the backend, it simply no longer appears in listings.
- **API version**: Use v4 Artifacts API or higher.
- **In-progress downloads**: Not a limitation with v4. Artifacts are immediately available for download as soon as they are uploaded, even while the workflow run is still in progress.
- **Cross-workflow access**: Yes, based on commit SHA. The GitHub storage should support a tricky lookup: it seeks artifacts related to the current SHA, and if the current SHA is the result of a PR merge/squash, it should also seek artifacts for all commits in the PR from the head back to the first commit that has all commit checks successful.
- **Artifact naming**: The artifact name is provided by the implementation layer (e.g., PlanfileStore provides `{component}-{stack}`). SHA is not part of the artifact name — it comes from the workflow run's associated commit.
- **Listing mechanism**: Lookup is a graph traversal: SHA -> commit statuses -> workflow runs -> artifacts. The full list is fetched into memory, then filtered by component/stack parsed from artifact names. This is fundamentally different from Local/S3 backends which use filesystem glob or prefix listing.

### Local File System

- **Storage directory**: Configurable via `path` option in the `atmos.yaml` store configuration.
- **File locking**: Not in scope.
- **Intended use**: Dev/testing only. Production use is without warranty.

## Context and Metadata

- **CI context source**: Depends on the current provider from `pkg/ci/providers/*`. GitHub provider relies on environment variables. Generic provider uses environment variables only (no git fallback), leaving fields empty if env vars are not set.
- **Metadata extensibility**: No user-defined labels/tags. Only storage implementations can extend metadata (e.g., planfile storage adds `has_changes` field).
- **Metadata struct**: (**IMPLEMENTED**) Base artifact `Metadata` in `pkg/ci/artifact/metadata.go` contains: Stack, Component, SHA, BaseSHA, Branch, PRNumber, RunID, Repository, CreatedAt, ExpiresAt, SHA256, AtmosVersion, Custom. Planfile-specific fields (ComponentPath, PlanSummary, HasChanges, Additions, Changes, Destructions, MD5, TerraformVersion, TerraformTool) remain in the planfile `Metadata` in `pkg/ci/plugins/terraform/planfile/interface.go`.
- **Atmos version in metadata**: Yes, included as `AtmosVersion` in base artifact metadata.
- **Terraform version in metadata**: Yes, included as `TerraformVersion` and `TerraformTool` in planfile-specific metadata (not base artifact metadata, since these are terraform-specific).

## Scope

### In Scope

- **Initial backends**: Local, GitHub Artifacts, and S3. Azure and GCS are deferred to Phase 2+.
- **CLI subcommands**: All five subcommands are implemented: `upload`, `list`, `download`, `delete`, `show`. Upload is also triggered automatically on plan in CI mode.
- **Automatic upload/download**: Yes, automatic upload-on-plan and download-on-deploy are in scope when running in a CI environment or locally with the `--ci` flag. When auto-download is enabled and no planfile is found, deploy **fails with a fatal error** (enforces plan-before-deploy discipline in CI). Note: `apply` does NOT interact with planfile storage — only `deploy` downloads planfiles.
- **Garbage collection**: Out of scope.

### Out of Scope (Phase 2+)

The following are explicitly deferred to Phase 2+:
- **Artifact promotion workflows** (dev -> staging -> prod) — will be implemented eventually
- **Azure Blob Storage and GCS backends** — will be implemented eventually

Not planned:
- Cross-repo artifact sharing
- Webhook notifications on artifact events
- Dashboard / UI for browsing artifacts

Note: Artifact signing/provenance is not needed as a separate feature — SHA-256 is used for integrity verification.

## Testing Strategy

- **Integration tests**: No real cloud backends in tests. Use mocks for all backends.
- **Test credentials**: Not needed — mock all cloud backend calls.
- **Test harness**: Yes, use mocks for the API calls for each backend.
- **Coverage requirement**: 80% minimum code coverage.

## Security Considerations

Out of scope. Security concerns (sensitive data protection, client-side encryption, credential rotation, access control, audit) are deferred.

## Success Criteria

Manual acceptance test cases to verify:
1. GitHub Actions runs save artifacts to configured storage
2. User from local can list artifacts with filters (by component, stack, SHA, `--all`)
3. User from local can download artifacts to local file system
4. All backend methods implemented with unit tests covering 80% code coverage

## Migration Path

No existing data to migrate. No backwards-compatibility or deprecation concerns.

## References

- [Planfile Storage PRD](../terraform-plugin/planfile-storage.md)
- [Store Registry Pattern](../../../../pkg/store/registry.go)
- [CI Provider Detection](../../../../pkg/telemetry/ci.go)
- [GitHub Artifacts API v4](https://docs.github.com/en/actions/using-workflows/storing-workflow-data-as-artifacts)
