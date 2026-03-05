# Native CI Integration - Hooks & Plugin Architecture

> Related: [CI Detection](./ci-detection.md) | [Interfaces](./interfaces.md) | [Implementation Status](./implementation-status.md) | [Overview](../overview.md)

## Architecture Overview

CI behaviors are integrated via a **Plugin-Executor** architecture, not individual hook command files. The executor is a thin coordinator; plugins own their behavior.

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
| **Executor** | Thin coordinator: registers providers and plugins, detects current CI provider, resolves artifact storage per plugin, binds plugin hooks to lifecycle events, passes `(provider, store)` to plugin callbacks |
| **Plugin** | Business logic owner: subscribes to events, receives `(provider, store, event)`, decides what to do — parses output, renders templates, calls `provider.WriteSummary()`, calls `store.Upload()`, handles errors |
| **Provider** | Dumb I/O layer: `WriteSummary(renderedMarkdown)`, `WriteOutput(key, value)`, `SetStatus(name, status)`. Does not render templates or know about terraform |

### Plugin-Owns-Behavior Pattern

The plugin decides what to do on each hook event. The executor passes it the provider and store:

```go
func (p *TerraformPlugin) OnAfterPlan(provider Provider, store PlanfileStore, event HookEvent) error {
    // Plugin owns all logic
    result := p.ParseOutput(event.Output)
    ctx := p.BuildTemplateContext(event.Info, provider.Context(), event.Output)

    // Plugin renders its own template, provider is just a writer
    rendered := p.RenderTemplate("plan", ctx)
    if err := provider.WriteSummary(rendered); err != nil {
        log.Warn("Failed to write summary", "error", err)
    }

    // Plugin decides what outputs to write
    provider.WriteOutput("has_changes", strconv.FormatBool(result.HasChanges))

    // Plugin decides to upload — fatal on failure
    if err := store.Upload(key, planfile, metadata); err != nil {
        return err  // Fails the command
    }

    // Status check — warn and continue
    if err := provider.SetStatus("atmos/plan", result.Status()); err != nil {
        log.Warn("Failed to set status", "error", err)
    }

    return nil
}
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

## Lifecycle Events

CI behaviors integrate at existing hook points:

```go
BeforeTerraformPlan  = "before.terraform.plan"  // Create check run (in_progress)
AfterTerraformPlan   = "after.terraform.plan"   // Upload planfile, write summary, write outputs, update check run
BeforeTerraformApply = "before.terraform.apply"  // Download planfile, verify plan
AfterTerraformApply  = "after.terraform.apply"   // Write summary, write outputs, export terraform outputs
```

## Terraform Plugin Hook Bindings

```go
func (p *TerraformPlugin) GetHookBindings() []HookBinding {
    return []HookBinding{
        {Event: "before.terraform.plan",  Template: ""},
        {Event: "after.terraform.plan",   Template: "plan"},
        {Event: "before.terraform.apply", Template: ""},
        {Event: "after.terraform.apply",  Template: "apply"},
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

// Plugin receives its resolved store in hook callback
func (p *TerraformPlugin) OnAfterPlan(provider Provider, store PlanfileStore, event HookEvent) error {
    store.Upload(key, planfile, metadata)
}
```

## Integration Points

### Files to Modify

| File | Changes |
|------|---------|
| `pkg/ci/executor.go` | Refactor from god-object to thin coordinator; move action logic into plugins |
| `pkg/ci/internal/plugin/types.go` | Update Plugin interface with hook callback signature |
| `pkg/ci/plugins/terraform/plugin.go` | Implement hook callbacks (OnAfterPlan, OnBeforeApply, etc.) |
| `pkg/hooks/hooks.go` | Unchanged — already calls `RunCIHooks()` which delegates to executor |
