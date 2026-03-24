# PRD: Terraform Stack Overlays (Per-Stack/Workspace Migrations)

**Status:** Proposed
**Version:** 1.0
**Last Updated:** 2026-03-24
**Author:** Claude Code

---

## Executive Summary

Terraform supports [import blocks](https://developer.hashicorp.com/terraform/language/import) and [removed blocks](https://developer.hashicorp.com/terraform/language/resources/syntax#removing-resources) as declarative, code-reviewable mechanisms for state migration. However, these blocks must live in the component directory and would apply to *every* stack that uses the component. Atmos currently provides no way to inject per-stack `.tf` files into a component's working directory before execution, making one-off state migrations impractical without either:

1. Manually copying files into the component directory (dirty, not git-tracked), or
2. Creating a dedicated sub-component just for the migration (wasteful, confusing), or
3. Running `terraform state mv` / `terraform import` CLI commands directly (bypasses Atmos, hard to audit, impossible to review in a PR).

This PRD proposes **Stack Overlays**: a mechanism to define extra `.tf` files that are injected into a component's working directory for a specific stack, environment, stage, or workspace before terraform execution, and cleaned up afterward. This enables developers to write import/removed blocks (or any ephemeral terraform code) in version-controlled YAML or `.tf` files scoped to exactly the stacks they need to affect.

---

## Problem Statement

### Current Pain Points

1. **Import blocks can't be scoped per stack.** A `migrations.tf` placed in `components/terraform/vpc/` applies to every stack that runs `vpc`. The only workarounds are manual file operations or CLI commands.

2. **State migrations are not auditable.** When engineers run `terraform import` or `terraform state mv` manually, there is no PR review, no CI validation, and no record in git.

3. **One-off state operations are fragile.** Copy-then-delete workflows are error-prone. Files are often forgotten in the component directory, left-over from prior migrations.

4. **Partial apply failures require manual recovery.** When apply fails mid-way and resources are orphaned outside state, there is no clean declarative path to re-import them for the specific stack that had the failure.

5. **Component reuse conflict.** A component used across 20 stacks cannot have a `removed` block in it without affecting every stack—even those where the resource still exists.

### User Stories

1. **As a platform engineer**, I want to write an import block that only runs for `prod-us-east-1-vpc` without touching the 15 other stacks that use the `vpc` component.

2. **As a developer**, I want to declare a `removed` block for a resource that was deleted in one environment but still exists in others, without accidentally destroying state in the other environments.

3. **As a CI/CD system**, I want terraform migrations to be reviewed in pull requests alongside application code so that state changes are auditable and reversible.

4. **As a team lead**, I want migrations to clean up after themselves automatically so that a one-time import block doesn't linger in git forever (but can be done via a PR that adds and later removes it).

---

## Solution Overview

### Core Concept: Stack Overlays

A **stack overlay** is a set of extra `.tf` files that Atmos temporarily injects into a component's working directory for a specific execution context (stack, environment, tenant, stage, etc.) before running `terraform init/plan/apply` and removes after execution completes.

Overlays are defined in two complementary ways:

1. **Convention-based file layout** — drop `.tf` files in a special `overlays/` subdirectory within the component, organized by stack slug or stack context path.
2. **Stack YAML configuration** — list overlay file paths directly in the component's stack configuration.

Both approaches are version-controlled, PR-reviewable, and scoped to the exact stacks that need them.

---

## Design

### Approach 1: Convention-Based Overlay Directory

Place overlay files in a dedicated subdirectory of the component:

```
components/terraform/vpc/
├── main.tf
├── variables.tf
├── outputs.tf
└── overlays/
    ├── <stack-slug>/           # Exact stack slug match
    │   └── imports.tf
    ├── <tenant>/<env>/<stage>/ # Hierarchical path match
    │   └── migrate.tf
    └── <workspace>/            # Terraform workspace match
        └── removed.tf
```

**Lookup order** (first match wins):

1. `overlays/<stack-slug>/` — exact Atmos stack slug (e.g., `prod-us-east-1-vpc`)
2. `overlays/<tenant>/<environment>/<stage>/` — full context path
3. `overlays/<environment>/<stage>/` — environment + stage
4. `overlays/<stage>/` — stage only

All files in the matched directory are injected. Multiple directories can match simultaneously (all matches are injected without conflict).

**Example:**

```
components/terraform/vpc/overlays/prod-us-east-1-vpc/
└── import-legacy-vpc.tf
```

```hcl
# components/terraform/vpc/overlays/prod-us-east-1-vpc/import-legacy-vpc.tf
import {
  id = "vpc-0abc123"
  to = aws_vpc.main
}
```

When `atmos terraform apply vpc -s prod-us-east-1` is run, Atmos detects the `prod-us-east-1-vpc` overlay directory, copies `import-legacy-vpc.tf` into the working directory, runs terraform, then removes it.

### Approach 2: Stack YAML Configuration

Define overlays directly in the stack YAML:

```yaml
# stacks/prod-us-east-1.yaml
components:
  terraform:
    vpc:
      overlays:
        - path: "migrations/vpc/import-legacy-vpc.tf"
        - path: "migrations/vpc/remove-old-subnet.tf"
```

The `path` is relative to the `atmos.yaml` `base_path`.

Inline content is also supported:

```yaml
components:
  terraform:
    vpc:
      overlays:
        - name: "import-legacy-vpc.tf"
          content: |
            import {
              id   = "vpc-0abc123"
              to   = aws_vpc.main
            }
```

### Unified Configuration Model

Both approaches compose cleanly. The full configuration hierarchy is:

```
Convention-based overlay directory
  +
Stack YAML overlays section
  =
All files injected for this execution
```

Duplicates (same filename from both sources) are resolved in favor of the stack YAML (higher priority).

---

## Configuration Reference

### `overlays` Section in Stack YAML

```yaml
components:
  terraform:
    <component>:
      overlays:
        # Option 1: Reference an external file
        - path: "<relative-path-from-base-path>.tf"

        # Option 2: Inline content
        - name: "<filename>.tf"
          content: |
            <terraform HCL content>

        # Option 3: Reference a directory (all .tf files included)
        - dir: "<relative-path-from-base-path>/"
```

### `atmos.yaml` Global Configuration

```yaml
components:
  terraform:
    # Directory name for convention-based overlays (default: "overlays")
    overlays_dir: "overlays"

    # Whether to clean up injected overlay files after execution (default: true)
    # Set to false for debugging
    overlays_cleanup: true
```

---

## Execution Lifecycle

```
┌─────────────────────────────────────────────────────────────────┐
│              Terraform Execution with Stack Overlays             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Load component config (resolve stack, component, workspace)  │
│          ↓                                                       │
│  2. Resolve overlay files                                        │
│     ┌────────────────────────────────────────────────────┐      │
│     │ a. Scan convention-based overlays/ directory       │      │
│     │    Match by: slug > tenant/env/stage > env/stage   │      │
│     │    > stage > workspace                             │      │
│     │ b. Read stack YAML overlays: section               │      │
│     │ c. Merge (YAML overlays win on filename conflict)  │      │
│     └────────────────────────────────────────────────────┘      │
│          ↓                                                       │
│  3. Inject overlay files into working directory                  │
│     (copy to <workdir>/<overlay-filename>)                       │
│          ↓                                                       │
│  4. terraform init / plan / apply / destroy                      │
│          ↓                                                       │
│  5. Clean up injected overlay files                              │
│     (only files injected by Atmos are removed)                   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

Cleanup runs in a `defer` so that overlay files are removed even if terraform fails.

---

## Use Cases

### Use Case 1: Import Existing Resources (Terraform 1.5+ import blocks)

When migrating existing infrastructure into Atmos management:

```hcl
# components/terraform/vpc/overlays/prod-us-east-1-vpc/import-legacy.tf
import {
  id = "vpc-0abc123def456"
  to = aws_vpc.main
}

import {
  id = "subnet-0abc123"
  to = aws_subnet.private["us-east-1a"]
}
```

Run once, then remove the directory from git in the same or follow-up PR.

### Use Case 2: Remove Resources from State Without Destroying

When a resource is deleted from config but still exists in state in some environments:

```hcl
# components/terraform/eks/overlays/legacy-dev/remove-old-node-group.tf
removed {
  from = aws_eks_node_group.legacy

  lifecycle {
    destroy = false
  }
}
```

### Use Case 3: State Move After Refactor

After renaming a resource in terraform code:

```hcl
# components/terraform/rds/overlays/prod-us-east-1-rds/move-renamed.tf
moved {
  from = aws_db_instance.database
  to   = aws_db_instance.primary
}
```

### Use Case 4: Stack YAML Inline Migration

For migrations defined entirely in the stack configuration without extra files:

```yaml
# stacks/orgs/acme/prod/us-east-1/vpc.yaml
components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"
      overlays:
        - name: "import-legacy-vpc.tf"
          content: |
            import {
              id = "vpc-0abc123def456"
              to = aws_vpc.main
            }
```

---

## Implementation Plan

### Phase 1: Convention-Based Overlay Directory (MVP)

**Goal:** Enable `overlays/<stack-slug>/` directory lookup with automatic injection and cleanup.

**Implementation:**

1. **`internal/exec/overlay_utils.go`** — New file:
   - `resolveConventionOverlays(atmosConfig, info)` — Scans the component directory for matching overlay subdirectories.
   - `injectOverlayFiles(workingDir, files)` — Copies files into working directory, tracks injected files.
   - `cleanupOverlayFiles(workingDir, injected)` — Removes only the injected files.

2. **`internal/exec/terraform_execute_helpers_exec.go`** — Extend `runPreExecutionSteps`:
   - Call `resolveConventionOverlays` after path resolution.
   - Inject overlay files before `terraform init`.
   - Register cleanup in defer.

3. **Matching logic** (priority order):
   - `overlays/<stack-slug>/` — e.g., `overlays/prod-us-east-1-vpc/`
   - `overlays/<tenant>-<environment>-<stage>/` — dash-joined context
   - `overlays/<environment>-<stage>/`
   - `overlays/<stage>/`

**Files Changed:**
- `internal/exec/overlay_utils.go` (new)
- `internal/exec/terraform_execute_helpers_exec.go`
- `internal/exec/terraform_execute_helpers_exec_test.go`

**Acceptance Criteria:**
- ✅ Running `atmos terraform apply vpc -s prod-us-east-1` injects `overlays/prod-us-east-1-vpc/*.tf`
- ✅ Files are removed after execution (success or failure)
- ✅ No overlay directory → execution proceeds normally (no-op)
- ✅ Multiple match levels → all matching directories are injected

### Phase 2: Stack YAML `overlays` Section

**Goal:** Enable declarative overlay definitions in stack YAML.

**Implementation:**

1. **`pkg/schema/schema.go`** — Add `Overlays` field to `ConfigAndStacksInfo` and component schema:
   ```go
   type ComponentOverlay struct {
     Path    string `yaml:"path,omitempty"`
     Dir     string `yaml:"dir,omitempty"`
     Name    string `yaml:"name,omitempty"`
     Content string `yaml:"content,omitempty"`
   }
   ```

2. **`internal/exec/overlay_utils.go`** — Extend:
   - `resolveYAMLOverlays(atmosConfig, info)` — Reads `info.ComponentOverlaysSection`, resolves paths.
   - Merge YAML overlays with convention overlays (YAML wins on filename conflict).

3. **Stack processing** — Parse `overlays:` section during component config resolution.

**Files Changed:**
- `pkg/schema/schema.go`
- `internal/exec/overlay_utils.go`
- `internal/exec/stack_processor_utils.go`

**Acceptance Criteria:**
- ✅ `overlays: [{path: "migrations/import.tf"}]` in stack YAML → file injected
- ✅ `overlays: [{name: "import.tf", content: "..."}]` → inline content written and injected
- ✅ Inheritance: `overlays` defined at base component level is inherited by derived components
- ✅ Override: stack-level `overlays` takes precedence over base component `overlays`

### Phase 3: CLI Visibility and Dry-Run

**Goal:** Surface overlay information in `describe component` and dry-run.

**Implementation:**

1. **`atmos describe component <component> -s <stack>`** — Include resolved overlays in output.
2. **`atmos terraform plan --dry-run`** — Log which overlay files would be injected without running terraform.

**Files Changed:**
- `internal/exec/describe_component.go`
- `cmd/describe/component.go`

**Acceptance Criteria:**
- ✅ `atmos describe component vpc -s prod-us-east-1` shows `overlays: [...]` section
- ✅ Dry-run logs list of overlay files without executing terraform

---

## Schema Changes

### Component Schema Extension

```yaml
# stacks/prod-us-east-1.yaml
components:
  terraform:
    vpc:
      overlays:           # List of overlay definitions (optional)
        - path: string    # Relative path to a .tf file (from base_path)
          dir:  string    # Relative path to a directory of .tf files
          name: string    # Filename for inline content
          content: string # Inline HCL content
```

### `atmos.yaml` Extension

```yaml
components:
  terraform:
    overlays_dir: "overlays"   # Subdirectory name for convention-based overlays
    overlays_cleanup: true     # Auto-cleanup injected files after execution
```

---

## Security Considerations

1. **File injection is sandboxed to the working directory.** Overlay files are only copied into the component's working directory. Path traversal (e.g., `../../etc/passwd`) is rejected.

2. **Inline content is written as a regular file.** No execution happens outside of the normal terraform workflow.

3. **Cleanup is unconditional.** Even if terraform returns an error or panics, the defer ensures injected files are removed so they don't accidentally persist.

4. **Overlay files do not modify the component source directory.** Files are injected into the execution working directory (which may be a copy via the workdir provisioner), not the original source.

---

## Backward Compatibility

- No breaking changes. Existing components without an `overlays/` directory behave identically.
- The `overlays` YAML key is new and ignored by older versions of Atmos.
- Convention-based detection is purely additive — no configuration required to opt in.

---

## Alternatives Considered

### Alternative 1: Separate `migrations/` Directory at Repo Root

A top-level `migrations/` directory with naming conventions like `<component>-<stack>.tf`. **Rejected:** Loses co-location with the component, harder to discover, and doesn't follow Atmos's convention of keeping component assets with the component.

### Alternative 2: Custom GHA Slash Commands

Slash commands (e.g., `/terraform import ...`) that trigger state operations in CI. **Rejected:** Requires custom infrastructure, bypasses review, not declarative, and creates audit gaps.

### Alternative 3: Hooks (`before-terraform-apply`)

A lifecycle hook that runs a script to copy migration files before apply. **Rejected:** Requires imperative scripting (not declarative HCL), no cleanup semantics, and doesn't integrate with `terraform plan` (migrations should also be visible in plan output).

### Alternative 4: Per-Stack Terraform Root Module Override

Allow stacks to specify an entirely different `component_path`. **Rejected:** Too coarse-grained—would require duplicating the entire component directory for a one-file change.

### Alternative 5: Terraform `tfvars` Side-car

Use the existing varfile injection mechanism to pass migration data. **Rejected:** Varfiles are for variable values; they cannot contain resource blocks or import/removed blocks.

---

## Success Metrics

1. Teams can perform import block migrations without manually copying files.
2. Migration files are committed to git and appear in PRs for review.
3. Zero regression: existing terraform workflows are unaffected when no overlays exist.
4. Overlay cleanup is 100% reliable (defer-based, tested with forced errors).

---

## Test Plan

### Unit Tests

- `resolveConventionOverlays`: stack slug match, tenant/env/stage match, no-match (returns empty).
- `injectOverlayFiles`: copies files, returns list of injected paths.
- `cleanupOverlayFiles`: removes exactly the injected files, does not remove pre-existing files.
- `resolveYAMLOverlays`: path-based, dir-based, inline content.
- Filename conflict resolution (YAML wins over convention).

### Integration Tests

- `atmos terraform plan vpc -s prod-us-east-1` with `overlays/prod-us-east-1-vpc/import.tf` → plan output includes import.
- Overlay files absent after plan.
- `overlays:` in stack YAML with inline content → file created, terraform runs, file removed.
- Dry-run logs overlay files without executing terraform.

### Test Fixtures

```
tests/test-cases/overlays/
├── atmos.yaml
├── stacks/
│   ├── prod-us-east-1.yaml
│   └── dev-us-east-1.yaml
└── components/
    └── terraform/
        └── vpc/
            ├── main.tf
            └── overlays/
                ├── prod-us-east-1-vpc/
                │   └── import-legacy.tf
                └── dev/
                    └── import-dev.tf
```

---

## Related Work

- [Component Workdir Provisioner](component-workdir.md) — overlays are injected into the workdir copy, not the source directory.
- [Source Provisioner](source-provisioner.md) — source downloads happen before overlay injection.
- [Code Generation PRD](code-generation.md) — the `generate` section also injects files; overlays differ in that they are ephemeral (cleaned up after execution) and targeted at state migration HCL specifically.
- [GitHub Issue: Allow terraform state migration blocks per stack/workspace](https://github.com/cloudposse/atmos/issues/...)

---

## Open Questions

1. **Should overlays apply to `terraform plan` in addition to `apply`?** Yes — import blocks appear in plan output, so plan must also inject overlays. `removed` and `moved` blocks should also be visible in plans.

2. **Should `overlays_cleanup: false` be supported for debugging?** Yes — when debugging a migration, it is useful to inspect the injected files. A global config flag and a per-invocation `--no-cleanup-overlays` flag should both be supported.

3. **Should overlay injection be logged at the INFO level?** Yes — users should see a message like `Injecting overlay: overlays/prod-us-east-1-vpc/import-legacy.tf` so they know the overlay was applied.

4. **How do overlays interact with the workdir provisioner?** Overlays are injected into the resolved working directory (after workdir provisioner has run), so they work correctly whether or not a workdir copy is being used.

5. **Should glob patterns be supported in `overlays_dir` lookup?** Future consideration — for MVP, exact directory name matching is sufficient.
