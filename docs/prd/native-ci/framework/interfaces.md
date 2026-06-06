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
    Number     int
    Title      string
    Branch     string
    BaseBranch string
    URL        string
    Checks     []*CheckStatus
    AllPassed  bool
}

type CheckStatus struct {
    Name       string
    Status     string  // "queued", "in_progress", "completed"
    Conclusion string  // "success", "failure", "neutral", etc.
    DetailsURL string
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

The plugin owns its CI behavior via callback-based dispatch. The executor builds a `HookContext` and invokes the plugin's handler. See [Hooks Integration](./hooks-integration.md) for the full architecture.

```go
// pkg/ci/internal/plugin/types.go

// HookHandler is the callback signature for plugin event handlers.
type HookHandler func(ctx *HookContext) error

// HookContext provides all dependencies a plugin handler needs.
type HookContext struct {
    Event        string
    Command      string                       // extracted from event (e.g., "plan")
    EventPrefix  string                       // "before" or "after"
    Config       *schema.AtmosConfiguration
    Info         *schema.ConfigAndStacksInfo
    Output       string
    CommandError error
    Provider     provider.Provider
    CICtx        *provider.Context
    TemplateLoader *templates.Loader
    CheckRunStore  CheckRunStore
    CreatePlanfileStore func() (any, error)   // Lazy factory for planfile store
}

// CheckRunStore correlates check run IDs across before/after events.
type CheckRunStore interface {
    Store(key string, id int64)
    LoadAndDelete(key string) (int64, bool)
}

// HookBinding declares what happens at a specific hook event.
type HookBinding struct {
    Event   string       // "after.terraform.plan"
    Handler HookHandler  // Plugin callback that owns all action logic
}

// Plugin is implemented by component types that support CI integration.
type Plugin interface {
    GetType() string
    GetHookBindings() []HookBinding
}
```

### Terraform Plugin Hook Bindings (IMPLEMENTED)

```go
// pkg/ci/plugins/terraform/plugin.go

func (p *Plugin) GetHookBindings() []plugin.HookBinding {
    return []plugin.HookBinding{
        {Event: "before.terraform.plan",  Handler: p.onBeforePlan},
        {Event: "after.terraform.plan",   Handler: p.onAfterPlan},
        {Event: "before.terraform.apply", Handler: p.onBeforeApply},
        {Event: "after.terraform.apply",  Handler: p.onAfterApply},
    }
}
```

## Package Structure (IMPLEMENTED)

```
pkg/ci/
  ├── executor.go              # Thin CI coordinator (~250 lines): detect platform, resolve plugin, invoke handler
  ├── executor_test.go
  ├── checkrun_store.go        # CheckRunStore interface + sync.Map-backed singleton
  ├── checkrun_store_test.go
  ├── provider.go              # Type alias for internal/provider.Provider
  ├── status.go                # Type aliases for status types
  ├── plugin_registry.go       # Plugin registry: RegisterPlugin(), GetPlugin(), GetPluginForEvent()
  ├── plugin_registry_test.go
  ├── registry_provider.go     # Provider registry: Register(), Detect(), DetectOrError(), IsCI()
  ├── registry_provider_test.go
  ├── mock_plugin_test.go      # Mock plugin for executor tests (2-method interface)
  ├── internal/
  │   ├── plugin/
  │   │   └── types.go         # Plugin interface (2 methods), HookHandler, HookContext, CheckRunStore, OutputResult, TemplateContext
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
  │   ├── plugin.go            # Terraform CI plugin (GetType, GetHookBindings + private helpers)
  │   ├── plugin_test.go
  │   ├── handlers.go          # All handler implementations (onBeforePlan, onAfterPlan, onBeforeApply, onAfterApply)
  │   ├── handlers_test.go
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
