# Generate Files

This example demonstrates how to use `atmos terraform generate files` to generate an entire Terraform component from stack configuration.

## Overview

The `generate` section in component configuration defines files that Atmos will create. This example generates a complete Terraform component including:

- `variables.tf` - Variable definitions
- `outputs.tf` - Output definitions
- `versions.tf` - Terraform version constraints
- `locals.tf` - Local values
- `terraform.tfvars` - Variable values
- `config.json` - Environment-specific configuration
- `README.md` - Component documentation

## Content Types

| Content Type | Behavior |
|--------------|----------|
| String (multiline) | Written as-is, supports Go templates |
| Map with `.json` extension | Serialized as pretty-printed JSON |
| Map with `.yaml`/`.yml` extension | Serialized as YAML |
| Map with `.tf`/`.hcl` extension | Serialized as HCL blocks |
| Map with `.tfvars` extension | Serialized as HCL attributes (no blocks) |

### HCL Generation

When using map syntax for `.tf` files, Atmos automatically handles:

- **Labeled blocks** (`variable`, `output`, `resource`, `data`, `module`, `provider`):
  ```yaml
  variable:
    app_name:
      type: string
      description: "Application name"
  ```
  Generates: `variable "app_name" { type = "string" ... }`

- **Unlabeled blocks** (`terraform`, `locals`):
  ```yaml
  terraform:
    required_version: ">= 1.0.0"
  ```
  Generates: `terraform { required_version = ">= 1.0.0" }`

**Note:** For blocks that need HCL expressions (like `value = var.app_name`), use string templates instead of map syntax.

## Template Variables

Available in templates via `{{ .variable }}`:

| Variable | Description |
|----------|-------------|
| `atmos_component` | Component name |
| `atmos_stack` | Stack name |
| `vars` | Component variables (map) |
| `vars.stage` | Stage from component vars |

## Auto-Generation

Enable automatic file generation on any terraform command:

```yaml
# atmos.yaml
components:
  terraform:
    auto_generate_files: true
```

With this enabled, files are regenerated before `init`, `plan`, `apply`, etc.

## Usage

### Generate the component

```bash
cd examples/generate-files
mkdir -p components/terraform/demo
atmos terraform generate files demo -s dev
```

### Preview without writing (dry-run)

```bash
atmos terraform generate files demo -s dev --dry-run
```

### Delete generated files (clean)

```bash
atmos terraform generate files demo -s dev --clean
```

## Generated Files

After running `atmos terraform generate files demo -s dev`:

**versions.tf** (HCL map syntax):
```hcl
terraform {
  required_version = ">= 1.0.0"
}
```

**locals.tf** (HCL map syntax with templates):
```hcl
locals {
  app_name    = "myapp-dev"
  environment = "dev"
}
```

**variables.tf** (string template):
```hcl
variable "app_name" {
  type        = string
  description = "Application name"
}
```

**terraform.tfvars** (flat HCL attributes):
```hcl
app_name = "myapp-dev"
version  = "1.0.0-dev"
```

**config.json** (JSON from map):
```json
{
  "app": "myapp-dev",
  "stage": "dev",
  "version": "1.0.0-dev"
}
```

## Project Structure

```text
generate-files/
├── atmos.yaml                    # Atmos configuration (auto_generate_files: true)
├── components/                   # Generated (gitignored)
│   └── terraform/
│       └── demo/
│           ├── variables.tf      # Generated
│           ├── outputs.tf        # Generated
│           ├── versions.tf       # Generated
│           ├── locals.tf         # Generated
│           ├── terraform.tfvars  # Generated
│           ├── config.json       # Generated
│           └── README.md         # Generated
└── stacks/
    ├── catalog/
    │   └── demo.yaml             # Component with generate section
    └── deploy/
        ├── dev.yaml              # Dev environment
        └── prod.yaml             # Prod environment
```

## Try It

```bash
cd examples/generate-files

# Create component directory
mkdir -p components/terraform/demo

# Generate files for dev
atmos terraform generate files demo -s dev

# See what was generated
ls components/terraform/demo/
cat components/terraform/demo/versions.tf
cat components/terraform/demo/locals.tf

# Generate for prod (different values)
atmos terraform generate files demo -s prod
cat components/terraform/demo/config.json

# Clean up
atmos terraform generate files demo -s dev --clean
```
