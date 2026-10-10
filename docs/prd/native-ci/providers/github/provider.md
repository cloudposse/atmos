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
- `provider.go` — Detect, Context (from env vars, including `RunURL`), OutputWriter (creates `FileOutputWriter` using `$GITHUB_OUTPUT` and `$GITHUB_STEP_SUMMARY`)
- `client.go` — GitHub API client wrapper (go-github); see [API base URL](#api-base-url)
- `env.go` — `EnvExporter` (`WriteEnv`, `AddPath`)
- `mask.go` — `ValueMasker` (`MaskValue`)
- `checks.go` — CreateCheckRun, UpdateCheckRun (uses Commit Status API via `Repositories.CreateStatus`)
- `status.go` — GetStatus (combined commit status + check runs + PRs)

**NOT IMPLEMENTED** (Phase 4):
- `comment.go` — PR comment API (create/update/upsert with HTML markers)

## API Base URL

`NewClient()` resolves the REST API base URL in this order:

1. `ATMOS_CI_GITHUB_API_URL` — explicit override for the CI provider only.
2. `GITHUB_API_URL` — exported natively by GitHub Actions, and set to the enterprise API URL on GitHub Enterprise Server (GHES) runners.
3. `https://api.github.com/` — the public default.

The value must be an absolute `http(s)` URL; a trailing slash is added when missing, and the upload URL follows the base URL. An invalid value fails with an error wrapping `ErrInvalidURL` instead of silently sending the token to the wrong host.

## Context: RunURL

`Context.RunURL` links to the current run: `<GITHUB_SERVER_URL>/<repository>/actions/runs/<run id>`. It is empty when `GITHUB_SERVER_URL`, the repository, or the run ID is unavailable. It replaces the terraform plugin's private `getGitHubActionsRunURL` and is the default details URL for `Reporter.Check`.

## Context: pull request fork detection

For `pull_request` and `pull_request_target` events, `Context.PullRequest.Fork` is read from the event payload (`$GITHUB_EVENT_PATH`). The head repository is a fork when `pull_request.head.repo.fork` is true or when `head.repo.full_name` differs from `base.repo.full_name`. A missing or unreadable payload, a deleted head repository, or missing repository objects report false.

A `workflow_run` event has no `pull_request` object. When `workflow_run.head_repository` is a fork, or its `full_name` differs from the repository, the provider reports a fork pull request (`Fork` true, the head branch from `workflow_run.head_branch`, and the number and base from `workflow_run.pull_requests[0]` when present, otherwise 0). Without this the fork gate would never see the fork. A same-repository `workflow_run` leaves `PullRequest` nil.

The reporter's fork gate uses `Fork` together with `ElevatedEvent`. See [Interfaces](../../framework/interfaces.md).

## Environment Export and Masking

The GitHub provider implements two optional capabilities used by the [Reporter](../../framework/interfaces.md#reporter-script-and-step-facing-seam):

- **`EnvExporter`.** `WriteEnv(key, value)` appends a `ghactions.FormatValue`-formatted entry (collision-safe heredoc for multiline values) to the file named by `$GITHUB_ENV`. `AddPath(dir)` appends `dir` to the file named by `$GITHUB_PATH`. When the variable is unset the call returns an error wrapping `ErrCIEnvWriteFailed` rather than falling back to stdout, because the export could not take effect.
- **`ValueMasker`.** `MaskValue(value)` emits `::add-mask::<escaped value>` on the data stream (stdout), using the same escaper as annotations, because the runner parses workflow commands from the step log. An empty value is ignored. Write failures wrap `ErrCIMaskFailed`.

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
- `pkg/ci/providers/github/ghtest` fake GitHub REST API (with a `SetEnv` Actions fixture and `RegisterProvider`) for end-to-end provider and Reporter tests without network access
- Mock storage backends for planfile store tests
- Table-driven tests for output formatting
- Interface-based testing with generated mocks (`go.uber.org/mock/mockgen`)
- Golden file tests for template rendering (plan, apply, with changes, no changes, errors)
- Coverage target: 80%
