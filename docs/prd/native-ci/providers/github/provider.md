# Native CI Integration - GitHub Provider

> Related: [Overview](../../overview.md) | [Interfaces](../../framework/interfaces.md) | [Status Checks](./status-checks.md) | [Configuration](../../framework/configuration.md)

## GitHub Actions Permissions

Different CI features require different GitHub Actions permissions. Add only what you need:

```yaml
permissions:
  id-token: write    # Required: OIDC authentication with AWS/cloud providers
  contents: read     # Required: Checkout repository
  statuses: write    # Optional: Post commit status checks (ci.checks.enabled: true)
  pull-requests: write  # Optional: Post PR comments (ci.comments.enabled: true)
```

| Permission | Required | Enables |
|------------|----------|---------|
| `id-token: write` | Yes | OIDC authentication via `atmos auth` for AWS, Azure, GCP |
| `contents: read` | Yes | Checkout repository code |
| `statuses: write` | No | Commit status checks showing "Plan in progress" / resource change counts (`ci.checks.enabled: true`) |
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
  statuses: write
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

// IsExperimental returns true because CI commands are experimental.
func (c *CICommandProvider) IsExperimental() bool {
    return true
}
```

Commands are registered via blank imports in `cmd/root.go`:

```go
import (
    _ "github.com/cloudposse/atmos/cmd/ci"
)
```

## Implementation Status

**IMPLEMENTED** (`pkg/ci/providers/github/`):
- `provider.go` — Detect, Context (from env vars), OutputWriter (creates `FileOutputWriter` using `$GITHUB_OUTPUT` and `$GITHUB_STEP_SUMMARY`)
- `client.go` — GitHub API client wrapper (go-github)
- `checks.go` — CreateCheckRun, UpdateCheckRun (uses Commit Status API via `Repositories.CreateStatus`)
- `status.go` — GetStatus (combined commit status + check runs + PRs)

**NOT IMPLEMENTED** (Phase 4):
- `comment.go` — PR comment API (create/update/upsert with HTML markers)

## GitHub API Endpoints

The GitHub provider uses the following API endpoints:

| Endpoint | Purpose | Status |
|----------|---------|--------|
| `GET /repos/{owner}/{repo}/commits/{ref}/status` | Combined commit status | Done |
| `GET /repos/{owner}/{repo}/commits/{ref}/check-runs` | GitHub Actions check runs (read) | Done |
| `POST /repos/{owner}/{repo}/statuses/{sha}` | Create/update commit status | Done |
| `GET /repos/{owner}/{repo}/pulls?head={owner}:{branch}` | PRs for current branch | Done |
| `GET /user` | Authenticated user info | Phase 4 |
| `GET /search/issues?q=...` | Search for user's PRs | Phase 4 |
| `POST /repos/{owner}/{repo}/issues/{number}/comments` | Create PR comment | Phase 4 |
| `PATCH /repos/{owner}/{repo}/issues/comments/{id}` | Update PR comment | Phase 4 |

## Testing Strategy

**Mocks + golden files. No real API calls.**

- Mock GitHub API client for provider tests
- Mock storage backends for planfile store tests
- Table-driven tests for output formatting
- Interface-based testing with generated mocks (`go.uber.org/mock/mockgen`)
- Golden file tests for template rendering (plan, apply, with changes, no changes, errors)
- Coverage target: 80%
