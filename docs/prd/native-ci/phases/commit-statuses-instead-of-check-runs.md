# Use Commit Statuses Instead of Check Runs

> Related: [Status Checks](../providers/github/status-checks.md) | [GitHub Provider](../providers/github/provider.md) | [Configuration](../framework/configuration.md)

> **Note**: This PRD supersedes `move-checkrun-store-to-provider.md` for the GitHub provider. If commit statuses are adopted, the `sync.Map` ID correlation problem disappears entirely and that PRD becomes unnecessary. That PRD is paused pending this decision.

## Problem Statement

The current GitHub CI provider uses the **Checks API** (`POST /repos/{owner}/{repo}/check-runs`) to report operation status on commits. This is problematic because:

1. **Permissions**: The Checks API requires `checks: write` permission, which is **not a default permission** for `GITHUB_TOKEN` in GitHub Actions workflows. Users must explicitly declare `permissions: checks: write` in their workflow files. Classic PATs with `repo` scope cannot use the Checks API at all.

2. **Complexity**: Check runs have a two-phase lifecycle (create → update) requiring ID correlation via `sync.Map`. The provider must track `name → int64` mappings and handle fallback creation when no prior `CreateCheckRun` was called.

3. **Compatibility**: Tools like Atlantis, Spacelift, and most CI/CD integrations use **commit statuses** (the Status API), not check runs. Users expect Atmos status checks to appear alongside these tools in the same "Statuses" section of the PR, not in a separate "Checks" section.

4. **Simplicity**: The Status API is fire-and-forget — each call sets the status to a specific state. There is no create/update lifecycle, no ID tracking, no fallback logic. Setting a status to `pending` then later to `success` is two independent API calls using the same `context` string as the correlation key.

## Desired State

Replace the Checks API implementation with the **Commit Status API** (`POST /repos/{owner}/{repo}/statuses/{sha}`). Additionally, introduce a richer status model with per-component per-operation statuses for branch protection integration.

### GitHub Commit Status API

The Status API uses these fields:

| Field | Description |
|-------|-------------|
| `state` | Required. `error`, `failure`, `pending`, or `success` |
| `target_url` | URL linking back to the CI build/details |
| `description` | Short (140 character max) description of the status |
| `context` | Unique identifier for this status. Defaults to `default` |

Key differences from Check Runs:

| Aspect | Check Runs API | Commit Status API |
|--------|---------------|-------------------|
| Permission | `checks: write` (not default) | `statuses: write` or `repo` scope |
| Lifecycle | Create → Update (ID-based) | Set state (context-based, idempotent) |
| Correlation | Numeric ID from create response | `context` string (acts as upsert key) |
| Rich output | Title, summary, annotations (markdown) | `description` (140 chars) + `target_url` |
| States | queued, in_progress, completed + conclusion | pending, success, failure, error |
| ID tracking | Required (`sync.Map` name→ID) | Not needed (context string is the key) |

### Status Model

Each Atmos operation produces multiple commit statuses per component. Statuses are organized hierarchically using `/` as a separator.

#### Per-Component Statuses

For a `terraform plan` on component `vpc` in stack `plat-ue2-dev`:

| Context | When Created | State | Description |
|---------|-------------|-------|-------------|
| `atmos/plan/plat-ue2-dev/vpc` | Always (before + after) | `pending` → `success`/`failure` | `3 to add, 1 to change, 2 to destroy` |
| `atmos/plan/plat-ue2-dev/vpc/add` | Only if `to_add > 0` | `success` | `3 resources` |
| `atmos/plan/plat-ue2-dev/vpc/change` | Only if `to_change > 0` | `success` | `1 resource` |
| `atmos/plan/plat-ue2-dev/vpc/destroy` | Only if `to_destroy > 0` | `success` | `2 resources` |

The per-operation statuses (`/add`, `/change`, `/destroy`) are **informational only** — they always use `state: success`. Their presence signals the condition (e.g., "there are resources to destroy"). If there are no resources of that type, the status is simply not created.

This enables branch protection rules like:
- Require `atmos/plan/plat-ue2-dev/vpc` to pass before merging (ensure plan succeeded)
- Block merges when `atmos/plan/*/destroy` statuses appear (prevent accidental destroys)

#### Apply Statuses

Apply operations follow the same pattern:

| Context | When Created | State | Description |
|---------|-------------|-------|-------------|
| `atmos/apply/plat-ue2-dev/vpc` | Always | `pending` → `success`/`failure` | `3 added, 1 changed, 2 destroyed` |
| `atmos/apply/plat-ue2-dev/vpc/add` | Only if `added > 0` | `success` | `3 resources` |
| `atmos/apply/plat-ue2-dev/vpc/change` | Only if `changed > 0` | `success` | `1 resource` |
| `atmos/apply/plat-ue2-dev/vpc/destroy` | Only if `destroyed > 0` | `success` | `2 resources` |

#### Context Name Format

The `context` string is constructed using `ci.checks.context_prefix` from configuration:

```
{context_prefix}/{command}/{stack}/{component}
{context_prefix}/{command}/{stack}/{component}/{operation}
```

Where:
- `context_prefix` — from `ci.checks.context_prefix` config (default: `"atmos"`)
- `command` — `plan`, `apply`, `deploy`
- `stack` — stack name (e.g., `plat-ue2-dev`)
- `component` — component name (e.g., `vpc`)
- `operation` — `add`, `change`, `destroy` (conditional)

> **Note**: `FormatCheckRunName()` currently hardcodes `"atmos/"`. This PRD includes wiring `ci.checks.context_prefix` from configuration.

### Configuration

```yaml
ci:
  checks:
    enabled: true
    context_prefix: "atmos"    # Prefix for all status context strings
    statuses:
      component: true          # atmos/{command}/{stack}/{component}
      add: true                # atmos/{command}/{stack}/{component}/add
      change: true             # atmos/{command}/{stack}/{component}/change
      destroy: true            # atmos/{command}/{stack}/{component}/destroy
```

All status types are individually toggleable. The `ci.checks.enabled` config key name is unchanged (not renamed to `ci.statuses`) to avoid a breaking change — it's a user-facing abstraction, not an API detail.

### Provider Interface Changes

The `Provider` interface method names and types remain unchanged at the interface level. The naming (`CreateCheckRun`/`UpdateCheckRun`) is a provider-agnostic abstraction — it means "set status for an operation", not literally "use the GitHub Checks API".

The key change is in the **GitHub provider implementation**: replace Checks API calls with Status API calls.

### Provider Type Changes

#### `CheckRunState` Mapping to Status API

Current `CheckRunState` values map to commit status `state` as follows:

| CheckRunState | Commit Status `state` |
|---------------|----------------------|
| `pending` | `pending` |
| `in_progress` | `pending` (Status API has no "in_progress") |
| `success` | `success` |
| `failure` | `failure` |
| `error` | `error` |
| `cancelled` | `error` |

Note: The Status API does not have an `in_progress` state. Both `pending` and `in_progress` map to `pending`. The `description` field can indicate progress (e.g., "Plan in progress...").

#### `CreateCheckRunOptions` → Status API Mapping

| CreateCheckRunOptions field | Status API field | Notes |
|----------------------------|-----------------|-------|
| `Name` | `context` | The unique status identifier |
| `Status` | `state` | Mapped via table above |
| `Title` | `description` | Truncated to 140 chars (character-based) |
| `Summary` | (not used) | Status API has no rich summary |
| `DetailsURL` | `target_url` | GitHub Actions run URL |
| `ExternalID` | (not used) | Status API has no external ID |
| `Owner`, `Repo`, `SHA` | URL path params | `/repos/{owner}/{repo}/statuses/{sha}` |

#### `UpdateCheckRunOptions` → Status API Mapping

| UpdateCheckRunOptions field | Status API field | Notes |
|----------------------------|-----------------|-------|
| `Name` | `context` | Same context string as create |
| `Status` | `state` | Mapped via table above |
| `Title` | `description` | Truncated to 140 chars (character-based) |
| `Summary` | (not used) | Status API has no rich summary |
| `CompletedAt` | (not used) | Status API has no timestamp field |
| `Owner`, `Repo`, `SHA` | URL path params | `/repos/{owner}/{repo}/statuses/{sha}` |

### `target_url` Strategy

The `target_url` field links to the GitHub Actions run URL, constructed from environment variables:

```
$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID
```

This is set by the plugin handler when building `CreateCheckRunOptions.DetailsURL` / `UpdateCheckRunOptions.DetailsURL` (or by the provider if not set by the handler). The URL points to the full workflow run where users can see detailed logs and job summaries.

### Description Field Strategy

The Status API `description` field is limited to 140 characters. The description contains **only the resource change summary**, not the command or stack/component (which are already in the `context` string):

- **Before operation**: `"Plan in progress..."` or `"Apply in progress..."`
- **After operation (with changes)**: `"3 to add, 1 to change, 2 to destroy"`
- **After operation (no changes)**: `"No changes"`
- **After operation (failure)**: `"Failed"`
- **Per-operation statuses**: `"3 resources"` or `"1 resource"`

Truncation uses `utf8.RuneCountInString()` for character-based counting (not byte-based), ensuring Unicode stack/component names don't cause mid-character truncation.

### GitHub Provider Implementation

Both `createCheckRun` and `updateCheckRun` delegate to a single `setCommitStatus` private method, since the Status API is idempotent by `context` — there is no distinction between create and update.

```go
// setCommitStatus sets a commit status using the GitHub Status API.
func (p *Provider) setCommitStatus(ctx context.Context, owner, repo, sha, context, state, description, targetURL string) (*github.RepoStatus, error) {
    if err := p.ensureClient(); err != nil {
        return nil, err
    }

    repoStatus := &github.RepoStatus{
        State:       github.String(state),
        Context:     github.String(context),
        Description: github.String(truncateDescription(description)),
    }

    if targetURL != "" {
        repoStatus.TargetURL = github.String(targetURL)
    }

    status, _, err := p.client.GitHub().Repositories.CreateStatus(ctx, owner, repo, sha, repoStatus)
    return status, err
}

// createCheckRun sets a commit status on a commit.
func (p *Provider) createCheckRun(ctx context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
    state := mapCheckRunStateToStatusState(opts.Status)

    status, err := p.setCommitStatus(ctx, opts.Owner, opts.Repo, opts.SHA, opts.Name, state, opts.Title, opts.DetailsURL)
    if err != nil {
        return nil, fmt.Errorf("%w: %w", errUtils.ErrCICheckRunCreateFailed, err)
    }

    return &provider.CheckRun{
        ID:     status.GetID(),
        Name:   status.GetContext(),
        Status: opts.Status,
        Title:  status.GetDescription(),
    }, nil
}

// updateCheckRun updates a commit status on a commit.
// Since the Status API is idempotent by context, this is just another CreateStatus call.
func (p *Provider) updateCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
    state := mapCheckRunStateToStatusState(opts.Status)

    status, err := p.setCommitStatus(ctx, opts.Owner, opts.Repo, opts.SHA, opts.Name, state, opts.Title, "")
    if err != nil {
        return nil, fmt.Errorf("%w: %w", errUtils.ErrCICheckRunUpdateFailed, err)
    }

    return &provider.CheckRun{
        ID:     status.GetID(),
        Name:   status.GetContext(),
        Status: opts.Status,
        Title:  status.GetDescription(),
    }, nil
}
```

#### Helper Functions

```go
// mapCheckRunStateToStatusState maps CheckRunState to GitHub Status API state.
func mapCheckRunStateToStatusState(state provider.CheckRunState) string {
    switch state {
    case provider.CheckRunStatePending, provider.CheckRunStateInProgress:
        return "pending"
    case provider.CheckRunStateSuccess:
        return "success"
    case provider.CheckRunStateFailure:
        return "failure"
    case provider.CheckRunStateError, provider.CheckRunStateCancelled:
        return "error"
    default:
        return "pending"
    }
}

// truncateDescription truncates a description to 140 characters (GitHub API limit).
// Uses character count (runes), not byte count, to avoid mid-character truncation.
func truncateDescription(desc string) string {
    if utf8.RuneCountInString(desc) <= 140 {
        return desc
    }
    runes := []rune(desc)
    return string(runes[:137]) + "..."
}
```

### Plugin Handler Changes

The plugin handler (`pkg/ci/plugins/terraform/handlers.go`) needs changes to:

1. **Create per-operation statuses** after plan/apply based on `ResourceCounts`
2. **Read `ci.checks.statuses.*` config** to determine which statuses to create
3. **Use `context_prefix` from config** instead of hardcoded `"atmos/"`
4. **Set `DetailsURL`** to the GitHub Actions run URL
5. **Format descriptions** as resource change summaries only (not `command: summary`)

#### `updateCheckRun` handler changes (pseudo-code)

```go
func (p *Plugin) updateCheckRun(ctx *plugin.HookContext, result *plugin.OutputResult) error {
    prefix := getContextPrefix(ctx.Config) // reads ci.checks.context_prefix
    checksConfig := getChecksStatuses(ctx.Config) // reads ci.checks.statuses.*

    // Component-level status (atmos/{command}/{stack}/{component}).
    if checksConfig.Component {
        name := formatStatusContext(prefix, ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
        description := buildStatusDescription(result) // "3 to add, 1 to change, 2 to destroy"
        ctx.Provider.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{
            Name:   name,
            Status: status,
            Title:  description,
            ...
        })
    }

    // Per-operation statuses (only if count > 0).
    if tfData, ok := result.Data.(*plugin.TerraformOutputData); ok {
        if checksConfig.Add && tfData.ResourceCounts.Create > 0 {
            name := formatStatusContext(prefix, ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg, "add")
            desc := formatResourceCount(tfData.ResourceCounts.Create)
            ctx.Provider.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{
                Name:   name,
                Status: provider.CheckRunStateSuccess,
                Title:  desc, // "3 resources"
                ...
            })
        }
        if checksConfig.Change && tfData.ResourceCounts.Change > 0 {
            // same pattern for "change"
        }
        if checksConfig.Destroy && tfData.ResourceCounts.Destroy > 0 {
            // same pattern for "destroy"
        }
    }
}
```

### `FormatCheckRunName` Changes

Wire `context_prefix` from configuration:

```go
// FormatStatusContext creates a standardized status context string for Atmos.
// Parts are joined with "/" separator.
func FormatStatusContext(prefix string, parts ...string) string {
    allParts := append([]string{prefix}, parts...)
    return strings.Join(allParts, "/")
}
```

The old `FormatCheckRunName(action, stack, component)` is replaced or refactored to accept the prefix from config.

### What Gets Removed

The following code is no longer needed in the GitHub provider:

| Item | Reason |
|------|--------|
| `checkRunIDs sync.Map` field on `Provider` | No ID tracking needed — Status API uses `context` string |
| `createCompletedCheckRun()` | No fallback needed — `updateCheckRun` is just another `CreateStatus` call |
| `mapGitHubStatusToCheckRunState()` | No longer reading check run responses |
| `mapCheckRunStateToConclusion()` | Status API uses `state` not `conclusion` |
| `getCheckRunOutputTitle()` | No check run output to extract |
| `getCheckRunOutputSummary()` | No check run output to extract |
| Status constants `statusQueued`, `statusInProgress`, `statusCompleted` | Replaced by Status API states |

### What Stays the Same

| Item | Reason |
|------|--------|
| `Provider` interface (`CreateCheckRun`/`UpdateCheckRun` method signatures) | Provider-agnostic naming — callers don't know the underlying API |
| `CreateCheckRunOptions` / `UpdateCheckRunOptions` types | Superset of fields — unused fields are simply ignored |
| `CheckRun` return type | Still returns status info to callers |
| `status.go` (reading checks/statuses for `atmos ci status`) | Separate concern — reads both APIs for display |
| Generic provider | Already just prints to console |
| Config key `ci.checks.enabled` | User-facing abstraction — not renamed |

### Impact on `atmos ci status`

The `getCheckRuns()` function in `status.go` reads check runs via `Checks.ListCheckRunsForRef()`. The `getCombinedStatus()` function reads commit statuses via `Repositories.GetCombinedStatus()`. After this change, Atmos-created statuses will appear in `getCombinedStatus()` results instead of `getCheckRuns()` results. No code changes needed in `status.go` since it already reads both APIs.

### Permission Changes

| Before | After |
|--------|-------|
| `checks: write` | `statuses: write` |

For `GITHUB_TOKEN` in workflows:

```yaml
# Before
permissions:
  checks: write

# After
permissions:
  statuses: write
```

For PATs: `repo` scope covers `statuses: write`. No special GitHub App configuration needed.

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/ci/providers/github/checks.go` | Replace Checks API calls with Status API via `setCommitStatus`. Remove `sync.Map` ID tracking, `createCompletedCheckRun`, status constants, all check-run-specific helper functions. Add `setCommitStatus()`, `mapCheckRunStateToStatusState()`, `truncateDescription()` |
| `pkg/ci/providers/github/checks_test.go` | Update tests to mock `Repositories.CreateStatus` instead of `Checks.CreateCheckRun`/`Checks.UpdateCheckRun` |
| `pkg/ci/providers/github/provider.go` | Remove `checkRunIDs sync.Map` field from `Provider` struct |
| `pkg/ci/internal/provider/check.go` | Replace `FormatCheckRunName()` with `FormatStatusContext(prefix, parts...)` that reads `context_prefix` from config |
| `pkg/ci/plugins/terraform/handlers.go` | Update `createCheckRun`/`updateCheckRun` handlers to: set `DetailsURL` to GHA run URL, format descriptions as resource summaries only, create per-operation statuses (`/add`, `/change`, `/destroy`) based on `ResourceCounts` and `ci.checks.statuses.*` config, use `context_prefix` from config |
| `pkg/ci/plugins/terraform/handlers_test.go` | Add tests for per-operation statuses, config-driven status creation, description formatting |
| `pkg/schema/schema.go` | Add `Statuses` field to `CIChecksConfig` struct with `Component`, `Add`, `Change`, `Destroy` booleans |
| `docs/prd/native-ci/providers/github/provider.md` | Update permissions section (`checks: write` → `statuses: write`), update API endpoints table (check-run endpoints → status endpoint) |
| `docs/prd/native-ci/providers/github/status-checks.md` | Update FR-4 to describe commit statuses instead of check runs, update status model, add per-operation status documentation |
| `docs/prd/native-ci/framework/configuration.md` | Add `ci.checks.statuses` config block documentation |

## Files That Do NOT Change

| File | Reason |
|------|--------|
| `pkg/ci/internal/provider/types.go` | `Provider` interface unchanged |
| `pkg/ci/providers/github/status.go` | Reads both APIs — already handles commit statuses |
| `pkg/ci/providers/github/client.go` | `Repositories` service already available on `github.Client` |
| `pkg/ci/providers/generic/check.go` | Console-only provider, no API calls |

## Edge Cases

### Concurrent status updates for same context

The Status API is idempotent by `context` string. Multiple calls with the same `context` simply overwrite the previous state. No race conditions or ID conflicts possible.

### UpdateCheckRun without prior CreateCheckRun

No longer a special case. Since `updateCheckRun` just calls `Repositories.CreateStatus`, it works identically whether or not `createCheckRun` was called first. The `createCompletedCheckRun` fallback is unnecessary.

### Description truncation

Truncation uses `utf8.RuneCountInString()` for character-based counting. If the description exceeds 140 characters, it is truncated with `...` suffix. This is unlikely in practice — descriptions are short strings like `"3 to add, 1 to change, 2 to destroy"` or `"3 resources"`.

### Status API rate limits

The Status API has a limit of 1000 statuses per `sha` + `context` combination. Each Atmos operation creates at most 5 statuses per component (1 pending + 1 final + up to 3 per-operation), so this limit is not a concern even for large monorepos.

### Per-operation statuses with zero counts

Per-operation statuses (`/add`, `/change`, `/destroy`) are only created when the corresponding count is greater than zero. If a plan has no resources to destroy, the `atmos/plan/{stack}/{component}/destroy` status simply does not exist. This is by design — the presence of the status is the signal.

### No-changes plan

When a plan detects no changes, only the component-level status is created with `state: success` and `description: "No changes"`. No per-operation statuses are created.

## Verification

1. `go build ./...` — compiles cleanly
2. `go test ./pkg/ci/providers/github/...` — status API tests pass
3. `go test ./pkg/ci/plugins/terraform/...` — handler tests pass with per-operation status assertions
4. `make lint` — no linting issues
5. Manual: `atmos terraform plan vpc -s dev` in GitHub Actions with `permissions: statuses: write` — statuses appear on commit
6. Manual: Verify per-operation statuses appear only when counts > 0
7. Manual: Verify `atmos ci status` still shows Atmos-created statuses (via `getCombinedStatus`)
8. Manual: Verify `context_prefix` from config is used in status context strings
9. Manual: Verify `target_url` links to the GitHub Actions run page
