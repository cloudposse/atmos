---
name: atmos-migration
description: "Migrating to Atmos from existing IaC: techniques, tactics, and design patterns for native Terraform and Terraform Workspaces — minimum-disruption paths, file-layout options, workspace mapping, and the remote-state bridge for progressive migration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/from-native-terraform.md
  - references/from-terraform-workspaces.md
  - references/remote-state-bridge.md
---

# Migrating to Atmos

## Overview

This skill is the agent's decision guide for migrating an existing Terraform repository to Atmos.
Atmos is designed to **adopt an existing repo without forcing a reorganization** -- the canonical
`components/terraform/` layout is a recommendation, not a requirement. Lead with the minimum
change that delivers value, then escalate only as the user's needs grow.

For full prose tutorials aimed at end users, link to:

- [Migrating from Native Terraform](https://atmos.tools/migration/native-terraform)
- [Migrating from Terraform Workspaces](https://atmos.tools/migration/terraform-workspaces)
- [Migrating from Terragrunt](https://atmos.tools/migration/terragrunt) (not covered by this skill)

## Terraform or OpenTofu

Everything in this skill applies identically to **Terraform** and **OpenTofu**. Atmos invokes
whichever binary is configured (`components.terraform.command` in `atmos.yaml`, defaulting to
`terraform`). Migration paths, file layouts, and the remote-state bridge are the same regardless
of which binary the user is running. Use the user's terminology -- if they say "OpenTofu," use
"OpenTofu" in your responses.

## Core Principles

These principles override default agent instincts. Internalize them before proposing changes to a
user's repo.

1. **Migration is opt-in, not all-or-nothing.** Atmos does not require a filesystem
   reorganization. Point `base_path` at the user's existing layout (e.g., `base_path: "terraform"`
   or `base_path: "."`) when preserving layout lowers adoption risk. The `components/terraform/`
   convention is still the best-practice layout for new or fully migrated repos because Atmos
   supports multiple toolchains (Terraform, Helmfile, Packer, Ansible); it is not a prerequisite
   for adopting Atmos in Terraform-only repos.
2. **Existing `.tfvars` files may be kept during migration.** Use `!include` to pull them into
   stacks when the user wants minimal disruption. Converting values into native stack YAML remains
   the best-practice end state when the user wants deep-merge inheritance and richer stack
   composition, but it can happen progressively.
3. **No Terraform code changes are required.** Don't rewrite providers, backends, or modules
   during migration. Atmos generates `backend.tf.json` and `*.auto.tfvars.json` at runtime.
4. **Workspaces are not the enemy.** If the user has `terraform.workspace`-driven environments,
   Atmos can map onto their existing state via `metadata.terraform_workspace` and
   `workspace_key_prefix`. They do not have to abandon their workspace state to adopt Atmos.
5. **Prefer YAML functions over Gomplate datasources.** When both can express the same thing
   (`!include` vs `gomplate.datasources` for files, `!exec` vs templated shell, `!env` vs
   `gomplate getenv`, `!store` vs custom datasource URLs), reach for the YAML function first.
   YAML functions are type-safe, can't break YAML parsing, produce clear errors, and don't
   require enabling Gomplate. See the [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md)
   and [atmos-templates](../atmos-templates/SKILL.md) skills for the boundary.
6. **Crawl → walk → run.** Get the user to a working `atmos terraform plan` in 20 minutes; defer
   inheritance, catalogs, and multi-account hierarchies until they have a concrete need.

## Decide the Migration Shape First

Before proposing any change, identify which source pattern the user has. Each routes to a
different reference:

| User has...                                                          | Use reference                                    |
|----------------------------------------------------------------------|--------------------------------------------------|
| One TF root module, env config via `.tfvars` or env vars             | [from-native-terraform.md](references/from-native-terraform.md) |
| Multiple TF root modules in scattered dirs                           | [from-native-terraform.md](references/from-native-terraform.md) |
| `terraform.workspace`-driven environments with shared state backend  | [from-terraform-workspaces.md](references/from-terraform-workspaces.md) |
| Need to read outputs from un-migrated TF (legacy or another repo)    | [remote-state-bridge.md](references/remote-state-bridge.md) |

The remote-state-bridge pattern is what makes **progressive, component-by-component migration**
possible. Without it, a team is forced into a big-bang cutover. Cover it any time the user has
existing Terraform state they need to read from new Atmos components.

## The Minimum-Viable Migration

When a user says "I want to try Atmos on my existing repo," this is the checklist. Do not deviate
unless the user's setup requires it.

1. **Install Atmos.** See `atmos.tools/install`.
2. **Create `atmos.yaml`** at the repo root, pointing `base_path` and `components.terraform.base_path`
   at the user's existing layout. Do not ask them to move files.
3. **Create one stack file** for one environment. Use `!include` of an existing `.tfvars` file so
   nothing has to be rewritten:
   ```yaml
   # stacks/dev.yaml
   import:
     - _defaults
   components:
     terraform:
       vpc:
         vars: !include ../path/to/existing/dev.tfvars
   ```
4. **Run `atmos terraform plan vpc -s dev`** and confirm output matches what `terraform plan
   -var-file=dev.tfvars` produced before.

A working reference for this shape lives at `examples/native-terraform/` in the Atmos repo.

## File-Layout Options

Pick the layout that matches the user's migration goals. `components/terraform/` is the recommended
Atmos convention, especially for new repos or multi-toolchain projects, but existing layouts can be
preserved when the user wants a lower-disruption migration.

| `base_path`                              | Use when                                                                |
|------------------------------------------|-------------------------------------------------------------------------|
| `base_path: "."`                         | TF root modules live at the repo root; user wants zero file moves       |
| `base_path: "terraform"`                 | TF-only repo with code already in `terraform/`; preserve dir name       |
| `base_path: "."` + `components.terraform.base_path: "components/terraform"` | Multi-toolchain or new repo; canonical Atmos layout |

For deeper organization patterns (multi-region, multi-account, org hierarchies), defer to the
[atmos-design-patterns](../atmos-design-patterns/SKILL.md) skill.

## YAML Functions vs Gomplate Datasources

This is a recurring footgun -- agents reach for Gomplate datasources when a YAML function would
be safer and clearer. Prefer the right column:

| Goal                          | Reach for (NOT this)                              | Use instead                              |
|-------------------------------|---------------------------------------------------|------------------------------------------|
| Include a file's contents     | `gomplate.datasources` with file URL              | `!include path/to/file`                  |
| Read an environment variable  | `gomplate getenv "FOO"`                           | `!env FOO`                               |
| Run a shell command           | Template + `gomplate exec`                        | `!exec "command"`                        |
| Read a store value            | Custom datasource URL                             | `!store store_name component stack key`  |
| Read Terraform output         | Templated remote-state datasource                 | `!terraform.state component output`      |
| Get current AWS account ID    | `gomplate.datasources` AWS plugin                 | `!aws.account_id`                        |

YAML functions are type-safe, produce clear errors, work without enabling Gomplate, and don't
require keeping templates valid YAML. Reserve Go templates for control flow (conditionals, loops,
dynamic keys) that YAML functions cannot express. See [atmos-templates](../atmos-templates/SKILL.md)
for when Go templates are the right tool.

## What Does NOT Need to Change

Lead with this when a user fears a big rewrite. None of the following must change to adopt Atmos:

- **Terraform code** -- providers, resources, data sources, modules all stay as-is.
- **Module sources** -- `source = "../../modules/foo"` or registry sources keep working.
- **Backend code** -- delete the `backend "s3" {}` block from `.tf` files (Atmos generates
  `backend.tf.json`), or leave it and disable backend generation in `atmos.yaml`. Either works.
- **`.tfvars` files** -- consumed via `!include`; convert to YAML later if/when the user wants
  deep-merge inheritance.
- **Custom provider configuration** -- providers stay in `.tf` files; pass env vars via stack
  `env:` or vars via stack `vars:`.

## When to Escalate to Other Skills

After the minimum migration is working, the user will often ask "how do I do X next?" Route
those questions to the right skill:

- **Organizing many stacks (orgs, tenants, accounts, regions)** → [atmos-design-patterns](../atmos-design-patterns/SKILL.md)
- **Abstract components, inheritance, catalog patterns** → [atmos-components](../atmos-components/SKILL.md)
- **Deep merging, imports, overrides** → [atmos-stacks](../atmos-stacks/SKILL.md)
- **Vendoring third-party components** → [atmos-vendoring](../atmos-vendoring/SKILL.md)
- **Authentication / provider credentials** → [atmos-auth](../atmos-auth/SKILL.md)
- **Validation policies (OPA, JSON Schema)** → [atmos-validation](../atmos-validation/SKILL.md)
- **CI/CD with affected-detection** → [atmos-ci](../atmos-ci/SKILL.md)
- **Cross-component data sharing via stores** → [atmos-stores](../atmos-stores/SKILL.md)

## Anti-Patterns

Things to push back on if a user (or another agent) proposes them during migration:

- **"You must move all Terraform into `components/terraform/` before using Atmos."** No -- that is
  the recommended layout, not a requirement. Let the user choose between adopting the best-practice
  layout now or pointing `base_path` at the existing layout and reorganizing later.
- **"You must rewrite all `.tfvars` as YAML before running Atmos."** No -- native stack YAML is the
  best-practice destination for inheritance and composition, but `!include` lets users keep
  existing `.tfvars` during a progressive migration.
- **"Delete your workspace state and start over."** No -- bridge it with
  `metadata.terraform_workspace` and the remote-state-bridge pattern.
- **"Add Gomplate datasources for everything."** No -- reach for YAML functions first.
- **"Adopt the full multi-account org hierarchy on day one."** No -- start with one stack file.

## Additional Resources

- [References/from-native-terraform.md](references/from-native-terraform.md) -- scenario-keyed
  recipes for vanilla TF migration
- [References/from-terraform-workspaces.md](references/from-terraform-workspaces.md) -- mapping
  workspaces to stacks without losing state
- [References/remote-state-bridge.md](references/remote-state-bridge.md) -- the dummy-component
  and abstract-component patterns for reading state from un-migrated or external Terraform
