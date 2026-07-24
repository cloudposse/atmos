# Tfmigrate Migration Patterns

Use these patterns when writing `tfmigrate` files for Atmos Terraform components. Atmos invokes `tfmigrate` from the
resolved component working directory after init and workspace selection.

## Single-State Migrations

Use `migration "state"` for moves inside one Terraform state.

```hcl
migration "state" "rename_security_groups" {
  actions = [
    "mv aws_security_group.web aws_security_group.app",
    "mv aws_security_group.worker aws_security_group.jobs",
  ]
}
```

Set `dir` only when the migration needs a working directory different from the command working directory.

```hcl
migration "state" "rename_from_subdir" {
  dir = "components/terraform/vpc"
  actions = [
    "mv aws_subnet.private aws_subnet.private_primary",
  ]
}
```

Set `workspace` only when not relying on Atmos workspace selection.

```hcl
migration "state" "workspace_specific_move" {
  workspace = env.ATMOS_TERRAFORM_WORKSPACE
  actions = [
    "mv aws_route_table.private aws_route_table.private_primary",
  ]
}
```

## Common Actions

Rename or move addresses:

```hcl
migration "state" "move_to_module" {
  actions = [
    "mv aws_iam_role.app module.iam.aws_iam_role.app",
  ]
}
```

Remove state bindings without destroying remote infrastructure:

```hcl
migration "state" "forget_legacy_object" {
  actions = [
    "rm aws_s3_bucket_ownership_controls.legacy",
  ]
}
```

Import existing infrastructure into a new address:

```hcl
migration "state" "import_existing_bucket" {
  actions = [
    "import aws_s3_bucket.logs my-company-logs",
  ]
}
```

Replace provider source addresses:

```hcl
migration "state" "replace_provider" {
  actions = [
    "replace-provider registry.terraform.io/-/aws registry.terraform.io/hashicorp/aws",
  ]
}
```

Wildcard moves with `xmv`:

```hcl
migration "state" "rename_many_instances" {
  actions = [
    "xmv aws_security_group.* aws_security_group.$${1}_primary",
  ]
}
```

## for_each and Count Addresses

Quote addresses with embedded string keys so the shell-like action parser preserves them.

```hcl
migration "state" "count_to_for_each" {
  actions = [
    "mv aws_subnet.private[0] 'aws_subnet.private[\"az-a\"]'",
    "mv aws_subnet.private[1] 'aws_subnet.private[\"az-b\"]'",
  ]
}
```

Keep exact source addresses from `atmos terraform state list`. Do not infer index or key names from Terraform code.

## Multi-State Migrations

Use `migration "multi_state"` when splitting, merging, or moving resources between state files or component
directories.

```hcl
migration "multi_state" "move_dns_to_dns_component" {
  from_dir = "components/terraform/network"
  to_dir   = "components/terraform/dns"
  actions = [
    "mv aws_route53_zone.primary aws_route53_zone.primary",
    "mv aws_route53_record.app aws_route53_record.app",
  ]
}
```

Use wildcard multi-state moves only after confirming the matched addresses.

```hcl
migration "multi_state" "move_all_dns_resources" {
  from_dir = "components/terraform/network"
  to_dir   = "components/terraform/dns"
  actions = [
    "xmv aws_route53_*.* $1",
  ]
}
```

## History Configuration

History mode applies unapplied files from `migration_dir` in filename order and records applied migrations in durable
storage. Use this mode for hooks and CI.

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

For GCS-backed Terraform state, Atmos exposes the bucket value as `ATMOS_TFMIGRATE_HISTORY_BUCKET`:

```hcl
tfmigrate {
  migration_dir = "./tfmigrate"

  history {
    storage "gcs" {
      bucket = env.ATMOS_TFMIGRATE_HISTORY_BUCKET
      key    = env.ATMOS_TFMIGRATE_HISTORY_KEY
    }
  }
}
```

Local history is acceptable only when the file is persisted across reruns:

```hcl
tfmigrate {
  migration_dir = "./tfmigrate"

  history {
    storage "local" {
      path = env.ATMOS_TFMIGRATE_HISTORY_PATH
    }
  }
}
```

## Atmos Hook Examples

Single-file hook:

```yaml
hooks:
  state-migration:
    events:
      - before.terraform.plan
      - before.terraform.apply
    kind: tfmigrate
    migration: migrations/20260527090000_refactor.hcl
    mode: dynamic
```

History-mode hook:

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

Hook with explicit backend config entries passed to `tfmigrate`:

```yaml
hooks:
  state-migration:
    events:
      - before.terraform.plan
      - before.terraform.apply
    kind: tfmigrate
    config: .tfmigrate.hcl
    backend_config:
      - bucket=my-state-bucket
      - region=us-east-1
```

## Review Checklist

- Migration file has exactly one `migration` block.
- File extension is `.hcl` or `.json`.
- Filename is sortable when history mode is used.
- All addresses are copied from state output or reviewed Terraform plan output.
- `plan` succeeds before `apply`.
- Post-migration Terraform plan does not show unexpected replacement, destroy, or create operations.
- Durable history storage is configured for hooks and CI.
