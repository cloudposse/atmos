# PRD: Terraform Stack Overlays (Per-Stack/Workspace Migrations)

**Status:** Proposed
**Version:** 1.1
**Last Updated:** 2026-03-24
**Author:** rb

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
    ├── <stage>/                      # Broad: all stacks in this stage
    │   └── shared-migration.tf
    ├── <environment>-<stage>/        # Mid: all stacks for env+stage
    │   └── migrate.tf
    ├── <tenant>-<environment>-<stage>/ # Narrow: full context match
    │   └── migrate.tf
    └── <stack-slug>/                 # Exact: single stack only
        └── imports.tf
```

**Lookup order** (all matching levels are injected; more-specific levels are injected last so their files overwrite less-specific ones on filename collision):

1. `overlays/<stage>/` — broadest scope (e.g., `overlays/prod/`)
2. `overlays/<environment>-<stage>/` — environment + stage (e.g., `overlays/us-east-1-prod/`)
3. `overlays/<tenant>-<environment>-<stage>/` — full context path (e.g., `overlays/acme-us-east-1-prod/`)
4. `overlays/<stack-slug>/` — exact Atmos stack slug, most specific (e.g., `overlays/prod-us-east-1-vpc/`)

All matching directories at every level are injected. Files from a more-specific level overwrite files with the same name from a less-specific level. This additive model allows shared migrations at broad scope with per-stack overrides.

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

### Event Notation

This PRD uses the canonical dot-notation for hook events (e.g., `before.terraform.apply`), which matches the Go constants defined in `pkg/hooks/event.go`. Existing Atmos documentation (including the hooks-component-scoping PRD and stack YAML examples) uses hyphen-notation (e.g., `after-terraform-apply`). These refer to the same events; the dot-notation is canonical going forward.

### `overlays` Section in Stack YAML

```yaml
components:
  terraform:
    <component>:
      overlays:
        # Option 1: Reference an external file
        - path: "<relative-path-from-base-path>.tf"

        # Option 2: Inline content (denied block types: terraform{}, provider{}, terraform_remote_state)
        - name: "<filename>.tf"
          content: |
            <terraform HCL content>

        # Option 3: Reference a directory (all .tf files included)
        - dir: "<relative-path-from-base-path>/"
```

### List-Append Semantics and Override

The `overlays:` list **appends** across the Atmos catalog hierarchy. When a component inherits from a base component, the overlay lists are concatenated (base first, child after):

```yaml
# catalog/vpc-base.yaml (base component)
components:
  terraform:
    vpc-base:
      overlays:
        - path: "migrations/shared/common-import.tf"   # applied to all stacks using vpc-base

# stacks/prod-us-east-1.yaml (derived component)
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc-base]
      overlays:
        - name: "import-legacy-vpc.tf"                 # appended; both overlays are injected
          content: |
            import {
              id = "vpc-0abc123def456"
              to = aws_vpc.main
            }
```

To **replace** the entire inherited list (opt out of append semantics), set `_override: true` on the `overlays:` list:

```yaml
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc-base]
      overlays:
        _override: true                                # discards inherited overlays
        - name: "fresh-import.tf"
          content: |
            import { id = "vpc-new123", to = aws_vpc.main }
```

### `atmos.yaml` Global Configuration

```yaml
components:
  terraform:
    # Directory name for convention-based overlays (default: "overlays")
    overlays_dir: "overlays"

    # Whether to clean up injected overlay files after execution (default: true)
    # Set to false for debugging; also overridable per-invocation with --no-cleanup-overlays
    overlays_cleanup: true

settings:
  # Phase 1 ships as an experimental feature.
  # Set to true to enable stack overlay injection.
  # Without this flag, overlay configuration is parsed but injection is skipped with a warning.
  experimental:
    stack_overlays: true
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

## Concurrent Execution Safety

Overlay injection **must always target the per-execution working directory**, never the component source directory. This requirement is unconditional—it does not depend on the workdir provisioner being enabled.

### Why This Matters

The component source directory (e.g., `components/terraform/vpc/`) is shared across all concurrent executions of that component. Writing overlay files directly into the source directory would:

1. Create race conditions when two stacks run the same component concurrently.
2. Leave stale overlay files visible to other executions if a process is killed.
3. Cause unintended overlay leakage—stack A's import block appearing in stack B's plan.

### Implementation Requirement

Before injecting overlay files, Atmos **must** resolve the per-execution working directory. This is the directory where `backend.tf.json` and the generated varfile live. The resolution order is:

1. If the workdir provisioner is active (`provision.workdir.enabled: true`), use the provisioned copy under `.workdir/terraform/<stack>-<component>/`.
2. Otherwise, create a **temporary execution directory** for overlay injection:
   - Copy the component source to a temp directory under `<base_path>/.atmos/workdir/<stack>-<component>/`.
   - Inject overlays there.
   - Terraform runs against the temp directory.
   - Temp directory is cleaned up (along with overlays) after execution.

This guarantees that the component source directory is never modified, regardless of provisioner configuration.

### Concurrency Contract

```
Stack A: vpc -s prod-us-east-1  →  .atmos/workdir/prod-us-east-1-vpc/  (overlays A injected here)
Stack B: vpc -s dev-us-east-1   →  .atmos/workdir/dev-us-east-1-vpc/   (overlays B injected here)
Source:  components/terraform/vpc/                                       (NEVER modified)
```

---

## Plan/Apply Lifecycle

Overlay injection occurs for all terraform operations that interact with state or produce a plan. The behavior per operation is:

### `terraform plan` (plan-only)

1. Resolve and inject overlay files into the working directory.
2. Run `terraform plan`.
3. Clean up overlay files (defer).

Import, removed, and moved blocks appear in plan output, giving reviewers full visibility before any apply.

### `terraform apply` (direct, no plan file)

1. Resolve and inject overlay files into the working directory.
2. Run `terraform apply`.
3. Clean up overlay files (defer).

### `terraform apply --from-plan` (apply from saved plan file)

When applying from a previously saved plan file, the overlay files that were present during `plan` **must** be re-injected for `apply` to succeed. Additionally, Atmos computes a SHA-256 hash of all overlay file contents at plan time and stores it alongside the plan file. At apply time:

1. Re-resolve overlay files.
2. Compute hash of current overlays.
3. If hash differs from plan-time hash, **abort with an error**: the overlays have changed since the plan was generated and the saved plan may no longer be valid.
4. If hashes match, inject overlays and run `terraform apply -input=false <planfile>`.
5. Clean up overlay files (defer).

### `terraform destroy`

1. Resolve and inject overlay files into the working directory.
2. Run `terraform destroy`.
3. Clean up overlay files (defer).

> **Note:** For most destroy operations, no overlays will be present (migrations are typically one-shot). However, `removed { lifecycle { destroy = false } }` blocks are an exception and may be injected to prevent accidental destruction.

---

## OpenTofu Compatibility

Stack Overlays work with both the `terraform` (HashiCorp Terraform 1.5+) and `tofu` (OpenTofu 1.6+) binaries. Both support `import {}`, `removed {}`, and `moved {}` blocks.

The binary used for execution is determined by the component's `command` field (defaults to `terraform`). Overlay injection is binary-agnostic—files are copied before `init` regardless of which binary runs them.

**Phase 1 acceptance criterion:** All overlay tests pass when `command: tofu` is configured for the component.

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
- ✅ Running `atmos terraform apply vpc -s prod-us-east-1` also injects `overlays/prod/*.tf` and `overlays/us-east-1-prod/*.tf` when present (all levels injected)
- ✅ More-specific level files overwrite less-specific files with the same name
- ✅ Files are removed after execution (success or failure)
- ✅ No overlay directory → execution proceeds normally (no-op)
- ✅ Works for both `terraform` (HashiCorp) and `tofu` (OpenTofu) binary
- ✅ Feature is gated by `settings.experimental.stack_overlays: true`; execution without the flag logs a warning and skips overlay injection
- ✅ `pkg/datafetcher/schema/` (atmos-manifest JSON schema) updated to allow `overlays:` on component nodes

### Phase 2: Stack YAML `overlays` Section

**Goal:** Enable declarative overlay definitions in stack YAML with full inheritance and merge semantics.

**Implementation:**

1. **`pkg/schema/schema.go`** — Add `Overlays` field to `ConfigAndStacksInfo` and component schema:
   ```go
   type ComponentOverlay struct {
     Path     string `yaml:"path,omitempty"`
     Dir      string `yaml:"dir,omitempty"`
     Name     string `yaml:"name,omitempty"`
     Content  string `yaml:"content,omitempty"`
     Override bool   `yaml:"_override,omitempty"`
   }
   ```

2. **`internal/exec/overlay_utils.go`** — Extend:
   - `resolveYAMLOverlays(atmosConfig, info)` — Reads `info.ComponentOverlaysSection`, resolves paths, validates inline content against denied block types.
   - Merge YAML overlays with convention overlays (YAML wins on filename conflict).
   - Respect `_override: true` to discard inherited entries.

3. **`internal/exec/stack_processor_utils.go`** — Parse `overlays:` section during component config resolution with list-append merge semantics.

4. **`internal/exec/describe_component.go`** and **`cmd/describe/component.go`** — Add `--show-overlays` flag:
   - Without flag: `overlays` section omitted from output (to avoid noise for components without overlays).
   - With `--show-overlays`: show resolved overlay list including source (convention-based or YAML).

**Files Changed:**
- `pkg/schema/schema.go`
- `internal/exec/overlay_utils.go`
- `internal/exec/stack_processor_utils.go`
- `internal/exec/describe_component.go`
- `cmd/describe/component.go`

**Acceptance Criteria:**
- ✅ `overlays: [{path: "migrations/import.tf"}]` in stack YAML → file injected
- ✅ `overlays: [{name: "import.tf", content: "..."}]` → inline content written and injected
- ✅ Inline content with denied block types (`terraform {}`, `provider {}`, `terraform_remote_state`) → error before injection
- ✅ Inheritance: `overlays` list from base component is prepended to derived component's list
- ✅ `_override: true` on derived component's list → inherited overlays discarded
- ✅ `atmos describe component vpc -s prod-us-east-1 --show-overlays` shows resolved overlay list
- ✅ `atmos describe component vpc -s prod-us-east-1` (no flag) → `overlays` section absent from output

### Phase 3: Dry-Run and Hash Verification

**Goal:** Surface plan-time overlay hash and enable dry-run inspection.

**Implementation:**

1. **`atmos terraform plan --dry-run`** — Log which overlay files would be injected without running terraform.
2. **Plan-time hash storage** — Store SHA-256 of overlay contents alongside the plan file.
3. **Apply-time hash comparison** — Verify overlay contents match plan-time hash before injecting for `--from-plan` apply.

**Files Changed:**
- `internal/exec/terraform_execute_helpers_exec.go`
- `internal/exec/overlay_utils.go`

**Acceptance Criteria:**
- ✅ Dry-run logs list of overlay files without executing terraform
- ✅ `terraform apply --from-plan` with changed overlays since plan → abort with descriptive error
- ✅ `terraform apply --from-plan` with matching overlays → proceeds normally

---

## Schema Changes

### Component Schema Extension (`pkg/datafetcher/schema/`)

The atmos-manifest JSON schema must be updated in Phase 1 to allow the `overlays:` key on component nodes. Without this change, stack YAML files containing `overlays:` will fail schema validation.

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
          _override: bool # When true, discard all inherited overlay entries
```

The `settings.experimental.stack_overlays` key must also be added to the `settings` schema.

---

## Security Considerations

1. **File injection is sandboxed to the working directory.** Overlay files are only copied into the component's per-execution working directory. Path traversal (e.g., `../../etc/passwd`) is rejected with an error.

2. **Inline content is written as a regular file.** No execution happens outside of the normal terraform workflow.

3. **Cleanup is unconditional.** Even if terraform returns an error or panics, the defer ensures injected files are removed so they don't accidentally persist.

4. **Overlay files do not modify the component source directory.** Files are injected into the per-execution working directory (see Concurrent Execution Safety above).

5. **Denied HCL block types in inline `content:`.** Inline overlay content is parsed before injection. The following top-level HCL block types are **denied** and cause injection to abort with an error:

   | Denied block type | Reason |
   |---|---|
   | `terraform {}` | Could override backend, required_providers, or required_version globally |
   | `provider {}` | Provider configuration must be managed at the component level, not per-stack |
   | `data "terraform_remote_state"` | Remote state access must be explicit in component code, not injected |

   Allowed block types include: `import`, `removed`, `moved`, `resource`, `locals`, `variable`, `output`. This list may be tightened in future versions.

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

### Alternative 3: Extend Existing Hooks Infrastructure (`inject` command type)

**`pkg/hooks/event.go` already defines `before.terraform.plan` and `before.terraform.apply`** — the hook events required for overlay injection exist. A hooks-based approach would add a new `inject` command type alongside the existing `store` command, e.g.:

```yaml
hooks:
  inject-migration:
    events:
      - before.terraform.plan
      - before.terraform.apply
    command: inject
    files:
      - path: "migrations/vpc/import-legacy.tf"
```

**Why this is not adopted for MVP:** Two gaps block it:

1. **Events are not yet fired.** `before.terraform.plan` and `before.terraform.apply` are defined in `event.go` but `RunAll` is not called at those lifecycle points in `internal/exec/terraform_execute_helpers_exec.go` (only `before.terraform.init` and `after.terraform.apply` are wired today).
2. **No `inject` command type exists.** Adding a new command type to the hooks system requires implementing `InjectCommand`, wiring cleanup semantics, and integrating the content-hash comparison for `--from-plan` apply—all non-trivial.

**Future consideration:** Once `before.terraform.plan` and `before.terraform.apply` are fired and a general-purpose `inject` command type is implemented, the `overlays:` feature could be unified under hooks. The overlay system proposed here is designed to be structurally compatible with that future migration: `overlays:` is essentially syntactic sugar for a `before.terraform.plan/apply` hook with `inject` + `cleanup` semantics. See the Open Questions section.

### Alternative 4: Per-Stack Terraform Root Module Override

Allow stacks to specify an entirely different `component_path`. **Rejected:** Too coarse-grained—would require duplicating the entire component directory for a one-file change.

### Alternative 5: Terraform `tfvars` Side-car

Use the existing varfile injection mechanism to pass migration data. **Rejected:** Varfiles are for variable values; they cannot contain resource blocks or import/removed blocks.

### Alternative 6: `generate:` Section (Persistent File Generation)

The [Code Generation PRD](code-generation.md) proposes a `generate:` section that writes files into the component working directory. This is **persistent** — generated files remain on disk and are committed to git or re-generated on each run.

**Key distinction:**

| Feature | `generate:` | `overlays:` (this PRD) |
|---|---|---|
| Lifetime | Persistent (committed to repo or re-generated) | Ephemeral (injected before execution, removed after) |
| Purpose | Rendering context, backend config, auto-generated HCL | One-shot state migrations (import/removed/moved blocks) |
| Commit to git? | Yes (or re-generated each run) | The overlay *source* is committed; the injected copy is not |
| Post-apply cleanup | No | Yes, automatic via defer |
| Hash comparison for `--from-plan` | Not applicable | Required |

`generate:` is the right tool for files that should always be present (e.g., a rendered `backend.tf`). `overlays:` is the right tool for ephemeral, one-time state operations that should not persist in the working directory.

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

- [Component Workdir Provisioner](component-workdir.md) — overlays are injected into the per-execution workdir, regardless of whether the workdir provisioner is enabled (see Concurrent Execution Safety).
- [Source Provisioner](source-provisioner.md) — source downloads happen before overlay injection.
- [Code Generation PRD](code-generation.md) — the `generate:` section also injects files; overlays differ in that they are ephemeral (cleaned up after execution) while generated files are persistent. See Alternative 6.
- [Lifecycle Hooks PRD](hooks-component-scoping.md) — `before.terraform.plan` and `before.terraform.apply` events are defined in `pkg/hooks/event.go` but not yet fired. See Alternative 3.
- [Experimental Features System](experimental-features-system.md) — overlays ship under `settings.experimental.stack_overlays: true`.
- [GitHub Issue: Allow terraform state migration blocks per stack/workspace](https://github.com/cloudposse/atmos/issues/...)

---

## Open Questions

1. **Future unification with hooks.** Once `before.terraform.plan` and `before.terraform.apply` are fired and an `inject` command type exists in the hooks system, should `overlays:` be reimplemented as syntactic sugar over hooks? This would reduce the number of injection mechanisms. Tracked as a future consideration; the current design is structurally compatible.

2. **Should `overlays_cleanup: false` be supported for debugging?** Yes — when debugging a migration, it is useful to inspect the injected files. A global config flag (`overlays_cleanup: false`) and a per-invocation `--no-cleanup-overlays` CLI flag should both be supported.

3. **Should overlay injection be logged at the INFO level?** Yes — users should see a message like `Injecting overlay: overlays/prod-us-east-1-vpc/import-legacy.tf → <workdir>/import-legacy.tf` so they know the overlay was applied.

4. **Should glob patterns be supported in `overlays_dir` lookup?** Future consideration — for MVP, exact directory name matching is sufficient.

5. **Interaction with `terraform plan -out=<planfile>` and the `--from-plan` apply.** The SHA-256 hash of overlay contents is stored alongside the plan file. If overlays change between plan and apply, Atmos aborts with an error (see Plan/Apply Lifecycle section). The hash storage format is defined in Phase 3.
