# PRD: Component Rename Workspace Stability

**Version:** 1.0
**Last Updated:** 2026-04-14
**Issue:** [#2244](https://github.com/cloudposse/atmos/issues/2244)

---

## Executive Summary

Renaming an Atmos component (changing its YAML key) silently changes the
Terraform workspace name, causing Terraform to treat the renamed component as
a brand-new workspace. Existing infrastructure in the old workspace is
effectively "lost" from Terraform's perspective: resources will be recreated
in the new workspace while the old workspace remains orphaned with stale state.

This PRD analyses the three approaches proposed in the issue and the maintainer
discussion, evaluates trade-offs, and recommends a two-phase solution:

1. **Phase 1 (immediate):** Extend the existing `metadata.name` mechanism — already
   used to stabilise backend key prefixes — to also control the Terraform
   workspace name, giving users a simple, declarative way to pin workspace
   identity independently of the YAML key.
2. **Phase 2 (follow-up):** Introduce an `atmos rename component` interactive
   migration command that automates workspace rename across all stacks where a
   component is deployed.

---

## Problem Statement

### How Workspace Names Are Computed Today

`BuildTerraformWorkspace` in `internal/exec/stack_utils.go` builds the workspace
name using the following priority chain:

| Priority | Source | Example |
|----------|--------|---------|
| 1 | `metadata.terraform_workspace_template` | Go template string |
| 2 | `metadata.terraform_workspace_pattern` | Token-substitution pattern |
| 3 | `metadata.terraform_workspace` | Static string |
| 4 | Stack prefix only (no base component) | `ue1-dev` |
| 5 | `{stack_prefix}-{component_name}` | `ue1-dev-rds-hello-world-service` |

For priority 5, `component_name` is the raw Atmos YAML key with `/` replaced by
`-`. This means the workspace name is **tightly coupled to the YAML key**.

### The Rename Problem

When a component is renamed in the YAML stack manifest:

```diff
 components:
   terraform:
-    rds/hello-world-service:
+    hello-world-service/rds:
```

The workspace name changes from `ue1-dev-rds-hello-world-service` to
`ue1-dev-hello-world-service-rds`. Terraform finds no resources in the new
workspace and plans to create all of them from scratch, while the old workspace
retains orphaned state.

### Why the Current Workaround Is Insufficient

The current escape hatch is to set a static workspace name:

```yaml
hello-world-service/rds:
  metadata:
    terraform_workspace: "ue1-dev-rds-hello-world-service"
```

This works but has several problems:

1. **Must be set proactively.** Users who rename _without_ setting
   `terraform_workspace` first will trigger a plan with full resource
   recreation. There is no warning.
2. **Hard-codes the stack name.** The workspace value embeds `ue1-dev`, so the
   same component definition cannot be reused across stacks via inheritance.
3. **Disconnected from identity.** `metadata.name` is already the recommended
   field for expressing stable component identity (see `workspace-key-prefixes.md`),
   yet it has no effect on the workspace _name_.
4. **Scales poorly.** In a large mono-repo a component may be deployed in dozens
   of stacks. Setting a static workspace override in every stack manifest is
   error-prone.

---

## Analysed Options

### Option A — Extend `metadata.name` to Stabilise Workspace Names (Recommended for Phase 1)

#### Description

`metadata.name` was introduced to give a component a stable logical identity
for backend key prefix generation (see `workspace-key-prefixes.md`). Extend
`BuildTerraformWorkspace` to use `metadata.name` as the component segment of
the workspace name when it is set.

**New logic (pseudocode):**

```go
// Resolve the "component identity" used in the workspace name.
// Priority: metadata.name > Atmos component YAML key.
componentIdentity := component  // YAML key (current behavior)
if name, ok := componentMetadata["name"].(string); ok && name != "" {
    componentIdentity = name
}

// Build workspace (existing priority chain is unchanged above this).
if configAndStacksInfo.Context.BaseComponent == "" {
    workspace = contextPrefix
} else {
    workspace = fmt.Sprintf("%s-%s", contextPrefix, componentIdentity)
}
return strings.Replace(workspace, "/", "-", -1), nil
```

#### Before / After

```yaml
# BEFORE (rename causes workspace change):
rds/hello-world-service:          # Workspace: ue1-dev-rds-hello-world-service
  vars:
    name: hello-world-service-rds
```

```yaml
# AFTER rename, without metadata.name — workspace BREAKS:
hello-world-service/rds:         # Workspace: ue1-dev-hello-world-service-rds ← CHANGED
  vars:
    name: hello-world-service-rds
```

```yaml
# AFTER rename, with metadata.name — workspace is STABLE:
hello-world-service/rds:         # Workspace: ue1-dev-rds-hello-world-service ← SAME
  metadata:
    name: rds/hello-world-service  # Pins logical identity
  vars:
    name: hello-world-service-rds
```

#### Trade-offs

| | |
|-|--|
| ✅ Backwards compatible | Only active when `metadata.name` is set. Existing configs are unchanged. |
| ✅ No new state management | Identity stored in YAML, no external registry required. |
| ✅ Consistent | Aligns workspace name with workspace_key_prefix identity (same field). |
| ✅ Inheritable | `metadata.name` can be defined once in a base/abstract component and inherited everywhere. |
| ✅ DRY | Rename once in YAML; all stacks that inherit pick up the stable identity automatically. |
| ⚠️ Requires proactive setup | Users must set `metadata.name` _before_ renaming, or perform a manual workspace migration. |
| ⚠️ Naming discipline | The value of `metadata.name` becomes an immutable commitment. Changing it causes the same workspace churn the feature aims to prevent. |

---

### Option B — `atmos rename component` Migration Command (Phase 2)

#### Description

An interactive CLI command that:

1. Finds every stack where the component is deployed.
2. Prompts the user to confirm which stacks to migrate.
3. For each selected stack, renames the Terraform workspace (selects old workspace → renames via `terraform workspace new` + state pull/push, or `terraform workspace delete` of old).
4. Updates YAML manifests to reflect the new name.

```
> atmos rename component rds/hello-world-service hello-world-service/rds

Found 2 stacks where rds/hello-world-service is deployed:
  1. ue1-dev
  2. ue2-dev

Rename component and migrate workspaces? [y/N]: y

  [ue1-dev] Migrating workspace ue1-dev-rds-hello-world-service
             → ue1-dev-hello-world-service-rds ... ✓
  [ue2-dev] Migrating workspace ue2-dev-rds-hello-world-service
             → ue2-dev-hello-world-service-rds ... ✓

Updated 2 stack manifests. Run `atmos terraform plan` to verify.
```

#### Trade-offs

| | |
|-|--|
| ✅ Full automation | No manual workspace migration step. |
| ✅ Discoverable | Users learn about the safe rename path via the command itself. |
| ✅ Self-documenting | The command makes the consequences of a rename explicit. |
| ⚠️ Complex implementation | Requires Terraform execution, workspace state pull/push, error recovery, dry-run mode. |
| ⚠️ Multi-stack serialisation | Workspaces in separate stacks may use different backends requiring distinct credentials. |
| ⚠️ Scope of YAML mutation | Programmatic YAML rewrites risk destroying formatting, comments, and anchors. |
| ⚠️ Not atomic | A partial failure mid-migration leaves some stacks migrated and others not. |

---

### Option C — GUID-Based Immutable Workspace IDs

#### Description

Generate a UUID when a component is first provisioned. Store it in the YAML
manifest (auto-written by Atmos). Use the UUID as the workspace name or a
workspace name suffix.

#### Trade-offs

| | |
|-|--|
| ✅ Truly immutable | Workspace is forever decoupled from the YAML key. |
| ⚠️ Requires Atmos to write YAML | Auto-rewriting manifests is fragile and unexpected. |
| ⚠️ Requires separate state | If stored outside YAML, a new state file or database is needed. |
| ⚠️ Opaque | UUIDs are meaningless to humans; debugging workspace issues becomes harder. |
| ⚠️ Breaking change for existing components | Existing workspaces would need migration to the UUID-keyed names. |
| ⚠️ Git noise | Every first-time provision of a new component writes to YAML, polluting commits. |

**Conclusion:** Option C is the least viable. It introduces significant
operational and Git workflow complexity with no meaningful benefit over Option A.
@osterman's comment identified the same concern: "this would require either
a) separate atmos state, b) atmos dynamically updating the YAML to persist
that state."

---

## Recommended Approach

### Phase 1 — `metadata.name` Controls Workspace Name

Extend `BuildTerraformWorkspace` to honour `metadata.name` as the component
segment of the workspace name. The workspace name derivation priority becomes:

| Priority | Source | Notes |
|----------|--------|-------|
| 1 | `metadata.terraform_workspace_template` | Unchanged |
| 2 | `metadata.terraform_workspace_pattern` | Unchanged |
| 3 | `metadata.terraform_workspace` | Unchanged |
| 4 | Stack prefix only (no base component) | Unchanged |
| 5 | `{stack_prefix}-{metadata.name}` | **New** — when `metadata.name` is set |
| 6 | `{stack_prefix}-{component_yaml_key}` | Current fallback (unchanged) |

This keeps all existing behaviour intact while offering a clean opt-in path.

### Phase 2 — `atmos rename component` Command

Implement the migration command as a follow-up once Phase 1 is stable. Phase 2
is deliberately out of scope for the initial implementation to keep the change
minimal and reviewable.

---

## Implementation

### Phase 1 — Code Changes

#### `internal/exec/stack_utils.go`

Update `BuildTerraformWorkspace` to extract `metadata.name` and use it in the
fallback workspace construction:

```go
func BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
    defer perf.Track(atmosConfig, "exec.BuildTerraformWorkspace")()

    if !isWorkspacesEnabled(atmosConfig, &configAndStacksInfo) {
        return cfg.TerraformDefaultWorkspace, nil
    }

    // ... existing contextPrefix resolution unchanged ...

    var workspace string
    componentMetadata := configAndStacksInfo.ComponentMetadataSection

    // Priority 1-3: explicit workspace overrides (unchanged)
    if terraformWorkspaceTemplate, ok := componentMetadata["terraform_workspace_template"].(string); ok {
        // ... unchanged ...
    } else if terraformWorkspacePattern, ok := componentMetadata["terraform_workspace_pattern"].(string); ok {
        // ... unchanged ...
    } else if terraformWorkspace, ok := componentMetadata["terraform_workspace"].(string); ok {
        workspace = terraformWorkspace
    } else if configAndStacksInfo.Context.BaseComponent == "" {
        // Priority 4: stack prefix only (no base component, unchanged)
        workspace = contextPrefix
    } else {
        // Priority 5/6: component identity.
        // Use metadata.name if set; otherwise fall back to the YAML key.
        componentIdentity := configAndStacksInfo.Context.Component
        if name, ok := componentMetadata["name"].(string); ok && name != "" {
            componentIdentity = name
        }
        workspace = fmt.Sprintf("%s-%s", contextPrefix, componentIdentity)
    }

    return strings.Replace(workspace, "/", "-", -1), nil
}
```

#### `pkg/schema/schema.go`

`metadata.name` already exists on `ComponentMetadata` (used for workspace key
prefix). No schema change is required — the same field now also influences the
workspace name.

#### JSON Schema

No change to `pkg/datafetcher/schema/` is required because `metadata.name` is
already documented in the schema (added by `workspace-key-prefixes.md`).

### Phase 1 — Test Plan

**Unit tests** (`internal/exec/stack_utils_test.go`):

| Scenario | `metadata.name` | YAML key | Expected workspace |
|----------|-----------------|----------|--------------------|
| Name set, component renamed | `rds/hello-world-service` | `hello-world-service/rds` | `ue1-dev-rds-hello-world-service` |
| Name not set | — | `rds/hello-world-service` | `ue1-dev-rds-hello-world-service` |
| Name not set, component renamed | — | `hello-world-service/rds` | `ue1-dev-hello-world-service-rds` |
| Static override takes precedence | `rds/hello-world-service` | any | value from `metadata.terraform_workspace` |
| Template override takes precedence | `rds/hello-world-service` | any | result of `metadata.terraform_workspace_template` |
| No base component | `rds/hello-world-service` | any | stack prefix only (unchanged) |

### Phase 2 — Scope (Future Issue)

The `atmos rename component` command will be tracked in a separate issue and
PRD. It is mentioned here for completeness. Key design points to address in
that PRD:

- Dry-run mode (`--dry-run`).
- Per-stack filtering (`--stack`).
- Backend-aware workspace migration (S3, GCS, Azure, HTTP).
- Transactional safety (rollback on partial failure).
- YAML rewrite strategy (preserve comments/anchors).
- Credential handling for multi-account environments.

---

## Migration Guide

### New Projects

Add `metadata.name` to every component from the start. The name is the stable
identity of the component regardless of how the YAML key evolves:

```yaml
components:
  terraform:
    rds/hello-world-service:
      metadata:
        name: rds/hello-world-service   # Stable identity
        component: rds                   # Terraform source directory
```

### Existing Projects — Safe Rename Procedure

Follow these steps to rename a component without changing its workspace:

**Step 1** — Add `metadata.name` with the _current_ component name:

```diff
 components:
   terraform:
     rds/hello-world-service:
+      metadata:
+        name: rds/hello-world-service
```

**Step 2** — Commit, apply, and verify. Run `atmos terraform plan` to confirm
zero changes (the workspace name is identical to before `metadata.name` was
set). This step is critical: it proves the name is stable.

**Step 3** — Rename the YAML key:

```diff
 components:
   terraform:
-    rds/hello-world-service:
+    hello-world-service/rds:
       metadata:
         name: rds/hello-world-service   # Unchanged — workspace stays stable
```

**Step 4** — Commit and run `atmos terraform plan` again. Confirm zero changes.

### Existing Projects — Emergency Recovery (Rename Already Happened)

If a component was already renamed without `metadata.name`, the old workspace
is orphaned and a new workspace was created. To recover:

**Option 1 — Revert and follow safe procedure.** Revert the YAML rename,
follow Steps 1–4 above.

**Option 2 — Migrate Terraform state.** Select the old workspace, pull state,
select/create the new workspace, push state, then delete the old workspace.
Consult Terraform's state migration documentation for backend-specific
instructions.

---

## Backwards Compatibility

This change is **fully backwards compatible**:

- `metadata.name` is an opt-in field.
- Components that do not set `metadata.name` behave exactly as today.
- All existing workspace overrides (`terraform_workspace`, `terraform_workspace_pattern`,
  `terraform_workspace_template`) continue to take higher priority.

No configuration flag or feature toggle is required.

---

## Relationship to Related PRDs

| PRD | Relationship |
|-----|-------------|
| `workspace-key-prefixes.md` | Introduced `metadata.name` for backend key prefix stability. This PRD extends the same field to workspace name stability. |
| `terraform-workspace-key-prefix-slash-preservation.md` | Controls `/`→`-` substitution in backend key prefixes. The same slash-replacement logic applies to workspace names and is unchanged. |
| `metadata-inheritance.md` | `metadata.name` can be inherited from abstract/base components, allowing a single catalog entry to pin the workspace identity for all derived components. |

---

## Open Questions

1. **Should `metadata.name` in Phase 1 also affect the workspace when there is
   no base component?** Currently, priority 4 returns the stack prefix alone when
   `BaseComponent == ""`. Should `metadata.name` add a suffix in that case? The
   conservative choice (do not change priority 4 behaviour) is proposed here.

2. **Should a deprecation warning be emitted when a component has a base
   component but no `metadata.name`?** A warning could help users adopt the
   stable identity pattern proactively, but may be too noisy. A `--strict` lint
   rule would be less intrusive.

3. **`atmos rename component` timing.** Should Phase 2 be implemented
   immediately after Phase 1, or gated on user demand? A follow-up GitHub issue
   should be created to track demand.

---

## Success Criteria

1. `BuildTerraformWorkspace` uses `metadata.name` as the component identity when
   set, and the YAML key otherwise (no change for existing configurations).
2. A component renamed from `rds/hello-world-service` to
   `hello-world-service/rds` with `metadata.name: rds/hello-world-service` set
   produces the identical workspace name in both cases.
3. All existing workspace override mechanisms (`terraform_workspace`,
   `terraform_workspace_pattern`, `terraform_workspace_template`) continue to
   take priority over `metadata.name`.
4. Unit tests cover all scenarios in the test plan above.
5. Documentation is updated to recommend `metadata.name` as part of the
   component definition best practice.
