# PRD: Per-Stack Ephemeral HCL Injection via `generate:` (generate++)

**Status:** Proposed
**Version:** 2.0
**Last Updated:** 2026-03-25
**Author:** rb
**Supersedes:** Stack Overlays approach (v1.1)

---

## Executive Summary

Terraform supports [import blocks](https://developer.hashicorp.com/terraform/language/import),
[removed blocks](https://developer.hashicorp.com/terraform/language/resources/syntax#removing-resources),
and [moved blocks](https://developer.hashicorp.com/terraform/language/modules/develop/refactoring)
as declarative, code-reviewable mechanisms for state migration. These blocks must live in the
component directory and apply to *every* stack that uses the component. Atmos currently provides
no way to inject per-stack `.tf` files into a component's working directory before execution,
making one-off state migrations impractical.

This PRD proposes **generate++**: a backward-compatible extension of the existing `generate:`
section in Atmos stack configuration that adds a `mode: ephemeral` option. An ephemeral generate
entry is injected into the component's per-execution working directory before terraform runs and
cleaned up immediately after — including on failure or signal. All the work is done inside the
existing `generate:` feature; no new YAML key or API surface is introduced.

> **Revision note (v2.0):** An earlier version of this PRD proposed a separate `overlays:` key.
> That approach was rejected in PR review as redundant with `generate:`. v2.0 evolves `generate:`
> instead. The problem statement is unchanged; only the solution changes.

---

## Problem Statement

### Current Pain Points

1. **Import blocks cannot be scoped per stack.** A `migrations.tf` placed in
   `components/terraform/vpc/` applies to every stack that runs `vpc`. The only workarounds are
   manual file operations or CLI commands.

2. **State migrations are not auditable.** When engineers run `terraform import` or
   `terraform state mv` manually, there is no PR review, no CI validation, and no record in git.

3. **One-off state operations are fragile.** Copy-then-delete workflows are error-prone. Files
   are often forgotten in the component directory, left over from prior migrations.

4. **Partial apply failures require manual recovery.** When apply fails mid-way and resources are
   orphaned outside state, there is no clean declarative path to re-import them for the specific
   stack that had the failure.

5. **Component reuse conflict.** A component used across 20 stacks cannot have a `removed` block
   in it without affecting every stack — even those where the resource still exists.

### User Stories

1. **As a platform engineer**, I want to write an import block that only runs for `ue1-prod-vpc`
   without touching the 15 other stacks that use the `vpc` component.

2. **As a developer**, I want to declare a `removed` block for a resource that was deleted in one
   environment but still exists in others, without accidentally destroying state in the other
   environments.

3. **As a CI/CD system**, I want terraform migrations to be reviewed in pull requests alongside
   application code so that state changes are auditable and reversible.

4. **As a team lead**, I want migrations to clean up after themselves automatically so that a
   one-time import block does not linger in git forever.


---

## Why Evolve `generate:` Instead of a New Key

The existing `generate:` section already handles file injection into the component working
directory. Adding a separate `overlays:` key would create two mechanisms that do 90% of the same
thing, doubling maintenance burden and confusing users about which to use.

The delta needed to make `generate:` solve the migration use case is small:

| Property | `generate:` today | `generate:` with mode=ephemeral |
|---|---|---|
| File lifetime | Persistent (committed or re-generated) | Ephemeral (injected before, removed after) |
| Injection target | Component source dir | Per-execution working dir only |
| Trigger | Explicit CLI or auto_generate_files | Before each matching hook event |
| Cleanup | None | Deferred; fires on all exit paths |
| Per-stack scope | No (same file for all stacks) | Yes (defined in stack YAML at component level) |
| Backward compatibility | Unchanged | Default mode=persistent preserves existing behavior |

---

## Solution: generate++

### Core Concept

Every entry in the `generate:` map has a filename key and a value (the content or serialized
data). generate++ adds optional metadata fields to the value object. When the value is a plain
string or a map without the reserved keys, it behaves exactly as today (mode=persistent). When
the value includes `mode: ephemeral`, the file is ephemeral.

### Minimal Example

```yaml
# stacks/ue1-prod.yaml
components:
  terraform:
    vpc:
      generate:
        # Existing behavior (unchanged) — persistent file written to source dir
        "context.auto.tfvars.json":
          namespace: "{{ .vars.namespace }}"
          environment: "{{ .vars.environment }}"

        # New: ephemeral file injected into workdir before plan/apply, removed after
        "import-legacy-vpc.tf":
          mode: ephemeral
          target: workdir
          content: |
            import {
              id = "vpc-0abc123def456"
              to = aws_vpc.main
            }
```

When `atmos terraform apply vpc -s ue1-prod` runs:

1. `context.auto.tfvars.json` is written to the component source directory (existing behavior).
2. `import-legacy-vpc.tf` is staged in a temp directory, then atomically moved into the
   per-execution working directory.
3. A `defer` is registered to remove `import-legacy-vpc.tf` from the workdir.
4. `terraform init / apply` runs. The import block is executed.
5. On exit (success or failure), the defer removes `import-legacy-vpc.tf` from the workdir.

The component source directory is never modified by ephemeral entries.

---

## Configuration Reference

### New Fields on `generate:` Entries

Each value in the `generate:` map may include the following new fields. All are optional;
omitting them preserves current behavior.

#### `mode`

```yaml
mode: persistent   # default — existing behavior; file written to source dir and kept
mode: ephemeral    # new — file injected before events, removed after; requires target: workdir
```

#### `target`

```yaml
target: source     # default — write to component source dir (existing behavior)
target: workdir    # new — inject into per-execution working dir only; required when mode=ephemeral
```

> `target: workdir` is rejected if `mode: persistent` because there is no meaningful workdir for
> a persistent file that must survive across executions.

#### `events`

Which hook points trigger injection. Only used when `target: workdir`.

Default when `mode: ephemeral`:

```yaml
events:
  - before.terraform.plan
  - before.terraform.apply
  - before.terraform.destroy
  - terraform.shell.enter
  - terraform.test.start
```

`before.terraform.init` is intentionally excluded from the default — import/removed/moved blocks
do not affect init. Include it explicitly if your use case requires it.

#### `cleanup`

```yaml
cleanup: on_exit   # default for mode=ephemeral — deferred removal fires on all exit paths
cleanup: never     # keep the file in workdir after execution (for debugging only)
```

#### `bind_to_planfile`

```yaml
bind_to_planfile: false   # default
bind_to_planfile: true    # write a sidecar manifest next to the plan for from-plan validation
```

When `true` and `atmos terraform plan -out=<planfile>` is used, Atmos writes
`<planfile>.generated-manifest.json` alongside the plan file. At apply-from-plan time Atmos
re-reads the manifest and verifies that the content hashes still match before injecting.

#### `policy`

Controls which HCL block types are permitted in generated content. The policy is validated at
staging time using the Go `github.com/hashicorp/hcl/v2` AST parser — before any file is written
to the workdir and before terraform is invoked.

```yaml
policy:
  allowed_blocks:
    - import
    - removed
    - moved
    - resource
    - data
    - locals
    - variable
    - output
  denied_top_level_blocks:
    - terraform   # prevents overriding backend/required_providers
    - provider    # provider config belongs in the component source
  denied_data_sources:
    - terraform_remote_state   # remote state access must be explicit in component code
  fail_on_missing: true   # hard error if path/dir is missing (default true)
```

**Denied top-level block types (default):**

| Block type | Reason |
|---|---|
| `terraform` | Could override backend, `required_providers`, or `required_version` for the execution |
| `provider` | Provider configuration must live in the component source |

**Denied data source types (default):**

| Data source | Reason |
|---|---|
| `data "terraform_remote_state"` | Remote state access must be explicit in component code |

All other `data {}` source types are allowed. A policy violation is a hard Atmos error that
aborts before terraform is invoked.

#### `logging`

```yaml
logging:
  mask_logs: true   # default — route DEBUG content through secrets masker before emitting
```

#### `content`, `path`, `dir`

Specify the file content. Exactly one must be provided for generate++ entries:

```yaml
# Inline HCL (Go template supported)
content: |
  import {
    id = "{{ .vars.legacy_vpc_id }}"
    to = aws_vpc.main
  }

# Path to a .tf file (resolved relative to atmos.yaml base_path)
path: "migrations/vpc/import-legacy-vpc.tf"

# Directory of .tf files (all files in the directory are injected)
dir: "migrations/vpc/"
```

### Full Entry Syntax

```yaml
components:
  terraform:
    vpc:
      generate:
        "<filename>.tf":
          # Content (choose one)
          content: |
            import { ... }
          # path: "..."
          # dir:  "..."

          # Lifecycle
          mode: ephemeral       # ephemeral | persistent (default: persistent)
          target: workdir       # workdir | source (default: source; workdir required for ephemeral)
          cleanup: on_exit      # on_exit | never (default: on_exit for ephemeral)
          events:
            - before.terraform.plan
            - before.terraform.apply
            - before.terraform.destroy
            - terraform.shell.enter
            - terraform.test.start

          # Plan/Apply validation
          bind_to_planfile: false

          # Content policy
          policy:
            allowed_blocks: [import, removed, moved, resource, data, locals, variable, output]
            denied_top_level_blocks: [terraform, provider]
            denied_data_sources: [terraform_remote_state]
            fail_on_missing: true

          # Logging
          logging:
            mask_logs: true
```

### Backward Compatibility

The following existing syntaxes continue to work unchanged (implicitly
`mode: persistent, target: source`):

```yaml
generate:
  # String value — literal template (existing)
  "backend.tf": |
    terraform {
      backend "s3" {
        bucket = "{{ .backend.bucket }}"
      }
    }

  # Map value — serialized as JSON/YAML based on file extension (existing)
  "context.auto.tfvars.json":
    namespace: "{{ .vars.namespace }}"
    environment: "{{ .vars.environment }}"
```

A map value is interpreted as generate++ only when it contains at least one of these reserved
keys: `mode`, `target`, `events`, `cleanup`, `bind_to_planfile`, `policy`, `logging`, `content`,
`path`, `dir`. Otherwise it is treated as serialization data (existing behavior).

---

## Inheritance and Merge Semantics

generate++ entries participate in the standard Atmos five-level deep merge (global, component
type, base component, component, overrides). Entries merge by filename key; a higher-priority
entry completely replaces a lower-priority entry with the same filename.

### Stack-Level Scoping

Because the `generate:` section lives inside the component definition within a stack YAML file,
ephemeral entries are automatically scoped to the stack that defines them:

```yaml
# stacks/ue1-prod.yaml — ephemeral entry scoped to this stack only
components:
  terraform:
    vpc:
      generate:
        "import-legacy-vpc.tf":
          mode: ephemeral
          target: workdir
          content: |
            import { id = "vpc-0abc123", to = aws_vpc.main }
```

Other stacks that use the `vpc` component do not see `import-legacy-vpc.tf`.

### Multi-Stack Scope via Catalog Inheritance

To run the same ephemeral file across a group of stacks, define it in a catalog base component:

```yaml
# catalog/vpc-base.yaml
components:
  terraform:
    vpc-base:
      generate:
        "import-shared.tf":
          mode: ephemeral
          target: workdir
          path: "migrations/shared/import-shared.tf"
```

> **Caution — future/undeployed stacks:** A catalog-level ephemeral entry runs for every stack
> that inherits the base component, including stacks not yet deployed. If the entry contains an
> `import {}` block for a resource that does not exist in a given stack's state, terraform will
> fail for that stack. Use catalog-level entries only when the migration applies uniformly to all
> existing deployed stacks, or scope entries to individual stacks.

---

## Execution Lifecycle

```
Terraform Execution with generate++ Ephemeral Entries

  1. Load component config (resolve stack, component, workspace)
  2. Run persistent generate: entries -> write to source dir
  3. Resolve ephemeral generate: entries for the current event
       a. Filter entries where mode=ephemeral AND event matches
       b. Resolve content (inline/path/dir) + render template
       c. Validate content against policy (HCL AST parse)
       d. Stage all files to temp dir
       e. Atomically move staged files into per-execution workdir
       f. Register defer to remove injected files on all exits
  4. terraform init / plan / apply / destroy / test
  5. Deferred cleanup: remove injected files from workdir
     (only files injected by Atmos; pre-existing files untouched)
```

### Injection Atomicity

Injection into the workdir is performed atomically to prevent partial states:

1. All resolved files are **staged** in a temp directory
   (`.atmos/workdir/.generate-staging-<uuid>/`).
2. Once all files stage successfully, they are **moved** as a batch into the workdir.
3. If any staging step fails (content read error, policy violation, disk full), the staging
   directory is removed and the workdir is **not modified**. Atmos aborts with a non-zero exit
   code and prints the failing filename and reason. Terraform **does not run**.

The cleanup defer is registered immediately after the atomic move succeeds — before
`terraform init` is invoked — ensuring cleanup fires even if `init` fails (e.g., backend
provisioning error).

---

## Concurrent Execution Safety

Ephemeral entries **always** target the per-execution working directory, never the component
source directory. The source directory (`components/terraform/vpc/`) is shared across all
concurrent executions.

```
Stack A: vpc -s ue1-prod  ->  .atmos/workdir/ue1-prod-vpc/   (ephemeral files A injected here)
Stack B: vpc -s ue1-dev   ->  .atmos/workdir/ue1-dev-vpc/    (ephemeral files B injected here)
Source:  components/terraform/vpc/                           (never modified by ephemeral entries)
```

---

## Plan/Apply Lifecycle

### `terraform plan` (plan-only)

1. Resolve and inject ephemeral entries for `before.terraform.plan`.
2. Run `terraform plan`.
3. Cleanup (defer).

Import, removed, and moved blocks appear in plan output, giving reviewers full visibility.

### `terraform apply` (direct, no plan file)

1. Resolve and inject ephemeral entries for `before.terraform.apply`.
2. Run `terraform apply`.
3. Cleanup (defer).

### `terraform apply --from-plan` (apply from saved plan file)

When `bind_to_planfile: true` is set on any ephemeral entry, Atmos writes a sidecar manifest at
plan time and verifies it at apply time:

1. Re-resolve ephemeral entries for `before.terraform.apply`.
2. Compare content hashes against the sidecar manifest.
3. If any hash differs, **abort** — the entries changed since the plan was generated.
4. If hashes match, inject and run `terraform apply -input=false <planfile>`.
5. Cleanup (defer).

If the sidecar is absent at apply time (e.g., plan generated before this feature), Atmos logs a
warning and skips hash verification for backward compatibility.

#### Sidecar Manifest Format

Written to `<planfile>.generated-manifest.json`:

```json
{
  "schema_version": 1,
  "atmos_version": "1.x.y",
  "created_at": "2026-03-25T00:00:00Z",
  "stack": "ue1-prod",
  "component": "vpc",
  "entries_hash": "sha256:<hex>",
  "entries": [
    {
      "name": "import-legacy-vpc.tf",
      "source_type": "inline",
      "source_path": "stack:ue1-prod:vpc:inline",
      "sha256": "<hex>"
    }
  ]
}
```

`entries_hash` is the SHA-256 of the newline-joined `<name>:<sha256>` pairs, sorted by `name`,
with no trailing newline. This is the value compared at apply time.

### `terraform destroy`

Same as `terraform apply` (direct). Ephemeral entries for `before.terraform.destroy` are injected.
For most destroy operations no entries will be present, but
`removed { lifecycle { destroy = false } }` blocks are a valid use case.

### `atmos terraform shell`

Ephemeral entries for `terraform.shell.enter` are injected before the shell is handed to the
user. Cleanup is deferred to shell exit. Injected files are logged so the user knows what is
present.

### `atmos terraform test`

Ephemeral entries for `terraform.test.start` are injected before `terraform test` runs. Cleanup
is deferred.

### Command Coverage Summary

| Command | Inject before | Clean up after |
|---|---|---|
| `atmos terraform plan` | yes (`before.terraform.plan`) | yes (defer) |
| `atmos terraform apply` (direct) | yes (`before.terraform.apply`) | yes (defer) |
| `atmos terraform apply --from-plan` | yes + hash check | yes (defer) |
| `atmos terraform destroy` | yes (`before.terraform.destroy`) | yes (defer) |
| `atmos terraform shell` | yes (`terraform.shell.enter`) | yes on shell exit |
| `atmos terraform test` | yes (`terraform.test.start`) | yes (defer) |
| `atmos terraform init` (standalone) | no-op | no-op |

---

## OpenTofu Compatibility

generate++ works with both the `terraform` (HashiCorp Terraform 1.5+) and `tofu` (OpenTofu 1.6+)
binaries. Both support `import {}`, `removed {}`, and `moved {}` blocks.

The binary used is determined by the component `command` field (defaults to `terraform`). File
injection is binary-agnostic — files are staged and moved before `init` regardless of binary.

---

## Path Resolution

### `path:` and `dir:` Resolution

Paths follow the same rules as the existing `generate:` section:

- Bare paths (no leading `./`) are resolved relative to `atmos.yaml` `base_path`.
- `./`-prefixed paths are resolved relative to the current working directory.

```yaml
generate:
  "import-legacy.tf":
    mode: ephemeral
    target: workdir
    path: "migrations/vpc/import-legacy.tf"  # -> <base_path>/migrations/vpc/import-legacy.tf
```

### Injection Target

```
Read from:   <base_path>/migrations/vpc/import-legacy.tf
Written to:  <workdir>/import-legacy.tf
Never:       <base_path>/components/terraform/vpc/import-legacy.tf  (source dir never touched)
```

Workdir path resolves to `.atmos/workdir/<stack>-<component>/`. If the workdir provisioner is
not enabled, Atmos creates a temporary execution directory at that path.

---

## Use Cases

### Use Case 1: Import Existing Resources (per-stack)

```yaml
# stacks/ue1-prod.yaml
components:
  terraform:
    vpc:
      generate:
        "import-legacy-vpc.tf":
          mode: ephemeral
          target: workdir
          content: |
            import {
              id = "vpc-0abc123def456"
              to = aws_vpc.main
            }
            import {
              id = "subnet-0abc123"
              to = aws_subnet.private["us-east-1a"]
            }
```

Run once, then remove the entry from git in a follow-up PR.

### Use Case 2: Remove Resources from State Without Destroying

```yaml
# stacks/legacy-dev.yaml
components:
  terraform:
    eks:
      generate:
        "remove-old-node-group.tf":
          mode: ephemeral
          target: workdir
          content: |
            removed {
              from = aws_eks_node_group.legacy
              lifecycle {
                destroy = false
              }
            }
```

### Use Case 3: State Move After Refactor (from a shared file)

```yaml
components:
  terraform:
    rds:
      generate:
        "move-renamed.tf":
          mode: ephemeral
          target: workdir
          path: "migrations/rds/move-renamed.tf"
          bind_to_planfile: true   # verify hash at apply time
```

### Use Case 4: Template-Driven Import Block

The `content:` field supports Go templates with full component context:

```yaml
components:
  terraform:
    vpc:
      vars:
        legacy_vpc_id: "vpc-0abc123def456"
      generate:
        "import-legacy-vpc.tf":
          mode: ephemeral
          target: workdir
          content: |
            import {
              id = "{{ .vars.legacy_vpc_id }}"
              to = aws_vpc.main
            }
```

---

## `atmos.yaml` Global Configuration

```yaml
components:
  terraform:
    # Whether to clean up ephemeral generate files after execution (default: true)
    # Set to false for debugging; also overridable per-invocation with --no-cleanup-generate
    ephemeral_generate_cleanup: true

settings:
  # Ships as an experimental feature in Phase 1.
  # Without this flag, ephemeral entries are parsed but injection is skipped with a warning.
  experimental:
    ephemeral_generate: true
```

---

## Implementation Plan

### Phase 1: Ephemeral Injection into Workdir (MVP)

**Goal:** Enable `mode: ephemeral, target: workdir` with event-based injection, policy
validation, deferred cleanup, and structured logging.

**Schema changes (`pkg/schema/schema.go`):**

```go
type GenerateEntry struct {
    // Content (one of)
    Content string `yaml:"content,omitempty"`
    Path    string `yaml:"path,omitempty"`
    Dir     string `yaml:"dir,omitempty"`

    // Lifecycle
    Mode    string   `yaml:"mode,omitempty"`    // "persistent" | "ephemeral"
    Target  string   `yaml:"target,omitempty"`  // "source" | "workdir"
    Events  []string `yaml:"events,omitempty"`
    Cleanup string   `yaml:"cleanup,omitempty"` // "on_exit" | "never"

    // Plan/apply
    BindToPlanfile bool `yaml:"bind_to_planfile,omitempty"`

    // Policy
    Policy *GeneratePolicy `yaml:"policy,omitempty"`

    // Logging
    Logging *GenerateLogging `yaml:"logging,omitempty"`
}

type GeneratePolicy struct {
    AllowedBlocks        []string `yaml:"allowed_blocks,omitempty"`
    DeniedTopLevelBlocks []string `yaml:"denied_top_level_blocks,omitempty"`
    DeniedDataSources    []string `yaml:"denied_data_sources,omitempty"`
    FailOnMissing        bool     `yaml:"fail_on_missing"`
}

type GenerateLogging struct {
    MaskLogs bool `yaml:"mask_logs"`
}
```

**New execution helpers (`internal/exec/generate_ephemeral.go`):**

- `resolveEphemeralEntries(atmosConfig, info, event)` — Filters generate entries for current event.
- `stageEphemeralFiles(entries, stagingDir)` — Renders templates, validates policy, writes staging.
- `atomicMoveToWorkdir(stagingDir, workdir)` — Batch-moves staged files into workdir.
- `cleanupEphemeralFiles(workdir, injected)` — Removes only the injected files.

**Wiring (`internal/exec/terraform_execute_helpers_exec.go`):**

- Before each matching event in `runPreExecutionSteps`, call resolution + staging + move pipeline.
- Register `defer cleanupEphemeralFiles(...)` immediately after the atomic move.

**Files changed:**

- `pkg/schema/schema.go`
- `internal/exec/generate_ephemeral.go` (new)
- `internal/exec/terraform_execute_helpers_exec.go`
- `pkg/datafetcher/schema/` (atmos-manifest JSON schema — add new generate entry fields)

**Acceptance criteria:**

- `mode: ephemeral, target: workdir` entry injected before `before.terraform.plan`.
- File removed from workdir after execution (success and failure).
- Injection failure (policy violation, missing path) aborts before terraform runs.
- Component source dir is never modified by ephemeral entries.
- Concurrent stacks use isolated workdirs.
- Feature gated by `settings.experimental.ephemeral_generate: true`.
- Existing `generate:` behavior (string/map values without new fields) unaffected.

### Phase 2: Plan/Apply Hash Verification

**Goal:** Enable `bind_to_planfile: true` with sidecar manifest generation and apply-time hash
verification.

**Acceptance criteria:**

- `atmos terraform plan -out=tfplan.bin` writes `tfplan.bin.generated-manifest.json`.
- Apply-from-plan with changed ephemeral entries aborts with diff summary.
- Apply-from-plan with missing sidecar logs warning and proceeds (backward compat).

### Phase 3: CLI Introspection

**Goal:** Allow operators to inspect which ephemeral entries would be injected.

**Commands:**

- `atmos terraform generate files --dry-run --show-ephemeral` — logs ephemeral entries without
  executing terraform.
- `atmos describe component vpc -s ue1-prod --show-generate` — shows resolved generate section
  including ephemeral entries and which events they match.

---

## Schema Changes

### JSON Schema (`pkg/datafetcher/schema/`)

The atmos-manifest JSON schema must be updated in Phase 1 to allow the new generate entry fields.
Without this change, stack YAML files using generate++ fields will fail schema validation.

The `settings.experimental.ephemeral_generate` key must also be added to the `settings` schema.

---

## Observability and Audit Logging

Ephemeral generate injection and cleanup events are emitted as structured log entries at `INFO`
level using Atmos's standard structured log format.

### Log Fields

| Field | Type | Description |
|---|---|---|
| `generate_name` | string | Filename as it appears in the workdir |
| `source_type` | enum | `inline`, `path`, or `dir` |
| `source_path` | string | Resolved path or `stack:<stack>:<component>:inline` |
| `stack` | string | Atmos stack slug |
| `component` | string | Component name |
| `event` | enum | `inject` or `cleanup` |
| `mode` | string | `ephemeral` |
| `timestamp` | string | RFC 3339 |

### Example Log Output

```
INFO  generate inject   generate_name=import-legacy-vpc.tf  source_type=inline  stack=ue1-prod  component=vpc
INFO  generate cleanup  generate_name=import-legacy-vpc.tf  stack=ue1-prod  component=vpc
```

### Secrets Masking

Ephemeral generate content emitted in log output passes through Atmos's Gitleaks secrets masking
pipeline. Inline `content:` blocks may embed resource IDs or attribute values derived from
sensitive stack variables.

| Log level | Content emitted | Masking applied |
|---|---|---|
| `INFO` | Event fields only (no file content) | N/A |
| `DEBUG` | Filename + first 256 bytes of content | Required — passes through Gitleaks pipeline |
| `TRACE` | Full raw content | Masking disabled; operators accepting this risk must opt in |

---

## Security Considerations

1. **File injection is sandboxed to the working directory.** Path traversal (e.g.,
   `../../etc/passwd`) in `path:` values is rejected with a hard error.

2. **Policy validation blocks dangerous HCL before injection.** The `terraform {}` and `provider
   {}` block types are denied by default to prevent backend or provider override attacks.

3. **Cleanup is unconditional.** Go `defer` ensures files are removed even if terraform panics or
   the process receives a signal. The only exception is `cleanup: never`, which is for debugging
   only and must never be set permanently in CI.

4. **Source directory is never modified.** The component source directory is only read, never
   written, by ephemeral generate entries.

5. **Content templating is sandboxed.** Templates run with the same function map as existing
   `generate:` templates. No additional functions are exposed by ephemeral entries.

---

## Comparison with Kustomize

A common analogy is Kustomize, which patches Kubernetes YAML manifests per environment:

| Concern | Kustomize | generate++ (ephemeral) |
|---|---|---|
| Target format | YAML (Kubernetes manifests) | HCL (Terraform resource blocks) |
| Merge strategy | Strategic merge, JSON patch | Files injected wholesale; no HCL field merging |
| Persistence | Output committed to repo | Injected files ephemeral — removed after execution |
| Scope of use | All resource configuration | Primarily state migration blocks |
| Surface area | Separate tool | Extension of existing `generate:` feature |

**Scope-creep warning:** Because ephemeral entries inject raw `.tf` files, there is no technical
barrier preventing injection of `resource` blocks or provider overrides. This is **strongly
discouraged**. Files that introduce general-purpose terraform changes outside state migrations
make component behavior environment-dependent in a hidden way, making debugging significantly
harder. The intended use is strictly:

- `import {}` blocks (state import)
- `removed {}` blocks (state removal / lifecycle protection)
- `moved {}` blocks (state address rename)

For environment-specific resource configuration, use Atmos `vars:` and component inheritance.

---

## Success Metrics

1. Teams can perform import block migrations without manually copying files.
2. Migration files are committed to git and appear in PRs for review.
3. Zero regression: existing `generate:` entries (string/map without new fields) are unaffected.
4. Ephemeral cleanup is 100% reliable (defer-based, tested with forced errors).

---

## Test Plan

### Unit Tests

- `resolveEphemeralEntries`: event match, event no-match, mixed persistent+ephemeral entries.
- `stageEphemeralFiles`: inline content, path-based, dir-based, policy violation produces error.
- `atomicMoveToWorkdir`: moves all files; source dir untouched.
- `cleanupEphemeralFiles`: removes exactly the injected files, not pre-existing files.
- Hash sidecar format and verification logic.

### Integration Tests

- `atmos terraform plan vpc -s ue1-prod` with ephemeral inline entry — plan output includes import.
- Ephemeral file absent from workdir after plan.
- Policy violation in `content:` — Atmos error before terraform runs, workdir unmodified.
- `bind_to_planfile: true` — sidecar written at plan; apply with changed entry aborts.
- **Injection failure aborts execution:** missing `path:` produces non-zero exit, no injected
  files in workdir.
- **Concurrent workdir isolation:** run `atmos terraform plan vpc -s ue1-prod` and
  `atmos terraform plan vpc -s ue1-dev` concurrently. Assert that each workdir contains only its
  stack-specific ephemeral files and that all injected files are removed after both plans complete.
- **Existing generate: behavior regression:** entries without new fields write to source dir
  (not workdir) and are not cleaned up after execution.

### Test Fixtures

```
tests/fixtures/scenarios/ephemeral-generate/
├── atmos.yaml
├── migrations/
│   └── vpc/
│       └── import-legacy.tf
├── stacks/
│   ├── ue1-prod.yaml
│   └── ue1-dev.yaml
└── components/
    └── terraform/
        └── vpc/
            ├── main.tf
            └── variables.tf
```

---

## CI/CD Integration

### Recommended Pipeline Flow

```
PR opened/updated
        |
        v
atmos terraform plan <component> -s <stack>
  (ephemeral entries injected; plan includes import/removed blocks)
        |
        v
Plan output reviewed in PR (Atlantis / GitHub Actions CI summary)
        |
        v
PR approved and merged
        |
        v
atmos terraform apply <component> -s <stack>
  (ephemeral entries re-injected; apply executes; entries cleaned up)
        |
        v
Migration complete -> remove generate entry from git in follow-up PR
```

### Plan-Apply Pipelines (`bind_to_planfile`)

```yaml
# GitHub Actions example
- name: Plan
  run: atmos terraform plan vpc -s ue1-prod -- -out=tfplan.bin
- name: Upload plan artifacts
  uses: actions/upload-artifact@v4
  with:
    name: tfplan
    path: |
      tfplan.bin
      tfplan.bin.generated-manifest.json   # must travel with the plan
```

### Cleanup in CI

Cleanup runs unconditionally via `defer` including on non-zero terraform exit codes. CI jobs do
not need a cleanup step; injected files are never left in the runner workspace.

### Debugging (`--no-cleanup-generate`)

Setting `--no-cleanup-generate` (or `ephemeral_generate_cleanup: false` in `atmos.yaml`)
preserves injected files after execution. This must never be set permanently in CI.

---

## Alternatives Considered

### Alternative 1: Separate `overlays:` Key (v1.1 of this PRD)

The initial design proposed a dedicated `overlays:` key in component stack YAML with convention-
based directory lookup (`overlays/<stack-slug>/`). **Rejected** because:

- It introduces a second file-injection surface alongside `generate:`, doubling maintenance
  burden and creating user confusion.
- All required behavior can be provided by adding a small number of fields to existing
  `generate:` entries.
- The `generate:` map structure is already well-understood by Atmos users.

### Alternative 2: Convention-Based `overlays/` Directory in Component

Drop `.tf` files in `components/terraform/vpc/overlays/<stack-slug>/`. **Rejected** in favor of
generate++ because convention-based detection adds ambiguous scanning rules, is harder to test
and lint, and is not visible in `atmos describe component`.

### Alternative 3: Extend Hooks Infrastructure (`inject` command type)

`pkg/hooks/event.go` already defines `before.terraform.plan` and `before.terraform.apply`.
A hooks-based `inject` command type could be added. **Not adopted for MVP** because those events
are defined but not yet fired, implementing `InjectCommand` is non-trivial, and generate++ is
structurally compatible with future hooks unification.

### Alternative 4: Atmos Custom Commands (Structured Tasks)

Multi-step custom commands (copy -> terraform -> cleanup). **Rejected** because cleanup is not
guaranteed across process boundaries (SIGTERM, OOM kill), stack context (workdir path) is not
automatically available inside command steps, and there is no atomicity guarantee.

### Alternative 5: Per-Stack Terraform Root Module Override

Allow stacks to specify a different `component_path`. **Rejected:** Too coarse-grained; requires
duplicating the entire component directory for a one-file change.

### Alternative 6: `tfvars` Side-car

Use the existing varfile injection mechanism. **Rejected:** Varfiles are for variable values;
they cannot contain resource blocks or import/removed blocks.

---

## Related Work

- [Code Generation PRD](code-generation.md) — defines the `generate:` section that this PRD extends.
- [Component Workdir Provisioner](component-workdir.md) — ephemeral entries target the per-execution workdir.
- [Source Provisioner](source-provisioner.md) — source downloads happen before ephemeral injection.
- [Lifecycle Hooks PRD](hooks-component-scoping.md) — future unification target for ephemeral entries.
- [Experimental Features System](experimental-features-system.md) — ships under `settings.experimental.ephemeral_generate: true`.
- [GitHub Issue #673](https://github.com/cloudposse/atmos/issues/673) — Allow terraform state migration blocks per stack/workspace.

---

## Open Questions

1. **Future unification with hooks.** Once `before.terraform.plan` and `before.terraform.apply`
   are fired, should ephemeral generate entries be reimplemented as hooks with `inject + cleanup`
   semantics? The current design is structurally compatible with that migration.

2. **Should glob patterns be supported in `dir:` lookup?** Future consideration — for MVP, all
   `.tf` files in the specified directory are included.

3. **Should `events:` support wildcards?** E.g., `before.terraform.*` to match all terraform
   pre-execution events. Future consideration.
