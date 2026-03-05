# Native CI Integration - Implementation Status

> Related: [Overview](../overview.md) | [Artifact Storage](./artifact-storage.md) | [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Hooks Integration](./hooks-integration.md)

## Implementation Phases

### Phase 1: Core Infrastructure

1. Create `pkg/ci/` package structure
2. Implement Provider interface and GitHub provider
3. Implement Context and detection
4. Add schema types to `pkg/schema/ci.go`
5. Add `--ci` flag to terraform commands
6. Implement `atmos ci status` command

### Phase 2: Artifact & Planfile Storage

1. Define generic artifact.Store interface (`pkg/ci/artifact/`) — Done
2. Implement artifact store registry and priority-based selector — Done
3. Implement local artifact storage backend (`pkg/ci/artifact/local/`) — Done
4. Implement planfile adapter wrapping artifact.Store (`planfile/adapter/`) — Done
5. Implement PlanfileStore interface — Done
6. Implement S3 store (no DynamoDB) — Done
7. Implement local filesystem planfile store — Done
8. Add `atmos terraform planfile` commands — Done
9. Implement GitHub Artifacts store — Phase 4
10. Azure Blob and GCS stores — Deferred

### Phase 3: Hook Integration

1. Create CI hook commands
2. Register hooks in `pkg/hooks/hooks.go`
3. Integrate into `internal/exec/terraform.go`
4. Implement `--verify-plan` using plan-diff

### Phase 4: Outputs and Comments

1. Implement `$GITHUB_OUTPUT` writer
2. Implement `$GITHUB_STEP_SUMMARY` writer
3. Implement PR comment templates
4. Implement comment upsert behavior
5. Integrate terraform outputs export (using `pkg/terraform/output/`)

### Phase 5: Describe Affected Matrix

1. Add `--format=matrix` flag
2. Implement matrix JSON output
3. Update documentation

### Phase 6: Documentation

1. Archive old GitHub Actions docs
2. Write new CI integration docs
3. Update command reference docs

## Implementation Status Table

| Phase | Description | Status | Completion |
|-------|-------------|--------|------------|
| **Phase 1** | Core Infrastructure | Complete | 100% |
| | pkg/ci/ package structure | Done | |
| | Provider interface and GitHub provider | Done | |
| | Context and detection | Done | |
| | Schema types in pkg/schema/ci.go | Done | |
| | `atmos ci status` command | Done | |
| **Phase 2** | Planfile Storage | In Progress | ~85% |
| | Artifact Store interface (`pkg/ci/artifact/`) | Done | |
| | Artifact local backend (`pkg/ci/artifact/local/`) | Done | |
| | Artifact store registry + selector | Done | |
| | PlanfileStore interface | Done | |
| | PlanfileStore adapter (`planfile/adapter/`) | Done | |
| | S3 store | Done | |
| | GitHub Artifacts store | Phase 4 | |
| | Local filesystem store | Done | |
| | Azure Blob store | Deferred | |
| | GCS store | Deferred | |
| | `atmos terraform planfile` commands | Done | |
| **Phase 3** | Hook Integration | Not Started | 0% |
| | CI hook commands | Not Started | |
| | Register hooks in pkg/hooks/hooks.go | Not Started | |
| | Integrate into internal/exec/terraform.go | Not Started | |
| | `--verify-plan` using plan-diff | Not Started | |
| **Phase 4** | Outputs and Comments | Not Started | 0% |
| | $GITHUB_OUTPUT writer | Not Started | |
| | $GITHUB_STEP_SUMMARY writer | Not Started | |
| | PR comment templates | Not Started | |
| | Comment upsert behavior | Not Started | |
| | Terraform outputs export | Not Started | |
| **Phase 5** | Describe Affected Matrix | Not Started | 0% |
| | `--format=matrix` flag | Not Started | |
| | Matrix JSON output | Not Started | |
| **Phase 6** | Documentation | Not Started | 0% |
| | Archive old GitHub Actions docs | Not Started | |
| | Write new CI integration docs | Not Started | |
| | Update command reference docs | Not Started | |

## Files to Create

| File | Purpose | Status |
|------|---------|--------|
| **pkg/ci/** | | |
| `pkg/ci/provider.go` | Provider interface definition | Done |
| `pkg/ci/context.go` | Context struct (run ID, PR, SHA, etc.) | Done |
| `pkg/ci/status.go` | Status, BranchStatus, PRStatus, CheckStatus structs | Done |
| `pkg/ci/output.go` | OutputWriter interface | Done |
| `pkg/ci/registry.go` | Provider registry (detect and select provider) | Done |
| `pkg/ci/check.go` | CheckRun types and constants | Done |
| `pkg/ci/executor.go` | Execute() - unified action executor | Done |
| `pkg/ci/generic.go` | Generic CI provider fallback | Done |
| `pkg/ci/plugin.go` | Plugin interface | Done |
| `pkg/ci/plugin_registry.go` | Plugin registry | Done |
| **pkg/ci/artifact/** | Generic artifact storage layer | |
| `pkg/ci/artifact/store.go` | Store interface, FileEntry/FileResult, StoreFactory | Done |
| `pkg/ci/artifact/metadata.go` | Metadata, ArtifactInfo structs | Done |
| `pkg/ci/artifact/query.go` | Query struct for filtering | Done |
| `pkg/ci/artifact/registry.go` | Backend registry: Register(), NewStore(), GetRegisteredTypes() | Done |
| `pkg/ci/artifact/selector.go` | EnvironmentChecker, SelectStore() | Done |
| `pkg/ci/artifact/mock_store.go` | Generated mock via mockgen | Done |
| `pkg/ci/artifact/local/store.go` | Local filesystem artifact backend | Done |
| **pkg/ci/plugins/terraform/planfile/** | Planfile storage (wraps artifact layer) | |
| `pkg/ci/plugins/terraform/planfile/interface.go` | planfile.Store interface, Metadata | Done |
| `pkg/ci/plugins/terraform/planfile/registry.go` | Store registry | Done |
| `pkg/ci/plugins/terraform/planfile/adapter/store.go` | Adapter: planfile.Store -> artifact.Store | Done |
| `pkg/ci/plugins/terraform/planfile/adapter/factory.go` | StoreFactory for registry integration | Done |
| `pkg/ci/plugins/terraform/planfile/s3/store.go` | S3 implementation | Done |
| `pkg/ci/plugins/terraform/planfile/github/store.go` | GitHub Artifacts store | Phase 4 |
| `pkg/ci/plugins/terraform/planfile/azure/store.go` | Azure Blob implementation | Deferred |
| `pkg/ci/plugins/terraform/planfile/gcs/store.go` | GCS implementation | Deferred |
| **pkg/ci/github/** | Implements `ci.Provider` interface for GitHub Actions | |
| `pkg/ci/github/provider.go` | GitHub Actions Provider (implements ci.Provider) | Done |
| `pkg/ci/github/client.go` | GitHub API client wrapper (uses go-github v59) | Done |
| `pkg/ci/github/status.go` | GetStatus, GetCombinedStatus, GetCheckRuns | Done |
| `pkg/ci/github/checks.go` | Check runs API | Done |
| `pkg/ci/github/pulls.go` | GetPullRequestsForBranch, GetPullRequestsCreatedByUser, etc. | Phase 4 |
| `pkg/ci/github/user.go` | GetAuthenticatedUser for current user info | Phase 4 |
| `pkg/ci/github/output.go` | $GITHUB_OUTPUT, $GITHUB_STEP_SUMMARY writer | Phase 4 |
| `pkg/ci/github/comment.go` | PR comment templates (tfcmt-inspired) | Phase 4 |
| **pkg/ci/terraform/** | Terraform-specific CI provider | |
| `pkg/ci/terraform/provider.go` | Terraform CI provider | Done |
| `pkg/ci/terraform/parser.go` | Parse plan/apply output | Done |
| `pkg/ci/terraform/context.go` | Terraform template context | Done |
| `pkg/ci/terraform/templates/plan.md` | Default plan template | Done |
| `pkg/ci/terraform/templates/apply.md` | Default apply template | Done |
| **pkg/ci/templates/** | Template loading system | |
| `pkg/ci/templates/loader.go` | Template loading with override support | Done |
| **cmd/terraform/planfile/** | New subcommand group | |
| `cmd/terraform/planfile/planfile.go` | Planfile command group | Done |
| `cmd/terraform/planfile/upload.go` | `atmos terraform planfile upload` | Done |
| `cmd/terraform/planfile/download.go` | `atmos terraform planfile download` | Done |
| `cmd/terraform/planfile/list.go` | `atmos terraform planfile list` | Done |
| `cmd/terraform/planfile/delete.go` | `atmos terraform planfile delete` | Done |
| `cmd/terraform/planfile/show.go` | `atmos terraform planfile show` | Done |
| **cmd/ci/** | New command group | |
| `cmd/ci/ci.go` | CI command group + CICommandProvider | Done |
| `cmd/ci/status.go` | `atmos ci status` | Done |
| **pkg/hooks/** | | |
| `pkg/hooks/ci_upload.go` | CI upload hook command | Phase 3 |
| `pkg/hooks/ci_download.go` | CI download hook command | Phase 3 |
| `pkg/hooks/ci_comment.go` | CI comment hook command | Phase 3 |
| `pkg/hooks/ci_summary.go` | CI summary hook command | Phase 3 |
| `pkg/hooks/ci_output.go` | CI output hook command | Phase 3 |

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/schema/schema.go` | Add top-level `CI CIConfig` field; add `Priority []string` to `PlanfilesConfig` (Done) |
| `pkg/hooks/hooks.go` | Register new CI hook commands |
| `cmd/root.go` | Add blank import `_ "github.com/cloudposse/atmos/cmd/ci"` for registry |
| `cmd/terraform/terraform.go` | Add `--ci` persistent flag |
| `cmd/terraform/plan.go` | Add `--upload-planfile` flags |
| `cmd/terraform/deploy.go` | Add `--download-planfile`, `--verify-plan` flags |
| `cmd/describe/affected.go` | Add `--format=matrix` support |
| `internal/exec/describe_affected.go` | Implement matrix format output |
| `internal/exec/terraform.go` | Integrate CI hooks at lifecycle points |
| `errors/errors.go` | Add CI + artifact + planfile sentinel errors (Done) |
| `pkg/datafetcher/schema/atmos-manifest/*.json` | JSON schema updates |

## Sentinel Errors

```go
// CI errors
ErrCIProviderNotDetected      = errors.New("CI provider not detected")
ErrCIOutputWriteFailed        = errors.New("failed to write CI output")
ErrCIJobSummaryWriteFailed    = errors.New("failed to write job summary")
ErrCIPRCommentFailed          = errors.New("failed to write PR comment")

// Artifact storage errors (IMPLEMENTED)
ErrArtifactNotFound           = errors.New("artifact not found")
ErrArtifactUploadFailed       = errors.New("failed to upload artifact")
ErrArtifactDownloadFailed     = errors.New("failed to download artifact")
ErrArtifactDeleteFailed       = errors.New("failed to delete artifact")
ErrArtifactListFailed         = errors.New("failed to list artifacts")
ErrArtifactStoreNotFound      = errors.New("artifact store not found")
ErrArtifactStoreInvalidArgs   = errors.New("invalid artifact store arguments")
ErrArtifactMetadataFailed     = errors.New("failed to load artifact metadata")
ErrArtifactIntegrityFailed    = errors.New("artifact integrity check failed")

// Planfile storage errors (IMPLEMENTED)
ErrPlanfileNotFound           = errors.New("planfile not found")
ErrPlanfileUploadFailed       = errors.New("failed to upload planfile")
ErrPlanfileDownloadFailed     = errors.New("failed to download planfile")
ErrPlanfileDeleteFailed       = errors.New("failed to delete planfile")
ErrPlanfileListFailed         = errors.New("failed to list planfiles")
ErrPlanfileStoreNotFound      = errors.New("planfile store not found")
ErrPlanfileKeyInvalid         = errors.New("planfile key generation failed: stack, component, and SHA are required")
ErrPlanfileStatFailed         = errors.New("failed to check planfile status")
ErrPlanfileMetadataFailed     = errors.New("failed to load planfile metadata")
ErrPlanfileStoreInvalidArgs   = errors.New("invalid planfile store arguments")
ErrPlanfileDeleteRequireForce = errors.New("deletion requires --force flag")

// GitHub errors
ErrGitHubTokenNotFound        = errors.New("GitHub token not found")
ErrGitHubRuntimeURLNotFound   = errors.New("GitHub runtime URL not found")
ErrGitHubArtifactAPIError     = errors.New("GitHub Artifacts API error")
```

## Additional Components Implemented (Beyond Original PRD)

| Component | File | Purpose |
|-----------|------|---------|
| Check types | `pkg/ci/check.go` | CheckRun types and constants |
| Generic provider | `pkg/ci/generic.go` | Fallback CI provider for non-GitHub environments |
| Plugin | `pkg/ci/plugin.go` | Plugin interface for terraform/helmfile |
| Plugin registry | `pkg/ci/plugin_registry.go` | Registry for component-type plugins |
| Executor | `pkg/ci/executor.go` | Unified action executor |
| Terraform provider | `pkg/ci/terraform/` | Terraform-specific CI behavior |
| Template loader | `pkg/ci/templates/loader.go` | Template loading with override support |
| GitHub checks | `pkg/ci/github/checks.go` | GitHub check runs API |

## Artifact Storage Implementation Details

### Phase 1: Artifact Interface (SHIPPED)

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

### Phase 2: Local Backend (SHIPPED)

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

### Phase 3: Planfile Adapter (SHIPPED)

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

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-12-18 | Updated PRD with implementation status, documented additional components |
| 1.0 | 2025-12-17 | Initial PRD |
