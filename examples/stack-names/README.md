# Stack Names Example

This example demonstrates imperative stack naming using the `name` field in stack manifests.

## Overview

When no `name_template` or `name_pattern` is configured in `atmos.yaml`, stacks are identified by:

1. **`name`** (highest priority) - Explicit name from the stack manifest
2. **Filename** (fallback) - Basename of the stack file

## Stacks

| File | Name Field | Canonical Name |
|------|------------|----------------|
| `stacks/dev.yaml` | (none) | `dev` |
| `stacks/prod.yaml` | `production` | `production` |

## Usage

```bash
# List all stacks
atmos list stacks

# Dev stack - uses filename
atmos terraform plan mock -s dev

# Prod stack - uses explicit name
atmos terraform plan mock -s production

# This will NOT work (filename is not the canonical name):
# atmos terraform plan mock -s prod
```

## Key Points

- The `name` field takes precedence over the filename
- Only the canonical name is valid for the `-s` flag
- This is useful for migrations, legacy infrastructure, or matching existing Terraform workspace names
