# Native CI Integration - Open Questions

> Related: [Overview](./overview.md) | [Framework](./framework/) | [Providers](./providers/) | [Terraform Plugin](./terraform-plugin/)

This document captures ambiguities found during PRD review that need decisions before implementation can proceed. Each section presents what the PRDs left unclear, what the current code does (if relevant), and asks a specific question.

**How to use**: Answer each question by writing your decision under the `### Decision` heading. Once all questions are answered, this document becomes the authoritative reference for implementers.

---

## 1. Executor architecture needs to be inverted — Plugin should own behavior

### Context

The current executor (`pkg/ci/executor.go`) is a god object that orchestrates all CI actions:

```go
// Current: Executor knows everything
func Execute(opts ExecuteOptions) error {
    plugin, binding := getPluginAndBinding(opts)
    actCtx := buildActionContext(opts, platform, plugin, binding)
    executeActions(actCtx, binding.Actions) // Executor calls each action
}

// Executor implements every action:
func executeSummaryAction(ctx) { ... }   // Executor renders template
func executeOutputAction(ctx) { ... }    // Executor writes outputs
func executeUploadAction(ctx) { ... }    // Executor uploads planfile
func executeCheckAction(ctx) { ... }     // Executor manages check runs
```

This creates **double dependency inversion** — the executor depends on plugin methods (`ParseOutput`, `BuildTemplateContext`, `GetOutputVariables`) to get data, then uses provider methods to write it. The plugin is passive; it just returns data but never acts.

### Decision: Invert to Plugin-Owns-Behavior

The executor should be a **thin coordinator** that:
1. Registers CI providers and CI plugins
2. Detects and holds the current CI provider
3. Resolves the current artifact storage
4. Binds plugin hooks to lifecycle events
5. Passes `(provider, store)` to plugin hook callbacks

The **plugin should own its behavior** — it decides what to do on each hook:

```go
// New: Plugin is active, executor is thin
func (p *TerraformPlugin) OnAfterPlan(provider Provider, store Store, event HookEvent) error {
    // Plugin owns all logic
    result := p.ParseOutput(event.Output)
    ctx := p.BuildTemplateContext(event.Info, provider.Context(), event.Output)

    // Plugin renders its own template, provider is just a writer
    rendered := p.RenderTemplate("plan", ctx)
    provider.WriteSummary(rendered)

    // Plugin decides what outputs to write
    provider.WriteOutput("has_changes", strconv.FormatBool(result.HasChanges))
    provider.SetStatus("atmos/plan", result.Status())

    // Plugin decides to upload
    store.Upload(key, planfile, metadata)

    return nil
}
```

**Key design points:**

- **Provider is a dumb I/O layer** — `WriteSummary(renderedMarkdown)`, `WriteOutput(key, value)`, `SetStatus(name, status)`. Provider doesn't render templates or know about terraform.
- **Template rendering stays in the plugin** — plugin already has `GetDefaultTemplates()` and `BuildTemplateContext()`. It renders templates itself, passes rendered output to provider.
- **Error severity is plugin's decision** — the terraform plugin knows that upload failure is more critical than summary failure. It can choose to continue or return an error per action.
- **Adding new actions (PR comments) means adding code to the plugin**, not touching the executor.

**Dependency flow:**

```
Executor (thin coordinator)
  ├── Owns: provider registry, plugin registry, store resolution
  ├── Does: detect provider, select store, bind hooks
  └── Passes: (provider, store) → plugin hook callbacks

Plugin (business logic owner)
  ├── Subscribes: "after.terraform.plan", "before.terraform.apply", etc.
  ├── Receives: (provider, store, event context)
  └── Acts: parse output, render templates, call provider.WriteSummary(),
            call store.Upload(), call provider.SetStatus()
```

**Impact on PRDs:**

- `hooks-integration.md` — Rewrite to document this inverted architecture. Remove references to 5 deprecated hook command files (`ci_upload.go`, etc.).
- `implementation-status.md` — Update Phase 3 to reflect plugin-owns-behavior pattern. Mark deprecated hook command files as "Superseded."
- `interfaces.md` — Update Plugin interface to show hook callback signature instead of passive data methods.

---

## 2. Planfile upload/download failure is fatal in CI

### Decision

**Upload and download failures fail the command. All other CI actions log warnings and continue.**

This applies whenever CI mode is active — whether auto-detected or set via `--ci` flag. The rationale: if upload fails, the downstream apply job won't find the planfile. Fail fast with a clear error rather than let the user discover it later in a different workflow step.

The plugin owns this decision (per Q1 architecture). Error severity is hardcoded per action — no config needed:

| Action | On Failure | Rationale |
|--------|-----------|-----------|
| Upload | **Fatal** — return error | Apply workflow depends on it |
| Download | **Fatal** — return error | Apply can't proceed without planfile |
| Summary | Warn, continue | Nice-to-have CI feature |
| Output | Warn, continue | Nice-to-have CI feature |
| Status check | Warn, continue | Nice-to-have CI feature |
| PR comment | Warn, continue | Nice-to-have CI feature |

```go
func (p *TerraformPlugin) OnAfterPlan(provider Provider, store Store, event HookEvent) error {
    // Summary — warn and continue
    if err := provider.WriteSummary(rendered); err != nil {
        log.Warn("Failed to write summary", "error", err)
    }

    // Upload — fatal (workflow depends on artifacts)
    if err := store.Upload(key, planfile, metadata); err != nil {
        return err
    }

    // Status check — warn and continue
    if err := provider.SetStatus(...); err != nil {
        log.Warn("Failed to set status", "error", err)
    }

    return nil
}
```

**Impact**: The current executor's `executeActions()` must be refactored — it currently swallows all errors. In the new plugin-owns-behavior model (Q1), this is handled naturally since the plugin controls its own error flow.

---

## 3–5. PR Comments — Deferred

### Decision

PR comment design (marker identification, comment granularity, template strategy) is **deferred** until we have more information. Preliminary direction:

- **Marker-based upsert** using HTML comments (`<!-- atmos:ci:plan:{component}:{stack} -->`) — tfcmt pattern
- **One comment per component+stack** as the starting point

These are directional, not final. A dedicated PR comments PRD should be written before implementation begins.

---

## 6. CI output variables use last-writer-wins, no prefix

### Decision

**No component/stack prefix.** Variable names stay simple (`has_changes`, `plan_summary`). If two components run in the same job step, the last one's values win.

Users who need per-component isolation should use matrix strategy (one component per job) — which is the recommended workflow pattern via `describe affected --format=matrix`.

---

## 7. `--verify-plan` downloads stored planfile, generates fresh plan, compares with plan-diff

### Decision

The verification workflow:

1. Download the stored planfile from planfile storage to a **temp file** (not the normal planfile path)
2. Run `terraform plan` to generate a **fresh planfile** at the normal path
3. Compare the two binary planfiles using the existing `plan-diff` implementation (`internal/exec/terraform_plan_diff*.go`)
4. If plan-diff detects meaningful differences → fail with a clear error showing what drifted
5. If no differences → proceed with apply using the fresh planfile

**Performance**: Adds one full `terraform plan` (~30-60s). This is acceptable — verification is opt-in and safety matters more than speed.

**Opt-in**: `--verify-plan` flag on the deploy/apply command. Not mandatory, not auto-enabled in CI.

---

## 8. Matrix format is fixed: component + stack only

### Decision

**Fixed schema.** `--format=matrix` outputs only `{"component":"...", "stack":"..."}` per entry. Users who need custom fields should use `--format=json` + `jq` to build their matrix.

---

## 9. `ci.enabled: false` is a hard kill switch — overrides everything

### Decision

`ci.enabled` is the **master switch** for native CI support. When `false`, all CI features are disabled — auto-detection is skipped, `--ci` flag is ignored.

| `ci.enabled` config | `--ci` flag | CI env detected | Result |
|--------------------:|:-----------:|:---------------:|--------|
| false | any | any | **CI disabled** — config is a hard kill switch |
| true | true | any | CI enabled (detected provider or generic fallback) |
| true | false | yes | CI enabled (auto-detected provider) |
| true | false | no | CI disabled (no provider available) |
| unset (default) | true | any | CI enabled (default is enabled) |
| unset (default) | false | yes | CI enabled (auto-detected) |
| unset (default) | false | no | CI disabled |

**Impact**: The current implementation has this wrong — `--ci` bypasses `ci.enabled: false`. The `RunCIHooks()` check needs to be:

```go
// ci.enabled: false is a hard kill switch
if atmosConfig != nil && !atmosConfig.CI.Enabled {
    return nil  // Skip CI regardless of --ci flag
}
```

---

## 10. Per-plugin storage with priority-based selection via executor

### Decision

**Each plugin owns its artifact storage type, registry, and priority list. The executor brokers store resolution per plugin.**

There is no global artifact storage that all plugins share. Instead:

- `artifact.Store` is a **base interface** defining the common contract: keyed by SHA/Component/Stack, query mechanism, common metadata (timestamps, branch, PR number, etc.), and CRUD operations (Upload/Download/List/Delete/Exists/GetMetadata).
- Each plugin **extends** `artifact.Store` with domain-specific metadata and behavior:
  - `planfile.Store` — terraform-specific (HasChanges, Additions, TerraformVersion, planfile bundling)
  - Future `helmfile.ChartStore` — helmfile-specific metadata and bundling
- Each plugin **owns its own store registry, backends, and priority config**:
  - Terraform: `components.terraform.planfiles.stores` + `components.terraform.planfiles.priority`
  - Future helmfile: `components.helmfile.charts.stores` + `components.helmfile.charts.priority`

**Executor's role as store broker:**

1. During initialization, each plugin registers its stores with the executor
2. The executor holds per-plugin store registries (not one global registry)
3. When binding hooks, the executor resolves the active store for each plugin using that plugin's config priority list
4. The resolved store is passed to the plugin's hook callback

```go
// Plugin registers its stores during init
executor.RegisterPluginStores("terraform", planfileStoreRegistry)

// Executor resolves store using terraform's priority list
store := executor.ResolveStore("terraform", atmosConfig)

// Plugin receives its resolved store in hook callback
func (p *TerraformPlugin) OnAfterPlan(provider Provider, store PlanfileStore, event HookEvent) error {
    store.Upload(key, planfile, metadata)
}
```

**Impact on current code:**

- `createPlanfileStore()` in executor — replace with per-plugin store resolution using the priority list from config
- `artifact.SelectStore()` + `EnvironmentChecker` — wire into the executor's per-plugin resolution
- Remove env-var fallback logic from executor (`ATMOS_PLANFILE_BUCKET`, `GITHUB_ACTIONS`) — store selection should come from config priority, not hardcoded env checks
- `artifact.Store` base interface — keep as the common foundation that plugin stores extend

---

## 11. Wire `context_prefix` from config into `FormatCheckRunName()`

### Decision

**Yes, wire it now.** `FormatCheckRunName()` should read `ci.checks.context_prefix` from config instead of using a hardcoded `"atmos"` prefix. Default remains `"atmos"` when unset.

Format: `{context_prefix}/{command} — {component} in {stack}` (e.g., `atmos/plan — vpc in plat-ue2-dev`).

---

## 12. GitHub Artifacts: start with simple SHA lookup, encapsulate for future traversal

### Decision

1. **Start with simple SHA-based lookup.** But encapsulate the lookup logic behind a method/interface so we can later add merge-commit traversal (walking PR commit history) and squash-merge support (lookup by PR number) without changing callers.

2. **Cross-workflow artifact access: yes.** The GitHub Artifacts store must support downloading artifacts from other workflow runs (e.g., apply workflow downloading planfiles uploaded by the plan workflow). This is the primary use case — plan and apply are separate jobs/workflows.

Future enhancement (not Phase 4): merge-commit traversal and PR-number-based lookup as described in `artifact-storage.md`.

---

## 13. Accept all store types in config, fail at runtime

### Decision

**Accept all types in config validation.** Unimplemented backends (`azure-blob`, `gcs`) fail at runtime only when actually selected via `--store` or priority. Users can pre-configure future backends without breaking current functionality.

---

## 14. Mocks and golden files for Phases 3-5 testing

### Decision

**Mocks + golden files. No real API calls.**

- **Hook integration**: Mock plugin registry and provider to test hooks fire at correct lifecycle points. Test error propagation (command fails → hooks fire with `CommandError`).
- **PR comments**: Mock GitHub API for upsert tests (list → find marker → create/update).
- **Templates**: Golden file tests for all default templates (plan, apply, with changes, no changes, errors, with outputs).
- **Describe affected matrix**: Table-driven tests for JSON generation. Test `--output-file` writes correct `key=value` format.

Coverage target: 80%.

---

## 15. Rewrite `hooks-integration.md` to reflect Plugin-Executor architecture

### Decision

**Yes, rewrite `hooks-integration.md`.** Remove references to the 5 deprecated hook command files and document the actual Plugin-Executor architecture (as decided in Q1). Also update `implementation-status.md` to mark those files as "Superseded" and update Phase 3 description accordingly.

---

## Summary of Decisions

| # | Decision | Impact |
|---|----------|--------|
| 1 | Invert executor — plugin owns behavior, executor is thin coordinator | Refactor `pkg/ci/executor.go`, update Plugin interface |
| 2 | Upload/download fatal, all other CI actions warn-and-continue | Plugin controls error flow |
| 3–5 | PR comments deferred — preliminary: HTML markers, per-component | Needs dedicated PRD later |
| 6 | Last-writer-wins for CI outputs, no prefix | No change needed |
| 7 | `--verify-plan`: download stored planfile to tmp, fresh plan, compare with plan-diff | New verification flow in apply |
| 8 | Matrix format fixed: component + stack only | No change needed |
| 9 | `ci.enabled: false` is hard kill switch — overrides `--ci` flag | Fix current implementation |
| 10 | Per-plugin storage with priority-based selection via executor | Refactor store resolution |
| 11 | Wire `context_prefix` from config into `FormatCheckRunName()` | Small fix |
| 12 | Start with simple SHA lookup, encapsulate for future traversal; cross-workflow: yes | GitHub Artifacts Phase 4 |
| 13 | Accept all store types in config, fail at runtime | No change needed |
| 14 | Mocks + golden files, no real API calls, 80% coverage | Testing standard |
| 15 | Rewrite `hooks-integration.md` to reflect Plugin-Executor architecture | PRD update |
