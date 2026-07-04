---
title: Custom Component Types
tags: [Components]
cast:
  file: /casts/examples/custom-components/script-command.cast
  title: atmos custom component command
---

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

1. Inspect the custom component type configuration:
   ```bash
   atmos describe config --format=yaml --query .components.script
   ```

2. Run the custom command:
   ```bash
   cd examples/custom-components
   atmos script deploy-app -s dev
   ```

3. Expected output:
   ```
   Component: deploy-app
   Stack: dev
   App: myapp
   Version: 1.0.0
   Replicas: 1
   Env: deploying v1.0.0 to us-east-1
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

4. **Environment Variables**:
   - The component `env` section is exported as real environment variables to every step,
     just like the built-in `terraform`/`helmfile`/`packer`/`ansible` component types.
   - Steps (and scripts they invoke) read them directly as `$APP_VERSION`, `$DEPLOY_REGION`, etc.
   - For sensitive values, use `!secret NAME` in the `env` section so the value resolves from a
     secret backend and is masked in output — never inline a secret into the command string.
     See [Passing secrets](https://atmos.tools/cli/configuration/secrets).
