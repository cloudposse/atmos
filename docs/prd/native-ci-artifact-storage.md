# Native CI Integration - Artifact Storage

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

**Questions to clarify:**
- **Artifact types**: Basic artifact storage should support any files as artifacts. Planfile artifact is a group of files: `planfile`, `lock file` + optionally issue summary and drift summary. In the future, other artifact storage implementations will define their own types of artifacts.
- **Artifact bundling**: A planfile artifact's files are always uploaded and downloaded together as an atomic operation. The packaging format is backend-specific: GitHub uses a zip archive; S3/local use a directory with raw files. The `ArtifactStore` interface abstracts this — callers provide/receive a set of files, and the backend decides how to store them.
- **CRUD operations**: All of Upload, Download, List, Delete, GetMetadata are required for all artifact types.
- **Versioning**: Only one version per artifact (same component+stack+SHA). If the backend supports multiple versions, our code will deal only with the latest version.
- **Retention**: Backend's responsibility, but we can pass retention settings on artifact creation if the backend supports it (e.g., GitHub Artifacts allows defining TTL on upload).
- **Size limits**: No size constraints defined at the moment, neither per artifact nor per backend.
- **Integrity**: Use SHA-256 for integrity checking.
- **Concurrency**: Last-write-wins. If two CI runs upload artifacts for the same component+stack simultaneously, the last one overrides the other.
- **Authentication**: Most storage backends do not need auth or have it done externally. For AWS S3, we support the atmos auth identity mechanism. The artifact storage interface should have a method to confirm auth is done, e.g., `IsAuthenticated()`.
- **Encryption**: Out of scope for now. We rely on backends' native encryption mechanisms.

## Key Design Decisions

**Questions to clarify:**
- **Generalization vs. specialization**: `ArtifactStore` should be a generic interface. `PlanfileStore` builds on top of it.
- **Key schema**: Artifacts are keyed by `component/stack/sha`. Sufficient for now; we do not know additional contexts yet.
- **Metadata storage**: Stored as a JSON sidecar file named `{artifact-name}.metadata.json` alongside the artifact. Each backend stores the sidecar using its native mechanism (file for local, object for S3, etc.). For GitHub, metadata is embedded in the artifact archive.
- **Artifact naming**: The artifact name is defined by the storage implementation layer (e.g., PlanfileStore), not by the backend. For example, PlanfileStore generates names like `{component}-{stack}`. Backends receive this name and decide how to store it: GitHub uses it directly as the GitHub artifact name; Local/S3 backends use their `key_pattern` which can reference the artifact name.
- **Backend selection**: Configured per-project in `atmos.yaml`. The active backend is selected from `components.terraform.planfiles.priority` based on environment availability — not artifact availability. For example, if `GITHUB_ACTIONS=true` is set, the GitHub backend is selected; if not, the next backend in the priority list is checked. Once a backend is selected, all operations use it exclusively — there is no fallback on artifact-not-found. If an artifact is missing on the selected backend, the behavior is configurable: fail or ignore. The `--store` flag allows explicitly selecting a backend, bypassing priority detection. Example config:

```yaml
components:
  terraform:
    planfiles:
      priority:
        - "github"
        - "s3"
        - "local"
      stores:
        s3:
          type: s3
          options:
            bucket: "my-terraform-planfiles"
            prefix: "atmos/"
            region: "us-east-1"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"
        github:
          type: github-artifacts
          options:
            retention_days: 7
            owner: cloudposse
            repo: github-action-atmos-terraform-plan
            # GitHub uses the artifact name from the implementation layer directly
        azure:
          type: azure-blob
          options:
            account: "mystorageaccount"
            container: "planfiles"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"
        gcs:
          type: gcs
          options:
            bucket: "my-gcs-bucket"
            prefix: "planfiles/"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"
        local:
          type: local
          options:
            path: ".atmos/planfiles"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"
```
- **Multi-backend**: Yes, different artifact types can use different backends, but not a focus for now.
- **Registry pattern**: Use `pkg/ci/artifacts/registry.go` for backend registration.

## Configuration

Configuration lives in `atmos.yaml` under `components.terraform.planfiles`. It supports multiple named stores with a priority-based fallback mechanism. Backend-specific options are provided via the `options` map for each store.

```yaml
components:
  terraform:
    # Planfile storage backends (registry pattern)
    planfiles:
      # Stores are tried in priority order; if unavailable, fall through to next
      priority:
        - "github"
        - "s3"
        - "local"

      # Named stores — each backend has its own key/naming pattern
      stores:
        s3:
          type: s3
          options:
            bucket: "my-terraform-planfiles"
            prefix: "atmos/"
            region: "us-east-1"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"

        github:
          type: github-artifacts
          options:
            retention_days: 7
            owner: cloudposse
            repo: github-action-atmos-terraform-plan
            # GitHub uses the artifact name from the implementation layer directly

        azure:
          type: azure-blob
          options:
            account: "mystorageaccount"
            container: "planfiles"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"

        gcs:
          type: gcs
          options:
            bucket: "my-gcs-bucket"
            prefix: "planfiles/"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"

        local:
          type: local
          options:
            path: ".atmos/planfiles"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"
```

## Scope

### In Scope

**Questions to clarify:**
- **Initial backends**: Local, GitHub Artifacts, and S3. Azure and GCS are deferred to Phase 2+.
- **CLI subcommands**: `upload` is not a CLI command (transparent on plan). The following subcommands are in scope: `list`, `download`, `delete`, `show`.

  **list** — Lists planfile artifacts with filtering by component, stack, and SHA:
  ```bash
  atmos terraform planfile {component} -s {stack} list        # specific component+stack, current SHA
  atmos terraform planfile {component} -s {stack} list --all  # specific component+stack, all SHAs
  atmos terraform planfile list                                # all components+stacks, current SHA
  atmos terraform planfile list --all                          # all components+stacks, all SHAs
  atmos terraform planfile -s {stack} list                     # all components, specific stack, current SHA
  atmos terraform planfile -s {stack} list --all               # all components, specific stack, all SHAs
  ```

  **download** — Downloads a planfile artifact:
  ```bash
  atmos terraform planfile {component} -s {stack} download    # specific component+stack, current SHA
  ```

  **delete** — Deletes planfile artifacts with confirmation prompt (skippable with `--yes`):
  ```bash
  atmos terraform planfile {component} -s {stack} delete        # specific component+stack, current SHA
  atmos terraform planfile {component} -s {stack} delete --all  # specific component+stack, all SHAs
  atmos terraform planfile {component} delete                   # specific component, all stacks, current SHA
  atmos terraform planfile {component} delete --all             # specific component, all stacks, all SHAs
  atmos terraform planfile delete                                # all components+stacks, current SHA
  atmos terraform planfile delete --all                          # all components+stacks, all SHAs
  ```
  Before delete, show the list of artifacts to be removed and ask for confirmation. Skip with `--yes` flag.

  **show** — Shows planfile artifact metadata:
  ```bash
  atmos terraform planfile {component} -s {stack} show         # specific component+stack, current SHA
  ```
- **Automatic upload/download**: Yes, automatic upload-on-plan and download-on-apply are in scope when running in a CI environment or locally with the `--ci` flag.
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

## Architecture

### Package Structure

**Questions to clarify:**
- **Location**: `pkg/ci/artifact/` for the general interface; `pkg/ci/plugins/terraform/planfile/` for the planfile implementation.
- **Layering**: The planfile storage extends/implements the generic artifact storage interface. The planfile artifact carries plan-specific data (change counts, etc.). `pkg/ci/plugins/terraform` parses Terraform output and sets this data on the planfile artifact.
- **Backend packages**: Each backend is its own Go package, following the same pattern as `pkg/ci/providers/`.

### Core Interfaces

**Questions to clarify:**
- **Interface**: Generic `ArtifactStore` interface (as established in Key Design Decisions). `PlanfileStore` extends `ArtifactStore` — it bundles all planfile-related files (plan, lock, summaries) into a single artifact and delegates to `ArtifactStore` once.
- **Operations**: Upload, Download, GetMetadata, List, Delete are the right set. The interface should support a query object for filtering to implement the use cases defined in the CLI subcommands section (filter by component, stack, SHA, `--all`).
- **Upload input**: `io.Reader` (streaming).
- **Download output**: `io.ReadCloser` (streaming).
- **Atomicity**: No special conditional operations needed, but upload, download, and delete should each be atomic at the backend implementation level.

### Storage Key Format

- **Key format**: The key format is an **internal implementation detail** of each storage backend. It is not exposed to the user or displayed in CLI output like `atmos terraform planfile list`.
  - **Local/S3/Azure/GCS**: Use a configurable `key_pattern` option (e.g., `{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan`) for file/object paths. Listing and filtering use glob/prefix operations native to the backend.
  - **GitHub Artifacts**: Uses the artifact name provided by the implementation layer (e.g., PlanfileStore provides `{component}-{stack}`). SHA is derived from the workflow run's associated commit, not from the artifact name. Listing works by graph traversal: **SHA → commit statuses → workflow runs → artifacts**, then in-memory filtering by component/stack parsed from artifact names.
- **Artifact type in key**: No automatic prefix based on artifact type. The key format depends entirely on the backend implementation.
- **Timestamp/run ID in key**: No. These should be stored in metadata instead.
- **Key collisions**: Last-write-wins — the artifact is simply overridden.

## CLI Commands and Flags

**Questions to clarify:**
- **CLI subcommands**: `upload` is not a CLI command (transparent on plan). The following subcommands are in scope: `list`, `download`, `delete`, `show`.

  **list** — Lists planfile artifacts with filtering by component, stack, and SHA:
  ```bash
  atmos terraform planfile {component} -s {stack} list        # specific component+stack, current SHA
  atmos terraform planfile {component} -s {stack} list --all  # specific component+stack, all SHAs
  atmos terraform planfile list                                # all components+stacks, current SHA
  atmos terraform planfile list --all                          # all components+stacks, all SHAs
  atmos terraform planfile -s {stack} list                     # all components, specific stack, current SHA
  atmos terraform planfile -s {stack} list --all               # all components, specific stack, all SHAs
  ```

  **download** — Downloads a planfile artifact:
  ```bash
  atmos terraform planfile {component} -s {stack} download    # specific component+stack, current SHA
  ```

  **delete** — Deletes planfile artifacts with confirmation prompt (skippable with `--yes`):
  ```bash
  atmos terraform planfile {component} -s {stack} delete        # specific component+stack, current SHA
  atmos terraform planfile {component} -s {stack} delete --all  # specific component+stack, all SHAs
  atmos terraform planfile {component} delete                   # specific component, all stacks, current SHA
  atmos terraform planfile {component} delete --all             # specific component, all stacks, all SHAs
  atmos terraform planfile delete                                # all components+stacks, current SHA
  atmos terraform planfile delete --all                          # all components+stacks, all SHAs
  ```
  Before delete, show the list of artifacts to be removed and ask for confirmation. Skip with `--yes` flag.

  **show** — Shows planfile artifact metadata:
  ```bash
  atmos terraform planfile {component} -s {stack} show         # specific component+stack, current SHA
  ```

- **`--store` flag**: Accepts a named store from config.
- **`--format` flag**: Yes, support `json`, `yaml`, and `table` output formats.
- **`list` filtering**: For now, only filtering by SHA (current SHA by default, `--all` for all SHAs).
- **`download` output**: No stdout piping. Downloads to the local file system only. Files are placed into the component directory resolved from atmos stacks configuration:
  - **Plan file**: Uses the planfile name generated by atmos for the given `{component}` + `{stack}` (from stacks config). Can be overridden with `--planfile-name` flag.
  - **Lock file**: Downloaded as `.terraform.lock.hcl` into the component directory.
- **Command group**: `atmos terraform planfile` is correct. Artifacts in general do not need a CLI interface — they define a generic framework for artifact storage in atmos. Specific implementations (like planfile) expose their own CLI commands.

## Storage Backend Requirements

### Amazon S3

**Questions to clarify:**
- **S3 features**: Out of scope. Versioning, lifecycle policies, and encryption depend on the S3 bucket settings, which are external to atmos.
- **S3 metadata**: Use a sidecar file for metadata, not S3 object metadata.
- **IAM permissions**: External to atmos implementation.
- **Cross-account access**: Use atmos auth identities for S3 authentication. Cross-account is handled externally via identity configuration.

### GitHub Artifacts

**Questions to clarify:**
- **Retention limit**: No effect on design. If an artifact is destroyed by the backend, it simply no longer appears in listings.
- **API version**: Use v4 Artifacts API or higher.
- **In-progress downloads**: Not a limitation with v4. Artifacts are immediately available for download as soon as they are uploaded, even while the workflow run is still in progress.
- **Cross-workflow access**: Yes, based on commit SHA. The GitHub storage should support a tricky lookup: it seeks artifacts related to the current SHA, and if the current SHA is the result of a PR merge/squash, it should also seek artifacts for all commits in the PR from the head back to the first commit that has all commit checks successful.
- **Artifact naming**: The artifact name is provided by the implementation layer (e.g., PlanfileStore provides `{component}-{stack}`). SHA is not part of the artifact name — it comes from the workflow run's associated commit.
- **Listing mechanism**: Lookup is a graph traversal: SHA → commit statuses → workflow runs → artifacts. The full list is fetched into memory, then filtered by component/stack parsed from artifact names. This is fundamentally different from Local/S3 backends which use filesystem glob or prefix listing.

### Local File System

**Questions to clarify:**
- **Storage directory**: Configurable via `path` option in the `atmos.yaml` store configuration.
- **File locking**: Not in scope.
- **Intended use**: Dev/testing only. Production use is without warranty.

## Context and Metadata

**Questions to clarify:**
- **CI context source**: Depends on the current provider from `pkg/ci/providers/*`. GitHub provider relies on environment variables. Generic provider uses environment variables with fallback to git, or leaves fields empty if both fail.
- **Metadata extensibility**: No user-defined labels/tags. Only storage implementations can extend metadata (e.g., planfile storage adds `has_changes` field).
- **Metadata struct**: (**IMPLEMENTED**) Base artifact `Metadata` in `pkg/ci/artifact/metadata.go` contains: Stack, Component, SHA, BaseSHA, Branch, PRNumber, RunID, Repository, CreatedAt, ExpiresAt, SHA256, AtmosVersion, Custom. Planfile-specific fields (ComponentPath, PlanSummary, HasChanges, Additions, Changes, Destructions, MD5, TerraformVersion, TerraformTool) remain in the planfile `Metadata` in `pkg/ci/plugins/terraform/planfile/interface.go`.
- **Atmos version in metadata**: Yes, included as `AtmosVersion` in base artifact metadata.
- **Terraform version in metadata**: Yes, included as `TerraformVersion` and `TerraformTool` in planfile-specific metadata (not base artifact metadata, since these are terraform-specific).

## Implementation Phases

Incremental approach using TDD: create tests, implement to pass tests, refactor (reduce duplication) while verifying tests still pass.

1. **Phase 1**: Define Artifacts Basic Interface/Struct — **SHIPPED**
2. **Phase 2**: Implement local artifact storage — **SHIPPED**
3. **Phase 3**: Implement Planfile storage (on top of artifact storage) — **SHIPPED**
4. **Phase 4**: Implement GitHub backend

**Questions to clarify:**
- **Dependencies**: CI provider detection is already implemented in `pkg/ci/providers/*`. No blocking dependencies.

### Phase 1 Implementation Details (SHIPPED)

**Package**: `pkg/ci/artifact/`

**Files created:**

| File | Purpose |
|------|---------|
| `metadata.go` | `Metadata` struct (Stack, Component, SHA, BaseSHA, Branch, PRNumber, RunID, Repository, CreatedAt, ExpiresAt, SHA256, AtmosVersion, Custom) and `ArtifactInfo` struct (Name, Size, LastModified, Metadata) |
| `query.go` | `Query` struct with `Components []string`, `Stacks []string`, `SHAs []string`, `All bool` — supports multi-value filtering |
| `store.go` | `Store` interface (Name, Upload, Download, Delete, List, Exists, GetMetadata), `FileEntry`/`FileResult` structs for bundle upload/download, `StoreOptions`, `StoreFactory` type, `//go:generate mockgen` directive |
| `registry.go` | Thread-safe backend registry: `Register()`, `NewStore()`, `GetRegisteredTypes()` — follows same pattern as `pkg/ci/plugins/terraform/planfile/registry.go` |
| `selector.go` | `EnvironmentChecker` interface and `SelectStore()` function for priority-based backend selection with explicit `--store` override |
| `mock_store.go` | Generated mock via `go.uber.org/mock/mockgen` |
| `metadata_test.go` | JSON round-trip tests, nil optional fields |
| `registry_test.go` | Register/NewStore, panics on invalid args, GetRegisteredTypes |
| `selector_test.go` | Priority selection, explicit override, no-available-store error, no-checker-means-available |
| `store_test.go` | Interface compile checks, struct field assertions |

**Files modified:**

| File | Change |
|------|--------|
| `errors/errors.go` | Added 9 sentinel errors: `ErrArtifactNotFound`, `ErrArtifactUploadFailed`, `ErrArtifactDownloadFailed`, `ErrArtifactDeleteFailed`, `ErrArtifactListFailed`, `ErrArtifactStoreNotFound`, `ErrArtifactStoreInvalidArgs`, `ErrArtifactMetadataFailed`, `ErrArtifactIntegrityFailed` |
| `pkg/schema/schema.go` | Added `Priority []string` field to `PlanfilesConfig` for backend selection order |
| `pkg/ci/plugins/terraform/planfile/interface.go` | Added `TerraformVersion` and `TerraformTool` fields to planfile `Metadata` (moved from artifact layer — these are planfile-specific) |

**Design decisions applied:**
- `Upload` accepts `[]FileEntry` and `Download` returns `[]FileResult` to support multi-file artifact bundles (plan + lock + summaries).
- `Query` uses `[]string` slices (not single strings) for `Components`, `Stacks`, `SHAs` to support multi-value filtering in CLI commands.
- `TerraformVersion` and `TerraformTool` live in planfile `Metadata`, not artifact `Metadata` — they are terraform-specific concerns.
- `EnvironmentChecker.IsAvailable()` takes `context.Context` for consistency; backends without a checker are treated as available.
- 17 tests pass with 42.2% statement coverage (registry/selector logic fully covered; metadata structs covered via JSON round-trips).

### Phase 2 Implementation Details (SHIPPED)

**Package**: `pkg/ci/artifact/local/`

**Files created:**

| File | Purpose |
|------|---------|
| `store.go` | Local filesystem `Store` implementation — all 7 interface methods (Name, Upload, Download, Delete, List, Exists, GetMetadata), configurable `path` option with tilde expansion, SHA-256 integrity checking, metadata sidecar files (`.metadata.json`), multi-file artifact bundles, query-based listing with Components/Stacks/SHAs/All filtering, path traversal protection, empty directory cleanup, auto-registration via `init()` |
| `store_test.go` | 30 test functions covering: upload/download cycles, single and multi-file artifacts, deletion with cleanup, existence checks, metadata retrieval with and without sidecar, SHA-256 verification, listing with all filter combinations, path traversal security (20 subtests), name validation, full lifecycle integration test |

**Design decisions applied:**
- Metadata stored as JSON sidecar files (`{artifact-name}.metadata.json`) alongside the artifact directory — consistent with PRD's "sidecar file" decision.
- Path traversal protection rejects names containing `..` to prevent directory escape attacks.
- `GetMetadata` falls back to directory modification time when no sidecar exists.
- `List` returns results sorted newest-first by last modified time.
- `Delete` is idempotent — safe to call on nonexistent artifacts.
- Empty parent directories are cleaned up after deletion.
- Auto-registers with `artifact.Register("local", NewStore)` in `init()`.
- 30 tests pass with 81.3% statement coverage (exceeds 80% requirement).

### Phase 3 Implementation Details (SHIPPED)

**Package**: `pkg/ci/plugins/terraform/planfile/adapter/`

**Files created:**

| File | Purpose |
|------|---------|
| `store.go` | Adapter implementing `planfile.Store` by wrapping `artifact.Store` — wraps single `io.Reader` as `[]artifact.FileEntry{plan.tfplan}` on upload, extracts `plan.tfplan` from `[]artifact.FileResult` on download (closing other file handles), bidirectional metadata conversion via `artifact.Metadata.Custom` with `planfile.*` prefixed keys, prefix-to-query conversion for List, compile-time interface check |
| `factory.go` | `NewStoreFactory(artifactBackend)` returns a `planfile.StoreFactory` for registry integration |
| `store_test.go` | 16 tests using `artifact.MockStore`: Name delegation, Upload with metadata verification, Upload with nil metadata, Download with plan extraction, Download with no plan file error, Download not-found propagation, Delete delegation, List with prefix conversion, List empty, Exists delegation, GetMetadata conversion, GetMetadata not-found, metadata round-trip preservation, nil metadata handling, prefix-to-query table-driven tests, factory integration |

**Metadata mapping strategy:**

Planfile-specific fields are stored in `artifact.Metadata.Custom` using `planfile.` prefixed keys:

| Planfile Field | Custom Key | Conversion |
|---|---|---|
| `ComponentPath` | `planfile.component_path` | string |
| `PlanSummary` | `planfile.plan_summary` | string |
| `HasChanges` | `planfile.has_changes` | `strconv.FormatBool` / `strconv.ParseBool` |
| `Additions` | `planfile.additions` | `strconv.Itoa` / `strconv.Atoi` |
| `Changes` | `planfile.changes` | `strconv.Itoa` / `strconv.Atoi` |
| `Destructions` | `planfile.destructions` | `strconv.Itoa` / `strconv.Atoi` |
| `TerraformVersion` | `planfile.terraform_version` | string |
| `TerraformTool` | `planfile.terraform_tool` | string |

**Prefix-to-query conversion:**

The adapter parses `List(ctx, prefix)` prefixes based on the default key pattern `{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan`:

| Prefix | Query |
|---|---|
| `""` (empty) | `Query{All: true}` |
| `"stack1"` | `Query{Stacks: ["stack1"]}` |
| `"stack1/component1"` | `Query{Stacks: ["stack1"], Components: ["component1"]}` |
| `"stack1/component1/sha"` | `Query{Stacks: ["stack1"], Components: ["component1"], SHAs: ["sha"]}` |

**Design decisions applied:**
- Adapter pattern chosen over rewrite — existing `planfile.Store` consumers (6+ locations) remain unchanged.
- Each adapter method makes exactly one backend call, then translates the result.
- Non-plan file handles are closed on download to prevent resource leaks.
- Common metadata fields (Stack, Component, SHA, etc.) map directly between interfaces; planfile-specific fields use the `Custom` map.
- `NewStoreFactory` enables registry integration so the adapter can be registered as a planfile store type.
- No existing files modified — purely additive package.
- 16 tests pass with 95.6% statement coverage.

## Testing Strategy

**Questions to clarify:**
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

- [Existing Planfile Storage PRD](./native-ci-planfile-storage.md)
- [Store Registry Pattern](../../pkg/store/registry.go)
- [CI Provider Detection](../../pkg/telemetry/ci.go)
- [GitHub Artifacts API v4](https://docs.github.com/en/actions/using-workflows/storing-workflow-data-as-artifacts)
