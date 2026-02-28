# Atmos Workflow YAML Syntax Reference

Complete reference for workflow file format, all fields, and conventions.

## File Structure

Every workflow file must have a top-level `workflows:` key containing a map of named workflows.

```yaml
workflows:
  workflow-name-1:
    description: "Description of workflow 1"
    steps: []

  workflow-name-2:
    description: "Description of workflow 2"
    steps: []
```

## File Naming and Location

- Workflow files are stored in the directory configured by `workflows.base_path` in `atmos.yaml`
- Default path: `stacks/workflows/`
- File extension: `.yaml` (or `.yml`)
- File names can be anything; recommended: name by purpose, environment, or service
- The `--file` / `-f` flag value is relative to `workflows.base_path`
- The `.yaml` extension can be omitted in `--file` values

### Example Directory Layout

```text
stacks/workflows/
  deploy.yaml
  destroy.yaml
  networking.yaml
  eks.yaml
  database.yaml
  maintenance/
    backup.yaml
    rotate-credentials.yaml
```

## Workflow Definition Fields

```yaml
workflows:
  my-workflow:
    description: string          # Optional: Human-readable description
    stack: string                # Optional: Default Atmos stack for all steps
    working_directory: string    # Optional: Default working directory for all steps
    dependencies:                # Optional: Tool dependencies
      tools:
        tool-name: "version"
    steps:                       # Required: List of steps
      - ...
```

### description

Human-readable description of what the workflow does. Displayed in `atmos list workflows`
and the interactive workflow UI.

```yaml
description: |
  Deploy all networking infrastructure.
  Run this before deploying application components.
```

### stack

Default Atmos stack applied to all steps of type `atmos` that do not specify their own stack.
Can be overridden at the step level or on the command line with `--stack` / `-s`.

```yaml
stack: tenant1-ue2-dev
```

### working_directory

Default working directory for all steps. Can be overridden at the step level.

```yaml
working_directory: !repo-root    # Git repository root
working_directory: /tmp          # Absolute path
working_directory: scripts       # Relative to base_path
```

### dependencies

Declare tool dependencies auto-installed before execution:

```yaml
dependencies:
  tools:
    tflint: "0.54.0"            # Exact version
    checkov: "latest"           # Latest available
    terraform: "^1.10.0"        # SemVer compatible range
    kubectl: "~> 1.32.0"        # Pessimistic constraint
```

## Step Definition Fields

```yaml
steps:
  - command: string              # Required: Command to execute
    name: string                 # Optional: Step identifier
    type: atmos | shell          # Optional: Step type (default: atmos)
    stack: string                # Optional: Stack override for this step
    identity: string             # Optional: Authentication identity for this step
    working_directory: string    # Optional: Working directory override
    retry:                       # Optional: Retry configuration
      max_attempts: integer
      delay: duration
      backoff_strategy: string
      initial_delay: duration
      random_jitter: float
      multiplier: integer
      max_elapsed_time: duration
```

### command (Required)

The command to execute.

For **atmos** type: Write the command as you would after `atmos` on the command line. Atmos
automatically prepends the `atmos` binary name.

```yaml
# These are equivalent to running:
# atmos terraform plan vpc
# atmos terraform apply vpc -auto-approve
steps:
  - command: terraform plan vpc
  - command: terraform apply vpc -auto-approve
```

For **shell** type: Any shell command or script. Supports YAML multiline strings.

```yaml
steps:
  - command: echo "Hello"
    type: shell
  - command: |
      echo "Multi-line script"
      aws sts get-caller-identity
      if [ $? -eq 0 ]; then
        echo "Auth OK"
      fi
    type: shell
  - command: >-
      echo "Folded scalar,
      joins to single line"
    type: shell
```

### name

Step identifier used with `--from-step` flag to resume workflow execution from a specific step.

If omitted, Atmos auto-generates names: `step1`, `step2`, `step3`, etc. (1-indexed).

```yaml
steps:
  - command: terraform plan vpc
    name: plan-vpc
  - command: terraform apply vpc
    name: apply-vpc
  - command: echo "Done"           # Auto-named as step3
    type: shell
```

### type

Step type. Two values are supported:

| Type | Description | Implicit |
|------|-------------|----------|
| `atmos` | Atmos CLI command (atmos prefix auto-prepended) | Yes (default) |
| `shell` | Shell command or script | Must be explicit |

```yaml
steps:
  - command: terraform plan vpc              # type: atmos (implicit)
  - command: terraform apply vpc             # type: atmos (implicit)
    type: atmos                              # Explicit (optional)
  - command: echo "Hello"
    type: shell                              # Required for shell commands
```

### stack

Per-step stack override. Overrides the workflow-level `stack` attribute. Can itself be overridden
by the command-line `--stack` flag.

```yaml
steps:
  - command: terraform plan vpc
    stack: tenant1-ue2-dev
  - command: terraform plan vpc
    stack: tenant1-ue2-staging
```

### identity

Authentication identity for the step. Atmos authenticates using this identity before executing
the step, setting environment variables for credentials.

```yaml
steps:
  - command: terraform apply vpc -s prod
    identity: superadmin
  - command: terraform apply app -s prod
    identity: developer
```

**Precedence**: Step-level `identity` > `--identity` CLI flag > no authentication.

### working_directory

Per-step working directory override. Takes precedence over workflow-level `working_directory`.

```yaml
steps:
  - command: wget https://example.com/file.tar.gz
    working_directory: /tmp
    type: shell
  - command: make install              # Uses workflow-level working_directory
    type: shell
```

### retry

Retry configuration for the step. Retries the command on failure.

```yaml
retry:
  max_attempts: 3                    # Max retries (default: 1, meaning no retry)
  delay: 5s                          # Delay between retries (default: 5s)
  backoff_strategy: exponential      # constant | exponential | linear (default: constant)
  initial_delay: 3s                  # Initial delay for backoff strategies (default: 5s)
  random_jitter: 0.0                 # Random jitter added to delay (default: 0.0)
  multiplier: 2                      # Multiplier for exponential strategy (default: 2)
  max_elapsed_time: 4m               # Total timeout for all retries (default: 30m)
```

**Backoff strategies**:
- `constant` -- Same delay between each retry
- `exponential` -- Delay multiplied by `multiplier` each retry
- `linear` -- Delay increases by `initial_delay` each retry

## Stack Precedence

When multiple stack specifications exist, this is the priority order (highest first):

1. Command-line `--stack` / `-s` flag
2. Step-level `stack` attribute
3. Workflow-level `stack` attribute
4. Inline stack in the command string (e.g., `-s tenant1-ue2-dev` within the command)

The inline stack has the lowest priority and is overridden by all other methods.

## CLI Command Syntax

```shell
atmos workflow [workflow_name] [flags]
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Workflow file (relative to `workflows.base_path`) |
| `--stack` | `-s` | Override stack for all Atmos-type steps |
| `--from-step` | | Start execution from the named step |
| `--dry-run` | | Preview steps without executing |
| `--identity` | | Default identity for steps without explicit identity |

### Examples

```shell
# Auto-discovery
atmos workflow deploy -s plat-ue2-dev

# Explicit file
atmos workflow deploy -f networking -s plat-ue2-dev

# Resume from a step
atmos workflow deploy -f networking --from-step step3

# Dry run
atmos workflow deploy -f networking --dry-run

# With authentication
atmos workflow deploy -f networking -s plat-ue2-prod --identity superadmin

# Interactive UI (no arguments)
atmos workflow
```

## Configuration in atmos.yaml

```yaml
# atmos.yaml
workflows:
  # Base path for workflow files
  # Also: ATMOS_WORKFLOWS_BASE_PATH env var, --workflows-dir CLI flag
  base_path: "stacks/workflows"
```

The `base_path` can be absolute or relative. When the root `base_path` is set in `atmos.yaml`,
`workflows.base_path` is resolved relative to it.

## Complete Example

```yaml
# stacks/workflows/networking.yaml
workflows:
  deploy-networking:
    description: |
      Deploy all networking components to a stack.
      Usage: atmos workflow deploy-networking -f networking -s <stack>
    steps:
      - command: terraform deploy vpc
        name: deploy-vpc
        retry:
          max_attempts: 2
          backoff_strategy: constant
          delay: 10s
      - command: |
          echo "VPC deployed, verifying..."
          sleep 5
        type: shell
        name: verify-vpc
      - command: terraform deploy subnet
        name: deploy-subnet
      - command: terraform deploy nat-gateway
        name: deploy-nat
      - command: terraform deploy transit-gateway
        name: deploy-tgw
        identity: network-admin

  teardown-networking:
    description: Destroy all networking components in reverse order
    steps:
      - command: terraform destroy transit-gateway -auto-approve
        name: destroy-tgw
        identity: network-admin
      - command: terraform destroy nat-gateway -auto-approve
        name: destroy-nat
      - command: terraform destroy subnet -auto-approve
        name: destroy-subnet
      - command: terraform destroy vpc -auto-approve
        name: destroy-vpc

  plan-all-networking:
    description: Plan all networking components
    steps:
      - command: terraform plan vpc
        name: plan-vpc
      - command: terraform plan subnet
        name: plan-subnet
      - command: terraform plan nat-gateway
        name: plan-nat
      - command: terraform plan transit-gateway
        name: plan-tgw
```
