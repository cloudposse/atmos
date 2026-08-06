---
name: atmos-migration
description: "This skill helps you migrate a repository to Atmos. It covers native Terraform, Terraform Workspaces, Makefiles, Justfiles, and Taskfiles. It gives minimum-disruption paths, file-layout options, workspace mapping, task-to-command mapping, and the remote-state bridge for a step-by-step migration."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/from-native-terraform.md
  - references/from-terraform-workspaces.md
  - references/remote-state-bridge.md
  - references/from-makefile.md
  - references/from-justfile.md
  - references/from-taskfile.md
---

# Migrating to Atmos

## Overview

This skill is a decision guide. Use it to migrate an existing Terraform repository to Atmos.
Atmos can adopt an existing repository without a reorganization. The `components/terraform/`
layout is a recommendation. It is not a requirement. Start with the smallest change that gives
value. Add more only when the user has a real need for it.

For full tutorials for end users, see:

- [Migrating from Native Terraform](https://atmos.tools/migration/native-terraform)
- [Migrating from Terraform Workspaces](https://atmos.tools/migration/terraform-workspaces)
- [Migrating from Terragrunt](https://atmos.tools/migration/terragrunt) (this skill does not cover Terragrunt)
- [Migrating from Makefiles](https://atmos.tools/migration/makefile)
- [Migrating from Justfiles](https://atmos.tools/migration/justfile)
- [Migrating from Taskfile.yml](https://atmos.tools/migration/taskfile)

## Terraform or OpenTofu

This skill applies the same way to Terraform and to OpenTofu. Atmos runs the binary set in
`components.terraform.command` in `atmos.yaml`. The default binary is `terraform`. The migration
steps, file layouts, and the remote-state bridge do not change based on the binary. Use the same
word the user uses. If the user says "OpenTofu," write "OpenTofu" in your response.

## Core Principles

These principles come before your normal instincts. Read them before you propose a change to the
user's repository.

1. **Migration is optional. It is not all-or-nothing.** Atmos does not require a new file layout.
   Point `base_path` at the user's existing layout, for example `base_path: "terraform"` or
   `base_path: "."`. This lowers the risk of adoption. The `components/terraform/` layout is
   still the best practice for a new repository or a fully migrated repository, because Atmos
   supports more than one tool, such as Terraform, Helmfile, Packer, and Ansible. It is not a
   requirement for a Terraform-only repository.
2. **You can keep existing `.tfvars` files during migration.** Use `!include` to add them to a
   stack. This gives the smallest disruption. Native stack YAML is still the best final state, if
   the user wants deep-merge inheritance and richer stack composition. The user can convert to
   YAML step by step.
3. **Terraform code changes are not necessary.** Do not rewrite providers, backends, or modules
   during migration. Atmos creates the files `backend.tf.json` and `*.auto.tfvars.json` when it
   runs.
4. **Terraform workspaces are not a problem.** If the user has environments controlled by
   `terraform.workspace`, Atmos can map onto the existing state. Use
   `metadata.terraform_workspace` and `workspace_key_prefix` to do this. The user does not need to
   delete their workspace state to adopt Atmos.
5. **Use a YAML function before you use a Gomplate datasource.** Some tasks have both options:
   `!include` instead of `gomplate.datasources` for files, `!exec` instead of a templated shell
   command, `!env` instead of `gomplate getenv`, and `!store` instead of a custom datasource URL.
   Choose the YAML function first. A YAML function checks its own types. It cannot break YAML
   parsing. It gives a clear error message. It does not require you to turn on Gomplate. See the
   skills [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) and
   [atmos-templates](../atmos-templates/SKILL.md) for more detail on this choice.
6. **Start small. Add complexity later.** Get a working `atmos terraform plan` command for the
   user in 20 minutes. Wait to add inheritance, catalogs, and multi-account hierarchies until the
   user has a real need for them.
7. **Task runners are not a problem.** Atmos custom commands and workflows can replace the
   targets, recipes, and tasks that Make, Just, and Task provide. This does not need to happen at
   once. A Makefile, Justfile, or Taskfile can stay as a thin wrapper around `atmos` commands
   during the migration. This is the same method used for a Terraform-wrapping Makefile,
   described in Principle 6. The final state has each leaf target as a custom command and each
   target chain as a workflow. Reach this state step by step.

## Decide the Migration Shape First

Find the user's source pattern before you propose any change. Each pattern points to a different
reference file:

| User has...                                                          | Use reference                                    |
|----------------------------------------------------------------------|--------------------------------------------------|
| One TF root module, env config via `.tfvars` or env vars             | [from-native-terraform.md](references/from-native-terraform.md) |
| Multiple TF root modules in scattered dirs                           | [from-native-terraform.md](references/from-native-terraform.md) |
| `terraform.workspace`-driven environments with shared state backend  | [from-terraform-workspaces.md](references/from-terraform-workspaces.md) |
| Need to read outputs from un-migrated TF (legacy or another repo)    | [remote-state-bridge.md](references/remote-state-bridge.md) |
| User has a Makefile driving builds/tests/deploys                     | [from-makefile.md](references/from-makefile.md) |
| User has a Justfile (`just` command runner)                          | [from-justfile.md](references/from-justfile.md) |
| User has a Taskfile.yml (go-task)                                    | [from-taskfile.md](references/from-taskfile.md) |

The remote-state-bridge pattern makes progressive migration possible. It lets a team migrate one
component at a time. Without it, the team must migrate everything at once. Use this pattern when
the user has existing Terraform state that a new Atmos component must read.

### Two Common Problems in Task-Runner Migration

Two behaviors apply to every task runner. Check them before you open a reference file:

- **The default order can change.** Task runs `deps:` at the same time by default. Make and Just
  run dependencies one after another, unless the user adds a flag such as `make -j`. Atmos steps
  always run one after another, unless you put them inside a `parallel` or `matrix` step. Check
  the source tool's real default. Do not assume the step order stays the same when you move it to
  Atmos.
- **Atmos has no file-freshness cache.** Task's `sources:`/`generates:` fields and non-`.PHONY`
  Make targets both skip work when a file has not changed. Atmos steps always run. The
  `require`/`assert` step type does not replace this. It only checks that a file exists. It does
  not check if the file is new. Tell the user this directly.

Each reference file has its own "Common Problems" section with the exact field names and steps
for that tool. This section is only a short summary.

## The Minimum-Viable Migration

Use this checklist when the user wants to try Atmos on an existing repository. Do not change the
order unless the user's setup requires it.

1. **Install Atmos.** See `atmos.tools/install`.
2. **Create `atmos.yaml`** at the root of the repository. Set `base_path` and
   `components.terraform.base_path` to match the user's current layout. Do not ask the user to
   move files.
3. **Create one stack file** for one environment. Use `!include` to add an existing `.tfvars`
   file. This avoids a rewrite:
   ```yaml
   # stacks/dev.yaml
   import:
     - _defaults
   components:
     terraform:
       vpc:
         vars: !include ../path/to/existing/dev.tfvars
   ```
4. **Run `atmos terraform plan vpc -s dev`**. Compare the output to the output of
   `terraform plan -var-file=dev.tfvars` from before the migration. Confirm the two outputs
   match.

A working example of this shape is at `examples/native-terraform/` in the Atmos repository.

## File-Layout Options

Pick the layout that matches the user's goals. Atmos recommends the `components/terraform/`
layout, especially for a new repository or a multi-tool project. You can keep an existing layout
when the user wants less disruption.

| `base_path`                              | Use when                                                                |
|------------------------------------------|-------------------------------------------------------------------------|
| `base_path: "."`                         | TF root modules live at the repo root; user wants zero file moves       |
| `base_path: "terraform"`                 | TF-only repo with code already in `terraform/`; preserve dir name       |
| `base_path: "."` + `components.terraform.base_path: "components/terraform"` | Multi-toolchain or new repo; canonical Atmos layout |

For more organization patterns, such as multi-region, multi-account, and organization
hierarchies, see the skill [atmos-design-patterns](../atmos-design-patterns/SKILL.md).

## YAML Functions vs Gomplate Datasources

This is a common mistake: an agent chooses a Gomplate datasource when a YAML function is safer
and clearer. Use the option in the right column:

| Goal                          | Reach for (NOT this)                              | Use instead                              |
|-------------------------------|---------------------------------------------------|------------------------------------------|
| Include a file's contents     | `gomplate.datasources` with file URL              | `!include path/to/file`                  |
| Read an environment variable  | `gomplate getenv "FOO"`                           | `!env FOO`                               |
| Run a shell command           | Template + `gomplate exec`                        | `!exec "command"`                        |
| Read a store value            | Custom datasource URL                             | `!store store_name component stack key`  |
| Read Terraform output         | Templated remote-state datasource                 | `!terraform.state component output`      |
| Get current AWS account ID    | `gomplate.datasources` AWS plugin                 | `!aws.account_id`                        |

A YAML function checks its own types. It gives a clear error message. It works without Gomplate
turned on. It does not require the template text to stay valid YAML. Use a Go template only for
control flow, such as a conditional, a loop, or a dynamic key, that a YAML function cannot
express. See [atmos-templates](../atmos-templates/SKILL.md) for when to use a Go template.

## What Does NOT Need to Change

Tell the user this list first, if they are afraid of a large rewrite. None of these items must
change to adopt Atmos:

- **Terraform code.** Providers, resources, data sources, and modules stay the same.
- **Module sources.** A local path, such as `source = "../../modules/foo"`, or a registry
  source, keeps working.
- **Backend code.** You can delete the `backend "s3" {}` block from the `.tf` files, because
  Atmos creates `backend.tf.json`. Or you can keep the block and turn off backend generation in
  `atmos.yaml`. Both methods work.
- **`.tfvars` files.** Atmos reads them through `!include`. Convert them to YAML later, only if
  the user wants deep-merge inheritance.
- **Custom provider configuration.** Providers stay in the `.tf` files. Pass environment
  variables through stack `env:`. Pass Terraform variables through stack `vars:`.

## When to Escalate to Other Skills

After the minimum migration works, the user will often ask what to do next. Send each question
to the correct skill:

- **Organize many stacks**, such as by organization, tenant, account, or region. Use
  [atmos-design-patterns](../atmos-design-patterns/SKILL.md).
- **Build abstract components, inheritance, or catalog patterns.** Use
  [atmos-components](../atmos-components/SKILL.md).
- **Use deep merging, imports, or overrides.** Use [atmos-stacks](../atmos-stacks/SKILL.md).
- **Vendor third-party components.** Use [atmos-vendoring](../atmos-vendoring/SKILL.md).
- **Set up authentication or provider credentials.** Use [atmos-auth](../atmos-auth/SKILL.md).
- **Add validation policies, such as OPA or JSON Schema.** Use
  [atmos-validation](../atmos-validation/SKILL.md).
- **Set up CI/CD with affected-component detection.** Use [atmos-ci](../atmos-ci/SKILL.md).
- **Share data between components through a store.** Use
  [atmos-stores](../atmos-stores/SKILL.md).

## Anti-Patterns

Push back if a user or another agent proposes one of these methods during migration:

- **"You must move all Terraform into `components/terraform/` before you use Atmos."** This is
  false. That layout is a recommendation, not a requirement. Let the user pick: adopt the
  recommended layout now, or point `base_path` at the current layout and reorganize later.
- **"You must rewrite all `.tfvars` files as YAML before you run Atmos."** This is false. Native
  stack YAML is the best final format for inheritance and composition. But `!include` lets the
  user keep existing `.tfvars` files during a step-by-step migration.
- **"Delete your workspace state and start over."** This is false. Connect the existing state
  with `metadata.terraform_workspace` and the remote-state-bridge pattern.
- **"Add a Gomplate datasource for everything."** This is false. Use a YAML function first.
- **"Adopt the full multi-account organization hierarchy on day one."** This is false. Start
  with one stack file.
- **"Wrap atmos commands in a Makefile, Justfile, or Taskfile forever."** This is false. A
  wrapper is a good bridge while the user builds trust in Atmos. But it is not the final state.
  Change each leaf target to a custom command. Change each dependency chain to a workflow, once
  the team is ready.

## Additional Resources

- [References/from-native-terraform.md](references/from-native-terraform.md): steps for a plain
  Terraform migration, matched to each shape.
- [References/from-terraform-workspaces.md](references/from-terraform-workspaces.md): how to map
  workspaces to stacks without losing state.
- [References/remote-state-bridge.md](references/remote-state-bridge.md): the dummy-component and
  abstract-component patterns. Use them to read state from Terraform that is not yet migrated, or
  from an external repository.
- [References/from-makefile.md](references/from-makefile.md): steps for a Makefile, matched to
  each shape.
- [References/from-justfile.md](references/from-justfile.md): steps for a Justfile, matched to
  each shape.
- [References/from-taskfile.md](references/from-taskfile.md): steps for a Taskfile.yml (go-task)
  file, matched to each shape.
