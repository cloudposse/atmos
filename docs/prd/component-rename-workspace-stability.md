# PRD: Component Rename Workspace Stability

**Version:** 1.1
**Last Updated:** 2026-04-14
**Issue:** [#2244](https://github.com/cloudposse/atmos/issues/2244)

---

## Executive Summary

Renaming an Atmos component (changing its YAML key) silently changes the
Terraform workspace name, causing Terraform to treat the renamed component as
a brand-new workspace. Existing infrastructure in the old workspace is
effectively "lost" from Terraform's perspective: resources will be recreated
in the new workspace while the old workspace remains orphaned with stale state.

This PRD analyses four approaches for workspace-identity stability and recommends
a two-phase solution:

1. **Phase 1 (immediate):** Extend the existing `metadata.name` mechanism — already
   used to stabilise backend key prefixes — to also control the Terraform
   workspace name, giving users a simple, declarative way to pin workspace
   identity independently of the YAML key.
2. **Phase 2 (follow-up):** Introduce either an `atmos rename component` interactive
   migration command or a workspace identity lock file so that workspace names
   are _automatically_ stabilised without requiring any user action.

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

### Option C — Workspace Identity Lock File (Sticky Auto-Assigned Name)

#### Description

The root cause of the rename problem is that users must *proactively* set
`metadata.name` before renaming. Option C removes that requirement by having
Atmos automatically record the computed workspace name the first time a
component is provisioned — similar to how package managers write a `go.sum` or
`package-lock.json`.

**How it works:**

1. On the first `atmos terraform plan/apply` for `rds/hello-world-service -s ue1-dev`,
   Atmos computes the workspace name (`ue1-dev-rds-hello-world-service`) and
   writes it to a lock file: `.atmos/workspace-locks.yaml`:

   ```yaml
   # .atmos/workspace-locks.yaml
   # Auto-generated by Atmos. Commit this file to your repository.
   locks:
     rds/hello-world-service@ue1-dev: "ue1-dev-rds-hello-world-service"
     rds/hello-world-service@ue2-dev: "ue2-dev-rds-hello-world-service"
   ```

2. When the component is renamed to `hello-world-service/rds` in the YAML, the
   lock file still contains the old key `rds/hello-world-service@ue1-dev`.
   Atmos emits a warning:

   ```
   WARNING: Component 'hello-world-service/rds' has no workspace lock entry.
            Former name 'rds/hello-world-service' has a lock entry.
            Run `atmos workspace lock migrate rds/hello-world-service hello-world-service/rds`
            to transfer the lock, or add metadata.name to pin the workspace explicitly.
   ```

3. The lock file is committed to Git and shared across the team. All engineers
   and CI pipelines use the same frozen workspace names.

**Workspace name for lock key:** The lock key is the component's YAML key (not
`metadata.name`), so the lock persists even if `metadata.name` changes.

**Alternative sub-variant — GUID in lock file:** Instead of storing the computed
human-readable name, Atmos could generate a UUID and use that as the workspace
name. The trade-off analysis below covers both variants.

#### Trade-offs

| | Human-readable lock | UUID lock |
|--|---|---|
| ✅ No proactive setup | Workspace is frozen automatically on first provision. | Same. |
| ✅ Survives YAML key rename | Lock is keyed on old YAML key; migration command transfers it. | Same. |
| ✅ Survives stack rename | Lock value is the frozen workspace name, not derived from current stack name. | Same. |
| ✅ Git-committable | Lock file lives in the repo, reviewed in PRs. | Same. |
| ✅ Escape hatch | Lock file is plain YAML; users can edit it if needed. | Same, but UUID is opaque. |
| ✅ Human-readable | Workspace names remain meaningful (`ue1-dev-rds-hello-world-service`). | ❌ UUIDs are opaque. |
| ⚠️ Atmos must write files | Atmos writes/updates the lock file during `plan`/`apply`, which may surprise users. | Same. |
| ⚠️ Lock file is new state | A new file that must be committed, kept in sync, and merged carefully in PRs. | Same. |
| ⚠️ Merge conflicts | Two PRs that each provision a new component will conflict in the lock file. | Same. |
| ⚠️ Breaking change | Existing components have no lock entry. Atmos must decide: warn and compute normally, or require migration. | Same. |
| ⚠️ Lock key is YAML key | If the YAML key has never been locked, renaming without migrating first silently breaks the workspace. | Same. |

**Conclusion:** The lock file approach is significantly more viable than the
raw GUID approach and solves the "zero proactive effort" requirement. The main
cost is the new Atmos-managed file and the Git workflow discipline it demands.
It is a good candidate for Phase 2 alongside or instead of the migration
command. The UUID sub-variant offers no advantages over the human-readable
variant and is not recommended.

---

### Option D — GUID Written Directly into the Stack YAML

#### Description

Generate a UUID when a component is first provisioned. Write it into the
component's `metadata` block within the stack YAML file itself:

```yaml
# ue1-dev.yaml  (auto-modified by Atmos)
components:
  terraform:
    rds/hello-world-service:
      metadata:
        workspace_id: "a3f2c891-7d1b-4e6a-b3d9-0f8c2e5a1b7d"  # frozen by Atmos
```

#### Trade-offs

| | |
|-|--|
| ✅ Truly immutable per-component-per-file | UUID is frozen inside the file that defines the component. |
| ✅ No separate state file | Identity lives alongside the configuration that needs it. |
| ❌ Atmos mutates user YAML | Writing to user-managed stack files is unexpected and fragile; risks destroying formatting, anchors, and comments. |
| ❌ UUID is opaque | Workspace names become meaningless strings; debugging is hard. |
| ❌ Breaking change | All existing components need UUIDs generated and committed before the feature becomes effective. |
| ❌ Git noise on first run | Every newly-added component triggers a YAML write and a dirty working tree. |
| ❌ CI pipeline friction | A read-only CI run would fail to write the UUID; a two-phase workflow (write UUID, then plan) adds complexity. |

**Conclusion:** Option D is the least viable. Atmos mutating user YAML is
the core objection raised by @osterman ("b) atmos dynamically updating the YAML
to persist that state"). The lock file in Option C is a much safer alternative
for auto-assigned identity because it is a Atmos-owned file distinct from the
user's configuration.

---

## Options Comparison

| | A — `metadata.name` | B — Rename Command | C — Lock File | D — GUID in YAML |
|-|---|---|---|---|
| Requires proactive user action | ⚠️ Yes — before rename | ✅ No — migrate after | ✅ No — auto on first provision | ✅ No — auto on first provision |
| Human-readable workspace names | ✅ Yes | ✅ Yes | ✅ Yes (human-readable variant) | ❌ No |
| Survives component YAML rename | ✅ Yes (if set) | ✅ Yes (migrates name) | ✅ Yes (lock key migrated) | ✅ Yes |
| Survives stack/account rename | ⚠️ No — workspace prefix changes | ✅ Yes (migrates name) | ✅ Yes (lock stores frozen value) | ✅ Yes |
| Atmos writes files | ✅ No | ⚠️ YAML rewrite | ⚠️ Lock file (Atmos-owned) | ❌ User YAML rewrite |
| Backwards compatible | ✅ Fully | ✅ Fully | ⚠️ Requires migration for existing components | ❌ Breaking |
| Implementation complexity | ✅ Minimal (< 10 LOC) | ⚠️ High | ⚠️ Medium | ❌ High |
| Phase | 1 (immediate) | 2 (follow-up) | 2 (follow-up) | Not recommended |

### Note on "Survives stack/account rename"

Option A (`metadata.name`) stabilises the _component segment_ of the workspace
name, but not the _stack prefix_. If the stack is renamed from `ue1-dev` to
`us-east-1-dev`, the workspace name changes from `ue1-dev-rds-hello-world-service`
to `us-east-1-dev-rds-hello-world-service` even with `metadata.name` set.

The full workspace name can be pinned by using `metadata.terraform_workspace`
(a static string, already supported) or `metadata.terraform_workspace_template`
(a Go template, already supported). Neither Option A nor the lock file (Option C)
can independently stabilise a workspace name that is derived from the stack
prefix — that requires an explicit override or a lock file that stores the full
computed name (not just the component segment).

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

### Phase 2 — Workspace Identity Lock File or Rename Command

For Phase 2, two alternatives are available. They address different user
segments and can coexist:

**2a — `atmos rename component` command** *(targeted at users who know a rename is happening)*

Implement an interactive command that finds all stacks where the component is
deployed, migrates workspaces, and updates YAML manifests. This is the best
option for deliberate, planned renames.

**2b — Workspace identity lock file** *(targeted at users who want zero-effort permanence)*

Implement a `.atmos/workspace-locks.yaml` lock file that is automatically
written on first provision. Workspace names are frozen from that point forward,
surviving any future rename without any user action. This is the best option
for teams that want "set it and forget it" stability.

Both options should be tracked in separate follow-up issues. The lock file (2b)
is likely higher-value for large organizations managing many components across
many stacks.

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

### Phase 2a — `atmos rename component` Scope (Future Issue)

The `atmos rename component` command will be tracked in a separate issue and
PRD. It is mentioned here for completeness. Key design points to address in
that PRD:

- Dry-run mode (`--dry-run`).
- Per-stack filtering (`--stack`).
- Backend-aware workspace migration (S3, GCS, Azure, HTTP).
- Transactional safety (rollback on partial failure).
- YAML rewrite strategy (preserve comments/anchors).
- Credential handling for multi-account environments.

### Phase 2b — Workspace Identity Lock File Scope (Future Issue)

Key design points to address in a separate PRD:

- Lock file location (`{stacks-base-path}/../.atmos/workspace-locks.yaml` or
  configurable via `atmos.yaml`).
- Lock file format: keyed by `{component-yaml-key}@{stack}` → frozen workspace name.
- Behaviour when no lock entry exists: warn + compute normally (least-surprise
  default).
- Behaviour when the YAML key is renamed: warn and suggest
  `atmos workspace lock migrate <old> <new>`.
- Migration command for bootstrapping existing repos:
  `atmos workspace lock generate --all` to snapshot current workspace names.
- CI/CD considerations: lock file must be committed; read-only CI should not
  write the lock file by default (opt-in with `--update-lock`).

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

3. **Phase 2 prioritisation: rename command vs. lock file?** The lock file
   (Option C) is lower-friction for users but requires Atmos to own a new file.
   The rename command (Option B) is more surgical but only helps when a rename
   is consciously happening. Both should be tracked as separate issues; the lock
   file may provide higher aggregate value.

4. **Lock file bootstrap for existing repos.** If a lock file is adopted, large
   existing repos with hundreds of components need a way to snapshot current
   workspace names in bulk without running `atmos terraform plan` for every
   component. A `atmos workspace lock generate --all` command (dry-run aware)
   would address this.

5. **Lock file + `metadata.name` interaction.** If both are set, which takes
   priority? Proposed: `metadata.name` takes priority (explicit > implicit), so
   users can override the auto-frozen name when needed.

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
6. Follow-up issues are created for the workspace identity lock file (Phase 2b)
   and the `atmos rename component` command (Phase 2a).
