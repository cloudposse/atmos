# Native Terraform Migration Example

This example demonstrates migrating from native Terraform (with `.tfvars` files) to Atmos.

## Overview

The example shows two migration strategies side by side:

| Stack | File | Strategy | Stack Name |
|-------|------|----------|------------|
| `dev` | `stacks/dev.yaml` | Keep existing `.tfvars` via `!include` | `dev` (from filename) |
| `prod` | `stacks/prod.yaml` | Convert to native YAML | `production` (explicit `name`) |

## Directory Structure

```
├── atmos.yaml                        # Atmos configuration
├── components/terraform/vpc/         # Mock VPC component (null_resource)
├── legacy/                           # Existing .tfvars files (pre-migration)
│   ├── dev.tfvars
│   └── prod.tfvars
└── stacks/
    ├── _defaults.yaml                # Shared defaults (imported by both stacks)
    ├── dev.yaml                      # Uses !include for legacy tfvars
    └── prod.yaml                     # Fully converted to YAML
```

## Key Concepts

### `!include` for .tfvars

The dev stack uses `!include` to import existing `.tfvars` files directly:

```yaml
components:
  terraform:
    vpc:
      vars: !include ../legacy/dev.tfvars
```

This is the fastest migration path — your existing variable files keep working.

### Stack Names

The prod stack uses an explicit `name` field to override the filename:

```yaml
name: production
```

This means you reference it as `atmos terraform plan vpc -s production`, not `-s prod`.

### Imports

Both stacks import shared defaults from `_defaults.yaml`:

```yaml
import:
  - _defaults
```

## Usage

```bash
# List all stacks
atmos list stacks

# Dev stack (uses !include for tfvars)
atmos terraform plan vpc -s dev
atmos describe component vpc -s dev

# Prod stack (explicit name, converted to YAML)
atmos terraform plan vpc -s production
atmos describe component vpc -s production

# This will NOT work (filename is not the canonical name):
# atmos terraform plan vpc -s prod
```
