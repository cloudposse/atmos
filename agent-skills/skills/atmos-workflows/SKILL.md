---
name: atmos-workflows
description: "Workflow automation: multi-step workflows, Go template support, cross-component orchestration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Workflows

Atmos workflows combine multiple commands into executable units of work. They allow you to define
multi-step sequences of Atmos commands and shell scripts, run them in order, and coordinate
operations across multiple components and stacks.

## What Workflows Are

A workflow is a named sequence of steps defined in a YAML file. Each step is either:

- An **Atmos command** (type `atmos`, the default) -- any Atmos CLI command without the `atmos` prefix
- A **Shell command** (type `shell`) -- any shell script or command

Workflows solve the problem of needing to run multiple Terraform operations in a specific order,
such as provisioning a VPC before an EKS cluster, or deploying the same component across multiple
environments sequentially.

You can use Atmos Custom Commands in workflows, and workflows in Custom Commands -- they are
fully interoperable.

## Workflow File Location and Discovery

### Configuration

Workflow files are stored in the directory specified by `workflows.base_path` in `atmos.yaml`:

```yaml
# atmos.yaml
workflows:
  # Can also be set using ATMOS_WORKFLOWS_BASE_PATH env var or --workflows-dir flag
  base_path: "stacks/workflows"
```

If `base_path` is set at the root level of `atmos.yaml`, the `workflows.base_path` is resolved
relative to it.

### Directory Structure

Organize workflow files by purpose, environment, or team:

```text
stacks/workflows/
  deploy.yaml
  destroy.yaml
  validate.yaml
  networking.yaml
  eks.yaml
  maintenance/
    backup.yaml
    rotate-credentials.yaml
```

File names can be anything with a `.yaml` extension. Recommended naming conventions include
grouping by environment (`workflows-dev.yaml`, `workflows-prod.yaml`), by service
(`workflows-eks.yaml`), or by function (`deploy.yaml`, `validate.yaml`).

### Auto-Discovery

Atmos can automatically discover workflow files. When you run `atmos workflow <name>` without
the `--file` flag:

1. Atmos scans all YAML files in `workflows.base_path`
2. Finds workflows matching the given name
3. If exactly one file contains that workflow, runs it automatically
4. If multiple files contain a workflow with the same name, prompts for interactive selection
   (or errors in non-interactive mode with hints)

```shell
# Auto-discovery: Atmos finds the file automatically
atmos workflow eks-up --stack tenant1-ue2-dev

# Explicit file: always works, required when names conflict
atmos workflow eks-up -f networking --stack tenant1-ue2-dev
```

The `--file` value is relative to `workflows.base_path`. The `.yaml` extension is optional --
Atmos adds it automatically if not provided.

## Workflow YAML Syntax

Every workflow file must have a top-level `workflows:` key containing a map of named workflows.

### Basic Structure

```yaml
workflows:
  workflow-name:
    description: "What this workflow does"
    stack: tenant1-ue2-dev        # Optional: default stack for all steps
    steps:
      - command: terraform plan vpc
        name: plan-vpc            # Optional: step identifier
        type: atmos               # Optional: default is atmos
        stack: tenant1-ue2-dev    # Optional: per-step stack override
        identity: superadmin      # Optional: per-step authentication
```

### Step Types

**Atmos steps** (default type, `type: atmos` is implicit):

```yaml
steps:
  - command: terraform plan vpc
  - command: terraform apply vpc -auto-approve
  - command: terraform deploy eks/cluster
```

The `atmos` prefix is automatically prepended. Write commands as you would after `atmos` on the
command line.

**Shell steps** (require `type: shell`):

```yaml
steps:
  - command: echo "Starting deployment..."
    type: shell
  - command: |
      echo "Running multi-line script"
      aws sts get-caller-identity
      kubectl get nodes
    type: shell
```

Shell commands can be any simple or complex script. Use YAML multiline strings for complex scripts.

### Retry Configuration

Steps can be retried on failure:

```yaml
steps:
  - command: terraform apply vpc -auto-approve
    retry:
      max_attempts: 3               # Maximum attempts (default: 1, meaning no retry)
      delay: 5s                     # Delay between retries (default: 5s)
      backoff_strategy: exponential  # constant, exponential, or linear
      initial_delay: 3s             # Initial delay for backoff
      random_jitter: 0.0            # Random jitter added to delay
      multiplier: 2                 # Multiplier for exponential backoff
      max_elapsed_time: 4m          # Total timeout for all retries (default: 30m)
```

### Working Directory

Control where steps execute:

```yaml
workflows:
  build-all:
    description: Build from repository root
    working_directory: !repo-root    # Workflow-level default
    steps:
      - command: make build
        type: shell
      - command: wget https://example.com/archive.tar.gz
        working_directory: /tmp      # Step-level override
        type: shell
```

Path resolution:
- Absolute paths are used as-is
- Relative paths resolve against `base_path`
- `!repo-root` resolves to the git repository root

### Workflow Dependencies

Declare tool dependencies that are auto-installed before execution:

```yaml
workflows:
  validate:
    description: Validate infrastructure
    dependencies:
      tools:
        tflint: "^0.54.0"
        checkov: "latest"
    steps:
      - command: terraform validate vpc
      - command: tflint --recursive
        type: shell
```

Dependencies support SemVer constraints: exact (`1.10.3`), pessimistic (`~> 1.10.0`),
compatible (`^1.10.0`), or `latest`.

## Working with Stacks in Workflows

The stack for Atmos-type steps can be specified in four ways (lowest to highest priority):

### 1. Inline in the Command

```yaml
steps:
  - command: terraform plan vpc -s tenant1-ue2-dev
```

### 2. Workflow-Level Stack

```yaml
workflows:
  deploy-vpc:
    stack: tenant1-ue2-dev
    steps:
      - command: terraform plan vpc
      - command: terraform apply vpc -auto-approve
```

### 3. Step-Level Stack

```yaml
steps:
  - command: terraform plan vpc
    stack: tenant1-ue2-dev
  - command: terraform plan vpc
    stack: tenant1-ue2-staging
```

### 4. Command-Line Stack (Highest Priority)

```shell
atmos workflow deploy-vpc -f networking -s tenant1-ue2-dev
```

The command-line `--stack` flag overrides all other stack definitions.

**Best practice**: Create generic workflows without hardcoded stacks and provide the stack
on the command line. This makes workflows reusable across environments.

## Executing Workflows

### Basic Execution

```shell
# With auto-discovery
atmos workflow eks-up --stack tenant1-ue2-dev

# With explicit file
atmos workflow eks-up -f networking --stack tenant1-ue2-dev

# Dry run (preview steps without executing)
atmos workflow eks-up -f networking --dry-run
```

### Interactive Mode

Run `atmos workflow` without arguments to start an interactive TUI:

```shell
atmos workflow
```

Use arrow keys to navigate, `/` to search, and Enter to execute.

### Starting from a Specific Step

Use `--from-step` to resume a workflow from a named step:

```shell
# Start from step3 (skip step1 and step2)
atmos workflow eks-up -f networking --from-step step3
```

This is useful for resuming after a failure. If a step does not have an explicit `name`,
Atmos auto-generates names as `step1`, `step2`, `step3`, etc.

### Failure Handling

When a step fails, Atmos displays:
- Which step failed
- The exact command that failed
- A ready-to-use command to resume from the failed step

```console
Step 'step2' failed!

Command failed:
terraform plan vpc -s plat-ue2-staging

To resume the workflow from this step, run:
atmos workflow provision-vpcs -f networking --from-step step2
```

## Using Authentication with Workflows

### Per-Step Identity

Different steps can use different authentication identities:

```yaml
workflows:
  deploy-with-auth:
    description: Deploy with different identities
    steps:
      - command: terraform apply vpc -s plat-ue2-prod
        identity: superadmin
        name: deploy-vpc
      - command: terraform apply app -s plat-ue2-prod
        identity: developer
        name: deploy-app
      - command: |
          aws sts get-caller-identity
        type: shell
        identity: superadmin
        name: verify-identity
```

### Default Identity via Flag

Apply a default identity to all steps without their own:

```shell
atmos workflow deploy-all -f workflows --identity superadmin
```

**Precedence**: Step-level `identity` > `--identity` flag > no authentication.

### Mixed Authentication

Steps without an `identity` field use ambient credentials. Steps with `identity` authenticate
before execution. The `--identity` flag provides a default for steps without explicit identity.

```yaml
workflows:
  mixed:
    steps:
      - command: terraform plan vpc -s dev    # Uses ambient or --identity flag
      - command: terraform apply vpc -s prod
        identity: superadmin                  # Always uses superadmin
```

## Common Workflow Patterns

### Deploy All Components in Order

```yaml
workflows:
  deploy-all:
    description: Deploy all infrastructure in order
    steps:
      - command: terraform deploy vpc
      - command: terraform deploy eks/cluster
      - command: terraform deploy rds
      - command: terraform deploy app
```

```shell
atmos workflow deploy-all -f deploy -s plat-ue2-dev
```

### Mixed Atmos and Shell Commands

```yaml
workflows:
  deploy-and-verify:
    description: Deploy and verify infrastructure
    steps:
      - command: terraform deploy vpc
        name: deploy-vpc
      - command: kubectl get nodes
        type: shell
        name: verify
```

### Nesting Workflows

Workflows can call other workflows via `command: workflow <name>`.

For additional patterns (multi-env deploy, teardown, validation pipelines),
see [references/workflow-syntax.md](references/workflow-syntax.md).

## atmos workflow Command Reference

### Syntax

```shell
atmos workflow [workflow_name] [flags]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `workflow_name` | Name of the workflow to execute (optional; launches interactive UI if omitted) |

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Workflow file name (relative to `workflows.base_path`, `.yaml` extension optional) |
| `--stack` | `-s` | Override stack for all Atmos-type steps |
| `--from-step` | | Start execution from the named step |
| `--dry-run` | | Preview steps without executing |
| `--identity` | | Default identity for steps without explicit identity |

## Listing Workflows

```shell
# List all available workflows
atmos list workflows

# Describe all workflows with details
atmos describe workflows
```

## Best Practices

1. **Create generic, stack-agnostic workflows.** Do not hardcode stacks in workflow definitions.
   Instead, pass the stack on the command line with `-s`. This makes workflows reusable across
   all environments.

2. **Name your steps.** Use the `name` attribute so workflows can be resumed from a specific
   step with `--from-step` after a failure.

3. **Organize files by purpose.** Use separate files for deployment, teardown, validation, and
   maintenance workflows. Name files descriptively (e.g., `networking.yaml`, `eks.yaml`).

4. **Use dry-run before executing.** Always preview workflows with `--dry-run` before running
   them, especially in production environments.

5. **Order steps by dependency.** List steps in dependency order (VPC before EKS, database before
   application) since steps execute sequentially.

6. **Use shell steps for verification.** Insert shell steps between Atmos commands to verify
   infrastructure state before proceeding.

7. **Use retry for flaky operations.** Configure `retry` for steps that may fail transiently,
   such as cloud API calls that hit rate limits.

8. **Leverage authentication per step.** Use the `identity` field to switch credentials between
   steps that operate in different accounts or require different permission levels.

9. **Use unique workflow names.** Keep workflow names unique across all files to enable auto-discovery
   without ambiguity. If names must overlap, always use `--file` in scripts and CI/CD.

10. **Use `--from-step` for recovery.** When a workflow fails, fix the issue and resume from the
    failed step rather than re-running the entire workflow.

## Additional Resources

- For the complete workflow YAML syntax reference, see [references/workflow-syntax.md](references/workflow-syntax.md)
