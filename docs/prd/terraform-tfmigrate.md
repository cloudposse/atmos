# Terraform `tfmigrate` Integration

## Summary

Add Atmos support for running user-authored `tfmigrate` migrations in Terraform component context. Atmos does not generate migration files and does not persist `tfmigrate` history in v1.

The integration has two surfaces:

- `atmos terraform migrate plan|apply <component> -s <stack>`.
- `atmos terraform migrate list` to inspect selected instances, hooks, and history names.
- `kind: tfmigrate` hooks for lifecycle-driven migrations.

## Behavior

`atmos terraform migrate` prepares the component before invoking `tfmigrate`:

- Resolves the stack and component with the standard flag handler.
- Authenticates using the component identity.
- Runs the normal Terraform init path first, so source provisioning, workdir provisioning, backend generation, varfile generation, and workspace setup happen before `tfmigrate`.
- Resolves `tfmigrate` and Terraform/OpenTofu through `dependencies.tools` or PATH.
- Sets `TFMIGRATE_EXEC_PATH` to the resolved Terraform/OpenTofu binary unless the user already set it.
- Exposes per-instance history variables, including stack/component/workspace names and supported Terraform backend values, so `tfmigrate` history storage can reuse the Terraform backend bucket and identity configuration without duplicating stack YAML.
- Lists the same per-instance history values through `atmos terraform migrate list` for review and automation.

Example:

```shell
atmos terraform migrate plan vpc -s plat-ue2-dev
atmos terraform migrate apply vpc -s plat-ue2-dev
```

History mode is enabled by omitting `--migration`. Atmos does not require a config flag; `tfmigrate` discovers `.tfmigrate.hcl` by default. Use `--tfmigrate-config` only to override that path:

```shell
atmos terraform migrate apply vpc -s plat-ue2-dev --tfmigrate-config migrations/.tfmigrate.hcl
```

For S3 history storage, the config can reuse the Terraform backend bucket and role resolved by Atmos:

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

Atmos runs component auth before `tfmigrate`, so the storage client also inherits the same auth identity environment used for Terraform.

## List Command

```shell
atmos terraform migrate list -s plat-ue2-dev
atmos terraform migrate list --all --format json
atmos terraform migrate list --query '.settings.requires_migration == true' --columns component --columns stack --columns history_key
```

The list command uses the shared `pkg/list` renderer and reports hook name,
mode, migration file, config file, Terraform backend type, history storage,
history bucket, history role ARN, and the stack/component/workspace-scoped
history key.

## Hook Configuration

```yaml
components:
  terraform:
    s3-bucket:
      dependencies:
        tools:
          tfmigrate: "0.4.x"

      hooks:
        state-migration:
          kind: tfmigrate
          events:
            - before.terraform.plan
            - before.terraform.apply
          mode: dynamic
```

Hook `mode` values:

- `dynamic`: `before.terraform.plan` runs `tfmigrate plan`; `before.terraform.apply` and `before.terraform.deploy` run `tfmigrate apply`.
- `plan`: always runs `tfmigrate plan`.
- `apply`: always runs `tfmigrate apply`.

## Known Limitation: History Persistence

Single-file `tfmigrate apply path.hcl` is not inherently idempotent. If the file contains `state mv` or `state rm`, rerunning it can fail because the source address or removed address is already gone.

`tfmigrate` history mode can make reruns safe, but only when the history storage is durable across runs. Atmos v1 supports tfmigrate's default config discovery plus explicit `--tfmigrate-config` and `--backend-config` overrides, but it does not manage that persistence.

Users who need idempotent CI automation must configure durable `tfmigrate` history storage themselves, such as S3, GCS, or a local file persisted by CI.

Future work: add Atmos-managed helpers or hooks to store and retrieve `tfmigrate` history from a bucket, namespaced by stack, component, and Terraform workspace.

## Non-Goals

- Generate migration HCL files.
- Scan Terraform state to infer provider-bound addresses.
- Persist or repair `tfmigrate` history in v1.
