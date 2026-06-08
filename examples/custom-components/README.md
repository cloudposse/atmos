# Custom Components Example

This example demonstrates how to define custom component types in Atmos using custom commands.

## Overview

Custom component types allow you to extend Atmos beyond the built-in component types (Terraform, Helmfile, Packer). You can define any component type (e.g., `script`, `ansible`, `manifest`) and access its configuration from stack files via Go templates.

## Structure

```
examples/custom-components/
├── atmos.yaml                           # Atmos configuration with custom command
├── components/
│   └── script/
│       └── deploy-app/
│           └── deploy.sh                # Example script (not executed)
└── stacks/
    ├── catalog/
    │   └── script/
    │       └── deploy-app.yaml          # Base component configuration
    └── deploy/
        └── dev.yaml                     # Stack with overrides
```

## Usage

1. Build atmos (from repo root):
   ```bash
   make build
   ```

2. Run the custom command:
   ```bash
   cd examples/custom-components
   ../../build/atmos script deploy-app -s dev
   ```

3. Expected output:
   ```
   Component: deploy-app
   Stack: dev
   App: myapp
   Version: 1.0.0
   Replicas: 1
   ```

## How It Works

1. **Custom Command Definition** (`atmos.yaml`):
   - The `script` command defines a custom component type
   - Arguments and flags with `type: component` and `semantic_type: stack` identify which values to use
   - The `component:` section specifies the component type name

2. **Stack Configuration** (`stacks/`):
   - Components are defined under `components.script.<name>`
   - Standard Atmos inheritance and overrides work as expected

3. **Template Access**:
   - Steps can access component config via `{{ .Component.* }}`
   - All component sections are available: `vars`, `settings`, `metadata`, etc.
