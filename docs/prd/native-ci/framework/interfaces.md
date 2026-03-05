# Native CI Integration - Core Interfaces

> Related: [Overview](../overview.md) | [GitHub Provider](../providers/github/provider.md) | [Configuration](./configuration.md)

## Core Interfaces

### Provider Interface

```go
// pkg/ci/provider.go

// Provider represents a CI/CD provider (GitHub Actions, GitLab CI, etc.).
type Provider interface {
    // Name returns the provider name (e.g., "github", "gitlab").
    Name() string

    // Detect returns true if this provider is active in the current environment.
    Detect() bool

    // Context returns CI metadata (run ID, PR info, etc.).
    Context() (*Context, error)

    // GetStatus returns PR/commit status for the current branch (read).
    GetStatus(ctx context.Context, opts StatusOptions) (*Status, error)

    // CreateCheckRun creates a new check run on a commit (write, like Atlantis).
    CreateCheckRun(ctx context.Context, opts CreateCheckRunOptions) (*CheckRun, error)

    // UpdateCheckRun updates an existing check run (write).
    UpdateCheckRun(ctx context.Context, opts UpdateCheckRunOptions) (*CheckRun, error)

    // OutputWriter returns a writer for CI outputs ($GITHUB_OUTPUT, etc.).
    OutputWriter() OutputWriter
}

// OutputWriter writes CI outputs (environment variables, job summaries, etc.).
type OutputWriter interface {
    // WriteOutput writes a key-value pair to CI outputs (e.g., $GITHUB_OUTPUT).
    WriteOutput(key, value string) error

    // WriteSummary writes content to the job summary (e.g., $GITHUB_STEP_SUMMARY).
    WriteSummary(content string) error
}
```

### Context Struct

```go
// pkg/ci/context.go

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

## Plugin Interface

The plugin owns its CI behavior. The executor passes `(provider, store, opts)` to the plugin's hook callbacks. See [Hooks Integration](./hooks-integration.md) for the full architecture.

```go
// pkg/ci/internal/plugin/types.go

// HookAction is a callback function invoked by the executor for a lifecycle event.
// The executor resolves the current provider and store, then passes them to the callback.
// The store parameter is artifact.Store — plugins use their adapter layer to work with
// plugin-specific store types (e.g., terraform plugin wraps via planfile/adapter/).
type HookAction func(provider Provider, store artifact.Store, opts ExecuteOptions) error

// HookBinding maps a lifecycle event to a callback function.
type HookBinding struct {
    Event  string      // "after.terraform.plan"
    Action HookAction  // callback function provided by the plugin
}

// Plugin represents a component-type CI plugin (terraform, helmfile, etc.).
// The interface is generic — no terraform-specific methods. Each plugin wires
// its own methods as HookAction callbacks in GetHookBindings().
type Plugin interface {
    // GetType returns the component type (e.g., "terraform").
    GetType() string

    // GetHookBindings returns lifecycle events this plugin subscribes to,
    // each with a callback function the executor will invoke.
    GetHookBindings() []HookBinding

    // GetDefaultTemplates returns embedded default templates.
    GetDefaultTemplates() embed.FS
}
```

Methods like `ParseOutput`, `BuildTemplateContext`, `GetOutputVariables`, and `GetArtifactKey` are **internal helpers** on the terraform plugin — not part of the generic Plugin interface. Each plugin calls them from within its own hook callbacks.

Example: terraform plugin wiring its methods as callbacks:

```go
// pkg/ci/plugins/terraform/plugin.go

func (p *Plugin) GetHookBindings() []plugin.HookBinding {
    return []plugin.HookBinding{
        {Event: "before.terraform.plan",  Action: p.OnBeforePlan},
        {Event: "after.terraform.plan",   Action: p.OnAfterPlan},
        {Event: "before.terraform.apply", Action: p.OnBeforeApply},
        {Event: "after.terraform.apply",  Action: p.OnAfterApply},
    }
}
```

## Package Structure

```
pkg/ci/
  ├── check.go                 # CheckRun types and constants
  ├── plugin.go                # Plugin interface
  ├── plugin_registry.go       # Plugin registry
  ├── context.go               # Context struct (run ID, PR, SHA, etc.)
  ├── executor.go              # Thin coordinator: provider/plugin/store wiring
  ├── output.go                # OutputWriter interface
  ├── provider.go              # Provider interface definition
  ├── registry.go              # Provider registry (detect and select provider)
  ├── status.go                # Status, BranchStatus, PRStatus, CheckStatus structs
  ├── artifact/                # Base artifact storage layer (common interface)
  │   ├── store.go             # Store interface, FileEntry/FileResult, StoreFactory
  │   ├── metadata.go          # Metadata, ArtifactInfo structs (SHA, Component, Stack, timestamps, etc.)
  │   ├── query.go             # Query struct for filtering
  │   ├── registry.go          # Backend registry: Register(), NewStore()
  │   ├── selector.go          # EnvironmentChecker, SelectStore() for priority-based selection
  │   ├── mock_store.go        # Generated mock
  │   └── local/
  │       └── store.go         # Local filesystem backend
  ├── plugins/terraform/
  │   ├── plugin.go            # Terraform CI plugin (hook callbacks, output parsing)
  │   ├── parser.go            # Parse plan/apply output
  │   ├── context.go           # Terraform template context
  │   ├── templates/
  │   │   ├── plan.md          # Default plan template
  │   │   └── apply.md         # Default apply template
  │   └── planfile/            # Planfile storage (extends artifact.Store)
  │       ├── interface.go     # planfile.Store interface, Metadata
  │       ├── registry.go      # Planfile-specific storage registry
  │       ├── adapter/         # Adapter: planfile.Store -> artifact.Store
  │       │   ├── store.go     # Adapter implementation
  │       │   └── factory.go   # StoreFactory for registry
  │       ├── s3/
  │       │   └── store.go     # S3 store (metadata in S3, no DynamoDB)
  │       ├── github/
  │       │   └── store.go     # GitHub Artifacts store (Phase 4)
  │       └── local/
  │           └── store.go     # Local filesystem store
  ├── providers/
  │   ├── github/              # Implements ci.Provider for GitHub Actions
  │   │   ├── provider.go      # GitHub Actions Provider
  │   │   ├── client.go        # GitHub API client wrapper
  │   │   ├── checks.go        # Check runs API
  │   │   └── status.go        # GetStatus, GetCombinedStatus
  │   └── generic/             # Generic CI provider fallback
  │       └── provider.go      # Detects CI=true, basic context from env vars
  └── templates/
      └── loader.go            # Template loading with override support
```
