- Preview a migration for a component in history mode (no `.tfmigrate.hcl` needed: Atmos generates one that reuses the component's Terraform backend for history storage)

```shell
  $ atmos terraform migrate plan vpc -s prod-ue2
```

- Apply the previewed migration

```shell
  $ atmos terraform migrate apply vpc -s prod-ue2
```

- Run a single one-off migration file instead of history mode

```shell
  $ atmos terraform migrate plan vpc -s prod-ue2 --migration rename_bucket.hcl
```

- Inspect the migration context (workspace, history storage/key, wired hook) before trusting a real run

```shell
  $ atmos terraform migrate list vpc -s prod-ue2
```
