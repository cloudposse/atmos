- List every store backend configured under `stores:` in atmos.yaml

```shell
$ atmos store list
```

- Export the configured backends as JSON

```shell
$ atmos store list --format json
```

- List the key/value pairs stored inside a specific backend, scoped to a stack and component

```shell
$ atmos store list app-metadata --stack prod --component vpc
```

- List a backend's contents globally (no stack/component scope)

```shell
$ atmos store list app-metadata
```

- Export a backend's key/value pairs as YAML

```shell
$ atmos store list app-metadata --stack prod --component vpc --format yaml
```
