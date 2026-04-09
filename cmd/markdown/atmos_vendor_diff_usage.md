- Compare the differences between the local and vendored versions of the specified component.

```bash
 $ atmos vendor diff --component <component>
```shell

- Compare the differences between the local and vendored versions of the specified component, filtering by type (terraform or helmfile).

```bash
 $ atmos vendor diff --component <component> --type (terraform|helmfile)
```

- Simulate the comparison of differences between the local and vendored versions of the specified component without making any changes.

```bash
 $ atmos vendor diff --component <component> --dry-run
```shell
