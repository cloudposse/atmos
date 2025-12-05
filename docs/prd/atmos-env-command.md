# PRD: `atmos env` Command

## Overview

The `atmos env` command outputs global environment variables configured in the `env` section of `atmos.yaml`. It supports multiple output formats and integrates with GitHub Actions for CI/CD workflows.

## Problem Statement

Users configure environment variables in `atmos.yaml` but have no way to:
1. Export these variables to their shell for use with external tools
2. Inspect what environment variables Atmos will set
3. Pass environment variables to GitHub Actions workflows

## Solution

A new `atmos env` command that:
- Outputs environment variables from `atmos.yaml` in various formats
- Supports shell evaluation via `eval $(atmos env)`
- Writes directly to GitHub Actions `$GITHUB_ENV` file

## Command Interface

```bash
# Default: bash export format to stdout
atmos env

# Specific formats
atmos env --format=json
atmos env --format=dotenv
atmos env --format=bash

# GitHub Actions: write to $GITHUB_ENV file
atmos env --format=github                    # errors if $GITHUB_ENV not set
atmos env --format=github --output=/tmp/env  # write to specific file

# With profiles (works automatically)
atmos env --profile=ci
ATMOS_PROFILE=ci atmos env
```

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format` | `-f` | `bash` | Output format: `bash`, `json`, `dotenv`, `github` |
| `--output` | `-o` | (none) | Output file path. For `github` format, defaults to `$GITHUB_ENV` |

## Output Formats

| Format | Output Example | Use Case |
|--------|----------------|----------|
| `bash` | `export KEY='value'` | `eval $(atmos env)` |
| `dotenv` | `KEY='value'` | `.env` files |
| `json` | `{"KEY": "value"}` | Programmatic consumption |
| `github` | `KEY=value` | GitHub Actions `$GITHUB_ENV` |

### Format Details

**bash**: Shell export statements with proper escaping for single quotes (`'` → `'\''`).
```bash
export GITHUB_TOKEN='ghp_xxxx'
export TF_PLUGIN_CACHE_DIR='/tmp/terraform-plugin-cache'
```

**dotenv**: Standard .env file format with same escaping.
```
GITHUB_TOKEN='ghp_xxxx'
TF_PLUGIN_CACHE_DIR='/tmp/terraform-plugin-cache'
```

**json**: JSON object for programmatic use.
```json
{
  "GITHUB_TOKEN": "ghp_xxxx",
  "TF_PLUGIN_CACHE_DIR": "/tmp/terraform-plugin-cache"
}
```

**github**: GitHub Actions environment file format (no quotes, no export).
```
GITHUB_TOKEN=ghp_xxxx
TF_PLUGIN_CACHE_DIR=/tmp/terraform-plugin-cache
```

## GitHub Actions Integration

When `--format=github` is specified:

1. If `--output` is provided, write to that file
2. If `--output` is not provided, read `$GITHUB_ENV` environment variable
3. If `$GITHUB_ENV` is not set, return an error with helpful message
4. **Append** to file (not overwrite) - GitHub Actions accumulates env vars

### Example GitHub Actions Workflow

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Export Atmos environment
        run: atmos env --format=github

      - name: Use environment variables
        run: |
          echo "Token available: $GITHUB_TOKEN"
          terraform init
```

## Scope

**In Scope (v1)**:
- Global `env` section from `atmos.yaml`
- Profile merging (via existing `--profile` flag)
- Output formats: bash, json, dotenv, github
- File output with `--output` flag

**Out of Scope (future)**:
- Component-level env (`atmos env --component vpc --stack prod`)
- Stack-level env
- Environment variable filtering

## Technical Design

### Data Source

Environment variables come from `AtmosConfiguration.Env` (`map[string]string`) which is populated by `cfg.InitCliConfig()`. This automatically handles:
- Base `atmos.yaml` configuration
- Profile merging via `--profile` flag
- YAML function evaluation (`!env`, `!exec`, etc.)

### Implementation Pattern

Follow the command registry pattern using `cmd/about/about.go` as reference:
- Implement `CommandProvider` interface
- Register in `init()` via `internal.Register()`
- Add blank import to `cmd/root.go`

### Output Functions

Reuse patterns from `cmd/auth_env.go`:
- Shell escaping: `'` → `'\''`
- Sorted key output for deterministic results
- JSON output via `u.PrintAsJSON()`

## Files to Create

| File | Purpose |
|------|---------|
| `cmd/env/env.go` | Command with CommandProvider pattern |
| `internal/exec/env.go` | Business logic and output formatters |
| `cmd/env/env_test.go` | Unit tests |
| `website/docs/cli/commands/env.mdx` | Documentation |

## Testing Strategy

1. **Unit tests** for each output format function
2. **Test GitHub format** error when `$GITHUB_ENV` not set
3. **Test file output** writes correctly (append mode)
4. **Test empty env** handling (no env vars configured)
5. **Integration test** with profiles

## Success Metrics

- Command executes in <100ms for typical configurations
- All output formats produce valid, parseable output
- GitHub Actions integration works without manual file handling

## References

- [Global env documentation](/cli/configuration/env)
- [Profiles documentation](/cli/configuration/profiles)
- [GitHub Actions environment files](https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#setting-an-environment-variable)
