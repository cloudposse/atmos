# Native CI Integration - GitHub Provider

> Related: [Overview](./overview.md) | [Status Checks](./status-checks.md) | [Configuration](./configuration.md)

## GitHub Actions Permissions

Different CI features require different GitHub Actions permissions. Add only what you need:

```yaml
permissions:
  id-token: write    # Required: OIDC authentication with AWS/cloud providers
  contents: read     # Required: Checkout repository
  checks: write      # Optional: Post status checks (ci.checks.enabled: true)
  pull-requests: write  # Optional: Post PR comments (ci.comments.enabled: true)
```

| Permission | Required | Enables |
|------------|----------|---------|
| `id-token: write` | Yes | OIDC authentication via `atmos auth` for AWS, Azure, GCP |
| `contents: read` | Yes | Checkout repository code |
| `checks: write` | No | Status checks showing "Plan in progress" / "Plan complete" (`ci.checks.enabled: true`) |
| `pull-requests: write` | No | PR comments with plan summaries (`ci.comments.enabled: true`) |

**Minimal workflow** (job summaries only):

```yaml
permissions:
  id-token: write
  contents: read
```

**Full-featured workflow** (status checks + PR comments):

```yaml
permissions:
  id-token: write
  contents: read
  checks: write
  pull-requests: write
```

## Command Registry Pattern

All new commands use the command registry pattern (see `docs/prd/command-registry-pattern.md`):

```go
// cmd/ci/ci.go
package ci

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

func init() {
    internal.Register(&CICommandProvider{})
}

type CICommandProvider struct{}

func (c *CICommandProvider) GetCommand() *cobra.Command {
    return ciCmd
}

func (c *CICommandProvider) GetName() string {
    return "ci"
}

func (c *CICommandProvider) GetGroup() string {
    return "CI/CD Integration"
}

func (c *CICommandProvider) GetAliases() []internal.CommandAlias {
    return nil
}
```

Commands are registered via blank imports in `cmd/root.go`:

```go
import (
    _ "github.com/cloudposse/atmos/cmd/ci"
)
```

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

## GitHub API Endpoints

The GitHub provider uses the following API endpoints:

| Endpoint | Purpose |
|----------|---------|
| `GET /repos/{owner}/{repo}/commits/{ref}/status` | Combined commit status |
| `GET /repos/{owner}/{repo}/commits/{ref}/check-runs` | GitHub Actions check runs |
| `GET /repos/{owner}/{repo}/pulls?head={owner}:{branch}` | PRs for current branch |
| `GET /user` | Authenticated user info |
| `GET /search/issues?q=...` | Search for user's PRs |
| `POST /repos/{owner}/{repo}/issues/{number}/comments` | Create PR comment |
| `PATCH /repos/{owner}/{repo}/issues/comments/{id}` | Update PR comment |

## Testing Strategy

### Unit Tests

- Mock GitHub API client for provider tests
- Mock storage backends for planfile store tests
- Table-driven tests for output formatting
- Interface-based testing with generated mocks

### Integration Tests

- Test against real GitHub API (with test token)
- Test against real S3/Azure/GCS (with test credentials)
- Test CI detection in various environments

### End-to-End Tests

- Test full workflow in GitHub Actions
- Test planfile upload/download cycle
- Test PR comment creation/update
