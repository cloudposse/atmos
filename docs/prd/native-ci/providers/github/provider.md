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

## Context: pull request number and fork detection

For `pull_request` and `pull_request_target` events, `Context.PullRequest` is read from the event payload (`$GITHUB_EVENT_PATH`), with `GITHUB_REF` as the fallback. Under `pull_request_target` GitHub sets `GITHUB_REF` to the base branch, so the number comes from `pull_request.number` in the payload and falls back to `refs/pull/<n>/merge` only when the payload carries none.

`Context.PullRequest.Fork` is true when `head.repo.full_name` differs from `base.repo.full_name`. `head.repo.fork` is not consulted: it says the head repository is a fork of something, not that the pull request comes from a fork, so a repository that is itself a fork does not hold its own same-repository pull requests. The gate fails closed. A deleted head repository (`head.repo` is null) and an unreadable or missing payload report a fork.

A `workflow_run` event has no `pull_request` object. The provider reads `workflow_run.pull_requests[0]` for the number and base, and `workflow_run.head_repository` for the fork decision with the same `full_name` rule (a missing `head_repository` is a fork; the head branch comes from `workflow_run.head_branch`). A same-repository `workflow_run` reports the pull request the payload names, or leaves `PullRequest` nil when it names none. GitHub leaves `workflow_run.pull_requests` empty for fork runs, so a fork run has number 0 unless the payload supplies one. A pull request that is already known is never replaced by a run that only describes the same one. Repository names are compared case-insensitively.

The reporter's fork gate uses `Fork` together with `ElevatedEvent` to hold comments, commit statuses, environment and `PATH` exports, and SARIF uploads. See [Interfaces](../../framework/interfaces.md) and the [posting gate](../../framework/fork-pr-trust-gate.md#fr-34-posting-gate-for-fork-pull-requests).

## Comments, commit comments, and statuses

- **Pull request comments** use the issue comments endpoints and need `pull-requests: write`.
- **Commit comments** (`CommitCommenter`) use `POST/GET /repos/{owner}/{repo}/commits/{sha}/comments` and `PATCH /repos/{owner}/{repo}/comments/{id}`, find an earlier comment by its marker, and need `contents: write`. The reporter chooses them for `target="commit"`, and for `target="auto"` when no pull request number is known.
- **Statuses.** `ci.check` and the native checks write commit statuses (`statuses: write`). GitHub has four states, so `pending` and `in_progress` map to `pending`, `success` to `success`, `failure` to `failure`, and `error` and `cancelled` to `error`. The returned handle keeps the requested state and the details URL (the request URL, else the run URL). Statuses are keyed by context, so an update does not need the id.

## Environment Export and Masking

The GitHub provider implements two optional capabilities used by the [Reporter](../../framework/interfaces.md#reporter-script-and-step-facing-seam):

- **`EnvExporter`.** `WriteEnv(key, value)` appends a `provider.FormatOutputLine`-formatted entry (collision-safe heredoc for multiline values) to the file named by `$GITHUB_ENV`. `AddPath(dir)` appends `dir` to the file named by `$GITHUB_PATH`. When the variable is unset the call returns an error wrapping `ErrCIEnvWriteFailed` rather than falling back to stdout, because the export could not take effect.
- **`ValueMasker`.** `MaskValue(value)` emits `::add-mask::<escaped value>` on stderr, using the same escaper as annotations. The runner parses workflow commands from both streams, and stderr keeps stdout clean for piped data. An empty value is ignored. Write failures wrap `ErrCIMaskFailed`.

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
