---
name: atmos-workflows
description: "Workflow automation: native step types, multi-step workflows, parallel/matrix/wait/container/emulator steps, when: conditions (CEL), require/assert preconditions, output steps, retries, dependencies, and cross-component orchestration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Workflows

Atmos workflows combine multiple commands into executable units of work. They allow you to define
multi-step sequences of typed steps, run them in order, and coordinate operations across multiple
components and stacks.

## What Workflows Are

A workflow is a named sequence of typed steps. Default to native step types before inline shell
scripts; large shell blocks, repeated `echo`, hand-rolled parallelism, and sleeps are red flags.

Core command step types:

- `atmos` -- any Atmos CLI command without the `atmos` prefix
- `shell` or `exec` -- shell/process execution
- `container` -- build, run, push, or operate containers
- `emulator` -- start or operate emulators
- `http` -- HTTP calls
- `require` (alias `assert`) -- verify required tools/files/dirs are present before continuing;
  never installs anything (see [Preconditions with `require`](#preconditions-with-require-alias-assert))

Key orchestration step types:

- `parallel` -- run child steps concurrently
- `matrix` -- expand axes into a cartesian product and run with the parallel scheduler
- `wait` and `wait-all` -- wait for background services or multiple dependencies
- `cancel` -- cancel running work

Output and UI step types include `toast`, `markdown`, `table`, `pager`, `format`, `join`, `style`,
`log`, `spin`, `stage`, and `linebreak`; use them instead of `echo` for operator-facing output.

For the `cast` and `simulate` step types (recording a workflow run or a scripted demo session as
an asciicast), see [atmos-cast](../atmos-cast/SKILL.md).

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
  - type: toast
    level: info
    content: Starting deployment...
  - command: kubectl get nodes
    type: shell
```

Shell commands can be any script, but use them intentionally: external CLIs, short glue commands,
terminal-native tools, or checked-in scripts. If a block mostly prints status, branches, starts
services, loops, waits, formats output, or writes CI metadata, replace it with native step types.

### Conditional Execution with `when`

Any step can carry a `when` field that decides whether the step runs. Atmos evaluates `when` with
[CEL](https://github.com/google/cel-go) (Common Expression Language) rather than Go templating: CEL is
a small, side-effect-free expression language -- no file access, no loops, no arbitrary code execution
-- which makes it safe to evaluate against CI-controlled or otherwise untrusted values.

Built-in predicate keywords are checked first and keep a fixed meaning: `ci`, `local`, `always`,
`never`, `success`, `failure`.

```yaml
steps:
  - name: plan
    type: atmos
    command: terraform plan vpc
    when: ci               # only runs when Atmos detects it is running in CI
```

Any other non-empty scalar string that is not a predicate keyword is evaluated as a bare CEL
expression:

```yaml
steps:
  - name: prod-ci-check
    type: shell
    command: ./scripts/check-prod.sh
    when: stack == "prod" && ci
```

Use the explicit `!cel` tag when a value is intended as CEL, or could otherwise be mistaken for a
predicate keyword:

```yaml
steps:
  - name: prod-ci-check
    type: shell
    command: ./scripts/check-prod.sh
    when: !cel 'stack == "prod" && ci'
```

`when` also accepts a YAML list. Every item in the list must evaluate true (the items are ANDed, not
ORed) for the step to run:

```yaml
steps:
  - name: cleanup
    type: shell
    command: ./scripts/cleanup.sh
    when: [ci, always]      # runs only when BOTH ci and always evaluate true
```

Workflow step CEL expressions can use `ci`, `status`, `stack`, `component`, `workflow`, `step`, and
`env`. Predicate keywords win over CEL for bare strings, so `when: ci` uses the built-in predicate
while `when: !cel 'ci'` evaluates CEL. Workflow steps do not support the built-in `failure` predicate;
in this version, step CEL sees `status == "success"`.

`when: manual` is **not** an Atmos step predicate -- there is no built-in "requires manual approval"
gate on workflow steps. If you see `when: manual` in a CI example (for instance, inside a
`.gitlab-ci.yml` snippet in the planfile docs), that is GitLab CI's own job-level `when:` keyword,
unrelated to the Atmos workflow step `when:` documented here. Atmos's own manual-gate pattern is the
plan/apply split: generate a planfile in one step (or CI job), then require a human, or a CI
environment protection rule, to trigger the `apply --from-plan` step.

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

### Modern Orchestration Steps

Use `parallel` when independent steps can run at the same time:

```yaml
steps:
  - type: parallel
    name: plan-all
    max_concurrency: 4
    fail:
      mode: wait_all
    steps:
      - type: atmos
        command: terraform plan vpc
      - type: atmos
        command: terraform plan eks/cluster
```

Use `matrix` when the same step should run for many stack/component combinations:

```yaml
steps:
  - type: matrix
    axes:
      stack: [dev, staging]
      component: [vpc, eks/cluster]
    steps:
      - type: atmos
        command: terraform plan {{ .matrix.component }} -s {{ .matrix.stack }}
```

Use `wait` or `wait-all` for background containers/emulators that must become healthy before
dependent steps run.

#### Background Steps with `background: true`, `with:`, and `for:` Targeting

A step becomes a background step by setting `background: true`. Atmos starts it and immediately
continues to the next step while it supervises the background step in a run-scoped registry. In v1,
`background: true` is only accepted on `type: container` steps (background `shell`/`atmos` support is
planned); a background step also cannot set `tty` or `interactive`.

Container steps configure their build/run/push/inspect behavior through a `with:` block (the block's
shape depends on the container action). For a run action, `with:` accepts fields including: `image`,
`command`, `shell`, `provider`, `runtime_auto_start`, `runtime`, `pull`, `workspace`,
`workspace_read_only`, `cleanup`, `user`, `run_args`, `mounts` (each with `type`/`source`/`target`/
`read_only`), `ports` (each with `host`/`container`/`protocol`), `restart` (`policy`/`max_retries`),
and `healthcheck` (`test`/`interval`/`timeout`/`start_period`/`start_interval`/`retries`/`disable`).

`wait`, `wait-all`, and `cancel` steps then coordinate with background steps using `for:`:

- `wait` blocks until the background step(s) named in `for:` become ready/healthy. `for:` is
  **required** and names one or more background step names (a scalar or a YAML sequence).
- `wait-all` blocks until every background step started so far in the workflow is ready; it takes no
  `for:` and is useful after `parallel`/`matrix` steps started several background containers and you
  just need all of them ready without naming each one.
- `cancel` tears down the background step(s) named in `for:` (also **required**).

```yaml
steps:
  - name: start-postgres
    type: container
    background: true
    with:
      image: postgres:16
      ports:
        - host: 5432
          container: 5432
      healthcheck:
        test: ["CMD-SHELL", "pg_isready -U postgres"]
        interval: 2s
        retries: 10

  - name: wait-for-postgres
    type: wait
    for: [start-postgres]

  - command: terraform apply rds-consumer -auto-approve
    name: apply-consumer

  - name: stop-postgres
    type: cancel
    for: [start-postgres]
```

A background step's name must stay unique while it is live; reuse of the same name is only allowed
after it has been `cancel`led.

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

No separate `atmos toolchain install` step is needed for workflow-declared tools; Atmos installs and
injects them when the workflow runs. Reserve explicit installs for cache warming or job-level scripts.

### Preconditions with `require` (alias `assert`)

The `require` step type (also usable as `type: assert`) is a read-only preconditions gate: it checks
that tools, files, and/or directories already exist, and fails fast with a single aggregated, hinted
error listing everything missing. It never installs anything and never mutates `PATH` or the
environment.

```yaml
steps:
  - name: check-prereqs
    type: require
    tools:
      - kubectl
      - helm
    files:
      - kubeconfig.yaml
    dirs:
      - ./manifests
    hint: "Run `atmos toolchain install`, or open this repo in the devcontainer, then retry."
```

- `tools` -- executable names (or paths) that must resolve on `PATH` (supports templates).
- `files` -- paths that must exist (supports templates).
- `dirs` -- paths that must exist and be directories (supports templates).
- `hint` -- an optional extra remediation note appended to the failure error (supports templates).

At least one of `tools`, `files`, or `dirs` must be set; an empty `require` step fails validation.

**Unlike `dependencies.tools`, `require` never installs -- it only verifies.** Use it when a tool must
already be present (e.g. provided by the CI runner image or devcontainer) and you want a clear, early
failure instead of a confusing downstream error when the real command finally runs.

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

### Native Steps with Shell Where Needed

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

5. **Prefer native step types over inline shell orchestration.** Use `parallel`, `matrix`, `wait`,
   `wait-all`, `container`, `emulator`, output/UI steps, and explicit dependencies before writing
   shell loops, sleeps, background jobs, or status `echo` blocks.

6. **Keep shell steps small and purposeful.** Shell is fine for external CLIs, probes, repo scripts,
   and terminal-native commands, but a long shell block usually belongs in a script or should become
   typed workflow steps.

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
