# Native CI Integration - GitHub Provider

> Related: [Overview](../../overview.md) | [Interfaces](../../framework/interfaces.md) | [Status Checks](./status-checks.md) | [Configuration](../../framework/configuration.md)

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
