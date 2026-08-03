- Apply unapplied migrations in history mode

```shell
  $ atmos terraform migrate apply vpc -s prod-ue2
```

- Apply a single migration file (relative to `migration_dir`, default `./migrations` when that directory exists - do not include that prefix)

```shell
  $ atmos terraform migrate apply vpc -s prod-ue2 --migration rename_bucket.hcl
```

- Apply across all components in a stack

```shell
  $ atmos terraform migrate apply --all -s prod-ue2
```

- Apply, skipping Atmos's own `terraform init` pre-flight step (needed when the state has a legacy/unqualified provider address - tfmigrate handles that case internally)

```shell
  $ atmos terraform migrate apply vpc -s prod-ue2 --migration fix_provider.hcl --skip-init
```
