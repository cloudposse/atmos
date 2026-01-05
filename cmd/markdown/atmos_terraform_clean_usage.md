- Clean all components across all stacks

```shell
 $ atmos terraform clean
```

- Clean a specific component

```shell
 $ atmos terraform clean vpc
```

- Clean a component in a specific stack

```shell
 $ atmos terraform clean vpc -s prod-ue2
```

- Force clean without confirmation

```shell
 $ atmos terraform clean vpc -s prod-ue2 --force
```

- Clean everything including state files

```shell
 $ atmos terraform clean vpc -s prod-ue2 --everything
```

- Dry run to see what would be deleted

```shell
 $ atmos terraform clean --dry-run
```

- Skip deleting the lock file

```shell
 $ atmos terraform clean vpc -s prod-ue2 --skip-lock-file
```
