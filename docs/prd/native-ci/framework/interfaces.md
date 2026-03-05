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

## Package Structure

```
pkg/ci/
  ├── check.go                 # CheckRun types and constants
  ├── plugin.go                # Plugin interface
  ├── plugin_registry.go       # Plugin registry
  ├── context.go               # Context struct (run ID, PR, SHA, etc.)
  ├── executor.go              # Execute() - unified action executor
  ├── generic.go               # Generic CI provider fallback
  ├── output.go                # OutputWriter interface
  ├── provider.go              # Provider interface definition
  ├── registry.go              # Provider registry (detect and select provider)
  ├── status.go                # Status, BranchStatus, PRStatus, CheckStatus structs
  ├── artifact/                # Generic artifact storage layer
  │   ├── store.go             # Store interface, FileEntry/FileResult, StoreFactory
  │   ├── metadata.go          # Metadata, ArtifactInfo structs
  │   ├── query.go             # Query struct for filtering
  │   ├── registry.go          # Backend registry: Register(), NewStore()
  │   ├── selector.go          # EnvironmentChecker, SelectStore()
  │   ├── mock_store.go        # Generated mock
  │   └── local/
  │       └── store.go         # Local filesystem backend
  ├── plugins/terraform/
  │   └── planfile/            # Planfile artifact storage (wraps artifact.Store)
  │       ├── interface.go     # planfile.Store interface, Metadata
  │       ├── registry.go      # Storage backend registry
  │       ├── adapter/         # Adapter: planfile.Store -> artifact.Store
  │       │   ├── store.go     # Adapter implementation
  │       │   └── factory.go   # StoreFactory for registry
  │       └── s3/
  │           └── store.go     # S3 store (metadata in S3, no DynamoDB)
  ├── github/                  # Implements ci.Provider for GitHub Actions
  │   ├── provider.go          # GitHub Actions Provider
  │   ├── client.go            # GitHub API client wrapper
  │   ├── checks.go            # Check runs API
  │   └── status.go            # GetStatus, GetCombinedStatus
  ├── terraform/               # Terraform-specific CI provider
  │   ├── provider.go          # Terraform CI provider
  │   ├── parser.go            # Parse plan/apply output
  │   ├── context.go           # Terraform template context
  │   └── templates/
  │       ├── plan.md          # Default plan template
  │       └── apply.md         # Default apply template
  └── templates/
      └── loader.go            # Template loading with override support
```
