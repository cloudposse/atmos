# Migrating from Native Terraform

This reference is a scenario-keyed decision guide. Identify which shape the user has, then
follow the matching recipe. For the full user-facing prose tutorial, see
[atmos.tools/migration/native-terraform](https://atmos.tools/migration/native-terraform).

A working end-to-end example lives at `examples/native-terraform/` in the Atmos repo -- read it
when you need a complete reference for the minimum-viable migration.

## Identifying the User's Shape

Before proposing anything, ask the user to show you their repo layout, or read it directly.
Most vanilla-Terraform repos fall into one of three shapes:

| Shape                                                          | Recipe                          |
|----------------------------------------------------------------|---------------------------------|
| Per-environment dirs (`terraform/dev/`, `terraform/prod/`)     | [Shape A](#shape-a-per-environment-directories-with-tfvars)               |
| One TF dir, env config via `-var-file` from a Makefile/script  | [Shape B](#shape-b-single-dir-with-var-file-from-a-makefile)               |
| Multiple root modules (`terraform/vpc/`, `terraform/eks/`)     | [Shape C](#shape-c-multiple-root-modules-with-shared-modules)               |

Mixed shapes are common -- treat each root module independently.

## Shape A: Per-Environment Directories with `.tfvars`

**Before:**
```text
terraform/
├── vpc/
│   ├── main.tf
│   ├── variables.tf
│   ├── outputs.tf
│   └── envs/
│       ├── dev.tfvars
│       ├── staging.tfvars
│       └── prod.tfvars
└── database/
    ├── main.tf
    └── envs/
        ├── dev.tfvars
        └── prod.tfvars
```

**Recipe:**

1. `atmos.yaml` at repo root, **no file moves**:
   ```yaml
   base_path: "./"
   components:
     terraform:
       base_path: "terraform"        # Point at the existing dir
       apply_auto_approve: false
       deploy_run_init: true
       auto_generate_backend_file: false
   stacks:
     base_path: "stacks"
     included_paths: ["**/*"]
     excluded_paths: ["**/_defaults.yaml"]
   ```
2. Create `stacks/dev.yaml`:
   ```yaml
   import:
     - _defaults
   components:
     terraform:
       vpc:
         vars: !include ../terraform/vpc/envs/dev.tfvars
       database:
         vars: !include ../terraform/database/envs/dev.tfvars
   ```
3. Run `atmos terraform plan vpc -s dev`. Compare to the previous
   `cd terraform/vpc && terraform plan -var-file=envs/dev.tfvars` output.

The user keeps their `.tfvars` files and TF code unchanged. Later, they can convert per-env
`.tfvars` to native YAML to get deep-merge inheritance across environments.

## Shape B: Single Dir with `-var-file` from a Makefile

**Before:**
```text
terraform/
├── main.tf
├── variables.tf
└── envs/
    ├── dev.tfvars
    └── prod.tfvars
Makefile
```
With a Makefile like `terraform plan -var-file=envs/$(ENV).tfvars`.

**Recipe:**

1. `atmos.yaml`:
   ```yaml
   base_path: "./"
   components:
     terraform:
       base_path: "."              # The whole repo is one component
   stacks:
     base_path: "stacks"
   ```
2. Treat the single TF dir as one component (e.g., `infra`):
   ```yaml
   # stacks/dev.yaml
   components:
     terraform:
       infra:
         vars: !include ../terraform/envs/dev.tfvars
   ```
3. The Makefile can stay as a thin wrapper around `atmos terraform plan infra -s dev` during
   transition, then be deleted.

## Shape C: Multiple Root Modules with Shared Modules

**Before:**
```text
terraform/
├── vpc/                    # root module
├── eks/                    # root module
├── rds/                    # root module
└── modules/
    ├── label/              # shared module (consumed via source=...)
    └── tags/               # shared module
```

**Recipe:**

- **Only root modules become components.** Shared modules under `modules/` stay where they are
  and continue to be consumed via `source = "../../modules/foo"`. Atmos does not care about
  child modules -- it only orchestrates root modules.
- Point `components.terraform.base_path: "terraform"`.
- Define one component per root module (`vpc`, `eks`, `rds`).
- Per-component `.tfvars` get `!include`'d into stacks per the Shape A recipe.

Do not propose flattening shared modules into components -- that breaks reuse and inverts the
purpose of modules.

## Common Gotchas

### Backend ownership

Atmos generates `backend.tf.json` at plan/apply time by default. If the user's `.tf` files
contain a hand-written `backend "s3" {}` block, the two will conflict at `terraform init` time.

Two valid paths:

- **Recommended:** Delete the `backend "s3" {}` block from `.tf` files and let Atmos own the
  backend (configured per-stack). This is what enables per-stack backend isolation.
- **Alternative:** Set `auto_generate_backend_file: false` in `atmos.yaml` and keep the
  hand-written backend block. The user loses per-stack backend flexibility but the migration is
  zero-risk for code.

### `TF_VAR_*` environment variables

Replace exported `TF_VAR_foo=bar` with either:
```yaml
# Stack-level vars
components:
  terraform:
    vpc:
      vars:
        foo: bar
```
or, if the user really wants env-var semantics:
```yaml
components:
  terraform:
    vpc:
      env:
        TF_VAR_foo: bar
```
Stack-level `vars:` is strongly preferred -- it shows up in `atmos describe component` and is
deep-merged through inheritance.

### Provider authentication

Provider auth (AWS credentials, Azure subscription, etc.) is orthogonal to migration. The user's
existing auth setup (env vars, AWS profiles, instance profiles) keeps working. If they want
Atmos to manage identity chains and assume-role flows, route them to the
[atmos-auth](../../atmos-auth/SKILL.md) skill -- this is post-migration polish, not a
migration prerequisite.

### Remote state from un-migrated components

When migrating component-by-component, a new Atmos component will often need to read outputs
from a Terraform root module that hasn't been migrated yet. This is what the
[remote-state-bridge.md](remote-state-bridge.md) reference solves -- it lets a real Atmos
component query state from an un-migrated TF dir via `!terraform.state` without rewriting the
legacy code.

## What to NOT Do

- Do not propose moving files into `components/terraform/` as step 1. That comes last, after
  the user has felt the value of Atmos on their existing layout.
- Do not rewrite `.tfvars` as YAML on first pass. `!include` them.
- Do not delete hand-written `backend "s3" {}` blocks without asking -- ask first which path
  the user prefers (Atmos-managed vs hand-written).
- Do not introduce Gomplate datasources for things YAML functions can express. See the
  Core Principles in the [SKILL.md](../SKILL.md).
