# Native CI Integration - Implementation Status

> Related: [Overview](../overview.md) | [Artifact Storage](./artifact-storage.md) | [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Hooks Integration](./hooks-integration.md)

## Implementation Phases

Phases are organized by PRD workstream and functional requirement (FR). See [Overview](../overview.md) for FR definitions.

---

### Framework: Core Infrastructure — COMPLETE

> PRDs: [CI Detection](./ci-detection.md) | [Interfaces](./interfaces.md) | [Configuration](./configuration.md) | [Hooks Integration](./hooks-integration.md)

**FR-1: CI Environment Detection** — Done
1. GitHub Actions detection via `GITHUB_ACTIONS=true` — Done
2. Generic CI detection via `CI=true` — Done
3. `--ci` flag with `ATMOS_CI` and `CI` env var bindings — Done
4. `ci.enabled` config gate in `pkg/schema/schema.go` — Done

**FR-7: Command Parity** — Partial
1. `--ci` flag on `terraform plan` (full wiring: PreRunE, output capture, PostRunE, error defer) — Done
2. `--ci` flag on `terraform apply` (flag + env var bindings defined) — Done
3. Apply `PostRunE` fires `after.terraform.apply` CI hooks (empty output) — Done
4. Apply `PreRunE` for `before.terraform.apply` (download planfile) — **Not Started** (no PreRunE on applyCmd)
5. Apply output capture (stdout/stderr like plan.go) — **Not Started**
6. Apply error defer (check run failure update like plan.go) — **Not Started**
7. `deploy.go` `--ci` flag — **Not Started** (no `--ci` flag at all on deploy)

**Core Infrastructure** — Done
1. `pkg/ci/` package structure — Done
2. Provider interface (`pkg/ci/internal/provider/types.go`) — Done
3. Plugin interface with 2 methods (`pkg/ci/internal/plugin/types.go`) — Done (slimmed from 7 methods via callback refactoring)
4. Executor with callback-based dispatch (`pkg/ci/executor.go`, ~250 lines) — Done (refactored from ~850-line enum-based god-object)
5. Provider registry (`pkg/ci/registry_provider.go`) — Done
6. Plugin registry (`pkg/ci/plugin_registry.go`) — Done
7. Schema types (`CIConfig`, `PlanfilesConfig`) in `pkg/schema/schema.go` — Done
8. Config-based action enable/disable (plugin-internal `isSummaryEnabled`/`isOutputEnabled`/`isCheckEnabled`) — Done
9. Template loader with override support (`pkg/ci/templates/loader.go`) — Done
10. `CheckRunStore` interface + `sync.Map`-backed singleton (`pkg/ci/checkrun_store.go`) — Done
11. `HookContext` struct + `HookHandler` callback type (`pkg/ci/internal/plugin/types.go`) — Done

---

### Framework: Artifact Storage — COMPLETE

> PRD: [Artifact Storage](./artifact-storage.md)

1. Generic `artifact.Store` interface (`pkg/ci/artifact/store.go`) — Done
2. Artifact metadata with `Custom` map (`pkg/ci/artifact/metadata.go`) — Done
3. Query-based filtering (`pkg/ci/artifact/query.go`) — Done
4. Backend registry with `Register()` / `NewStore()` (`pkg/ci/artifact/registry.go`) — Done
5. Priority-based backend selector (`pkg/ci/artifact/selector.go`) — Done
6. Local filesystem backend (`pkg/ci/artifact/local/`) — Done
7. Generated mock via mockgen (`pkg/ci/artifact/mock_store.go`) — Done

---

### Providers: GitHub Actions — Mostly Complete

> PRDs: [GitHub Provider](../providers/github/provider.md) | [Job Summaries](../providers/github/job-summaries.md) | [CI Outputs](../providers/github/ci-outputs.md) | [Status Checks](../providers/github/status-checks.md) | [PR Comments](../providers/github/pr-comments.md)

**FR-2: Job Summary Output** — Done
1. Plugin handler `writeSummary()` renders template via `buildTemplateContext()` — Done
2. Writes to `$GITHUB_STEP_SUMMARY` via `FileOutputWriter.WriteSummary()` — Done
3. Default templates: `plan.md`, `apply.md` (`pkg/ci/plugins/terraform/templates/`) — Done

**FR-3: CI Output Variables** — Partial
1. Plugin handler `writeOutputs()` calls `getOutputVariables()` + adds common vars — Done
2. Writes to `$GITHUB_OUTPUT` via `FileOutputWriter.WriteOutput()` — Done
3. Whitelist filtering via `ci.output.variables` config — Done
4. `OutputHelpers` convenience methods (`WritePlanOutputs`, `WriteApplyOutputs`) — Done
5. Terraform output export after apply (`output_*` variables) — **Not Started** (Phase 4)

**FR-4: Status Checks** — Done
1. Plugin handler `createCheckRun()` creates check runs on "before" events — Done
2. Plugin handler `updateCheckRun()` updates check runs on "after" events with result summary — Done
3. Check run ID correlation via `CheckRunStore` interface (`sync.Map`-backed singleton) — Done
4. `ci.checks.enabled` config (disabled by default) — Done
5. `FormatCheckRunName()` with hardcoded `"atmos/"` prefix — Done (config wiring deferred)

**FR-9: CI Status Command** — Done
1. `atmos ci status` command (`cmd/ci/`) — Done
2. Shows current branch status, PRs by user, review requests — Done
3. Uses GitHub API (combined commit status + check runs + PRs) — Done

**GitHub Provider Core** — Done
1. `provider.go` — Detect, Context, OutputWriter — Done
2. `client.go` — GitHub API client wrapper (go-github) — Done
3. `checks.go` — CreateCheckRun, UpdateCheckRun — Done
4. `status.go` — GetStatus implementation — Done

**PR Comments** — Not Started (design deferred)
1. PR comment action type in executor — **Not Started**
2. Comment upsert behavior (HTML marker, find-and-update) — **Not Started**
3. `github/comment.go` — PR comment API — **Not Started**

---

### Providers: Generic CI — COMPLETE

> PRD: [Generic Provider](../providers/generic/generic.md)

1. Generic provider (`pkg/ci/providers/generic/provider.go`) — Done
2. `CI=true` detection, env var context, OutputWriter — Done
3. Generic check run support (`check.go`) — Done

---

### Terraform Plugin: Hook Bindings & Executor — COMPLETE (callback-based)

> PRD: [Hooks Integration](./hooks-integration.md)

The executor uses a **callback-based dispatch** pattern. Plugins own all action logic via `HookHandler` callbacks. The executor is a thin coordinator (~250 lines) that detects the CI platform, resolves the plugin, and invokes the handler.

1. Plugin interface slimmed to 2 methods: `GetType()`, `GetHookBindings()` — Done
2. `HookHandler` callback type + `HookContext` dependency bag — Done
3. `CheckRunStore` interface (replaces `sync.Map` for cross-event check run ID correlation) — Done
4. Hook bindings with `Handler` callbacks: `before.terraform.plan`, `after.terraform.plan`, `before.terraform.apply`, `after.terraform.apply` — Done
5. Output parser (`pkg/ci/plugins/terraform/parser.go`) — Done
6. Template context (`pkg/ci/plugins/terraform/context.go`) — Done
7. Handler logic in `pkg/ci/plugins/terraform/handlers.go`: summary, output, upload, download, check — Done
8. Error severity: upload/download fatal, summary/output/check warn-only — Done
9. Executor coverage 91%, terraform plugin coverage 81% — Done

---

### Terraform Plugin: Planfile Storage — COMPLETE

> PRDs: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Artifact Storage](./artifact-storage.md)

**FR-5: Planfile Storage** — Done (Azure/GCS deferred)
1. `planfile.Store` interface (`pkg/ci/plugins/terraform/planfile/interface.go`) — Done
2. Planfile adapter wrapping `artifact.Store` (`planfile/adapter/`) — Done
3. Planfile store registry (`planfile/registry.go`) — Done
4. S3 store — no DynamoDB (`planfile/s3/`) — Done
5. GitHub Artifacts store (`planfile/github/`) — Done
6. Local filesystem store (`planfile/local/`) — Done
7. Azure Blob store — **Deferred**
8. GCS store — **Deferred**
9. `atmos terraform planfile` commands (upload, download, list, delete, show) — Done
10. Automatic upload on `after.terraform.plan` via `uploadPlanfile()` handler — Done
11. Automatic download on `before.terraform.apply` via `downloadPlanfile()` handler — Done (binding exists, but apply.go `PreRunE` not wired — see FR-7)

---

### Terraform Plugin: Plan Verification — Not Started

> PRD: [Plan Verification](../terraform-plugin/plan-verification.md)

**FR-6: Plan Verification** — Not Started
1. `--verify-plan` flag on `terraform apply` — **Not Started**
2. Download stored planfile to temp path — **Not Started**
3. Generate fresh plan, compare via plan-diff — **Not Started**
4. Fail apply if drift detected — **Not Started**

---

### Terraform Plugin: Describe Affected Matrix — COMPLETE

> PRD: [Describe Affected Matrix](../terraform-plugin/describe-affected-matrix.md)

**FR-8: Describe Affected Matrix** — Done
1. `--format=matrix` flag (`cmd/describe_affected.go`) — Done
2. Matrix JSON output with `MatrixOutput`/`MatrixEntry` structs (4 fields: `component`, `stack`, `component_path`, `component_type`) — Done
3. `--output-file` for `$GITHUB_OUTPUT` (`matrix=<json>` + `affected_count=N`) — Done

---

### Terraform Output Export — Not Started

> PRD: [CI Outputs](../providers/github/ci-outputs.md) (Phase 4 section)

1. Export terraform outputs after successful apply (`output_*` variables) — **Not Started**
2. Flatten nested outputs, uppercase conversion via `pkg/terraform/output/` — **Not Started**

---

### Documentation — Not Started

1. Archive old GitHub Actions docs — **Not Started**
2. Write new CI integration docs — **Not Started**
3. Update command reference docs — **Not Started**
4. Update JSON schemas in `pkg/datafetcher/schema/` — **Not Started**

## Implementation Status by Functional Requirement

| FR | Requirement | PRD | Status | Completion |
|----|-------------|-----|--------|------------|
| **FR-1** | CI Environment Detection | [ci-detection.md](./ci-detection.md) | **Done** | 100% |
| | GitHub Actions detection (`GITHUB_ACTIONS=true`) | | Done | |
| | Generic CI detection (`CI=true`) | | Done | |
| | `--ci` flag with `ATMOS_CI`/`CI` env var bindings | | Done | |
| | `ci.enabled` config gate | | Done | |
| **FR-2** | Job Summary Output | [job-summaries.md](../providers/github/job-summaries.md) | **Done** | 100% |
| | Plugin handler `writeSummary()` with template rendering | | Done | |
| | `$GITHUB_STEP_SUMMARY` via `FileOutputWriter` | | Done | |
| | Default `plan.md` and `apply.md` templates | | Done | |
| **FR-3** | CI Output Variables | [ci-outputs.md](../providers/github/ci-outputs.md) | **Partial** | ~80% |
| | Plugin handler `writeOutputs()` with plugin variables | | Done | |
| | `$GITHUB_OUTPUT` via `FileOutputWriter` | | Done | |
| | Whitelist filtering via `ci.output.variables` | | Done | |
| | Terraform output export after apply (`output_*`) | | Not Started | |
| **FR-4** | Status Checks | [status-checks.md](../providers/github/status-checks.md) | **Done** | 100% |
| | Plugin handler `createCheckRun()` on "before" events | | Done | |
| | Plugin handler `updateCheckRun()` on "after" events | | Done | |
| | `ci.checks.enabled` config (disabled by default) | | Done | |
| | Check run ID correlation via `CheckRunStore` interface | | Done | |
| | `context_prefix` wired from config | | Not Started (hardcoded) | |
| **FR-5** | Planfile Storage | [planfile-storage.md](../terraform-plugin/planfile-storage.md) | **Done** | ~90% |
| | `planfile.Store` interface + adapter | | Done | |
| | S3 store (no DynamoDB) | | Done | |
| | GitHub Artifacts store | | Done | |
| | Local filesystem store | | Done | |
| | `atmos terraform planfile` CLI commands (5 subcommands) | | Done | |
| | Automatic upload on `after.terraform.plan` | | Done | |
| | Automatic download on `before.terraform.apply` | | Done (binding only — apply.go PreRunE not wired) | |
| | Azure Blob store | | Deferred | |
| | GCS store | | Deferred | |
| **FR-6** | Plan Verification | [plan-verification.md](../terraform-plugin/plan-verification.md) | **Not Started** | 0% |
| | `--verify-plan` flag on `terraform apply` | | Not Started | |
| | Download stored plan → fresh plan → plan-diff comparison | | Not Started | |
| **FR-7** | Command Parity | [ci-detection.md](./ci-detection.md) | **Partial** | ~60% |
| | `plan.go` full CI wiring (PreRunE, capture, PostRunE, error defer) | | Done | |
| | `apply.go` `--ci` flag defined | | Done | |
| | `apply.go` PostRunE fires CI hooks (empty output) | | Done | |
| | `apply.go` PreRunE (download planfile) | | Not Started | |
| | `apply.go` output capture (stdout/stderr) | | Not Started | |
| | `apply.go` error defer (check run failure update) | | Not Started | |
| | `deploy.go` `--ci` flag | | Not Started | |
| **FR-8** | Describe Affected Matrix | [describe-affected-matrix.md](../terraform-plugin/describe-affected-matrix.md) | **Done** | 100% |
| | `--format=matrix` flag | | Done | |
| | Matrix JSON with 4 fields (`component`, `stack`, `component_path`, `component_type`) | | Done | |
| | `--output-file` for `$GITHUB_OUTPUT` | | Done | |
| **FR-9** | CI Status Command | [status-checks.md](../providers/github/status-checks.md) | **Done** | 100% |
| | `atmos ci status` command | | Done | |
| | Current branch status + PRs + review requests | | Done | |
| **—** | PR Comments | [pr-comments.md](../providers/github/pr-comments.md) | **Not Started** | 0% |
| | PR comment action type in executor | | Not Started | |
| | Comment upsert behavior (HTML marker) | | Not Started | |
| | `github/comment.go` — PR comment API | | Not Started | |
| **—** | Terraform Output Export | [ci-outputs.md](../providers/github/ci-outputs.md) | **Not Started** | 0% |
| | Export terraform outputs after apply (`output_*`) | | Not Started | |
| | `pkg/terraform/output/` formatting | | Not Started | |
| **—** | Documentation | — | **Not Started** | 0% |
| | Archive old GitHub Actions docs | | Not Started | |
| | Write new CI integration docs | | Not Started | |
| | Update command reference docs | | Not Started | |
| | JSON schema updates (`pkg/datafetcher/schema/`) | | Not Started | |

### Summary

| Category | Done | Not Started | Deferred |
|----------|------|-------------|----------|
| Framework: Core Infrastructure | 11/11 | 0 | 0 |
| Framework: CI Detection (FR-1) | 4/4 | 0 | 0 |
| Framework: Artifact Storage | 7/7 | 0 | 0 |
| Providers: GitHub (FR-2, FR-3, FR-4, FR-9) | 14/14 | 0 | 0 |
| Providers: GitHub — PR Comments | 0/3 | 3 | 0 |
| Providers: Generic | 3/3 | 0 | 0 |
| Terraform Plugin: Hook Bindings (callback-based) | 9/9 | 0 | 0 |
| Terraform Plugin: Planfile Storage (FR-5) | 8/11 | 0 | 2 (Azure, GCS) |
| Terraform Plugin: Plan Verification (FR-6) | 0/4 | 4 | 0 |
| Terraform Plugin: Describe Affected Matrix (FR-8) | 3/3 | 0 | 0 |
| Command Parity (FR-7) | 3/7 | 4 | 0 |
| Terraform Output Export | 0/2 | 2 | 0 |
| Documentation | 0/4 | 4 | 0 |
| **Total** | **62/82** | **17** | **2** |

## Files Created

| File | Purpose | Status |
|------|---------|--------|
| **pkg/ci/ (core)** | | |
| `pkg/ci/executor.go` | Thin CI coordinator (~250 lines): detect platform, resolve plugin, invoke handler callback | Done |
| `pkg/ci/executor_test.go` | Executor tests (91% coverage) | Done |
| `pkg/ci/checkrun_store.go` | `CheckRunStore` interface + `sync.Map`-backed singleton for cross-event check run ID correlation | Done |
| `pkg/ci/checkrun_store_test.go` | CheckRunStore tests | Done |
| `pkg/ci/provider.go` | Type alias for `internal/provider.Provider` | Done |
| `pkg/ci/status.go` | Type aliases for status types | Done |
| `pkg/ci/registry_provider.go` | Provider registry: Register(), Detect(), DetectOrError(), IsCI() | Done |
| `pkg/ci/registry_provider_test.go` | Provider registry tests | Done |
| `pkg/ci/plugin_registry.go` | Plugin registry: RegisterPlugin(), GetPlugin(), GetPluginForEvent() | Done |
| `pkg/ci/plugin_registry_test.go` | Plugin registry tests | Done |
| `pkg/ci/mock_plugin_test.go` | Mock plugin for executor tests (slimmed 2-method interface) | Done |
| **pkg/ci/internal/plugin/** | Plugin interface and types | |
| `pkg/ci/internal/plugin/types.go` | Plugin interface (2 methods: GetType, GetHookBindings), HookHandler callback type, HookContext struct, CheckRunStore interface, HookBinding with Handler field, OutputResult, TemplateContext | Done |
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
| `pkg/ci/plugins/terraform/plugin.go` | Terraform CI plugin (2 Plugin methods + private helpers: buildTemplateContext, getOutputVariables, getArtifactKey) | Done |
| `pkg/ci/plugins/terraform/plugin_test.go` | Plugin tests | Done |
| `pkg/ci/plugins/terraform/handlers.go` | All handler implementations: onBeforePlan, onAfterPlan, onBeforeApply, onAfterApply + helpers (writeSummary, writeOutputs, uploadPlanfile, downloadPlanfile, createCheckRun, updateCheckRun) | Done |
| `pkg/ci/plugins/terraform/handlers_test.go` | Handler tests (81% coverage) | Done |
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
| `errors/errors.go` | Add CI + artifact + planfile + AWS sentinel errors (31 total) | Done |
| `internal/exec/clean_adapter_funcs.go` | Export `ConstructTerraformComponentPlanfilePath()` for planfile upload | Done |
| `cmd/terraform/plan.go` | Add `--ci` and `--skip-planfile` flags, full CI output capture + hook dispatch | Done |
| `cmd/terraform/apply.go` | Add `--ci` flag with env var bindings (`ATMOS_CI`, `CI`); `PostRunE` fires CI hooks (empty output) | Done (partial) |
| `cmd/terraform/apply.go` | `PreRunE` for `before.terraform.apply` (download), output capture, error defer | Not Started |
| `cmd/describe_affected.go` | Add `--format=matrix` support | Done |
| `internal/exec/describe_affected.go` | Implement matrix format output (`MatrixOutput`, `MatrixEntry`, `writeMatrixOutput`) | Done |
| `cmd/terraform/deploy.go` | Add `--verify-plan` flag | Not Started |
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
ErrAWSConfigLoadFailed        = errors.New("failed to load AWS configuration")

// GitHub errors
ErrGitHubTokenNotFound = errors.New("GitHub token not found")
```

## Key Implementation Details

### Executor Architecture (`pkg/ci/executor.go`)

The executor uses a **callback-based dispatch** pattern (~250 lines):

1. `Execute(opts)` → `detectPlatform()` → `getPluginAndBinding()` → `buildHookContext()` → `binding.Handler(hookCtx)`
2. `HookHandler` is `func(ctx *HookContext) error` — plugins own all action logic
3. `HookContext` provides all dependencies: `Config`, `Info`, `Output`, `CommandError`, `Provider`, `CICtx`, `TemplateLoader`, `CheckRunStore`, `CreatePlanfileStore`
4. `CheckRunStore` interface correlates check run IDs across before/after events (backed by `sync.Map` singleton)
5. `CreatePlanfileStore` is a lazy factory closure — only invoked when a handler needs artifact storage
6. Error severity is handler-controlled: upload/download return errors (fatal), summary/output/check log warnings (non-fatal)

### OutputWriter Implementation

- `FileOutputWriter` (`pkg/ci/internal/provider/output.go`) — writes to `$GITHUB_OUTPUT` (key=value, heredoc for multiline) and `$GITHUB_STEP_SUMMARY` (append)
- `NoopOutputWriter` — used when not in CI
- GitHub provider creates `FileOutputWriter` from env vars in `OutputWriter()` method
- Generic provider creates `FileOutputWriter` from env vars (`ATMOS_CI_OUTPUT`, `ATMOS_CI_SUMMARY`)
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
| 4.0 | 2026-03-06 | Callback-based refactoring COMPLETE. Executor refactored from ~850-line enum-based god-object to ~250-line thin coordinator. Plugin interface slimmed from 7 methods to 2 (GetType, GetHookBindings). Added HookHandler callback type, HookContext dependency bag, CheckRunStore interface. All action logic moved from executor into `plugins/terraform/handlers.go`. Error severity now handler-controlled (upload/download fatal, summary/output/check warn-only). New files: `checkrun_store.go`, `handlers.go`, `handlers_test.go`. Removed: HookAction enum, Actions field, Template field, ComponentConfigurationResolver interface, 5 Plugin methods. Coverage: executor 91%, terraform plugin 81%. |
| 3.0 | 2026-03-05 | Restructured implementation phases to align with PRD organization. Phases now map to PRD workstreams (Framework, Providers, Terraform Plugin) and functional requirements (FR-1 through FR-9). Replaced Phase 1-6 numbering with descriptive section names matching PRD directory structure. Added FR-level status table with PRD cross-references. Added summary table with counts. No status changes — all Done/Not Started markers preserved from v2.2. |
| 2.2 | 2026-03-05 | Eleventh sync pass: added missing `ErrAWSConfigLoadFailed` to sentinel errors list, fixed error count (31 total, was 22). Documented critical wiring gap: `apply.go` has no `PreRunE` so `before.terraform.apply` (download planfile) never fires — added notes to ci-detection.md and hooks-integration.md. `deploy.go` has no `--ci` flag at all. All code verified unchanged since last sync. |
| 2.1 | 2026-03-05 | Tenth sync pass: detailed apply.go CI integration status — PostRunE fires CI hooks (Done, but with empty output), PreRunE for before.terraform.apply download (Not Started), output capture (Not Started), error defer (Not Started). Fixed describe-affected-matrix.md: MatrixEntry has 4 fields (component, stack, component_path, component_type), not 2 as previously documented. |
| 2.0 | 2026-03-05 | Ninth sync pass: verified full codebase against PRD. Added: `--ci` flag on `terraform apply` (Done — flag + env vars defined), apply CI hooks integration (Not Started — flag exists but no output capture/CI hook dispatch like plan.go). All other statuses confirmed accurate. |
| 1.9 | 2026-03-05 | Eighth sync pass: added "current vs target" qualifier to overview.md NFR-2 (upload/download failure currently warn-only, not fatal); verified configuration schema fields, artifact metadata fields, executor sync.Map/check function names, ConstructTerraformComponentPlanfilePath — all correct |
| 1.8 | 2026-03-05 | Seventh sync pass: fixed artifact-storage.md scope (upload IS a CLI command, all 5 subcommands implemented); fixed ci-detection.md RunCIHooks() location (defined in pkg/hooks/hooks.go, called from cmd/terraform/utils.go); added missing IsExperimental() to provider.md CICommandProvider snippet |
| 1.7 | 2026-03-05 | Sixth sync pass: fixed ci-outputs.md Behavior bullet (output_* is Phase 4); fixed overview.md auth FAQ (GITHUB_TOKEN/GH_TOKEN, not ATMOS_GITHUB_TOKEN); rewrote planfile-storage.md CLI commands to match actual key-based cobra commands (list [prefix], upload --component --stack, download <key>, delete <key>, show <key>); updated hooks-integration.md Error Severity table to show current vs target behavior (current: all warn-and-continue; target: upload/download fatal) |
| 1.6 | 2026-03-05 | Fifth sync pass: added missing PRStatus fields (BaseBranch, URL) and CheckStatus.DetailsURL to interfaces.md; fixed ci-outputs.md apply variables (same as plan, no success/output_* yet — Phase 4); updated overview.md FAQ about terraform output export (Phase 4); fixed last-writer-wins example variable name; fixed providers/README.md generic provider description (no git fallback) |
| 1.5 | 2026-03-05 | Fourth sync pass: fixed ci-outputs.md Behavior section variable names, updated configuration.md whitelist example, rewrote hooks-integration.md Per-Plugin Storage and Integration Points sections to show current vs target, fixed plan-verification.md --download-planfile flag (automatic in CI), fixed artifact-storage.md generic provider context (no git fallback), fixed generic provider env vars (ATMOS_CI_OUTPUT not CI_OUTPUT), updated status-checks.md context_prefix (hardcoded not from config) |
| 1.4 | 2026-03-05 | Third sync pass: fixed output variable names (resources_to_create not has_additions_count), updated Files Modified table (plan.go/describe_affected done, --upload-planfile N/A), fixed ci.enabled truth table (--ci bypasses it), updated generic provider capabilities, fixed matrix output key (matrix= not affected=) |
| 1.3 | 2026-03-05 | Updated to match actual codebase: Plugin interface (7 methods), HookAction as enum, executor actions all implemented (summary/output/upload/download/check), GitHub Artifacts store done, FileOutputWriter done, sentinel errors synced with code |
| 1.2 | 2026-01-15 | Reorganized PRDs into framework/providers/terraform-plugin directories |
| 1.1 | 2025-12-18 | Updated PRD with implementation status, documented additional components |
| 1.0 | 2025-12-17 | Initial PRD |
