# Atmos Deployments - CI/CD Integration

This document describes the VCS and CI/CD provider abstraction for native integration with GitHub Actions, GitLab CI, and other platforms.

## Overview

Atmos provides **dual provider abstraction** - separate VCS and CI/CD providers:
- **VCS Providers**: Version control operations (PR comments, commit status, releases)
- **CI/CD Providers**: Automation operations (matrix generation, approvals, job summaries)

**Rationale**: Some platforms provide both (GitHub, GitLab), some only CI/CD (CircleCI, Jenkins), some only VCS (Gitea).

## VCS Provider Interface

Located at `pkg/vcs/interface.go` (matches Gotcha pattern):

```go
type Platform string

const (
    PlatformGitHub      Platform = "github"
    PlatformGitLab      Platform = "gitlab"
    PlatformBitbucket   Platform = "bitbucket"
    PlatformAzureDevOps Platform = "azuredevops"
    PlatformGitea       Platform = "gitea"
)

type Provider interface {
    DetectContext() (Context, error)
    CreateCommentManager(ctx Context) CommentManager
    GetCommitStatusWriter() CommitStatusWriter  // Optional
    GetReleasePublisher() ReleasePublisher      // Optional
    GetPlatform() Platform
    IsAvailable() bool
}

type CommentManager interface {
    PostOrUpdateComment(ctx context.Context, content string) error
    FindExistingComment(ctx context.Context, uuid string) (interface{}, error)
}
```

**VCS Providers are for**:
- PR/MR comments (deployment status, SBOM links)
- Commit status checks (✓ Deployment successful)
- Creating releases (tagging release records)

## CI/CD Provider Interface

Located at `pkg/cicd/interface.go`:

```go
type Platform string

const (
    PlatformGitHubActions   Platform = "github-actions"
    PlatformGitLabCI        Platform = "gitlab-ci"
    PlatformCircleCI        Platform = "circleci"
    PlatformJenkins         Platform = "jenkins"
    PlatformBuildkite       Platform = "buildkite"
    PlatformSpacelift       Platform = "spacelift"
)

type Provider interface {
    DetectContext() (Context, error)
    CreateApprovalManager(ctx Context) (ApprovalManager, error)
    CreateMatrixStrategy() MatrixStrategy
    GetWorkflowDispatcher() WorkflowDispatcher  // Optional
    GetJobSummaryWriter() JobSummaryWriter      // Optional
    GetArtifactPublisher() ArtifactPublisher    // Optional
    GetPlatform() Platform
    IsAvailable() bool
}

type MatrixStrategy interface {
    GenerateMatrix(deployment, target string) (*Matrix, error)
    GenerateMatrixForWaves(deployment, target string) (*WaveMatrix, error)
}

type ApprovalManager interface {
    RequestApproval(ctx context.Context, req ApprovalRequest) (*ApprovalResponse, error)
    IsApprovalSupported() bool
}
```

**CI/CD Providers are for**:
- Matrix generation (parallel component deployment)
- Approval workflows (Atmos Pro integration)
- Job summaries (deployment reports in CI UI)
- Workflow dispatch (trigger deployments programmatically)

## Dual Provider Pattern

### GitHub (Both VCS + CI/CD)

```go
// pkg/vcs/github/provider.go
func init() {
    vcs.RegisterProvider(vcs.PlatformGitHub, NewGitHubVCSProvider)
}

// pkg/cicd/github/provider.go
func init() {
    cicd.RegisterProvider(cicd.PlatformGitHubActions, NewGitHubCICDProvider)
}
```

### CircleCI (CI/CD Only)

```go
// pkg/cicd/circleci/provider.go
func init() {
    cicd.RegisterProvider(cicd.PlatformCircleCI, NewCircleCIProvider)
}
// No VCS provider - CircleCI doesn't manage repos
```

### Gitea (VCS Only)

```go
// pkg/vcs/gitea/provider.go
func init() {
    vcs.RegisterProvider(vcs.PlatformGitea, NewGiteaProvider)
}
// No CI/CD provider (yet)
```

## Usage in Atmos

```go
// Detect both providers independently
vcsProvider := vcs.DetectProvider()
cicdProvider := cicd.DetectProvider()

// Use VCS for PR comments
if vcsProvider != nil {
    commentMgr := vcsProvider.CreateCommentManager(ctx)
    commentMgr.PostOrUpdateComment(ctx, "Deployment started...")
}

// Use CI/CD for matrix generation
if cicdProvider != nil {
    matrixStrategy := cicdProvider.CreateMatrixStrategy()
    matrix, _ := matrixStrategy.GenerateMatrix("payment-service", "prod")
}
```

## GitHub Actions Integration

### Zero-Bash Workflow Example

```yaml
# .github/workflows/deploy.yml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  matrix:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.generate.outputs.matrix }}
    steps:
      - uses: actions/checkout@v4
      - uses: cloudposse/atmos@v1
      - id: generate
        run: atmos deployment matrix payment-service --target prod --format json

  deploy:
    needs: matrix
    runs-on: ubuntu-latest
    strategy:
      matrix: ${{ fromJson(needs.matrix.outputs.matrix) }}
    steps:
      - uses: actions/checkout@v4
      - uses: cloudposse/atmos@v1

      - name: Deploy component
        run: atmos deployment rollout payment-service --target prod --component ${{ matrix.component }}

      - name: Wait for approval (Atmos Pro)
        if: matrix.component == 'ecs/service'
        run: atmos deployment approve payment-service --target prod --component ${{ matrix.component }}
```

### Matrix Generation

```bash
# Generate matrix for GitHub Actions
atmos deployment matrix payment-service --target prod --format json

# Output:
{
  "include": [
    {"component": "ecr/api", "wave": 1},
    {"component": "nixpack/api", "wave": 2},
    {"component": "ecs/taskdef-api", "wave": 3},
    {"component": "ecs/service-api", "wave": 4}
  ]
}
```

### Job Summary

Atmos automatically writes GitHub Actions job summaries:

```markdown
## Deployment Summary

**Deployment**: payment-service
**Target**: prod
**Status**: ✅ Success

### Components

| Component | Type | Status | Duration |
|-----------|------|--------|----------|
| ecr/api | terraform | ✅ Success | 12s |
| nixpack/api | nixpack | ✅ Success | 2m 34s |
| ecs/taskdef-api | terraform | ✅ Success | 8s |
| ecs/service-api | terraform | ✅ Success | 45s |

### Artifacts

- Container Image: `123456789012.dkr.ecr.us-east-1.amazonaws.com/api@sha256:abc123...`
- SBOM: [Download](releases/payment-service/prod/sbom-xyz789.cdx.json)
- Release Record: [releases/payment-service/prod/release-xyz789.yaml](releases/payment-service/prod/release-xyz789.yaml)
```

## Atmos Pro Approval Integration

```go
// pkg/cicd/github/approval.go
type GitHubApprovalManager struct {
    atmosProClient *pro.Client
}

func (m *GitHubApprovalManager) RequestApproval(ctx context.Context, req ApprovalRequest) (*ApprovalResponse, error) {
    // Create approval request in Atmos Pro
    approval, err := m.atmosProClient.CreateApproval(ctx, &pro.ApprovalRequest{
        Deployment: req.Deployment,
        Target:     req.Target,
        Component:  req.Component,
        Requester:  req.Requester,
        RunURL:     req.RunURL,
    })
    if err != nil {
        return nil, err
    }

    // Poll for approval/rejection
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-ticker.C:
            status, err := m.atmosProClient.GetApprovalStatus(ctx, approval.ID)
            if err != nil {
                return nil, err
            }

            if status.IsApproved() {
                return &ApprovalResponse{Approved: true, ApprovedBy: status.ApprovedBy}, nil
            }
            if status.IsRejected() {
                return &ApprovalResponse{Approved: false, RejectedBy: status.RejectedBy}, nil
            }
        }
    }
}
```

## Future Providers

**Planned implementations**:
- GitLab CI (VCS + CI/CD)
- Bitbucket Pipelines (VCS + CI/CD)
- Azure DevOps (VCS + CI/CD)
- CircleCI (CI/CD only) - P1
- Jenkins (CI/CD only) - P2
- Buildkite (CI/CD only) - P2
- Spacelift (CI/CD only) - P2

## Design Principles

1. **Separation of Concerns**: VCS and CI/CD are independent
2. **Optional Capabilities**: Graceful degradation for unsupported features
3. **Auto-Detection**: Environment variable-based provider discovery
4. **Atmos Pro Integration**: Built-in approval workflows
5. **DAG-Aware**: Matrix strategies understand component dependencies
6. **Local-First**: Same commands work locally and in CI

## See Also

- **[overview.md](./overview.md)** - Core concepts
- **[concurrent-execution.md](./concurrent-execution.md)** - DAG-based parallel execution
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
