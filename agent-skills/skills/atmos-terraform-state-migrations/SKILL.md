---
name: atmos-terraform-state-migrations
description: "Terraform state migration workflow with tfmigrate in Atmos: writing migration HCL, running atmos terraform migrate plan/apply/list, wiring kind: tfmigrate hooks, configuring history mode, and handling state refactors, rerun safety, workspace context, backend history variables, and CI-safe migrations."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/tfmigrate-migration-patterns.md
---

# Atmos Terraform State Migrations

Use this skill when creating or reviewing Terraform state migrations for Atmos components. Atmos delegates state
migrations to `tfmigrate`, but runs it in the same component context as `atmos terraform plan` and `apply`: auth
identity setup, source/workdir provisioning, backend and varfile generation, Terraform init, workspace selection,
toolchain resolution, and `TFMIGRATE_EXEC_PATH` setup happen before `tfmigrate` runs.

For migration HCL syntax and examples, load
[references/tfmigrate-migration-patterns.md](references/tfmigrate-migration-patterns.md).

## Start With Resolved Context

Never guess stack names, component names, workspaces, backend paths, or state addresses.

```bash
atmos describe component <component> -s <stack>
atmos terraform migrate list <component> -s <stack>
atmos terraform state list <component> -s <stack>
```

Use the resolved component output to confirm:

- the final Terraform component and component path
- Terraform workspace name
- backend type and backend settings
- whether a `kind: tfmigrate` hook already exists
- history key and history bucket from `atmos terraform migrate list`

Use `atmos terraform state show <component> -s <stack> <address>` when resource identity or import IDs are unclear.
If a refactor can be handled with Terraform `moved` blocks and stays in the same state, prefer that unless the user
specifically needs `tfmigrate` automation or multi-state moves.

## One-Off CLI Workflow

Create a migration file in a project-owned migrations directory, then preview it before applying.

```bash
atmos terraform migrate plan <component> -s <stack> --migration migrations/20260527090000_refactor_vpc.hcl
atmos terraform migrate apply <component> -s <stack> --migration migrations/20260527090000_refactor_vpc.hcl
```

For multiple selected component instances:

```bash
atmos terraform migrate plan --components vpc,eks -s <stack> --migration migrations/20260527090000_refactor.hcl
atmos terraform migrate plan --query '.settings.requires_migration == true' --tfmigrate-config .tfmigrate.hcl
atmos terraform migrate plan --affected --migration migrations/20260527090000_refactor.hcl
```

Do not run `apply` until `plan` succeeds and the Terraform plan after migration does not show unintended destroy/create
changes. `--affected` does not support `--include-dependents` for migrate.

## Migration Files

Use `tfmigrate` HCL for state operations that need reviewable, repeatable files. A migration file contains exactly one
`migration` block.

```hcl
migration "state" "rename_subnet" {
  actions = [
    "mv aws_subnet.private aws_subnet.private_primary",
  ]
}
```

Common actions:

- `mv <source> <destination>` for renames and module/address moves in one state.
- `rm <addresses>...` for removing state bindings without destroying infrastructure.
- `import <address> <id>` for binding existing infrastructure.
- `replace-provider <from> <to>` for provider address migrations.
- `xmv <source-pattern> <destination-pattern>` for wildcard moves.
- `migration "multi_state"` for moving resources between component directories or state files.

Keep migration filenames sortable, usually with a timestamp prefix. In history mode, unapplied migrations are processed
in filename order.

## Hook Wiring

Use hooks when the migration should run as part of normal `plan`, `apply`, or `deploy` workflows.

```yaml
components:
  terraform:
    s3-bucket:
      dependencies:
        tools:
          tfmigrate: "0.4.x"

      hooks:
        state-migration:
          events:
            - before.terraform.plan
            - before.terraform.apply
          kind: tfmigrate
          migration: migrations/20260527090000_remove_template_provider.hcl
          mode: dynamic
```

`mode: dynamic` is the default: `before.terraform.plan` runs `tfmigrate plan`, while `before.terraform.apply` and
`before.terraform.deploy` run `tfmigrate apply`. Use `mode: plan` or `mode: apply` only when the hook must always run
one action.

Hook fields:

- `migration`: path to one migration file.
- `config`: path to `.tfmigrate.hcl`; omit `migration` when using history mode.
- `backend_config`: entries passed as repeated `tfmigrate --backend-config` flags for the Terraform state backend.
- `mode`: `dynamic`, `plan`, or `apply`.

## History Mode

Single-file `tfmigrate apply path.hcl` is not idempotent. A rerun can fail after a source address has already moved or
an address has already been removed. For CI-safe reruns, use `tfmigrate` history mode with durable storage.

```yaml
hooks:
  state-migration:
    events:
      - before.terraform.plan
      - before.terraform.apply
    kind: tfmigrate
    config: .tfmigrate.hcl
    mode: dynamic
```

Atmos exposes helper variables for `.tfmigrate.hcl`:

```hcl
tfmigrate {
  migration_dir = "./tfmigrate"

  history {
    storage "s3" {
      bucket   = env.ATMOS_TFMIGRATE_HISTORY_BUCKET
      key      = env.ATMOS_TFMIGRATE_HISTORY_KEY
      region   = env.ATMOS_TFMIGRATE_HISTORY_REGION
      role_arn = env.ATMOS_TFMIGRATE_HISTORY_ROLE_ARN
    }
  }
}
```

The default history key is `tfmigrate/<stack>/<component>/<workspace>/history.json`. Atmos passes history settings to
`tfmigrate`, but does not persist or repair history itself; configure durable S3, GCS, or CI-persisted local storage.

## Safety Checklist

Before committing migration work:

1. Confirm the migration addresses come from `atmos terraform state list`, not from code names alone.
2. Confirm the migration file targets the resolved component working directory and workspace.
3. Run `atmos terraform migrate plan` and inspect the post-migration Terraform plan.
4. Use history mode for hooks or CI workflows that may rerun.
5. Keep migration files and hook wiring in the same PR as the Terraform refactor they support.
6. Remove or disable one-shot hook wiring after the migration has safely run everywhere it is intended to run.
