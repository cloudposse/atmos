# Move CheckRunStore from Plugin to Provider

> Related: [Interfaces](../framework/interfaces.md) | [Hooks Integration](../framework/hooks-integration.md) | [Status Checks](../providers/github/status-checks.md)

## Problem Statement

`CheckRunStore` leaks provider-specific concerns (GitHub check run ID correlation) into the plugin layer. The plugin handler currently:

1. Calls `provider.CreateCheckRun()` → gets back an ID
2. Stores the ID in `ctx.CheckRunStore.Store(key, id)`
3. Later, calls `ctx.CheckRunStore.LoadAndDelete(key)` → gets the ID back
4. Calls `provider.UpdateCheckRun(id, ...)` to update the check run

This is wrong because:

- **GitHub provider** knows how to find check runs by name — it doesn't need the plugin to remember IDs for it. GitHub API supports `GET /repos/{owner}/{repo}/commits/{ref}/check-runs?check_name=...` or the provider can maintain its own internal ID map.
- **Generic provider** doesn't have real IDs — it uses `atomic.Int64` to generate synthetic IDs and just prints to console. It doesn't need ID correlation at all.
- **Future providers** (GitLab, etc.) may use completely different correlation mechanisms (commit status API uses `context` string, not numeric IDs).

The plugin shouldn't know about check run IDs. It should just say "create a check run with this name" and "update the check run with this name".

## Desired State

### Provider Interface Changes

Replace ID-based `UpdateCheckRun` with name-based lookup. The provider handles correlation internally:

```go
// Provider interface — updated
type Provider interface {
    Name() string
    Detect() bool
    Context() (*Context, error)
    GetStatus(ctx context.Context, opts StatusOptions) (*Status, error)
    CreateCheckRun(ctx context.Context, opts *CreateCheckRunOptions) (*CheckRun, error)
    UpdateCheckRun(ctx context.Context, opts *UpdateCheckRunOptions) (*CheckRun, error)
    OutputWriter() OutputWriter
}
```

### UpdateCheckRunOptions Changes

Remove `CheckRunID` field. Add context fields the provider needs to look up the check run:

```go
type UpdateCheckRunOptions struct {
    // Owner is the repository owner.
    Owner string

    // Repo is the repository name.
    Repo string

    // SHA is the commit SHA (for lookup scope).
    SHA string

    // Name is the check run name (used as correlation key).
    Name string

    // Status is the new status.
    Status CheckRunState

    // Conclusion is the final conclusion (required when status is "completed").
    Conclusion string

    // Title is the output title.
    Title string

    // Summary is an updated markdown summary.
    Summary string

    // CompletedAt is when the check run completed.
    CompletedAt *time.Time
}
```

### Provider Implementations

**GitHub provider** — Two options (choose one):

1. **Internal ID map** (simpler): Maintain a `sync.Map` inside the provider struct. `CreateCheckRun()` stores `name→id`, `UpdateCheckRun()` looks it up. Falls back to API lookup if not found.

2. **API lookup** (stateless): `UpdateCheckRun()` calls `GET /repos/{owner}/{repo}/commits/{ref}/check-runs?check_name={name}` to find the check run ID, then updates it. No internal state needed.

Option 1 is recommended — it avoids an extra API call and the provider struct already has state (the `client` field).

**Generic provider** — No changes needed. It already uses `opts.Name` for logging. It can ignore the ID entirely since it just prints to console.

### Plugin Handler Changes

Handlers simplify — no more `Store()`/`LoadAndDelete()` calls:

```go
// Before (current)
func (p *Plugin) createCheckRun(ctx *plugin.HookContext) error {
    checkRun, err := ctx.Provider.CreateCheckRun(context.Background(), opts)
    key := buildCheckRunKey(ctx.Info, ctx.Command)
    ctx.CheckRunStore.Store(key, checkRun.ID)
    return nil
}

func (p *Plugin) updateCheckRun(ctx *plugin.HookContext, result *plugin.OutputResult) error {
    key := buildCheckRunKey(ctx.Info, ctx.Command)
    checkRunID, ok := ctx.CheckRunStore.LoadAndDelete(key)
    if !ok {
        return p.createCompletedCheckRun(ctx, result, name)
    }
    _, err := ctx.Provider.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{
        CheckRunID: checkRunID,
        ...
    })
    return err
}

// After (proposed)
func (p *Plugin) createCheckRun(ctx *plugin.HookContext) error {
    _, err := ctx.Provider.CreateCheckRun(context.Background(), opts)
    return err
}

func (p *Plugin) updateCheckRun(ctx *plugin.HookContext, result *plugin.OutputResult) error {
    _, err := ctx.Provider.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{
        Owner: ctx.CICtx.RepoOwner,
        Repo:  ctx.CICtx.RepoName,
        SHA:   ctx.CICtx.SHA,
        Name:  name,
        ...
    })
    return err
}
```

### HookContext Changes

Remove `CheckRunStore` field from `HookContext`:

```go
type HookContext struct {
    Event              string
    Command            string
    EventPrefix        string
    Config             *schema.AtmosConfiguration
    Info               *schema.ConfigAndStacksInfo
    Output             string
    CommandError       error
    Provider           provider.Provider
    CICtx              *provider.Context
    TemplateLoader     *templates.Loader
    // CheckRunStore removed — provider handles correlation internally
    CreatePlanfileStore func() (any, error)
}
```

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/ci/internal/provider/check.go` | Remove `CheckRunID` from `UpdateCheckRunOptions`, add `SHA` field |
| `pkg/ci/internal/provider/types.go` | No interface signature changes needed |
| `pkg/ci/internal/plugin/types.go` | Remove `CheckRunStore` interface and field from `HookContext` |
| `pkg/ci/providers/github/checks.go` | Add internal `sync.Map` for name→ID mapping. `CreateCheckRun` stores ID, `UpdateCheckRun` looks up by name |
| `pkg/ci/providers/generic/check.go` | Remove `CheckRunID` usage from `UpdateCheckRun` (use `Name` only) |
| `pkg/ci/plugins/terraform/handlers.go` | Remove `CheckRunStore.Store()`/`LoadAndDelete()` calls, remove `createCompletedCheckRun()`, remove `buildCheckRunKey()` |
| `pkg/ci/plugins/terraform/handlers_test.go` | Remove `CheckRunStore` from test setup, simplify check run tests |
| `pkg/ci/executor.go` | Remove `defaultCheckRunStore` from `buildHookContext()` |
| `pkg/ci/mock_plugin_test.go` | Remove `CheckRunStore` from mock setup |

## Files to Delete

| File | Reason |
|------|--------|
| `pkg/ci/checkrun_store.go` | No longer needed — correlation moved to provider |
| `pkg/ci/checkrun_store_test.go` | Tests for deleted code |

## Edge Cases

### What if `CreateCheckRun` was never called?

The `UpdateCheckRun` handler may fire without a prior `CreateCheckRun` (e.g., if `before.terraform.plan` was not triggered, or check creation failed silently). The provider should handle this gracefully:

- **GitHub provider (internal map)**: Name not found in map → create a new completed check run instead of updating (same behavior as current `createCompletedCheckRun()` fallback, but inside the provider).
- **GitHub provider (API lookup)**: Name not found via API → create a new completed check run.
- **Generic provider**: Just print the status — no lookup needed.

### Concurrent executions

The `sync.Map` inside the GitHub provider handles concurrent access (same as the current `syncMapCheckRunStore`). Each provider instance is scoped to a process, and check run names include stack/component, so there are no collisions.

## Verification

1. `go build ./...` — compiles cleanly
2. `go test ./pkg/ci/...` — all tests pass
3. `go test ./pkg/ci/providers/github/...` — check run correlation works
4. `go test ./pkg/ci/plugins/terraform/...` — handler tests pass without `CheckRunStore`
5. `make lint` — no linting issues
6. Manual: `atmos terraform plan vpc -s dev --ci` with `GITHUB_ACTIONS=true` — check run created and updated correctly
