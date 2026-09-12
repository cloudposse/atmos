---
name: atmos-migration
description: "This skill helps you migrate a repository to Atmos. It covers native Terraform, Terraform Workspaces, Terramate, Terragrunt, Makefiles, Justfiles, and Taskfiles. It gives minimum-disruption paths, file-layout options, workspace mapping, task-to-command mapping, generate_hcl/script decomposition, and the remote-state bridge for a step-by-step migration. It also covers migrating CLI tool-version management from asdf, aqua, tfenv, tofuenv, tenv, mise, or a Homebrew Brewfile to the Atmos toolchain."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: state-versioning
references:
  - references/from-native-terraform.md
  - references/from-terraform-workspaces.md
  - references/remote-state-bridge.md
  - references/from-terramate.md
  - references/from-makefile.md
  - references/from-justfile.md
  - references/from-taskfile.md
  - references/from-component-updater.md
  - references/from-terragrunt.md
  - references/from-asdf.md
  - references/from-aqua.md
  - references/from-tfenv.md
  - references/from-tofuenv.md
  - references/from-tenv.md
  - references/from-homebrew-brewfile.md
  - references/from-mise.md
---

# Migrating to Atmos

## Overview

This skill is a decision guide. Use it to migrate an existing Terraform repository to Atmos.
Atmos can adopt an existing repository without a reorganization -- `components/terraform/` is a
recommendation, not a requirement. Start with the smallest change that gives value; add more only
when the user has a real need for it.

This skill also covers migrating CLI tool-version management from asdf, aqua, tfenv, tofuenv,
tenv, mise, or a Homebrew Brewfile to the Atmos toolchain -- see
[Replace a Tool-Version Manager](#replace-a-tool-version-manager) below.

Full end-user tutorials exist for [Native Terraform](https://atmos.tools/migration/native-terraform),
[Terraform Workspaces](https://atmos.tools/migration/terraform-workspaces),
[Terragrunt](https://atmos.tools/migration/terragrunt) (agent-actionable recipes in
[from-terragrunt.md](references/from-terragrunt.md)), [Makefiles](https://atmos.tools/migration/makefile),
[Justfiles](https://atmos.tools/migration/justfile), and [Taskfile.yml](https://atmos.tools/migration/taskfile).
Terramate has no atmos.tools tutorial yet -- this skill covers it via
[from-terramate.md](references/from-terramate.md).

## Terraform or OpenTofu

This skill applies the same way to Terraform and to OpenTofu. Atmos runs the binary set in
`components.terraform.command` in `atmos.yaml`. The default binary is `terraform`. The migration
steps, file layouts, and the remote-state bridge do not change based on the binary. Use the same
word the user uses. If the user says "OpenTofu," write "OpenTofu" in your response.

## Core Principles

These principles come before your normal instincts. Read them before you propose a change to the
user's repository.

1. **Migration is opt-in, not all-or-nothing.** No filesystem reorganization is required. Point
    `base_path` at the user's existing layout (e.g., `base_path: "terraform"` or `base_path: "."`)
    when preserving layout lowers adoption risk. `components/terraform/` is still the
    best-practice layout for new or fully migrated repos, since Atmos supports multiple toolchains
    (Terraform, Helmfile, Packer, Ansible) -- but it is not a prerequisite for a Terraform-only repo.
2. **Existing `.tfvars` files may be kept during migration.** Use `!include` to pull them into
    stacks for minimal disruption. Converting to native stack YAML is the best-practice end state
    for deep-merge inheritance and richer composition, but it can happen progressively.
3. **No Terraform code changes are required.** Don't rewrite providers, backends, or modules.
    Atmos generates `backend.tf.json` and `*.auto.tfvars.json` at runtime.
4. **Workspaces are not the enemy.** `terraform.workspace`-driven environments can map onto their
    existing state via `metadata.terraform_workspace` and `workspace_key_prefix` -- no need to
    abandon workspace state to adopt Atmos.
5. **Prefer YAML functions over Gomplate datasources.** When both can express the same thing
    (`!include` vs `gomplate.datasources`, `!exec` vs templated shell, `!env` vs `gomplate getenv`,
    `!store` vs custom datasource URLs), reach for the YAML function first: type-safe, can't break
    YAML parsing, clear errors, no Gomplate required. See
    [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) and
    [atmos-templates](../atmos-templates/SKILL.md) for the boundary.
6. **Crawl → walk → run.** Get the user to a working `atmos terraform plan` in 20 minutes; defer
    inheritance, catalogs, and multi-account hierarchies until they have a concrete need.
7. **Task runners are not a blocker.** Atmos custom commands and workflows can replace Make/Just/
    Task targets, recipes, and tasks -- incrementally: a Makefile, Justfile, or Taskfile can stay
    as a thin wrapper around `atmos` commands during migration (Principle 6). The end state turns
    each leaf target into a custom command; a target chain usually stays a custom command too,
    using `dependencies.commands`/`dependencies.workflows` for its prerequisites. Reserve
    workflows for fixed, multi-step orchestration across more than one component.

## Decide the Migration Shape First

Find the user's source pattern before you propose any change. Each pattern points to a different
reference file:

| User has... | Use reference |
|---|---|
| One TF root module, env config via `.tfvars` or env vars | [from-native-terraform.md](references/from-native-terraform.md) |
| Multiple TF root modules in scattered dirs | [from-native-terraform.md](references/from-native-terraform.md) |
| `terraform.workspace`-driven environments with shared state backend | [from-terraform-workspaces.md](references/from-terraform-workspaces.md) |
| `.tm.hcl` files, `stack.tm.hcl`, `generate_hcl` blocks (Terramate project) | [from-terramate.md](references/from-terramate.md) |
| Terragrunt (`terragrunt.hcl` or `terragrunt.stack.hcl`) | [from-terragrunt.md](references/from-terragrunt.md) |
| Need to read outputs from un-migrated TF (legacy or another repo) | [remote-state-bridge.md](references/remote-state-bridge.md) |
| User has a Makefile driving builds/tests/deploys | [from-makefile.md](references/from-makefile.md) |
| User has a Justfile (`just` command runner) | [from-justfile.md](references/from-justfile.md) |
| User has a Taskfile.yml (go-task) | [from-taskfile.md](references/from-taskfile.md) |
| `cloudposse/github-action-atmos-component-updater` | [from-component-updater.md](references/from-component-updater.md) |

This table covers only the IaC-layout shape. If the user's question is instead about a
tool-version manager (asdf, aqua, tfenv, tofuenv, tenv, mise, or a Homebrew Brewfile), skip this
table and go to [Replace a Tool-Version Manager](#replace-a-tool-version-manager) below.

The remote-state-bridge pattern makes progressive migration possible. It lets a team migrate one
component at a time. Without it, the team must migrate everything at once. Use this pattern when
the user has existing Terraform state that a new Atmos component must read.

### Common Problems in Task-Runner Migration

These behaviors apply to every task runner. Each reference file has the exact field names and
steps; this is only a short summary.

- **Default order differs by source tool.** Task runs `deps:` concurrently by default, so
  `dependencies.commands`/`dependencies.workflows` (also concurrent by default) is a direct match.
  Make and Just run dependencies sequentially by default (`make -j` opts into concurrency), so
  `dependencies.commands` changes the order and can introduce races between prerequisites that
  were only ever sequential by accident. For an ordinary Make/Just chain, keep ordered steps;
  reach for `dependencies.commands` only when the source used `-j`, the prerequisites are
  genuinely independent, or a prerequisite is shared by more than one caller (every one of these
  tools dedups a shared dependency to a single run). Check the source tool's real default first.
- **Freshness checks map to `inputs`/`artifacts`, scoped per step.** Task's `sources:`/`generates:`
  and non-`.PHONY` Make targets skip the *entire* recipe/task when nothing changed. Atmos's
  `inputs.sources`/`artifacts.paths` are the direct match: declaring them implies
  `when: checksum.changed`, skipping *that one step* only -- later steps in the same command still
  run. If a source recipe runs several commands that must be gated together, combine them into one
  `shell`/`script` step instead of spreading `inputs`/`artifacts` across several. This doesn't
  carry over automatically -- add it to the migrated step yourself. `require`/`assert` only checks
  that a file exists, not whether it's fresh.
- **`workflows.base_path` must be set explicitly once the user has their own `atmos.yaml`.** Only
  fixed, multi-step orchestration across more than one component becomes a workflow (Principle 7)
  -- most target chains stay a custom command with `dependencies.commands`. `atmos workflow <name>`
  fails with `'workflows.base_path' must be configured in 'atmos.yaml'` until you set it (e.g.
  `workflows.base_path: "stacks/workflows"`). Add it the moment the migration reaches its first
  workflow -- this skill's `atmos.yaml` snippets omit it by default.

## Replace a Tool-Version Manager

This section covers a topic separate from the IaC-layout question above. A user can migrate the
Terraform or OpenTofu layout, the tool-version manager, or both. Each choice is independent.

If the user currently pins CLI tool versions with asdf, aqua, tfenv, tofuenv, tenv, mise, or a
Homebrew Brewfile, use the matching reference below. Do not write a new config translation by
hand.

| Current tool | Reference |
|---|---|
| asdf | [from-asdf.md](references/from-asdf.md) |
| aqua CLI (`aqua.yaml`) | [from-aqua.md](references/from-aqua.md) |
| tfenv | [from-tfenv.md](references/from-tfenv.md) |
| tofuenv | [from-tofuenv.md](references/from-tofuenv.md) |
| tenv | [from-tenv.md](references/from-tenv.md) |
| mise | [from-mise.md](references/from-mise.md) |
| Homebrew Brewfile | [from-homebrew-brewfile.md](references/from-homebrew-brewfile.md) |

Each reference includes a command-mapping table (old tool's commands next to the Atmos toolchain
equivalent) and a Shell Integration section.

Most source tools (asdf, aqua, tfenv, tofuenv, tenv, mise) add themselves to every shell
automatically via a shim, proxy, or activation hook that re-resolves the nearest per-directory
config on every invocation. Homebrew instead puts one global `bin` directory on `PATH` via `brew
shellenv`, with no per-directory resolution. Either way, the Atmos toolchain does **not** add
itself to `PATH` by default -- it resolves tools only while an `atmos <subcommand>` runs.

A user can still get a plain `terraform` command working in any shell -- a supported feature, not
a gap. Wrap `atmos toolchain env` or `atmos toolchain path` in `eval`/`export` in their shell
profile, for example `eval "$(atmos toolchain env --format=bash)"`. See each reference's Shell
Integration section for the exact form per shell. Always mention this option to a user coming from
a tool that added itself to `PATH` automatically.

## The Minimum-Viable Migration

Use this checklist when the user wants to try Atmos on an existing repository. Do not change the
order unless the user's setup requires it.

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

A working example of this shape is at `examples/native-terraform/` in the Atmos repository.

## File-Layout Options

Pick the layout that matches the user's goals. Atmos recommends the `components/terraform/`
layout, especially for a new repository or a multi-tool project. You can keep an existing layout
when the user wants less disruption.

| `base_path` | Use when |
|---|---|
| `base_path: "."` | TF root modules live at the repo root; user wants zero file moves |
| `base_path: "terraform"` | TF-only repo with code already in `terraform/`; preserve dir name |
| `base_path: "."` + `components.terraform.base_path: "components/terraform"` | Multi-toolchain or new repo; canonical Atmos layout |

For more organization patterns, such as multi-region, multi-account, and organization
hierarchies, see the skill [atmos-design-patterns](../atmos-design-patterns/SKILL.md).

## YAML Functions vs Gomplate Datasources

This is a common mistake: an agent chooses a Gomplate datasource when a YAML function is safer
and clearer. Use the option in the right column:

| Goal | Reach for (NOT this) | Use instead |
|---|---|---|
| Include a file's contents | `gomplate.datasources` with file URL | `!include path/to/file` |
| Read an environment variable | `gomplate getenv "FOO"` | `!env FOO` |
| Run a shell command | Template + `gomplate exec` | `!exec "command"` |
| Read a store value | Custom datasource URL | `!store store_name component stack key` |
| Read Terraform output | Templated remote-state datasource | `!terraform.state component output` |
| Get current AWS account ID | `gomplate.datasources` AWS plugin | `!aws.account_id` |

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
- **Configure the toolchain**, such as `dependencies.tools`, registries, or verification. Use
  [atmos-toolchain](../atmos-toolchain/SKILL.md).

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
  wrapper is a good bridge while the user builds trust in Atmos, but it is not the final state.
  Change each leaf target to a custom command; a target chain usually stays a custom command too
  (ordered steps or `dependencies.commands`, per the default-order rule above) -- not a workflow.
  Reserve workflows for fixed, multi-step orchestration across more than one component.
- **"Copy `aqua.yaml` packages into Atmos verbatim."** This is false. Atmos supports only part of
  the Aqua registry schema. Check the Functional Gaps table in
  [from-aqua.md](references/from-aqua.md) first.
- **"Replace the whole Brewfile with Atmos toolchain."** This is false. Casks, `mas` entries, and
  source-built formulae are out of scope. See
  [from-homebrew-brewfile.md](references/from-homebrew-brewfile.md).

## Additional Resources

Every reference below includes a command-mapping table and shell-integration steps where
applicable.

| Reference | Covers |
|---|---|
| [from-native-terraform.md](references/from-native-terraform.md) | Plain Terraform migration, matched to each shape |
| [from-terraform-workspaces.md](references/from-terraform-workspaces.md) | Mapping workspaces to stacks without losing state |
| [remote-state-bridge.md](references/remote-state-bridge.md) | Dummy/abstract-component patterns for un-migrated or external Terraform state |
| [from-terramate.md](references/from-terramate.md) | Construct-by-construct Terramate mapping (`.tmtriggers` is the one known gap) |
| [from-terragrunt.md](references/from-terragrunt.md) | Concept mapping and migration workflow for classic Terragrunt and Terragrunt Stacks |
| [from-makefile.md](references/from-makefile.md) | Makefile migration, matched to each shape |
| [from-justfile.md](references/from-justfile.md) | Justfile migration, matched to each shape |
| [from-taskfile.md](references/from-taskfile.md) | Taskfile.yml (go-task) migration, matched to each shape |
| [from-asdf.md](references/from-asdf.md) | `.tool-versions`/asdf plugins to the Atmos toolchain |
| [from-aqua.md](references/from-aqua.md) | `aqua.yaml` packages to the Atmos toolchain, plus the schema-gap table |
| [from-tfenv.md](references/from-tfenv.md) | `.terraform-version`/`tfenv use` pins to the Atmos toolchain |
| [from-tofuenv.md](references/from-tofuenv.md) | `.opentofu-version`/`tofuenv use` pins to the Atmos toolchain |
| [from-tenv.md](references/from-tenv.md) | tenv's 5 version files (Terraform, OpenTofu, Terragrunt, Terramate, Atmos) to the Atmos toolchain |
| [from-homebrew-brewfile.md](references/from-homebrew-brewfile.md) | Brewfile CLI tools to the Atmos toolchain, plus the partial-scope rules |
| [from-mise.md](references/from-mise.md) | mise tool versions, tasks, and env vars to the Atmos toolchain |
