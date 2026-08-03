- Preview unapplied migrations in history mode

```shell
  $ atmos terraform migrate plan vpc -s prod-ue2
```

- Preview a single migration file (relative to `migration_dir`, default `./migrations` when that directory exists; omit that prefix)

```shell
  $ atmos terraform migrate plan vpc -s prod-ue2 --migration rename_bucket.hcl
```

- Preview across all affected components in dependency order

```shell
  $ atmos terraform migrate plan --affected -s prod-ue2
```

- Preview with a custom tfmigrate config instead of the Atmos-generated default

```shell
  $ atmos terraform migrate plan vpc -s prod-ue2 --tfmigrate-config .tfmigrate.hcl
```
