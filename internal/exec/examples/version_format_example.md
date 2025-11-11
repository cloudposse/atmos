## Valid Version Format Examples

### JSON Format
```bash
atmos version --format json
```

Output:
```json
{
  "version": "1.96.0",
  "os": "darwin",
  "arch": "arm64",
  "update_version": "1.97.0"
}
```

### YAML Format
```bash
atmos version --format yaml
```

Output:
```yaml
version: 1.96.0
os: darwin
arch: arm64
update_version: 1.97.0
```

### Pipe to jq
```bash
atmos version --format json | jq -r .version
```

Output:
```
1.96.0
```

### Check for Updates
```bash
atmos version --check --format json
```

Output:
```json
{
  "version": "1.96.0",
  "os": "darwin",
  "arch": "arm64",
  "update_version": "1.97.0"
}
```
