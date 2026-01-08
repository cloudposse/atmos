- Generate files for a single component

```shell
atmos terraform generate files vpc -s prod-ue2
```

- Generate files for all components

```shell
atmos terraform generate files --all
```

- Dry run to see what would be generated

```shell
atmos terraform generate files vpc -s prod-ue2 --dry-run
```

- Delete generated files

```shell
atmos terraform generate files --clean --all
```

- Filter by specific stacks

```shell
atmos terraform generate files --all --stacks "prod-*"
```

- Filter by specific components

```shell
atmos terraform generate files --all --components vpc,rds
```
