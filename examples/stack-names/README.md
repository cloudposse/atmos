# Stack Names Example

This example demonstrates imperative stack naming using the `name` field in stack manifests.

## Overview

Stack names are determined by the following precedence (highest to lowest):

1. **`name`** - Explicit name from the stack manifest (always wins)
2. **`name_template`** - Template-based naming (if configured)
3. **`name_pattern`** - Pattern-based naming (if configured)
4. **Filename** - Basename of the stack file (fallback)

This example demonstrates the `name` field, which always takes precedence over other naming methods.

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
