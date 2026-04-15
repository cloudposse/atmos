# PRD: Component Rename Workspace Stability

**Version:** 2.0  
**Last Updated:** 2026-04-15  
**Issue:** [#2244](https://github.com/cloudposse/atmos/issues/2244)

---

## Problem

Renaming an Atmos component (changing its YAML key, or renaming the stack/account it belongs to) silently changes the Terraform workspace name. Terraform treats the new workspace as empty and plans to recreate all resources, while the old workspace retains orphaned state.

### How workspace names are derived today

`BuildTerraformWorkspace` (`internal/exec/stack_utils.go`) uses this priority chain:

| Priority | Source | Example |
|----------|--------|---------|
| 1 | `metadata.terraform_workspace_template` | Go template |
| 2 | `metadata.terraform_workspace_pattern` | Token pattern |
| 3 | `metadata.terraform_workspace` | Static string |
| 4 | Stack prefix only (no base component) | `ue1-dev` |
| 5 | `{stack_prefix}-{yaml_key}` | `ue1-dev-rds-hello-world-service` |

Priority 5 couples the workspace name to the YAML key. Renaming `rds/hello-world-service` to `hello-world-service/rds` changes the workspace from `ue1-dev-rds-hello-world-service` to `ue1-dev-hello-world-service-rds`. The current workaround (`metadata.terraform_workspace: "ue1-dev-rds-hello-world-service"`) hard-codes the stack name, breaks inheritance, and must be set proactively — before the rename happens.

### The collision problem

Beyond rename-triggered breakage, a second hazard exists: **workspace name collision**. If component A is renamed to a YAML key that produces the same workspace name as component B, both components will operate against the same Terraform state. Without a registry that maps workspace names to their owning components, this collision is undetectable until state corruption occurs.

---

## Options

### Option A — `metadata.name` controls the workspace name

**What:** Extend `BuildTerraformWorkspace` to use `metadata.name` (already used for backend key prefix stability) as the component segment in the fallback workspace name. Add `metadata.name` as a new priority between the static override (3) and the stack-prefix-only case (4).

**New priority table:**

| Priority | Source | Notes |
|----------|--------|-------|
| … | (1–3 unchanged) | |
| 4 | Stack prefix only (no base component) | Unchanged |
| **5** | **`{stack_prefix}-{metadata.name}`** | **New** |
| 6 | `{stack_prefix}-{yaml_key}` | Current behaviour |

**Usage:**
```yaml
hello-world-service/rds:         # YAML key (can change freely)
  metadata:
    name: rds/hello-world-service # Stable identity → workspace: ue1-dev-rds-hello-world-service
```

**Devil's advocate:**
- This doesn't help users who didn't set `metadata.name` before renaming — it's still opt-in. Existing projects get nothing.
- `metadata.name` becomes an immutable commitment. Changing it recreates the problem.
- It only stabilises the _component segment_. If the stack is renamed (`ue1-dev` → `us-east-1-dev`), the workspace prefix still changes.
- No collision detection: two components can accidentally share the same `metadata.name` value, silently pointing at the same workspace. Nothing warns you.

**Verdict:** Excellent for new projects and deliberate migrations. Insufficient as a standalone solution for large existing repos or stack renames.

---

### Option B — `atmos rename component` migration command

**What:** An interactive CLI command that discovers every stack where a component is deployed, migrates Terraform workspace state (pull/push via backend API), and updates YAML manifests.

```
> atmos rename component rds/hello-world-service hello-world-service/rds

Found 2 stacks: ue1-dev, ue2-dev
Rename and migrate workspaces? [y/N]: y
  [ue1-dev] ue1-dev-rds-hello-world-service → ue1-dev-hello-world-service-rds ✓
  [ue2-dev] ue2-dev-rds-hello-world-service → ue2-dev-hello-world-service-rds ✓
```

**Devil's advocate:**
- Only helps users who _know_ they are renaming. Does nothing for stack/account renames.
- Requires Terraform execution: needs credentials for every backend across every stack. In multi-account environments this is a non-trivial orchestration problem.
- YAML rewriting risks destroying anchors, comments, and formatting.
- Not atomic: a partial failure mid-run leaves some stacks migrated and others not. Rollback is complex.
- High implementation cost; no immediate value for the most common case (accidental/unplanned rename).

**Verdict:** High value as a convenience tool for planned renames. Not a primary safety mechanism. Should come after Option A is stable.

---

### Option C — Workspace identity lock file (in-repo)

**What:** Atmos automatically writes a lock file (`.atmos/workspace-locks.yaml`) on the first `plan`/`apply` for a given component+stack pair, recording the computed workspace name. Future runs use the locked name regardless of YAML key changes. The lock file is committed to the main branch and shared across the team — similar to `go.sum` or `package-lock.json`.

```yaml
# .atmos/workspace-locks.yaml — auto-generated by Atmos; commit to repo
locks:
  rds/hello-world-service@ue1-dev: "ue1-dev-rds-hello-world-service"
  rds/hello-world-service@ue2-dev: "ue2-dev-rds-hello-world-service"
```

When a component is renamed and has no lock entry, Atmos warns and falls back to the normal derivation. A `atmos workspace lock migrate <old> <new>` command transfers the lock entry.

**Collision detection:** Before writing a new lock entry, Atmos scans for duplicate workspace values. If the target workspace name is already claimed by a different component, it fails with a clear error:
```
ERROR: Workspace 'ue1-dev-db' is already locked by component 'db@ue1-dev'.
       Cannot assign it to 'database@ue1-dev'. Resolve the conflict first.
```

**Devil's advocate:**
- Atmos writing files during `plan`/`apply` is unexpected. CI pipelines are often read-only and cannot commit back to the repo. If the lock file is not committed, the workspace name is re-derived on the next run.
- Concurrent CI runs (e.g., two PRs each adding a new component) will produce merge conflicts in the lock file.
- Bootstrapping an existing repo requires either running `plan` for every component or a separate `atmos workspace lock generate --all` command.
- The lock file is a new piece of Atmos-managed state that operators must understand and maintain.

**Verdict:** Solves the zero-proactive-effort requirement and provides collision detection. The main operational cost is the commit discipline. The right choice for Phase 2.

---

### Option D — `atmos-state` branch (remote auto-committed lock)

**What:** Store the workspace lock registry in a dedicated, non-protected Git branch (e.g., `atmos-state`), analogous to how GitHub Pages uses `gh-pages`. Atmos pushes updated lock entries to this branch after every `plan`/`apply`. No files are added to the main branch.

```
main branch:          stacks/*.yaml  (user-managed, no lock file)
atmos-state branch:   workspace-locks.yaml  (Atmos-managed, auto-committed)
```

**Devil's advocate:**
- Concurrent `plan`/`apply` runs will race to push to `atmos-state`, causing push conflicts. Atmos would need a fetch-rebase-retry loop, adding latency and fragility.
- Write access to the repo is required from every CI runner that runs `plan` or `apply` — including read-only plan jobs that currently need no push permission.
- The lock registry is invisible in PRs (it's on another branch). Reviewers cannot see or approve workspace assignment changes.
- If the `atmos-state` branch is force-pushed, deleted, or corrupted, all workspace assignments are lost with no recovery path.
- Non-standard Git usage makes the system harder to understand for new team members.
- This solves the "merge conflict in main" problem from Option C but introduces worse problems (race conditions, invisible state, access requirements).

**Verdict:** Clever but operationally fragile. The in-repo lock file (Option C) is strictly better for most teams. Consider only if the main-branch commit requirement is an absolute blocker.

---

### Option E — GUID written into stack YAML (auto-mutate user files)

**What:** On first provision, Atmos generates a UUID and writes it into the component's `metadata` block within the stack YAML file itself.

```yaml
rds/hello-world-service:
  metadata:
    workspace_id: "a3f2c891-7d1b-4e6a-b3d9-0f8c2e5a1b7d"  # written by Atmos
```

**Devil's advocate:**
- Atmos mutating user-managed YAML files is the worst possible approach: it destroys formatting, anchors, and comments. This was explicitly rejected in the original issue discussion.
- UUIDs are opaque. Debugging which workspace belongs to which component requires a lookup table.
- Read-only CI runs cannot write UUIDs, creating a two-phase bootstrap problem.
- Every new component in every new stack triggers a dirty working tree, polluting Git history.

**Verdict:** Not viable. Included only for completeness.

---

## Comparison

| | A — `metadata.name` | B — Rename cmd | C — Lock file (repo) | D — `atmos-state` branch | E — GUID in YAML |
|---|:---:|:---:|:---:|:---:|:---:|
| Zero proactive setup required | ❌ | ❌ | ✅ | ✅ | ✅ |
| Survives component rename | ✅ if set | ✅ migrates | ✅ locked | ✅ locked | ✅ |
| Survives stack/account rename | ❌ prefix changes | ✅ migrates | ✅ full name locked | ✅ full name locked | ✅ |
| Collision detection | ❌ | ✅ pre-check | ✅ on write | ✅ on write | ❌ |
| Reviewable in PRs | ✅ | ✅ | ✅ | ❌ separate branch | ✅ |
| No CI write-back required | ✅ | ✅ plan-only | ❌ must commit | ❌ must push | ❌ must commit |
| Human-readable workspaces | ✅ | ✅ | ✅ | ✅ | ❌ UUIDs |
| Backwards compatible | ✅ fully | ✅ | ⚠️ bootstrap needed | ⚠️ bootstrap needed | ❌ breaking |
| Implementation effort | Trivial | High | Medium | High | Medium |

---

## Recommendation

### Phase 1 — Ship now: `metadata.name` controls workspace name

This is a **< 10-line code change** that is fully backwards-compatible and immediately useful. It gives new projects a solid foundation and gives existing projects a safe rename path.

**Shipping Phase 1 alone is acceptable.** Teams with existing infrastructure can adopt `metadata.name` proactively. Stack/account renames remain handled by the existing `metadata.terraform_workspace_template` escape hatch (already supported).

### Phase 2 — Follow-up: in-repo workspace lock file

Option C (in-repo lock file) is the right long-term solution. It eliminates the proactive-setup requirement, survives all renames, and provides collision detection. The operational cost (commit discipline, concurrent-run conflicts) is manageable with standard GitOps practices.

Option D (`atmos-state` branch) should be offered as an **opt-in alternative storage backend** for the same lock data, configurable in `atmos.yaml`. This lets teams choose based on their CI topology — but the in-repo lock file should be the default.

Option B (`atmos rename component`) should also be built eventually as a convenience tool, but it is not a safety mechanism and should not be prioritised over the lock file.

Option E (GUID in YAML) is not viable and should not be implemented.

### Answered open questions

**Q1: Should `metadata.name` affect the workspace when there is no base component?**  
No. Priority 4 (stack prefix only, no base component) is unchanged. `metadata.name` only affects the component-suffix case (current priority 5).

**Q2: Should Atmos warn when a component has a base component but no `metadata.name`?**  
Yes, but as an opt-in lint rule (`atmos lint stacks --rule workspace-identity`), not a runtime warning. A lint rule is surfaced at review time, not during every `plan`.

**Q3: What is the priority when `metadata.name` and a lock file entry both exist?**  
`metadata.name` (explicit) takes priority over the lock file (implicit). A user who explicitly sets `metadata.name` is expressing intent. The lock file is a fallback.

**Q4: How do existing repos bootstrap the lock file?**  
A `atmos workspace lock generate` command scans all stacks and writes lock entries for every deployed component based on the _current_ computed workspace name. No Terraform runs are needed. This is a pure read operation over the YAML configuration.

**Q5: How are workspace name collisions prevented?**  
At lock-write time (Phase 2), Atmos verifies the resolved workspace name does not already appear in the lock file under a different key. For Phase 1 (`metadata.name` only), `atmos validate stacks` will detect duplicate workspace names within a stack and fail with a descriptive error.

**Q6: What about the `atmos-state` branch option?**  
Viable as an opt-in storage backend but not recommended as the default. The in-repo lock file is strictly better for visibility and reliability. Configure with `workspace_locks.backend: git-branch` in `atmos.yaml` if desired.

---

## Implementation (Phase 1)

### `internal/exec/stack_utils.go` — ~8 lines changed

```go
// In the Priority 5/6 fallback block:
componentIdentity := configAndStacksInfo.Context.Component  // current behaviour
if name, ok := componentMetadata["name"].(string); ok && name != "" {
    componentIdentity = name  // metadata.name takes precedence
}
workspace = fmt.Sprintf("%s-%s", contextPrefix, componentIdentity)
```

No schema changes required — `metadata.name` already exists.

### Unit tests (`internal/exec/stack_utils_test.go`)

| Scenario | `metadata.name` | YAML key | Expected workspace |
|----------|-----------------|----------|--------------------|
| Name set, component renamed | `rds/hello-world-service` | `hello-world-service/rds` | `ue1-dev-rds-hello-world-service` |
| Name not set | — | `rds/hello-world-service` | `ue1-dev-rds-hello-world-service` |
| Name not set, component renamed | — | `hello-world-service/rds` | `ue1-dev-hello-world-service-rds` |
| Static override wins | `rds/hws` | any | value of `metadata.terraform_workspace` |
| Template override wins | `rds/hws` | any | result of `metadata.terraform_workspace_template` |
| No base component | `rds/hws` | any | stack prefix only (unchanged) |

### Migration guide

**New projects:** Set `metadata.name` on every component from day one. The name is a permanent commitment.

**Existing projects — safe rename:**
1. Set `metadata.name` to the current YAML key. Plan → confirm zero diff.
2. Rename the YAML key. Plan → confirm zero diff.

**Existing projects — emergency recovery (rename already happened):**  
Revert the YAML rename, follow the safe procedure above, then re-apply the rename.

---

## Related PRDs

| PRD | Relationship |
|-----|-------------|
| `workspace-key-prefixes.md` | Introduced `metadata.name` for backend key prefix stability. This PRD extends it to workspace name stability. |
| `terraform-workspace-key-prefix-slash-preservation.md` | `/`→`-` substitution applies equally to workspace names. |
| `metadata-inheritance.md` | `metadata.name` can be inherited from abstract components, pinning workspace identity for all derived components in one place. |
