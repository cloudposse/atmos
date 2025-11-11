- Display version in JSON format

```
$ atmos version --format json
```

- Display version in YAML format

```
$ atmos version --format yaml
```

- Pipe JSON output to jq

```
$ atmos version --format json | jq -r .version
```

- Check for updates and display in JSON

```
$ atmos version --check --format json
```
