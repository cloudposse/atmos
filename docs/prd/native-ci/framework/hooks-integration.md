# Native CI Integration - Hooks & Plugin Architecture

> Related: [CI Detection](./ci-detection.md) | [Interfaces](./interfaces.md) | [Implementation Status](./implementation-status.md) | [Overview](../overview.md)

## Architecture Overview

CI behaviors are integrated via a **Plugin-Executor** architecture, not individual hook command files.

> **Current vs target architecture**: The current implementation uses an **enum-based action dispatch** pattern — `HookAction` is a string enum (`"summary"`, `"output"`, `"upload"`, `"download"`, `"check"`) and the executor switches on it to call self-contained handler functions. This document describes the **target callback-based** pattern where plugins own all action logic via function callbacks. See [Implementation Status](./implementation-status.md) for current state and refactoring plan.

### Two Independent Hook Systems

Atmos fires two independent hook systems at the same lifecycle points:

1. **User-defined hooks** — Configured in stack YAML under `hooks:` section. Routed through `Hooks.RunAll()`. Supports the `store` command. User-extensible.

2. **CI hooks (automatic)** — Triggered by `RunCIHooks()`, which delegates to `ci.Execute()`. Controlled only through the `ci:` config section (enable/disable features). Not user-configurable at the hook level.

```
Terraform Command (plan/apply)
  │
  ├── PreRunE / PostRunE
  │     ├── hooks.RunAll()     → User-defined hooks (store command, YAML-configured)
  │     └── hooks.RunCIHooks() → ci.Execute() → Plugin hook callbacks
  │
  └── defer (on error)
        └── hooks.RunCIHooks() → ci.Execute() (with CommandError set)
```

### Roles

> **Current implementation** uses enum-based dispatch (executor owns action logic). The target callback-based architecture would move action logic into plugins.

| Component | Current Role | Target Role |
|-----------|-------------|-------------|
| **Executor** | Action dispatcher: detects platform, gets plugin + binding, builds context, dispatches actions by switching on `HookAction` enum. Self-contained action handlers (`executeSummaryAction`, `executeOutputAction`, etc.) | Thin coordinator: passes `(provider, store, opts)` to plugin callbacks |
| **Plugin** | Data provider: 7 methods for parsing output, building context, getting variables, generating artifact keys. Does not execute actions directly | Business logic owner: subscribes to events via callbacks, receives `(provider, store, opts)`, owns all action logic |
| **Provider** | I/O layer: `OutputWriter().WriteSummary(content)`, `OutputWriter().WriteOutput(key, value)`, `CreateCheckRun(ctx, opts)`, `UpdateCheckRun(ctx, opts)`. Does not render templates or know about terraform | Same |

### Plugin-Owns-Behavior Pattern

The plugin decides what to do on each hook event. The executor resolves the current provider and store, then invokes the plugin's callback:

```go
// HookAction signature: func(provider Provider, store artifact.Store, opts ExecuteOptions) error
// The plugin wraps artifact.Store with its adapter to get domain-specific methods.
func (p *TerraformPlugin) OnAfterPlan(provider Provider, store artifact.Store, opts ExecuteOptions) error {
    // Wrap generic artifact.Store with planfile adapter for domain-specific operations
    planStore := planfile.NewAdapter(store)

    // Plugin owns all logic — ParseOutput, BuildTemplateContext are internal helpers
    result := p.ParseOutput(opts.Output)
    ctx := p.BuildTemplateContext(opts.Info, provider.Context(), opts.Output)

    // Plugin renders its own template, provider is just a writer
    rendered := p.RenderTemplate("plan", ctx)
    if err := provider.OutputWriter().WriteSummary(rendered); err != nil {
        log.Warn("Failed to write summary", "error", err)
    }

    // Plugin decides what outputs to write
    provider.OutputWriter().WriteOutput("has_changes", strconv.FormatBool(result.HasChanges))

    // Plugin decides to upload — fatal on failure
    if err := planStore.Upload(key, planfile, metadata); err != nil {
        return err  // Fails the command
    }

    // Status check — warn and continue
    if _, err := provider.CreateCheckRun(ctx, CreateCheckRunOptions{...}); err != nil {
        log.Warn("Failed to create check run", "error", err)
    }

    return nil
}
```

The data flow from terraform command to plugin callback:

```
RunCIHooks(event, atmosConfig, info, output, forceCIMode, cmdErr)
  → ci.Execute(ExecuteOptions{...})
    → executor resolves provider + store
    → executor calls plugin callback: action(provider, store, opts)
```

### Error Severity

The plugin controls error severity per action — no config needed:

| Action | On Failure | Rationale |
|--------|-----------|-----------|
| Upload | **Fatal** — return error | Apply workflow depends on artifacts |
| Download | **Fatal** — return error | Apply can't proceed without planfile |
| Summary | Warn, continue | Nice-to-have CI feature |
| Output | Warn, continue | Nice-to-have CI feature |
| Status check | Warn, continue | Nice-to-have CI feature |
| PR comment | Warn, continue | Nice-to-have CI feature |

## Lifecycle Events (IMPLEMENTED)

CI behaviors integrate at existing hook points. Actual actions per event from `pkg/ci/plugins/terraform/plugin.go`:

```go
BeforeTerraformPlan  = "before.terraform.plan"  // ActionCheck: create check run (in_progress)
AfterTerraformPlan   = "after.terraform.plan"   // ActionSummary + ActionOutput + ActionUpload + ActionCheck (template: "plan")
BeforeTerraformApply = "before.terraform.apply"  // ActionDownload: download planfile from store
AfterTerraformApply  = "after.terraform.apply"   // ActionSummary + ActionOutput (template: "apply")
```

> Note: `before.terraform.apply` does NOT have ActionCheck (no "Apply in progress" check run). `after.terraform.apply` does NOT have ActionCheck (no check run update after apply). These can be added later by modifying the plugin's `GetHookBindings()`.

## Terraform Plugin Hook Bindings (IMPLEMENTED)

> **Current implementation**: Hook bindings use enum-based `Actions` (not callbacks). The executor dispatches actions by switching on the `HookAction` enum. See [Interfaces](./interfaces.md) for the actual `HookBinding` struct.

```go
// pkg/ci/plugins/terraform/plugin.go — actual implementation
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
    return []plugin.HookBinding{
        {Event: "before.terraform.plan",  Actions: []plugin.HookAction{plugin.ActionCheck}},
        {Event: "after.terraform.plan",   Actions: []plugin.HookAction{plugin.ActionSummary, plugin.ActionOutput, plugin.ActionUpload, plugin.ActionCheck}, Template: "plan"},
        {Event: "after.terraform.apply",  Actions: []plugin.HookAction{plugin.ActionSummary, plugin.ActionOutput}, Template: "apply"},
        {Event: "before.terraform.apply", Actions: []plugin.HookAction{plugin.ActionDownload}},
    }
}
```

## Per-Plugin Storage (IMPLEMENTED)

Each plugin owns its artifact storage type, registry, and priority list. The executor brokers store resolution per plugin:

1. The executor's `createPlanfileStore()` resolves the active store using the plugin's config priority list (`planfilesConfig.Default` → environment detection → local fallback)
2. The resolved store is used directly by the executor's `executeUploadAction()` and `executeDownloadAction()` handlers
3. The plugin provides the artifact key via `GetArtifactKey(stack, component)`

> **Current vs target**: In the current enum-based architecture, the executor owns store resolution and upload/download logic. In the target callback-based architecture, the resolved store would be passed to the plugin's hook callback as `(provider, store, opts)`.

```go
// Current implementation: executor resolves store and executes actions directly
func (e *Executor) createPlanfileStore(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (planfile.Store, error) {
    // 1. Check explicit default
    // 2. Try environment-based detection via priority list
    // 3. Fall back to local store
}

func (e *Executor) executeUploadAction(ctx context.Context, ...) error {
    key := plugin.GetArtifactKey(stack, component)
    return store.Upload(ctx, key, reader, metadata)
}
```

## Integration Points

### Current State (IMPLEMENTED)

| File | Role |
|------|------|
| `pkg/ci/executor.go` | Action dispatcher: `executeActions()` switches on `HookAction` enum to call self-contained action handlers |
| `pkg/ci/internal/plugin/types.go` | Plugin interface with 7 passive data methods + HookAction string enum + HookBinding struct |
| `pkg/ci/plugins/terraform/plugin.go` | Terraform plugin implementing all 7 Plugin methods (data provider, no action logic) |
| `pkg/hooks/hooks.go` | Calls `RunCIHooks()` which delegates to `ci.Execute()` |

### Future Refactoring (Not Started)

| File | Planned Changes |
|------|---------|
| `pkg/ci/executor.go` | Refactor from action dispatcher to thin coordinator; move action logic into plugins |
| `pkg/ci/internal/plugin/types.go` | Update Plugin interface: remove passive data methods, add callback-based HookAction type |
| `pkg/ci/plugins/terraform/plugin.go` | Implement hook callbacks (OnAfterPlan, OnBeforeApply, etc.) |
