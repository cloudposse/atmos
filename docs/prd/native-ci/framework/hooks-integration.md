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

| Component | Role |
|-----------|------|
| **Executor** | Thin coordinator: registers providers and plugins, detects current CI provider, resolves artifact storage per plugin, passes `(provider, store, opts)` to plugin callbacks. API is an implementation detail. |
| **Plugin** | Business logic owner: subscribes to events via callback-based `HookBinding`, receives `(provider, store, opts)`, decides what to do — parses output, renders templates, calls provider/store methods, handles errors |
| **Provider** | Dumb I/O layer: `OutputWriter().WriteSummary(content)`, `OutputWriter().WriteOutput(key, value)`, `CreateCheckRun(ctx, opts)`, `UpdateCheckRun(ctx, opts)`. Does not render templates or know about terraform |

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

## Terraform Plugin Hook Bindings

Each plugin wires its own methods as callbacks — the generic Plugin interface has no terraform-specific methods:

```go
func (p *TerraformPlugin) GetHookBindings() []plugin.HookBinding {
    return []plugin.HookBinding{
        {Event: "before.terraform.plan",  Action: p.OnBeforePlan},
        {Event: "after.terraform.plan",   Action: p.OnAfterPlan},
        {Event: "before.terraform.apply", Action: p.OnBeforeApply},
        {Event: "after.terraform.apply",  Action: p.OnAfterApply},
    }
}
```

## Per-Plugin Storage

Each plugin owns its artifact storage type, registry, and priority list. The executor brokers store resolution per plugin:

1. During initialization, each plugin registers its stores with the executor
2. The executor holds per-plugin store registries
3. When binding hooks, the executor resolves the active store using that plugin's config priority list
4. The resolved store is passed to the plugin's hook callback

```go
// Plugin registers its stores during init
executor.RegisterPluginStores("terraform", planfileStoreRegistry)

// Executor resolves store using terraform's priority config
store := executor.ResolveStore("terraform", atmosConfig)

// Plugin receives artifact.Store in hook callback, wraps with adapter
func (p *TerraformPlugin) OnAfterPlan(provider Provider, store artifact.Store, opts ExecuteOptions) error {
    planStore := planfile.NewAdapter(store)
    planStore.Upload(key, planfile, metadata)
}
```

## Integration Points

### Files to Modify

| File | Changes |
|------|---------|
| `pkg/ci/executor.go` | Refactor from god-object to thin coordinator; move action logic into plugins |
| `pkg/ci/internal/plugin/types.go` | Update Plugin interface: remove passive data methods, add callback-based HookAction type |
| `pkg/ci/plugins/terraform/plugin.go` | Implement hook callbacks (OnAfterPlan, OnBeforeApply, etc.) |
| `pkg/hooks/hooks.go` | Unchanged — already calls `RunCIHooks()` which delegates to executor |
