- List Helm repositories associated with native Helm components.

```shell
 $ atmos helm repo list -s <stack>
```

- List Helm repositories for a specific component.

```shell
 $ atmos helm repo list <component> -s <stack>
```

- Output Helm repository associations as JSON.

```shell
 $ atmos helm repo list -s <stack> --format json
```
