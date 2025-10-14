- Only vendor the specified component

```bash
 $ atmos vendor pull --component <component>
```shell

- Pull the latest version of the specified component from the remote repository, filtering by type (terraform or helmfile).

```bash
 $ atmos vendor pull --component <component> --type <terraform|helmfile>
```

- Simulate pulling the latest version of the specified component from the remote repository without making any changes.

```bash
 $  atmos vendor pull --component <component> --dry-run
```shell

- Only vendor the components that have the specified tags

```bash
 $ atmos vendor pull --tags <dev,test>
```

- Vendor all components

```bash
 $ atmos vendor pull --everything
```shell
