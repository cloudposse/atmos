- Display version in JSON format

```bash
$ atmos version --format json
```

- Display version in YAML format

```bash
$ atmos version --format yaml
```

- Pipe JSON output to jq

```bash
$ atmos version --format json | jq -r .version
```

- Check for updates and display in JSON

```bash
$ atmos version --check --format json
```
