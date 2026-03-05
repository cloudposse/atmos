# Native CI Integration - Core Interfaces

> Related: [Overview](../overview.md) | [GitHub Provider](../providers/github/provider.md) | [Configuration](./configuration.md)

## Core Interfaces

### Provider Interface (IMPLEMENTED)

```go
// pkg/ci/internal/provider/types.go

// Provider represents a CI/CD provider (GitHub Actions, GitLab CI, etc.).
type Provider interface {
    Name() string
    Detect() bool
    Context() (*Context, error)
    GetStatus(ctx context.Context, opts StatusOptions) (*Status, error)
    CreateCheckRun(ctx context.Context, opts *CreateCheckRunOptions) (*CheckRun, error)
    UpdateCheckRun(ctx context.Context, opts *UpdateCheckRunOptions) (*CheckRun, error)
    OutputWriter() OutputWriter
}

// OutputWriter writes CI outputs (environment variables, job summaries, etc.).
type OutputWriter interface {
    WriteOutput(key, value string) error
    WriteSummary(content string) error
}
```

**Implementations** (IMPLEMENTED):
- `FileOutputWriter` — writes to `$GITHUB_OUTPUT` and `$GITHUB_STEP_SUMMARY` files (key=value format, heredoc for multiline)
- `NoopOutputWriter` — used when not in CI or outputs disabled
- `OutputHelpers` — convenience methods: `WritePlanOutputs()`, `WriteApplyOutputs()`

### Context Struct (IMPLEMENTED)

```go
// pkg/ci/internal/provider/types.go

// Context contains CI run metadata.
type Context struct {
    Provider    string
    RunID       string
    RunNumber   int
    Workflow    string
    Job         string
    Actor       string
    EventName   string  // "push", "pull_request"
    Ref         string  // Git ref
    Branch      string  // Branch name (GITHUB_HEAD_REF or GITHUB_REF_NAME)
    SHA         string  // Git commit SHA
    Repository  string  // owner/repo
    RepoOwner   string
    RepoName    string
    PullRequest *PRInfo // nil if not PR
}

type PRInfo struct {
    Number  int
    HeadRef string
    BaseRef string
    URL     string
}
```

### Status Structs

```go
// pkg/ci/status.go

// Status represents the CI status for display.
type Status struct {
    Repository     string
    CurrentBranch  *BranchStatus    // PR and checks for current branch
    CreatedByUser  []*PRStatus      // PRs created by current user
    ReviewRequests []*PRStatus      // PRs requesting review from user
}

type BranchStatus struct {
    Branch      string
    PullRequest *PRStatus          // nil if no PR for branch
    CommitSHA   string
    Checks      []*CheckStatus
}

type PRStatus struct {
    Number    int
    Title     string
    Branch    string
    Checks    []*CheckStatus
    AllPassed bool
}

type CheckStatus struct {
    Name       string
    Status     string  // "success", "failure", "pending", "skipped"
    Conclusion string
}
```

### Artifact Store Interface (IMPLEMENTED)

```go
// pkg/ci/artifact/store.go

// Store defines the interface for artifact storage backends.
type Store interface {
    Name() string
    Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error
    Download(ctx context.Context, name string) ([]FileResult, *Metadata, error)
    Delete(ctx context.Context, name string) error
    List(ctx context.Context, query Query) ([]ArtifactInfo, error)
    Exists(ctx context.Context, name string) (bool, error)
    GetMetadata(ctx context.Context, name string) (*Metadata, error)
}

// StoreFactory creates a Store from options.
type StoreFactory func(opts StoreOptions) (Store, error)
```

### Planfile Store Interface (IMPLEMENTED)

```go
// pkg/ci/plugins/terraform/planfile/interface.go

// Store defines the interface for planfile storage backends.
type Store interface {
    Name() string
    Upload(ctx context.Context, key string, data io.Reader, metadata *Metadata) error
    Download(ctx context.Context, key string) (io.ReadCloser, *Metadata, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]PlanfileInfo, error)
    Exists(ctx context.Context, key string) (bool, error)
    GetMetadata(ctx context.Context, key string) (*Metadata, error)
}

// StoreFactory creates a Store from configuration options.
type StoreFactory func(opts StoreOptions) (Store, error)
```

### Planfile Metadata (IMPLEMENTED)

```go
// pkg/ci/plugins/terraform/planfile/interface.go

type Metadata struct {
    Stack            string            `json:"stack"`
    Component        string            `json:"component"`
    ComponentPath    string            `json:"component_path"`
    SHA              string            `json:"sha"`
    BaseSHA          string            `json:"base_sha,omitempty"`
    Branch           string            `json:"branch,omitempty"`
    PRNumber         int               `json:"pr_number,omitempty"`
    RunID            string            `json:"run_id,omitempty"`
    Repository       string            `json:"repository,omitempty"`
    CreatedAt        time.Time         `json:"created_at"`
    ExpiresAt        *time.Time        `json:"expires_at,omitempty"`
    PlanSummary      string            `json:"plan_summary,omitempty"`
    HasChanges       bool              `json:"has_changes"`
    Additions        int               `json:"additions"`
    Changes          int               `json:"changes"`
    Destructions     int               `json:"destructions"`
    MD5              string            `json:"md5,omitempty"`
    TerraformVersion string            `json:"terraform_version,omitempty"`
    TerraformTool    string            `json:"terraform_tool,omitempty"`
    Custom           map[string]string `json:"custom,omitempty"`
}
```

## Plugin Interface (IMPLEMENTED)

The plugin owns its CI behavior. The executor calls plugin methods to parse output, build context, get variables, and generate artifact keys. See [Hooks Integration](./hooks-integration.md) for the full architecture.

```go
// pkg/ci/internal/plugin/types.go

// HookAction represents what CI action to perform (enum, not callback).
type HookAction string

const (
    ActionSummary  HookAction = "summary"   // Write to job summary ($GITHUB_STEP_SUMMARY)
    ActionOutput   HookAction = "output"    // Write to CI outputs ($GITHUB_OUTPUT)
    ActionUpload   HookAction = "upload"    // Upload artifact (e.g., planfile)
    ActionDownload HookAction = "download"  // Download artifact
    ActionCheck    HookAction = "check"     // Create/update check run
)

// HookBinding declares what happens at a specific hook event.
type HookBinding struct {
    Event    string        // "after.terraform.plan"
    Actions  []HookAction  // CI actions to perform at this event
    Template string        // Template name for summary action (e.g., "plan")
}

// Plugin is implemented by component types that support CI integration.
type Plugin interface {
    GetType() string
    GetHookBindings() []HookBinding
    GetDefaultTemplates() embed.FS
    BuildTemplateContext(info *schema.ConfigAndStacksInfo, ciCtx *provider.Context, output, command string) (any, error)
    ParseOutput(output string, command string) (*OutputResult, error)
    GetOutputVariables(result *OutputResult, command string) map[string]string
    GetArtifactKey(info *schema.ConfigAndStacksInfo, command string) string
}

// ComponentConfigurationResolver is an optional interface that Plugins can implement
// to resolve artifact paths (e.g., planfile paths) when not explicitly provided.
type ComponentConfigurationResolver interface {
    ResolveComponentPlanfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error)
}
```

> **Architecture note**: The current implementation uses an **enum-based action dispatch** pattern — `HookAction` is a string enum and the executor's `executeAction()` switches on it to call the appropriate handler (summary, output, upload, download, check). The [Hooks Integration](./hooks-integration.md) PRD describes a **callback-based** pattern as the target for Phase 3 refactoring, where plugins would own all action logic via function callbacks. Both approaches achieve the same result; the callback pattern moves more logic into plugins for better separation of concerns.

### Terraform Plugin Hook Bindings (IMPLEMENTED)

```go
// pkg/ci/plugins/terraform/plugin.go

func (p *Plugin) GetHookBindings() []plugin.HookBinding {
    return []plugin.HookBinding{
        {Event: "before.terraform.plan",  Actions: []plugin.HookAction{plugin.ActionCheck}},
        {Event: "after.terraform.plan",   Actions: []plugin.HookAction{plugin.ActionSummary, plugin.ActionOutput, plugin.ActionUpload, plugin.ActionCheck}, Template: "plan"},
        {Event: "before.terraform.apply", Actions: []plugin.HookAction{plugin.ActionDownload, plugin.ActionCheck}},
        {Event: "after.terraform.apply",  Actions: []plugin.HookAction{plugin.ActionSummary, plugin.ActionOutput, plugin.ActionCheck}, Template: "apply"},
    }
}
```

## Package Structure (IMPLEMENTED)

```
pkg/ci/
  ├── executor.go              # CI action orchestrator: detect platform, dispatch actions
  ├── executor_test.go
  ├── provider.go              # Type alias for internal/provider.Provider
  ├── status.go                # Type aliases for status types
  ├── plugin_registry.go       # Plugin registry: RegisterPlugin(), GetPlugin(), GetPluginForEvent()
  ├── plugin_registry_test.go
  ├── registry_provider.go     # Provider registry: Register(), Detect(), DetectOrError(), IsCI()
  ├── registry_provider_test.go
  ├── mock_plugin_test.go      # Mock plugin for executor tests
  ├── internal/
  │   ├── plugin/
  │   │   └── types.go         # Plugin interface, HookAction enum, HookBinding, OutputResult, TemplateContext
  │   └── provider/
  │       ├── types.go         # Provider interface, Context, PRInfo, CheckRun structs
  │       ├── check.go         # CheckRunState constants, Create/Update options
  │       ├── output.go        # OutputWriter interface, FileOutputWriter, NoopOutputWriter, OutputHelpers
  │       ├── output_test.go
  │       └── status.go        # StatusOptions, Status, BranchStatus, PRStatus, CheckStatus
  ├── artifact/                # Base artifact storage layer
  │   ├── store.go             # Store interface, FileEntry/FileResult, StoreFactory
  │   ├── store_test.go
  │   ├── metadata.go          # Metadata, ArtifactInfo structs
  │   ├── metadata_test.go
  │   ├── query.go             # Query struct for filtering
  │   ├── registry.go          # Backend registry: Register(), NewStore()
  │   ├── registry_test.go
  │   ├── selector.go          # EnvironmentChecker, SelectStore()
  │   ├── selector_test.go
  │   ├── mock_store.go        # Generated mock
  │   └── local/
  │       ├── store.go         # Local filesystem backend
  │       └── store_test.go
  ├── plugins/terraform/
  │   ├── plugin.go            # Terraform CI plugin (hook bindings, parsing, output vars, artifact keys)
  │   ├── plugin_test.go
  │   ├── parser.go            # Parse plan/apply output (regex-based)
  │   ├── parser_test.go
  │   ├── context.go           # TerraformTemplateContext
  │   ├── template_test.go
  │   ├── templates/
  │   │   ├── plan.md          # Default plan summary template
  │   │   └── apply.md         # Default apply summary template
  │   └── planfile/            # Planfile storage (extends artifact.Store)
  │       ├── interface.go     # planfile.Store interface, Metadata, KeyPattern, GenerateKey()
  │       ├── interface_test.go
  │       ├── registry.go      # Planfile-specific storage registry
  │       ├── adapter/
  │       │   ├── store.go     # Adapter: planfile.Store -> artifact.Store
  │       │   ├── store_test.go
  │       │   └── factory.go   # StoreFactory for registry integration
  │       ├── s3/
  │       │   ├── store.go     # S3 store (metadata sidecar, no DynamoDB)
  │       │   └── store_test.go
  │       ├── github/
  │       │   ├── store.go     # GitHub Artifacts store
  │       │   └── store_test.go
  │       └── local/
  │           ├── store.go     # Local filesystem store
  │           └── store_test.go
  ├── providers/
  │   ├── github/
  │   │   ├── provider.go      # GitHub Actions Provider (detect, context, OutputWriter via FileOutputWriter)
  │   │   ├── client.go        # GitHub API client wrapper (go-github)
  │   │   ├── checks.go        # CreateCheckRun, UpdateCheckRun
  │   │   ├── checks_test.go
  │   │   ├── status.go        # GetStatus implementation
  │   │   └── status_test.go
  │   └── generic/
  │       ├── provider.go      # Generic CI provider (CI=true detection, env var context, OutputWriter)
  │       ├── provider_test.go
  │       ├── check.go         # Generic check run support
  │       └── check_test.go
  └── templates/
      ├── loader.go            # Template loading with override support (config > base_path > embedded)
      └── loader_test.go
```
