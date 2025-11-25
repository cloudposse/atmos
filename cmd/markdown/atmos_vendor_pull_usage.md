- Only vendor the specified component

```shell
 $ atmos vendor pull --component <component>
```

- Pull the latest version of the specified component from the remote repository, filtering by type (terraform or helmfile).

```shell
 $ atmos vendor pull --component <component> --type <terraform|helmfile>
```

- Simulate pulling the latest version of the specified component from the remote repository without making any changes.

```shell
 $  atmos vendor pull --component <component> --dry-run
```

- Only vendor the components that have the specified tags

```shell
 $ atmos vendor pull --tags <dev,test>
```

- Vendor all components

```shell
 $ atmos vendor pull --everything
```
