# Custom Components Example

This example demonstrates how to define custom component types in Atmos using custom commands.

## Overview

Custom component types allow you to extend Atmos beyond the built-in component types (Terraform, Helmfile, Packer). You can define any component type (e.g., `script`, `ansible`, `manifest`) and access its configuration from stack files via Go templates.

## Structure

```text
examples/custom-components/
├── atmos.yaml                           # Custom commands + a Redis store
├── components/
│   ├── script/
│   │   └── deploy-app/
│   │       └── deploy.sh                # Example script (not executed)
│   └── hello/
│       └── world/
│           └── greeting.sh              # Example greeting script (illustrative)
└── stacks/
    ├── catalog/
    │   ├── script/
    │   │   └── deploy-app.yaml          # Base component configuration
    │   └── hello/
    │       └── world.yaml              # hello component + after.hello.greeting hook
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
   ```text
   Component: deploy-app
   Stack: dev
   App: myapp
   Version: 1.0.0
   Replicas: 1
   ```

## Hello-world: post-run hooks → store

The `hello` component type demonstrates the post-run lifecycle hooks: a custom
command publishes outputs, and a `store` hook persists them after the command
succeeds — the same loop `atmos terraform apply` + `after-terraform-apply` gives
Terraform components.

Lifecycle events are named `<phase>.<type>.<subcommand>`. `atmos hello greeting world`
fires `before.hello.greeting` then `after.hello.greeting`; `atmos hello describe world`
fires `before.hello.describe` / `after.hello.describe`. Because the subcommand is
part of the event name, the greeting store hook runs for `greeting` but **not** for
`describe`. (The verb is `greeting` rather than `apply` on purpose — `apply` is a
terraform-flavored verb; custom types pick their own verbs.)

1. Start a local Redis (the store this example writes to):
   ```bash
   docker run --rm -d -p 6379:6379 redis
   ```

2. Run the greeting command — this runs the steps (which append outputs to
   `$ATMOS_OUTPUTS`) and then fires `after.hello.greeting`, whose `store` hook
   writes the outputs to Redis:
   ```bash
   cd examples/custom-components
   ../../build/atmos hello greeting world -s dev
   ```

3. Confirm the values were stored (key layout is `<prefix>/<stack>/<component>/<key>`):
   ```bash
   redis-cli KEYS 'dev/world/*'
   redis-cli GET 'dev/world/greeting'        # -> "hello from dev"
   ```

4. `describe` does NOT trigger the greeting hook (different event name):
   ```bash
   ../../build/atmos hello describe world -s dev   # nothing written to Redis
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
