# Native CI Integration - Hooks & Plugin Architecture

> Related: [CI Detection](./ci-detection.md) | [Interfaces](./interfaces.md) | [Implementation Status](./implementation-status.md) | [Overview](../overview.md)

## Architecture Overview

CI behaviors are integrated via a **Plugin-Executor** architecture, not individual hook command files.

> The implementation uses a **callback-based dispatch** pattern. Plugins own all action logic via `HookHandler` callbacks. The executor is a thin coordinator (~250 lines) that detects the CI platform, resolves the plugin, builds a `HookContext`, and invokes the handler. See [Implementation Status](./implementation-status.md) for details.

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

### Roles (IMPLEMENTED)

| Component | Role |
|-----------|------|
| **Executor** | Thin coordinator (~250 lines): detects platform, resolves plugin + binding, builds `HookContext`, invokes `binding.Handler(hookCtx)`. No action logic. |
| **Plugin** | Business logic owner: 2-method interface (`GetType`, `GetHookBindings`). Subscribes to events via `HookHandler` callbacks, receives `HookContext` with all dependencies, owns all action logic (summary, output, upload, download, check). Controls error severity. |
| **Provider** | I/O layer: `OutputWriter().WriteSummary(content)`, `OutputWriter().WriteOutput(key, value)`, `CreateCheckRun(ctx, opts)`, `UpdateCheckRun(ctx, opts)`. Does not render templates or know about terraform. |

### Plugin-Owns-Behavior Pattern (IMPLEMENTED)

The plugin decides what to do on each hook event. The executor builds a `HookContext` with all dependencies and invokes the handler:

```go
// pkg/ci/plugins/terraform/handlers.go — actual implementation
func (p *Plugin) onAfterPlan(ctx *plugin.HookContext) error {
    result := p.parseOutputWithError(ctx)

    // Summary — warn-only
    if isSummaryEnabled(ctx.Config) {
        renderedSummary, err = p.writeSummary(ctx, result)
        if err != nil {
            log.Warn("CI summary failed", "error", err)
        }
    }

    // Output — warn-only
    if isOutputEnabled(ctx.Config) {
        if err := p.writeOutputs(ctx, result, renderedSummary); err != nil {
            log.Warn("CI output failed", "error", err)
        }
    }

    // Upload — FATAL (downstream apply depends on it)
    if err := p.uploadPlanfile(ctx); err != nil {
        return err
    }

    // Check — warn-only
    if isCheckEnabled(ctx.Config) {
        if err := p.updateCheckRun(ctx, result); err != nil {
            log.Warn("CI check run update failed", "error", err)
        }
    }

    return nil
}
```

The data flow from terraform command to plugin callback:

```
RunCIHooks(event, atmosConfig, info, output, forceCIMode, cmdErr)
  → ci.Execute(ExecuteOptions{...})
    → detectPlatform() → getPluginAndBinding() → buildHookContext()
    → binding.Handler(hookCtx)
```

### Error Severity (IMPLEMENTED)

Error severity is handler-controlled. Each plugin handler decides which operations are fatal vs warn-only:

| Action | Behavior | Rationale |
|--------|----------|-----------|
| Upload | **Fatal** — handler returns error | Apply workflow depends on artifacts |
| Download | **Fatal** — handler returns error | Apply can't proceed without planfile |
| Summary | Warn, continue | Nice-to-have CI feature |
| Output | Warn, continue | Nice-to-have CI feature |
| Status check | Warn, continue | Nice-to-have CI feature |
| PR comment | Warn, continue | Nice-to-have CI feature |

## Lifecycle Events (IMPLEMENTED)

CI behaviors integrate at existing hook points. Each event maps to a handler callback in `pkg/ci/plugins/terraform/handlers.go`:

```go
"before.terraform.plan"  → onBeforePlan()   // createCheckRun (in_progress)
"after.terraform.plan"   → onAfterPlan()    // writeSummary + writeOutputs + uploadPlanfile + updateCheckRun
"before.terraform.apply" → onBeforeApply()  // downloadPlanfile
"after.terraform.apply"  → onAfterApply()   // writeSummary + writeOutputs
```

> Note: `before.terraform.apply` does NOT have ActionCheck (no "Apply in progress" check run). `after.terraform.apply` does NOT have ActionCheck (no check run update after apply). These can be added later by modifying the plugin's `GetHookBindings()`.

> **Wiring gap**: While the terraform plugin defines a `before.terraform.apply` binding for `ActionDownload`, `apply.go` does NOT have a `PreRunE` — so `before.terraform.apply` hooks are **never triggered**. Additionally, `apply.go`'s `PostRunE` fires `after.terraform.apply` but passes empty output (no stdout/stderr capture). The `plan.go` command is fully wired (PreRunE, output capture, error defer); `apply.go` needs the same treatment to complete CI integration.

## Terraform Plugin Hook Bindings (IMPLEMENTED)

Hook bindings use `Handler` callbacks. Each handler owns all action logic for its event:

```go
// pkg/ci/plugins/terraform/plugin.go — actual implementation
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
    return []plugin.HookBinding{
        {Event: "before.terraform.plan",  Handler: p.onBeforePlan},
        {Event: "after.terraform.plan",   Handler: p.onAfterPlan},
        {Event: "before.terraform.apply", Handler: p.onBeforeApply},
        {Event: "after.terraform.apply",  Handler: p.onAfterApply},
    }
}
```

## Per-Plugin Storage (IMPLEMENTED)

Each plugin owns its artifact storage type, registry, and priority list. Store resolution is provided to plugins via a lazy factory in `HookContext`:

1. The executor's `createPlanfileStore()` resolves the active store using the config priority list (`planfilesConfig.Default` → environment detection → local fallback)
2. It's wrapped as a lazy factory closure in `HookContext.CreatePlanfileStore` — only invoked when a handler needs it
3. Plugin handlers call `ctx.CreatePlanfileStore()` to get the store, then use `p.getArtifactKey()` for the key

```go
// Executor provides lazy factory in HookContext
CreatePlanfileStore: func() (any, error) {
    return createPlanfileStore(opts)
},

// Plugin handler uses it when needed
func (p *Plugin) uploadPlanfile(ctx *plugin.HookContext) error {
    storeAny, err := ctx.CreatePlanfileStore()
    store := storeAny.(planfile.Store)
    key := p.getArtifactKey(ctx.Info, ctx.Command)
    return store.Upload(context.Background(), key, reader, metadata)
}
```

## Integration Points (IMPLEMENTED)

| File | Role |
|------|------|
| `pkg/ci/executor.go` | Thin coordinator (~250 lines): `detectPlatform()` → `getPluginAndBinding()` → `buildHookContext()` → `binding.Handler(hookCtx)` |
| `pkg/ci/checkrun_store.go` | `CheckRunStore` interface + `sync.Map`-backed singleton for cross-event check run ID correlation |
| `pkg/ci/internal/plugin/types.go` | Plugin interface (2 methods), `HookHandler` callback type, `HookContext` dependency bag, `CheckRunStore` interface |
| `pkg/ci/plugins/terraform/plugin.go` | Terraform plugin: `GetType()`, `GetHookBindings()` with handler callbacks + private helpers |
| `pkg/ci/plugins/terraform/handlers.go` | All handler implementations: `onBeforePlan`, `onAfterPlan`, `onBeforeApply`, `onAfterApply` + sub-handlers |
| `pkg/hooks/hooks.go` | Calls `RunCIHooks()` which delegates to `ci.Execute()` |
