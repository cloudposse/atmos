# Migrating from Terramate

This reference is the agent's decision guide for users coming from a Terramate project (`.tm.hcl`
files, `stack.tm.hcl`, `generate_hcl`). For the full user-facing prose tutorial, see
[atmos.tools/migration](https://atmos.tools/migration) (no Terramate-specific tutorial exists yet;
this reference is currently the primary source). This reference assumes Terraform/OpenTofu is the
underlying IaC tool, as in the common Terramate quickstart shape.

## Recognizing a Terramate Project

| Signal in the repo                                                 | Terramate construct                   |
|----------------------------------------------------------------------|----------------------------------------|
| Any `*.tm.hcl` file                                                   | Terramate config file (any kind)       |
| A `stack {}` block in any `.tm.hcl` file (conventionally `stack.tm.hcl`) | Stack definition                       |
| `terramate { config { ... } }` block                                 | Root Terramate config                  |
| `globals "a" "b" { }` blocks                                         | Namespaced globals                     |
| `generate_hcl "file.tf" { ... }`                                     | Code generator                         |
| `script "name" { job { commands = [...] } }`                         | Orchestration script                   |
| `import { source = "..." }`                                          | Terramate import                       |
| `.tmtriggers/` directory                                             | CLI-managed change-detection override  |
| `tm_*` function calls (`tm_try`, `tm_contains`, `tm_dynamic`, ...)    | Terramate templating functions         |

If any of these are present, this is a Terramate migration -- use this reference, not
[from-native-terraform.md](from-native-terraform.md) or
[from-terraform-workspaces.md](from-terraform-workspaces.md).

## Construct Mapping Overview

| Terramate construct | Atmos equivalent | Parity |
|---|---|---|
| `stack.tm.hcl` | Stack manifest + component instance | Full (conceptual shift) |
| `globals "a" "b" {}` | `vars:`/`locals:` deep-merge inheritance | Full (superset) |
| `generate_hcl` mixins (backend/provider/kubernetes) | `backend:`/`providers:` stack sections | Full |
| `generate_hcl` generators (module wiring) | Component `vars:` + vendored/sourced component | Full (different mental model) |
| `script { job { commands } }` | `workflows:` + `commands:` + `dependencies.components` | Partial -- 3-way decomposition |
| Tags/labels (`tags=[...]`, `--tags`) | `metadata.tags`/`metadata.labels` + stack-wide defaults + `--tags`/`--labels` CLI flags | Full |
| `after = ["tag:x"]` dependency ordering | `dependencies.components[].name` (the only way to declare the edge) + `--include-dependencies`/`--include-dependents` to expand a seed through it | Partial |
| `tm_*` functions | Go templates (Sprig/Gomplate/`atmos.*`) + YAML functions incl. `!tags`/`!labels`/`!labels.keys`/`!labels.values` | Full (superset) |
| Terramate Cloud sync flags | Atmos Pro `settings.pro` | Full (GitHub-centric today) |
| `_bootstrap/` two-phase pattern | One-time bootstrap component/stack, run outside the dependency graph | Full (pattern) |
| `.tmtriggers/` ignore-change records | *(none)* | **Gap** |
| `terramate clone`/`terramate create --tags` | `atmos scaffold generate` (first-class templating) + catalog import for env cloning | Full |

## Recipe: `stack.tm.hcl` → Stack Manifest

**Before:**
```hcl
# stacks/terraform/envs/stg/vpc/stack.tm.hcl
stack {
  name        = "vpc-stg"
  description = "Staging VPC"
  id          = "db0aac90-33e0-48e2-a5b1-5b680b8b2749"  # backend state key
  tags        = ["vpc"]
}
```

**After:**
```yaml
# stacks/plat-ue2-stg.yaml
components:
  terraform:
    vpc:
      metadata:
        description: "Staging VPC"
        tags: [vpc]
      vars:
        cidr: "10.0.0.0/16"
```

Atmos's stack concept is **not filesystem-bound**. A Terramate stack directory typically becomes
one component instance inside a named Atmos stack (org/tenant/account/region/stage), not a 1:1
directory mapping -- do not create one Atmos stack file per Terramate directory by default.

**Preserving existing state:** Terramate's `id` is stack metadata (used for Cloud sync) that's
commonly interpolated into a `generate_hcl` backend template, e.g.
`key = "terraform/stacks/by-id/${terramate.stack.id}/terraform.tfstate"` -- the UUID is only one
piece of the full generated key, not the key itself. Atmos derives its backend key from stack
context by default. To keep reading/writing the same state file after migration, inspect the
actual backend mixin template and set the Atmos backend `key`/`workspace_key_prefix` explicitly to
match the *full* existing path (prefix, UUID, and filename) exactly -- this is the same "must match
exactly" risk pattern documented in
[from-terraform-workspaces.md](from-terraform-workspaces.md#path-1-keep-the-workspace-state-easiest).

## Recipe: Globals → `vars:`/`locals:`

**Before:**
```hcl
globals "vpc" {
  vpc_name = "vpc-${global.terraform.env}"
  cidr     = "10.0.0.0/16"
}
```

**After:**
```yaml
vars:
  vpc_name: "vpc-{{ .vars.environment }}"
  cidr: "10.0.0.0/16"
```

Terramate's `globals` are namespaced and hierarchically deep-merged root -> env -> stack, exactly
like Atmos stack `vars:`/`locals:` inheritance via `import:`. Atmos is a superset here: use
`!terraform.output`/`!terraform.state`/`atmos.Component()` for cross-stack reads that Terramate
globals cannot express natively (Terramate needs its own remote-state wiring for that).

## Recipe: `generate_hcl` Mixins → `backend:`/`providers:` Sections

**Before (`imports/mixins/backend.tm.hcl`):**
```hcl
generate_hcl "backend.tf" {
  content {
    terraform {
      backend "s3" {
        region = global.terraform.backend.region
        bucket = global.terraform.backend.bucket
        key    = "terraform/stacks/by-id/${terramate.stack.id}/terraform.tfstate"
        encrypt = true
      }
    }
  }
}
```

**After:**
```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: terraform-state
      region: us-east-1
```

Atmos auto-generates `backend.tf.json` from `terraform.backend`/`backend_type` at plan/apply
time -- no hand-written `generate_hcl` block needed. `imports/mixins/kubernetes.tm.hcl` (and any
other provider-block wiring) collapses into the stack-level `providers:` section, which Atmos
generates into `providers_override.tf.json`. `imports/mixins/terraform.tm.hcl`'s
`required_version`/`required_providers` pins are a separate concern -- they map to the stack-level
`terraform.required_version`/`terraform.required_providers` sections, generated into a *different*
file, `terraform_override.tf.json`. See `website/docs/stacks/backend.mdx` and
`website/docs/stacks/providers.mdx`.

## Recipe: `generate_hcl` Generators (Module Wiring) → Component Instantiation

This is a mental-model shift, not a syntax swap. Terramate hand-templates a `module "vpc" { ... }`
block per stack via `tm_dynamic`/`global.*` interpolation. Atmos instead has the module *be* the
component (`components/terraform/vpc/`), and the stack manifest only supplies `vars:`.

**Before (`imports/generators/v1/generate_vpc.tm.hcl`):**
```hcl
generate_hcl "main.tf" {
  condition = global.generators.version == "v1"
  stack_filter {
    project_paths = ["**/stacks/terraform/envs/*/vpc"]
  }
  content {
    module "vpc" {
      source  = "terraform-aws-modules/vpc/aws"
      version = "5.19.0"
      name    = global.vpc.vpc_name
      cidr    = global.vpc.cidr
    }
  }
}
```

**After:**
```hcl
# components/terraform/vpc/main.tf
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.19.0"
  name    = var.vpc_name
  cidr    = var.cidr
}
```
```yaml
# stack manifest
components:
  terraform:
    vpc:
      vars:
        vpc_name: "vpc-stg"
        cidr: "10.0.0.0/16"
```

**Staged generator-version rollout:** Terramate's `generators/v1` + `v2` directories, gated by
`condition = global.generators.version == "vN"` and a second `import` line, are Atmos's version
management problem. Two options: (a) versioned component folders (`vpc/v1`, `vpc/v2`, per
[folder-based versioning](https://atmos.tools/design-patterns/version-management/folder-based-versioning))
selected per-stack via `metadata.component: vpc/v2` -- use for a breaking module rewrite; (b) a
per-stack `source`/version override on the same component -- use for a plain version bump. Prefer
(a) when the module's variable contract changes; use `metadata.name: vpc` alongside it to keep the
backend `workspace_key_prefix` stable across the version bump (see
[Configure Component Metadata](https://atmos.tools/stacks/components/component-metadata#name)).

## Recipe: `script{}` → Three-Way Decomposition

Terramate's `script{}` bundles job commands, filtering, and Terramate Cloud sync into one
construct. Atmos splits the same job across three primitives:

| Script does... | Atmos primitive |
|---|---|
| `terramate list --changed` | `atmos list affected` (human-facing table/JSON/YAML/CSV/tree view) or `atmos describe affected` (deep, machine-readable output for scripting/CI) |
| `after`/`before`/`wants` stack ordering | `dependencies.components[]` |
| `--parallel N` | `--max-concurrency N` |
| Named multi-step job (`init` -> `plan` -> `sync_preview`) | `workflows:` (`atmos workflow <name>` is already the direct equivalent of `terramate script run -- <name>`) |
| User-invoked named command needing a shorter top-level verb | `commands:` (custom CLI command) wrapping the `workflows:` entry above |
| `sync_preview`/`sync_deployment`/`sync_drift_status` | Atmos Pro `settings.pro` |

**Before:**
```hcl
script "deploy" {
  job {
    commands = [
      ["terraform", "validate"],
      ["terraform", "plan", "-out", "out.tfplan", "-lock=false"],
      ["terraform", "apply", "-input=false", "-auto-approve", "out.tfplan", {
        sync_deployment = true
      }],
    ]
  }
}
```

**After:**
```yaml
# workflows/deploy.yaml
workflows:
  deploy:
    steps:
      - type: atmos
        command: terraform validate vpc
      - type: atmos
        command: terraform deploy vpc
```
```bash
atmos workflow deploy -s stg
```

The DAG scheduler behind `dependencies.components` and `--max-concurrency` is actively evolving
(see `docs/prd/dag-concurrent-execution.md`, still Draft) -- flag cross-tool/cross-workflow
ordering as evolving, not fully finished parity, when a migration depends on it.

## Recipe: Tags/Labels → `metadata.tags`/`metadata.labels`

Atmos ships native component tags and labels (component JSON Schema, `--tags`/`--labels` CLI
flags, stack-wide defaults, and `!tags`/`!labels` YAML functions) -- this is real, shipped syntax,
not a workaround.

**Component tag** (list, matched with OR/any semantics by `--tags`):
```yaml
components:
  terraform:
    vpc:
      metadata:
        tags: [vpc, production]
```

**Component labels** (map, matched with AND/all semantics by `--labels`):
```yaml
components:
  terraform:
    vpc:
      metadata:
        labels:
          cost-center: platform
          compliance: sox
```

**Stack-wide tags/labels** (closest analog to Terramate's `stack.tm.hcl`-level `tags = [...]`) --
set at the root of a stack manifest, deep-merged as the **lowest-precedence** default into every
component's own `metadata` in that stack:
```yaml
# _defaults.yaml
metadata:
  tags: [kubernetes]
  labels:
    org: acme
```

Only a restricted allowlist is valid at this global scope: `labels`, `tags`, `custom`, `enabled`,
`locked`, `terraform_workspace_pattern`. Anything else (`component`, `inherits`, `type`, `name`,
`terraform_workspace`) is a hard schema error if set here.

**CLI/CI filtering** (Terramate `terramate list --tags`, `terramate script run --tags`):
```bash
atmos list components --tags kubernetes
atmos terraform plan --tags production,tier-1        # OR match
atmos terraform apply --labels cost-center=platform,compliance=sox   # AND match
atmos workflow deploy --tags networking --labels deployment:dev      # forwarded into type: atmos steps
```

**Reading a component's own tags/labels at runtime** (Terramate
`tm_contains(terramate.stack.tags, "kubernetes")` used for conditional generation) -> the `!tags`,
`!labels`, `!labels.keys`, `!labels.values` YAML functions:
```yaml
vars:
  tags: !tags      # -> ["vpc", "production"]
  owner: !labels    # -> {cost-center: platform, compliance: sox}
```
Atmos rarely *needs* this the way Terramate does, since Atmos components don't hand-generate
conditional HCL the way `generate_hcl` mixins do -- but it's available for cases like feeding a
component's own tags into a cloud-resource tag map.

**Drift-reconcile-only-tagged-and-drifted pattern**
(`terramate list --status=drifted --tags reconcile`) -> combine `atmos terraform apply --tags reconcile`
with Atmos Pro drift status.

**`after = ["tag:vpc"]` dependency ordering -- the one remaining nuance, not a full 1:1.** No
manifest-level tag selector exists on `dependencies.components[]` (still only
`name`/`component`/`stack`/`kind`/`path`), and the closure flags below don't derive ordering from
tags either -- they only expand an *already-declared* dependency graph. There is exactly one way
to express the ordering itself:

```yaml
components:
  terraform:
    eks:
      dependencies:
        components:
          - name: vpc
```

Once that edge is declared, `--include-dependencies`/`--include-dependents` become useful for ad
hoc/CI-scoped runs on top of it: `--tags`/`--labels`/`--affected` pick a seed set, and the closure
flag expands it through the graph -- `--include-dependencies` pulls in the seed's prerequisites
(what it depends on), `--include-dependents` pulls in what depends on the seed:

```bash
# eks (tagged kubernetes) plus whatever it depends on, e.g. vpc
atmos terraform plan --tags kubernetes --include-dependencies

# vpc (tagged vpc) plus everything that depends on it, e.g. eks
atmos terraform plan --tags vpc --include-dependents
```

Expanded prerequisite/dependent components are processed even when they don't themselves match
the tag/label filter, and can live in other stacks.

**Selector purity constraint:** `metadata.tags` and `metadata.labels` values are evaluated
*before* authentication, template processing, and YAML function execution, so they must be
resolvable without credentials or process execution. Allowed: plain strings, simple Go templates
over stack context (`{{ .vars.stage }}`), and `!env`/`!git.*`/`!include`. Rejected with a hard
error: `!terraform.state`, `!secret`, `atmos.Component`, and other auth/exec-requiring constructs.
If a Terramate `tags`/`global` value driving a tag needs one of those, move it into `vars` instead.

## Recipe: `tm_*` Functions → Go Templates / YAML Functions

| Terramate function | Atmos equivalent |
|---|---|
| `tm_try(a, b, default)` | Sprig `default`, or Go template `{{ if }}` |
| `tm_contains(list, item)` | Sprig `has` |
| `tm_alltrue(list)` | Sprig/Go template `and` chain, or logic in `locals:` |
| `tm_length(x)` / `tm_split(sep, x)` | Sprig `len` / `splitList` |
| `tm_can(expr)` | No direct 1:1 -- use error-tolerant `{{ if }}` guards |
| `tm_dynamic` (dynamic block generation) | Stays in `.tf` (Terraform's own `dynamic` block), or `!template`/Go-template loops for stack-level generation |

This is the same YAML-function-first preference as the parent skill's Core Principle 5 (prefer
YAML functions over Gomplate datasources) -- reach for `!include`/`!env`/`!terraform.state`/`!store`
before reaching for a Go template when replacing `tm_*` calls that read files/env/state, and
reserve Go templates for actual control flow.

## Recipe: Terramate Cloud Sync → Atmos Pro

**Before:** trailing maps on script commands -- `sync_preview = true`, `sync_deployment = true`,
`sync_drift_status = true`.

**After:** `settings.pro.drift_detection`, native CI (`atmos-ci` skill), `atmos pro lock`/
`atmos pro unlock`, `atmos pro commit`.

Atmos Pro is GitHub OIDC + GitHub App centric today. Confirm the user's CI provider before
promising full parity with Terramate Cloud on non-GitHub CI (GitLab, generic CI).

## Recipe: `_bootstrap/` Two-Phase Pattern → One-Time Bootstrap Component

**Before:** a `_bootstrap/` stack with `tags = ["no-backend"]` while on local state,
`terramate run -C dir -- tool cmd` (low-level, outside `stacks/{terraform,opentofu}`), then remove
the tag and regenerate to migrate to an S3 backend.

**After:** a dedicated bootstrap component/stack (e.g. `components/terraform/bootstrap/`, stack
`core-bootstrap`) deliberately **excluded** from `dependencies.components`/the affected graph, run
manually once (`atmos terraform apply bootstrap -s core-bootstrap`). It can start on a local
backend (or no backend config) and be switched to `s3` once the state bucket exists -- this is a
manual runbook step, not an automated Atmos feature.

## Recipe: `.tmtriggers/` Ignore-Change Records -- Known Gap

Atmos has no equivalent to `terramate trigger --ignore-change`. Terramate's `.tmtriggers/`
directory holds CLI-managed records that suppress a stack from `--changed` detection for a given
commit. There is no post-hoc "ignore this specific change" mechanism in either `atmos list affected`
or `atmos describe affected`.

**Implication for the user:** immediately after migration, `atmos list affected`/`atmos describe affected`
may report more components as affected than they expect, since any suppression from `.tmtriggers/` is lost.
The closest lever is scoping `dependencies.files`/`dependencies.folders` more precisely up front,
but that only prevents future false positives -- it doesn't retroactively suppress a specific
commit's diff the way a trigger record did.

## Recipe: `terramate clone`/`terramate create --tags` → `atmos scaffold`

**Before:** `terramate clone src dst` (duplicate stack tree, new stack IDs);
`terramate create --tags kubernetes` (scaffold a new stack).

**After:** `terramate create --tags` is scaffolding a new stack from an implicit template.
`atmos scaffold generate` is Atmos's first-class equivalent -- a governed, versioned
templating system (`scaffold.yaml`), not a manual copy:

```bash
atmos scaffold generate terraform-component ./components/terraform/vpc \
  --set component_name=vpc --set environments=dev,staging
```

It supports validated prompts, conditional file generation, generation hooks, and
`--update`/`--merge-strategy` to bring existing output forward when the template changes --
capabilities Terramate's `create`/`clone` don't have. Author a `scaffold.yaml` for the team's
stack/component shape once, then generate new instances from it instead of copy-pasting YAML.

For `terramate clone`'s specific "duplicate an existing env" use case, the more idiomatic Atmos
migration path is usually to add a new stack file that imports the same catalog/`_defaults` as the
source stack with different `vars` (see the abstract-component/catalog pattern in
[atmos-components](../../atmos-components/SKILL.md)) rather than cloning files at all -- reserve
`atmos scaffold generate` for bootstrapping genuinely new component/stack shapes.

## Version Pinning: Module `source`/`version` → Component `source:` or `vendor.yaml`

For a module reused across many stacks with a shared version -- e.g. all four modules in the
common Terramate quickstart shape (`terraform-aws-modules/vpc/aws`, `terraform-aws-modules/eks/aws`,
an OIDC-provider module) -- consolidate into one `vendor.yaml` entry
(`atmos vendor pull`/`atmos vendor diff`) as the preferred default. This mirrors the
centralized-version intent Terramate already expresses via `config.tm.hcl` globals, and gives an
auditable, diffable manifest instead of scattered per-generator version pins. For a module used by
exactly one component with no reuse, a direct `source =` pin inside the component's `.tf` is
sufficient and vendoring is optional. Route to [atmos-vendoring](../../atmos-vendoring/SKILL.md)
for the full workflow.

## Common Mistakes

- **Treating one Terramate stack directory as one Atmos stack 1:1.** Leads to over-fragmented
  stack files -- Atmos stacks are named contexts, not directories.
- **Confusing tags(OR)/labels(AND) filter semantics.** `--tags` matches any given tag; `--labels`
  requires all given key=value pairs.
- **Expecting `dependencies.components[]` itself to accept a `tags:` selector, or expecting
  `--tags`/`--include-dependents` to derive ordering without one.** Neither works -- the edge must
  be declared with an explicit `name:` entry first; `--tags`/`--labels` + `--include-dependencies`/
  `--include-dependents` only expand an already-declared graph at CLI runtime.
- **Setting stack-root `metadata:` keys outside the allowlist.** Only `labels`, `tags`, `custom`,
  `enabled`, `locked`, `terraform_workspace_pattern` are valid at global scope -- anything else
  (`component`, `inherits`, `type`, `name`, `terraform_workspace`) is a hard schema error.
  Component-identity fields have to stay inside a component's own `metadata:` block.
- **Putting auth/exec-requiring YAML functions in `metadata.tags`/`metadata.labels`.** Selectors
  must be resolvable pre-auth/pre-template (plain strings, simple Go templates, `!env`/`!git.*`/
  `!include` only) -- `!terraform.state`, `!secret`, `atmos.Component`, etc. are rejected with a
  hard error on any stack-enumerating command.
- **Silently dropping `.tmtriggers/` intent instead of calling it out.** Tell the user explicitly
  that change-detection overrides don't carry over.
- **Rewriting the underlying Terraform module "to fit Atmos."** Unnecessary -- Atmos components
  are just Terraform root modules; only the wiring changes.
- **Leaving `_bootstrap/`-equivalent stacks inside the affected/dependency graph.** They should be
  excluded and run manually.

## Related Skills

- [atmos-workflows](../../atmos-workflows/SKILL.md) -- for `script{}` -> `workflows:` step/job decomposition
- [atmos-custom-commands](../../atmos-custom-commands/SKILL.md) -- for `script{}` -> user-invoked `commands:` mapping
- [atmos-components](../../atmos-components/SKILL.md) -- for `dependencies.components[]` syntax and abstract-component/catalog patterns replacing `generate_hcl` generators
- [atmos-stacks](../../atmos-stacks/SKILL.md) -- for deep-merge inheritance replacing `globals`
- [atmos-vendoring](../../atmos-vendoring/SKILL.md) -- for consolidating module `source`/`version` pins into `vendor.yaml`
- [atmos-yaml-functions](../../atmos-yaml-functions/SKILL.md) -- for `!terraform.state`/`!include`/`!env`/`!tags`/`!labels` mapping from `tm_*` functions
- [atmos-templates](../../atmos-templates/SKILL.md) -- for Go template control flow replacing `tm_dynamic`
- [atmos-pro](../../atmos-pro/SKILL.md) -- for Terramate Cloud sync flag equivalents
- [atmos-introspection](../../atmos-introspection/SKILL.md) -- for `atmos describe affected`/`atmos list affected` replacing `terramate list --changed`
- [atmos-ci](../../atmos-ci/SKILL.md) -- for GitHub Actions CI pattern replacement
- [atmos-scaffold](../../atmos-scaffold/SKILL.md) -- for authoring `scaffold.yaml` templates replacing `terramate create`/`clone`
