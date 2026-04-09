# Source Provisioning

Demonstrates JIT (Just-in-Time) source provisioning from both **local** and **remote** sources.

## Components

| Component | Source Type | URI |
|-----------|-------------|-----|
| `weather` | Local | `../demo-library/weather` |
| `ipinfo` | Remote | `github.com/cloudposse/atmos//examples/demo-library/ipinfo` |

## Usage

```bash
cd examples/source-provisioning

# Local source - no network required
atmos terraform plan weather --stack dev

# Remote source - vendors from GitHub
atmos terraform plan ipinfo --stack dev

# List provisioned workdirs
atmos terraform workdir list
```

## Key Concepts

1. **Local Paths** - Relative paths like `../demo-library/weather`
2. **Remote URIs** - GitHub URLs with version pinning
3. **Workdir Isolation** - Both types provision to `.workdir/`

## Cleanup

```bash
rm -rf .workdir/
```
