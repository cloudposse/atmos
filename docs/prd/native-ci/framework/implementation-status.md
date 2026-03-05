# Native CI Integration - Implementation Status

> Related: [Overview](../overview.md) | [Artifact Storage](./artifact-storage.md) | [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Hooks Integration](./hooks-integration.md)

## Implementation Phases

### Phase 1: Core Infrastructure — COMPLETE

1. Create `pkg/ci/` package structure — Done
2. Implement Provider interface and GitHub provider — Done
3. Implement Context and detection — Done
4. Add schema types to `pkg/schema/schema.go` (`CIConfig`, `PlanfilesConfig`) — Done
5. Add `--ci` flag to terraform commands — Done
6. Implement `atmos ci status` command — Done

### Phase 2: Artifact & Planfile Storage — COMPLETE

1. Define generic artifact.Store interface (`pkg/ci/artifact/`) — Done
2. Implement artifact store registry and priority-based selector — Done
3. Implement local artifact storage backend (`pkg/ci/artifact/local/`) — Done
4. Implement planfile adapter wrapping artifact.Store (`planfile/adapter/`) — Done
5. Implement PlanfileStore interface — Done
6. Implement S3 store (no DynamoDB) — Done
7. Implement local filesystem planfile store — Done
8. Add `atmos terraform planfile` commands (upload, download, list, delete, show) — Done
9. Implement GitHub Artifacts planfile store — Done
10. Azure Blob and GCS stores — Deferred

### Phase 3: Plugin-Executor Integration — COMPLETE (enum-based)

The executor uses an **enum-based action dispatch** pattern (not the callback-based pattern described in hooks-integration.md):

1. Executor orchestrates all CI actions via `Execute()` → `executeActions()` → switch on `HookAction` enum — Done
2. Plugin interface with 7 methods (GetType, GetHookBindings, GetDefaultTemplates, BuildTemplateContext, ParseOutput, GetOutputVariables, GetArtifactKey) — Done
3. Terraform plugin implements all 7 Plugin methods — Done
4. Executor wired: `ci.Execute(ExecuteOptions{...})` → detect platform → get plugin → build context → execute actions — Done
5. Upload action (`executeUploadAction`) — Done
6. Download action (`executeDownloadAction`) — Done
7. Check run action (`executeCheckAction`) — create on before, update on after — Done
8. Summary action (`executeSummaryAction`) — renders template, writes to `$GITHUB_STEP_SUMMARY` — Done
9. Output action (`executeOutputAction`) — writes variables to `$GITHUB_OUTPUT` with whitelist filtering — Done
10. `--verify-plan` using plan-diff — Not Started

> **Future refactoring**: The hooks-integration.md PRD describes a callback-based pattern where plugins own all logic via `HookAction` function callbacks. The current enum-based approach works but the executor is a "god-object" that knows about all action types. Refactoring to callbacks would move action logic into plugins for better separation of concerns.

### Phase 4: PR Comments & Terraform Output Export — Not Started

1. Implement PR comment action type in executor — Not Started
2. Implement comment upsert behavior (HTML marker, find-and-update) — Not Started
3. Add `github/comment.go` with PR comment API — Not Started
4. Integrate terraform outputs export after apply (using `pkg/terraform/output/`) — Not Started

### Phase 5: Describe Affected Matrix — Not Started

1. Add `--format=matrix` flag — Not Started
2. Implement matrix JSON output — Not Started
3. Update documentation — Not Started

### Phase 6: Documentation — Not Started

1. Archive old GitHub Actions docs — Not Started
2. Write new CI integration docs — Not Started
3. Update command reference docs — Not Started

## Implementation Status Table

| Phase | Description | Status | Completion |
|-------|-------------|--------|------------|
| **Phase 1** | Core Infrastructure | Complete | 100% |
| | pkg/ci/ package structure | Done | |
| | Provider interface (`pkg/ci/internal/provider/types.go`) | Done | |
| | GitHub provider (`pkg/ci/providers/github/`) | Done | |
| | Generic provider (`pkg/ci/providers/generic/`) | Done | |
| | Context and detection | Done | |
| | Schema types (`pkg/schema/schema.go` — CIConfig, PlanfilesConfig) | Done | |
| | Provider registry (`pkg/ci/registry_provider.go`) | Done | |
| | Plugin registry (`pkg/ci/plugin_registry.go`) | Done | |
| | `atmos ci status` command (`cmd/ci/`) | Done | |
| **Phase 2** | Artifact & Planfile Storage | Complete | 100% |
| | Artifact Store interface (`pkg/ci/artifact/`) | Done | |
| | Artifact local backend (`pkg/ci/artifact/local/`) | Done | |
| | Artifact store registry + selector | Done | |
| | PlanfileStore interface | Done | |
| | PlanfileStore adapter (`planfile/adapter/`) | Done | |
| | S3 store (`planfile/s3/`) | Done | |
| | GitHub Artifacts store (`planfile/github/`) | Done | |
| | Local filesystem store (`planfile/local/`) | Done | |
| | Azure Blob store | Deferred | |
| | GCS store | Deferred | |
| | `atmos terraform planfile` commands (upload/download/list/delete/show) | Done | |
| **Phase 3** | Plugin-Executor Integration | Complete (enum-based) | ~90% |
| | Executor action dispatch (`pkg/ci/executor.go`) | Done | |
| | Plugin interface — 7 methods (`pkg/ci/internal/plugin/types.go`) | Done | |
| | Terraform plugin (`pkg/ci/plugins/terraform/plugin.go`) | Done | |
| | Output parser (`pkg/ci/plugins/terraform/parser.go`) | Done | |
| | Template context (`pkg/ci/plugins/terraform/context.go`) | Done | |
| | Template loader with override support (`pkg/ci/templates/loader.go`) | Done | |
| | Summary action (renders template → `$GITHUB_STEP_SUMMARY`) | Done | |
| | Output action (writes variables → `$GITHUB_OUTPUT` with whitelist filtering) | Done | |
| | Upload action (uploads planfile to store) | Done | |
| | Download action (downloads planfile from store) | Done | |
| | Check run action (create on before, update on after) | Done | |
| | `FileOutputWriter` for `$GITHUB_OUTPUT`/`$GITHUB_STEP_SUMMARY` | Done | |
| | `OutputHelpers` — `WritePlanOutputs()`, `WriteApplyOutputs()` | Done | |
| | Config-based action enable/disable (`isActionEnabled()`) | Done | |
| | `--verify-plan` using plan-diff | Not Started | |
| **Phase 4** | PR Comments & TF Output Export | Not Started | 0% |
| | PR comment action type | Not Started | |
| | Comment upsert behavior | Not Started | |
| | `github/comment.go` — PR comment API | Not Started | |
| | Terraform outputs export after apply | Not Started | |
| **Phase 5** | Describe Affected Matrix | Not Started | 0% |
| | `--format=matrix` flag | Not Started | |
| | Matrix JSON output | Not Started | |
| **Phase 6** | Documentation | Not Started | 0% |
| | Archive old GitHub Actions docs | Not Started | |
| | Write new CI integration docs | Not Started | |
| | Update command reference docs | Not Started | |

## Files Created

| File | Purpose | Status |
|------|---------|--------|
| **pkg/ci/ (core)** | | |
| `pkg/ci/executor.go` | CI action orchestrator: detect platform, dispatch actions (summary/output/upload/download/check) | Done |
| `pkg/ci/executor_test.go` | Executor tests | Done |
| `pkg/ci/provider.go` | Type alias for `internal/provider.Provider` | Done |
| `pkg/ci/status.go` | Type aliases for status types | Done |
| `pkg/ci/registry_provider.go` | Provider registry: Register(), Detect(), DetectOrError(), IsCI() | Done |
| `pkg/ci/registry_provider_test.go` | Provider registry tests | Done |
| `pkg/ci/plugin_registry.go` | Plugin registry: RegisterPlugin(), GetPlugin(), GetPluginForEvent() | Done |
| `pkg/ci/plugin_registry_test.go` | Plugin registry tests | Done |
| `pkg/ci/mock_plugin_test.go` | Mock plugin for executor tests | Done |
| **pkg/ci/internal/plugin/** | Plugin interface and types | |
| `pkg/ci/internal/plugin/types.go` | Plugin interface (7 methods), HookAction enum, HookBinding, OutputResult, TemplateContext, ComponentConfigurationResolver | Done |
| **pkg/ci/internal/provider/** | Provider interface and types | |
| `pkg/ci/internal/provider/types.go` | Provider interface, Context, PRInfo, CheckRun structs | Done |
| `pkg/ci/internal/provider/check.go` | CheckRunState constants, CreateCheckRunOptions, UpdateCheckRunOptions | Done |
| `pkg/ci/internal/provider/output.go` | OutputWriter interface, FileOutputWriter, NoopOutputWriter, OutputHelpers (WritePlanOutputs, WriteApplyOutputs) | Done |
| `pkg/ci/internal/provider/output_test.go` | OutputWriter tests | Done |
| `pkg/ci/internal/provider/status.go` | StatusOptions, Status, BranchStatus, PRStatus, CheckStatus | Done |
| **pkg/ci/artifact/** | Generic artifact storage layer | |
| `pkg/ci/artifact/store.go` | Store interface, FileEntry/FileResult, StoreFactory | Done |
| `pkg/ci/artifact/metadata.go` | Metadata, ArtifactInfo structs | Done |
| `pkg/ci/artifact/query.go` | Query struct for filtering | Done |
| `pkg/ci/artifact/registry.go` | Backend registry: Register(), NewStore(), GetRegisteredTypes() | Done |
| `pkg/ci/artifact/selector.go` | EnvironmentChecker, SelectStore() | Done |
| `pkg/ci/artifact/mock_store.go` | Generated mock via mockgen | Done |
| `pkg/ci/artifact/local/store.go` | Local filesystem artifact backend | Done |
| `pkg/ci/artifact/*_test.go` | Tests for all artifact packages | Done |
| **pkg/ci/plugins/terraform/** | Terraform CI plugin | |
| `pkg/ci/plugins/terraform/plugin.go` | Terraform CI plugin (7 Plugin methods, hook bindings, output variables, artifact keys) | Done |
| `pkg/ci/plugins/terraform/plugin_test.go` | Plugin tests | Done |
| `pkg/ci/plugins/terraform/parser.go` | Parse plan/apply output (regex-based) | Done |
| `pkg/ci/plugins/terraform/parser_test.go` | Parser tests | Done |
| `pkg/ci/plugins/terraform/context.go` | TerraformTemplateContext | Done |
| `pkg/ci/plugins/terraform/template_test.go` | Template rendering tests | Done |
| `pkg/ci/plugins/terraform/templates/plan.md` | Default plan summary template | Done |
| `pkg/ci/plugins/terraform/templates/apply.md` | Default apply summary template | Done |
| **pkg/ci/plugins/terraform/planfile/** | Planfile storage (wraps artifact layer) | |
| `pkg/ci/plugins/terraform/planfile/interface.go` | planfile.Store interface, Metadata, KeyPattern, GenerateKey() | Done |
| `pkg/ci/plugins/terraform/planfile/interface_test.go` | Interface tests | Done |
| `pkg/ci/plugins/terraform/planfile/registry.go` | Store registry | Done |
| `pkg/ci/plugins/terraform/planfile/adapter/store.go` | Adapter: planfile.Store → artifact.Store | Done |
| `pkg/ci/plugins/terraform/planfile/adapter/factory.go` | StoreFactory for registry integration | Done |
| `pkg/ci/plugins/terraform/planfile/adapter/store_test.go` | Adapter tests (95.6% coverage) | Done |
| `pkg/ci/plugins/terraform/planfile/s3/store.go` | S3 implementation | Done |
| `pkg/ci/plugins/terraform/planfile/s3/store_test.go` | S3 store tests | Done |
| `pkg/ci/plugins/terraform/planfile/github/store.go` | GitHub Artifacts implementation | Done |
| `pkg/ci/plugins/terraform/planfile/github/store_test.go` | GitHub store tests | Done |
| `pkg/ci/plugins/terraform/planfile/local/store.go` | Local filesystem store | Done |
| `pkg/ci/plugins/terraform/planfile/local/store_test.go` | Local store tests (81.3% coverage) | Done |
| `pkg/ci/plugins/terraform/planfile/azure/store.go` | Azure Blob implementation | Deferred |
| `pkg/ci/plugins/terraform/planfile/gcs/store.go` | GCS implementation | Deferred |
| **pkg/ci/providers/github/** | GitHub Actions provider | |
| `pkg/ci/providers/github/provider.go` | GitHub Actions Provider (detect, context, OutputWriter via FileOutputWriter) | Done |
| `pkg/ci/providers/github/client.go` | GitHub API client wrapper (go-github) | Done |
| `pkg/ci/providers/github/checks.go` | CreateCheckRun, UpdateCheckRun | Done |
| `pkg/ci/providers/github/checks_test.go` | Check runs tests | Done |
| `pkg/ci/providers/github/status.go` | GetStatus implementation | Done |
| `pkg/ci/providers/github/status_test.go` | Status tests | Done |
| `pkg/ci/providers/github/comment.go` | PR comment API (tfcmt-inspired) | Phase 4 |
| **pkg/ci/providers/generic/** | Generic CI provider | |
| `pkg/ci/providers/generic/provider.go` | Generic provider (CI=true detection, env var context, OutputWriter) | Done |
| `pkg/ci/providers/generic/provider_test.go` | Provider tests | Done |
| `pkg/ci/providers/generic/check.go` | Generic check run support | Done |
| `pkg/ci/providers/generic/check_test.go` | Check tests | Done |
| **pkg/ci/templates/** | Template loading system | |
| `pkg/ci/templates/loader.go` | Template loading with override support (config > base_path > embedded) | Done |
| `pkg/ci/templates/loader_test.go` | Loader tests | Done |
| **cmd/terraform/planfile/** | Planfile subcommand group | |
| `cmd/terraform/planfile/planfile.go` | Planfile command group | Done |
| `cmd/terraform/planfile/upload.go` | `atmos terraform planfile upload` | Done |
| `cmd/terraform/planfile/download.go` | `atmos terraform planfile download` | Done |
| `cmd/terraform/planfile/list.go` | `atmos terraform planfile list` | Done |
| `cmd/terraform/planfile/delete.go` | `atmos terraform planfile delete` | Done |
| `cmd/terraform/planfile/show.go` | `atmos terraform planfile show` | Done |
| **cmd/ci/** | CI command group | |
| `cmd/ci/ci.go` | CI command group + CICommandProvider (experimental) | Done |
| `cmd/ci/status.go` | `atmos ci status` | Done |
| `cmd/ci/status_test.go` | Status command tests | Done |

## Files Modified

| File | Changes | Status |
|------|---------|--------|
| `pkg/schema/schema.go` | Add `CI CIConfig` field; add `PlanfilesConfig` with `Priority`, `Stores`, `Default` | Done |
| `cmd/root.go` | Add blank import `_ "github.com/cloudposse/atmos/cmd/ci"` for registry | Done |
| `cmd/terraform/terraform.go` | Register planfile subcommand (`planfile.PlanfileCmd`) | Done |
| `errors/errors.go` | Add CI + artifact + planfile sentinel errors (22 total) | Done |
| `internal/exec/clean_adapter_funcs.go` | Export `ConstructTerraformComponentPlanfilePath()` for planfile upload | Done |
| `cmd/terraform/plan.go` | Add `--upload-planfile` flags | Not Started |
| `cmd/terraform/deploy.go` | Add `--download-planfile`, `--verify-plan` flags | Not Started |
| `cmd/describe/affected.go` | Add `--format=matrix` support | Not Started |
| `internal/exec/describe_affected.go` | Implement matrix format output | Not Started |
| `pkg/datafetcher/schema/atmos-manifest/*.json` | JSON schema updates | Not Started |

## Sentinel Errors (IMPLEMENTED in `errors/errors.go`)

```go
// CI errors
ErrCIDisabled              = errors.New("CI integration is disabled")
ErrCIProviderNotDetected   = errors.New("CI provider not detected")
ErrCIProviderNotFound      = errors.New("CI provider not found")
ErrCIOperationNotSupported = errors.New("operation not supported by CI provider")
ErrCICheckRunCreateFailed  = errors.New("failed to create check run")
ErrCICheckRunUpdateFailed  = errors.New("failed to update check run")
ErrCIStatusFetchFailed     = errors.New("failed to fetch CI status")
ErrCIOutputWriteFailed     = errors.New("failed to write CI output")
ErrCISummaryWriteFailed    = errors.New("failed to write CI summary")

// Artifact storage errors
ErrArtifactNotFound         = errors.New("artifact not found")
ErrArtifactUploadFailed     = errors.New("failed to upload artifact")
ErrArtifactDownloadFailed   = errors.New("failed to download artifact")
ErrArtifactDeleteFailed     = errors.New("failed to delete artifact")
ErrArtifactListFailed       = errors.New("failed to list artifacts")
ErrArtifactStoreNotFound    = errors.New("artifact store not found")
ErrArtifactStoreInvalidArgs = errors.New("invalid artifact store arguments")
ErrArtifactMetadataFailed   = errors.New("failed to load artifact metadata")
ErrArtifactIntegrityFailed  = errors.New("artifact integrity check failed")

// Planfile storage errors
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
ErrGitHubTokenNotFound = errors.New("GitHub token not found")
```

## Key Implementation Details

### Executor Architecture (`pkg/ci/executor.go`)

The executor uses an **enum-based action dispatch** pattern:

1. `Execute(opts)` → detects platform → gets plugin + binding → builds context → executes actions
2. Actions are `HookAction` string enums: `summary`, `output`, `upload`, `download`, `check`
3. `executeAction()` switches on the enum to call: `executeSummaryAction()`, `executeOutputAction()`, `executeUploadAction()`, `executeDownloadAction()`, `executeCheckAction()`
4. Each action handler is self-contained in `executor.go`
5. `isActionEnabled()` checks `ci.summary.enabled`, `ci.output.enabled`, `ci.checks.enabled` from config

### OutputWriter Implementation

- `FileOutputWriter` (`pkg/ci/internal/provider/output.go`) — writes to `$GITHUB_OUTPUT` (key=value, heredoc for multiline) and `$GITHUB_STEP_SUMMARY` (append)
- `NoopOutputWriter` — used when not in CI
- GitHub provider creates `FileOutputWriter` from env vars in `OutputWriter()` method
- Generic provider creates `FileOutputWriter` from env vars (`CI_OUTPUT`, `CI_STEP_SUMMARY`)
- `OutputHelpers.WritePlanOutputs()` and `WriteApplyOutputs()` provide structured output

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

## Testing Strategy (Phases 3–5)

**Mocks + golden files. No real API calls.**

- **Hook integration**: Mock plugin registry and provider to test hooks fire at correct lifecycle points. Test error propagation (command fails → hooks fire with `CommandError`).
- **PR comments**: Mock GitHub API for upsert tests (list → find marker → create/update).
- **Templates**: Golden file tests for all default templates (plan, apply, with changes, no changes, errors, with outputs).
- **Describe affected matrix**: Table-driven tests for JSON generation. Test `--output-file` writes correct `key=value` format.

Coverage target: 80%.

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.3 | 2026-03-05 | Updated to match actual codebase: Plugin interface (7 methods), HookAction as enum, executor actions all implemented (summary/output/upload/download/check), GitHub Artifacts store done, FileOutputWriter done, sentinel errors synced with code |
| 1.2 | 2026-01-15 | Reorganized PRDs into framework/providers/terraform-plugin directories |
| 1.1 | 2025-12-18 | Updated PRD with implementation status, documented additional components |
| 1.0 | 2025-12-17 | Initial PRD |
