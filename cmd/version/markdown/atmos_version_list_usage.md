- List the last 10 releases (default)

```
$ atmos version list
```

- List the last 20 releases

```
$ atmos version list --limit 20
```

- List releases starting from offset 10

```
$ atmos version list --offset 10
```

- Include pre-releases

```
$ atmos version list --include-prereleases
```

- List releases since a specific date

```
$ atmos version list --since 2025-01-01
```

- Output as JSON

```
$ atmos version list --format json
```

- Output as YAML

```
$ atmos version list --format yaml
```

- Combine multiple options

```
$ atmos version list --limit 5 --include-prereleases --format json
```
