# Atmos Helmfile Commands Reference

Complete reference of all `atmos helmfile` subcommands with syntax and key flags.

## Command Syntax

```
atmos helmfile <subcommand> <component> -s <stack> [flags] [-- native-helmfile-flags]
```

The `component` argument and `--stack` / `-s` flag are required for all single-component operations.
Use `--` to pass flags directly to Helmfile without Atmos interpretation.

Atmos supports all Helmfile commands and options. In addition, the `component` argument and `stack` flag
are required to generate variables for the component in the stack.

## Core Lifecycle Commands

### diff

Show the differences between the current state and the desired state without making changes.

```shell
atmos helmfile diff <component> -s <stack> [flags]
```

```shell
atmos helmfile diff echo-server -s tenant1-ue2-dev
atmos helmfile diff echo-server -s tenant1-ue2-dev --redirect-stderr /dev/null
atmos helmfile diff nginx-ingress -s ue2-dev
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required)
- `--dry-run` -- Preview without executing
- `--redirect-stderr` -- Redirect stderr to file or descriptor
- `--global-options` -- Pass global Helmfile options

### apply

Install or upgrade Helm releases to match the desired state.

```shell
atmos helmfile apply <component> -s <stack> [flags]
```

```shell
atmos helmfile apply echo-server -s tenant1-ue2-dev
atmos helmfile apply echo-server -s tenant1-ue2-dev --redirect-stderr /dev/stdout
atmos helmfile apply nginx-ingress -s ue2-dev
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required)
- `--dry-run` -- Preview without executing
- `--redirect-stderr` -- Redirect stderr to file or descriptor
- `--global-options` -- Pass global Helmfile options

### sync

Synchronize the desired state with the cluster. Installs missing releases, upgrades existing ones.

```shell
atmos helmfile sync <component> -s <stack> [flags]
```

```shell
atmos helmfile sync echo-server --stack tenant1-ue2-dev
atmos helmfile sync echo-server --stack tenant1-ue2-dev --redirect-stderr ./errors.txt
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required)
- `--dry-run` -- Preview without executing
- `--redirect-stderr` -- Redirect stderr to file or descriptor
- `--global-options` -- Pass global Helmfile options

### destroy

Remove all releases managed by a component.

```shell
atmos helmfile destroy <component> -s <stack> [flags]
```

```shell
atmos helmfile destroy echo-server --stack=tenant1-ue2-dev
atmos helmfile destroy echo-server --stack=tenant1-ue2-dev --redirect-stderr /dev/stdout
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required)
- `--dry-run` -- Preview without executing
- `--redirect-stderr` -- Redirect stderr to file or descriptor
- `--global-options` -- Pass global Helmfile options

### deploy

Combine diff and apply in a single step with automatic approval.

```shell
atmos helmfile deploy <component> -s <stack> [flags]
```

```shell
atmos helmfile deploy nginx-ingress -s ue2-dev
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required)
- `--dry-run` -- Preview without executing
- `--redirect-stderr` -- Redirect stderr to file or descriptor
- `--global-options` -- Pass global Helmfile options

## Generation Commands

### generate varfile

Generate a variable file for a Helmfile component in a stack.

```shell
atmos helmfile generate varfile <component> -s <stack> [flags]
```

```shell
atmos helmfile generate varfile echo-server -s tenant1-ue2-dev
atmos helmfile generate varfile echo-server -s tenant1-ue2-dev -f vars.yaml
atmos helmfile generate varfile echo-server --stack tenant1-ue2-dev --file=vars.yaml
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required)
- `--file` / `-f` -- Custom output filename. If not specified, the varfile name is generated
  automatically from the context.
- `--dry-run` -- Preview without executing

## Source Management Commands

These commands manage Helmfile component sources with just-in-time (JIT) vendoring. Components declare
their source location inline using the top-level `source` field. Sources are automatically provisioned
when running Helmfile commands.

### source pull

Download and vendor a component source based on its `source` configuration.

```shell
atmos helmfile source pull <component> --stack <stack> [flags]
```

The pull command:
1. Reads the `source` configuration from the component's stack manifest
2. Downloads the source from the specified URI using go-getter
3. Applies any `included_paths` and `excluded_paths` filters
4. Copies the filtered content to the component directory

If the component is already vendored, it will be skipped unless `--force` is specified.

```shell
# Basic pull
atmos helmfile source pull ingress-nginx --stack dev

# Force re-vendor
atmos helmfile source pull ingress-nginx --stack dev --force

# With identity override for private sources
atmos helmfile source pull ingress-nginx --stack dev --identity admin
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required). Env: `ATMOS_STACK`.
- `--force` / `-f` -- Force re-vendor even if component directory exists.
- `--identity` / `-i` -- Identity for authentication when downloading from protected sources.

### source describe

Display the `source` configuration for a Helmfile component.

```shell
atmos helmfile source describe <component> --stack <stack>
```

Shows the source URI, version, and any path filters configured for vendoring. Output matches the
stack manifest schema format.

```shell
atmos helmfile source describe ingress-nginx --stack dev
```

Example output:
```yaml
components:
  helmfile:
    ingress-nginx:
      source:
        uri: github.com/cloudposse/helmfiles//releases/ingress-nginx
        version: 1.0.0
        included_paths:
          - "*.yaml"
          - "values/**"
        excluded_paths:
          - "*.md"
          - "tests/**"
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required). Env: `ATMOS_STACK`.

### source list

List all Helmfile components that have `source` configured.

```shell
atmos helmfile source list [component] [flags]
```

Automatically adjusts columns based on context:
- All stacks: Stack, Component, Folder, URI, Version
- Single stack (`--stack`): Component, Folder, URI, Version
- The Folder column only appears when any component uses `metadata.component`

```shell
# List all sources across all stacks
atmos helmfile source list

# Filter by stack
atmos helmfile source list --stack plat-ue2-dev

# Filter by component across all stacks
atmos helmfile source list ingress-nginx

# Output in different formats
atmos helmfile source list --format json
atmos helmfile source list --format yaml
atmos helmfile source list --format csv
atmos helmfile source list --format tsv
```

Key flags:
- `--stack` / `-s` -- Filter by stack name (optional). Env: `ATMOS_STACK`.
- `--format` / `-f` -- Output format: `table`, `json`, `yaml`, `csv`, `tsv` (default: `table`).
  Env: `ATMOS_FORMAT`.

### source delete

Remove the vendored source directory for a component. Requires `--force` for safety.

```shell
atmos helmfile source delete <component> --stack <stack> --force
```

Safety features:
- Requires `--force` to prevent accidental deletion
- Only works on components with `source` configured
- Shows a warning if the directory does not exist instead of failing

```shell
# Delete vendored component
atmos helmfile source delete ingress-nginx --stack dev --force
```

Key flags:
- `--stack` / `-s` -- Target Atmos stack (required). Env: `ATMOS_STACK`.
- `--force` / `-f` -- Required. Confirm deletion.

## Source Configuration Reference

The `source` field supports two formats.

### String Format

```yaml
source: "github.com/cloudposse/helmfiles//releases/ingress-nginx?ref=1.0.0"
```

### Map Format

```yaml
source:
  uri: github.com/cloudposse/helmfiles//releases/ingress-nginx
  version: 1.0.0
  included_paths:
    - "*.yaml"
    - "values/**"
  excluded_paths:
    - "*.md"
    - "tests/**"
  retry:
    max_attempts: 5
    initial_delay: 2s
    max_delay: 60s
    backoff_strategy: exponential
```

### Source Fields

| Field | Description |
|-------|-------------|
| `uri` | Go-getter compatible source URI. Supports git, s3, http, gcs, oci protocols. |
| `version` | Version tag, branch, or commit. Appended as `?ref=<version>` for git sources. |
| `included_paths` | Glob patterns for files to include. Only matching files are copied. |
| `excluded_paths` | Glob patterns for files to exclude. Applied after included_paths filtering. |
| `retry` | Optional retry config: `max_attempts`, `initial_delay`, `max_delay`, `backoff_strategy`. |

### Supported Source Protocols

| Protocol | URI Format |
|----------|------------|
| Git (GitHub) | `github.com/org/repo//path` |
| Git (generic) | `git::https://github.com/org/repo.git//path` |
| Git (SSH) | `git::ssh://git@github.com/org/repo.git//path` |
| S3 | `s3::https://s3-us-east-1.amazonaws.com/bucket/path.tar.gz` |
| HTTP/HTTPS | `https://releases.example.com/helmfiles/component.tar.gz` |
| OCI | `oci::registry.example.com/helmfiles/component:v1.0.0` |

### Authentication for Private Sources

Component-level identity configuration:

```yaml
components:
  helmfile:
    ingress-nginx:
      source:
        uri: github.com/my-org/private-helmfiles//releases/ingress-nginx
        version: v1.0.0
      auth:
        identities:
          github-deployer:
            default: true
            kind: github/app
            via:
              provider: github-app
```

Override with the `--identity` flag:

```shell
atmos helmfile source pull ingress-nginx --stack dev --identity admin
```

## EKS Integration Flags

These flags are available on all Helmfile lifecycle commands when `use_eks: true` is configured:

| Flag | Description |
|------|-------------|
| `--cluster-name` | Override EKS cluster name (takes precedence over all config options) |
| `--identity` | Identity for AWS authentication (replaces deprecated profile patterns) |

## Global Flags Available on All Commands

| Flag | Short | Description |
|------|-------|-------------|
| `--stack` | `-s` | Target Atmos stack (required) |
| `--dry-run` | | Preview without executing |
| `--redirect-stderr` | | Redirect stderr to file or file descriptor |
| `--global-options` | | Pass global options to Helmfile CLI |

## atmos.yaml Helmfile Configuration Reference

Settings under `components.helmfile` in `atmos.yaml`:

| Setting | Default | Env Variable | Description |
|---------|---------|-------------|-------------|
| `command` | `helmfile` | `ATMOS_COMPONENTS_HELMFILE_COMMAND` | Executable to run |
| `base_path` | `components/helmfile` | `ATMOS_COMPONENTS_HELMFILE_BASE_PATH` | Base path to Helmfile components |
| `use_eks` | `false` | `ATMOS_COMPONENTS_HELMFILE_USE_EKS` | Enable EKS kubeconfig integration |
| `kubeconfig_path` | | `ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH` | Directory for kubeconfig files |
| `cluster_name` | | `ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME` | Explicit EKS cluster name |
| `cluster_name_template` | | `ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_TEMPLATE` | Go template for dynamic cluster names |
| `cluster_name_pattern` | | `ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN` | Token replacement pattern (deprecated) |
| `helm_aws_profile_pattern` | | `ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN` | AWS profile pattern (deprecated) |

Command-line flag overrides:
- `--helmfile-command` -- Override the `command` setting
- `--helmfile-dir` -- Override the `base_path` setting

## Path-Based Component Resolution

Instead of specifying component names, you can use filesystem paths:

```shell
cd components/helmfile/echo-server
atmos helmfile diff . -s dev
atmos helmfile apply . -s dev
```

Supported path formats:
- `.` -- Current directory
- `./component` -- Relative path from current directory
- `../other-component` -- Relative path to sibling directory
- `/absolute/path/to/component` -- Absolute path

Path resolution requires the path to resolve to a single unique component in the stack. If multiple
components reference the same path, use the explicit component name instead.
