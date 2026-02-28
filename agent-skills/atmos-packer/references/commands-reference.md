# Atmos Packer Commands Reference

Complete reference for all `atmos packer` subcommands, flags, and usage patterns.

## General Syntax

```shell
atmos packer <sub-command> <atmos-component> --stack <atmos-stack> [atmos-flags] -- [packer-options]
```

- `<sub-command>` -- One of: `init`, `build`, `validate`, `inspect`, `output`, `version`, or `source`.
- `<atmos-component>` -- The Packer component name or filesystem path.
- `--stack` / `-s` -- The target Atmos stack (required for all commands except `version` and `source list`).
- `[atmos-flags]` -- Atmos-specific flags such as `--template`.
- `-- [packer-options]` -- Native Packer flags passed through after the `--` separator.

---

## atmos packer init

Initialize Packer and install plugins for a component in a stack.

### Syntax

```shell
atmos packer init <component> --stack <stack> [flags] -- [packer-options]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos Packer component name or path |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack |
| `--template` | `-t` | No | Packer template file or directory. Defaults to `.` (all `*.pkr.hcl` files). Can also be set via `settings.packer.template` in the stack manifest. CLI flag takes precedence. |

### Examples

```shell
# Initialize with all HCL files in the component directory
atmos packer init aws/bastion --stack nonprod

# Initialize with a specific template file
atmos packer init aws/bastion -s prod --template main.pkr.hcl

# Initialize with a non-default template
atmos packer init aws/bastion -s nonprod -t main.nonprod.pkr.hcl
```

---

## atmos packer build

Process a Packer template and build artifacts. Builds execute in parallel by default. A Packer
manifest (if configured via post-processor) is updated with build results.

### Syntax

```shell
atmos packer build <component> --stack <stack> [flags] -- [packer-options]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos Packer component name or path |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack |
| `--template` | `-t` | No | Packer template file or directory. Defaults to `.` (all `*.pkr.hcl` files). Can also be set via `settings.packer.template`. CLI flag takes precedence. |

### Examples

```shell
# Directory mode (default) -- all *.pkr.hcl files
atmos packer build aws/bastion --stack nonprod

# Explicit directory mode
atmos packer build aws/bastion --stack prod --template .

# Single file mode
atmos packer build aws/bastion -s prod --template main.pkr.hcl
atmos packer build aws/bastion -s nonprod -t main.nonprod.pkr.hcl

# Pass native Packer flags
atmos packer build aws/bastion -s prod -- -color=false
atmos packer build aws/bastion -s prod -- -force
atmos packer build aws/bastion -s prod -- -on-error=ask
```

---

## atmos packer validate

Validate the syntax and configuration of a Packer template.

### Syntax

```shell
atmos packer validate <component> --stack <stack> [flags] -- [packer-options]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos Packer component name or path |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack |
| `--template` | `-t` | No | Packer template file or directory. Defaults to `.`. Can also be set via `settings.packer.template`. CLI flag takes precedence. |

### Examples

```shell
atmos packer validate aws/bastion --stack prod
atmos packer validate aws/bastion -s prod --template main.pkr.hcl
atmos packer validate aws/bastion -s nonprod -t main.nonprod.pkr.hcl
```

---

## atmos packer inspect

Inspect a Packer template to see its variables, builders, provisioners, and post-processors.

### Syntax

```shell
atmos packer inspect <component> --stack <stack> [flags] -- [packer-options]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos Packer component name or path |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack |
| `--template` | `-t` | No | Packer template file or directory. Defaults to `.`. Can also be set via `settings.packer.template`. CLI flag takes precedence. |

### Examples

```shell
atmos packer inspect aws/bastion --stack nonprod
atmos packer inspect aws/bastion -s prod --template main.pkr.hcl
atmos packer inspect aws/bastion -s nonprod -t main.nonprod.pkr.hcl
```

### Sample Output

```text
Packer Inspect: HCL2 mode

> input-variables:

var.ami_name: "bastion-al2023-1754457104"
var.instance_type: "t4g.small"
var.region: "us-east-2"
var.source_ami: "ami-0013ceeff668b979b"
var.ssh_username: "ec2-user"

> local-variables:

> builds:

> <0>:
sources:
amazon-ebs.al2023

provisioners:
shell

post-processors:
0:
manifest
```

---

## atmos packer output

Retrieve output from a Packer manifest. This command is specific to Atmos -- Packer itself does
not have an `output` command. Manifests are generated during `packer build` when configured with
a manifest post-processor. Supports YQ expressions for extracting specific values.

### Syntax

```shell
atmos packer output <component> --stack <stack> [--query <yq-expression>]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos Packer component name or path |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack |
| `--query` | `-q` | No | YQ expression to extract sections/attributes from the Packer manifest |

### Examples

```shell
# Get full manifest output
atmos packer output aws/bastion -s prod

# Get artifact ID from the first build
atmos packer output aws/bastion -s prod --query '.builds[0].artifact_id'

# Extract just the AMI ID (second part after the colon)
atmos packer output aws/bastion -s prod -q '.builds[0].artifact_id | split(":")[1]'
```

### Sample Output (Full Manifest)

```yaml
builds:
- artifact_id: us-east-2:ami-0c2ca16b7fcac7529
  build_time: 1.753281956e+09
  builder_type: amazon-ebs
  custom_data: null
  files: null
  name: al2023
  packer_run_uuid: 5114a723-92f6-060f-bae4-3ac2d0324557
- artifact_id: us-east-2:ami-0b2b3b68aa3c5ada8
  build_time: 1.7540253e+09
  builder_type: amazon-ebs
  custom_data: null
  files: null
  name: al2023
  packer_run_uuid: a57874d1-c478-63d7-cfde-9d91e513eb9a
  last_run_uuid: a57874d1-c478-63d7-cfde-9d91e513eb9a
```

### Sample Output (With Query)

```shell
> atmos packer output aws/bastion -s nonprod --query '.builds[0].artifact_id'
us-east-2:ami-0c2ca16b7fcac7529

> atmos packer output aws/bastion -s nonprod -q '.builds[0].artifact_id | split(":")[1]'
ami-0c2ca16b7fcac7529
```

---

## atmos packer version

Display the currently installed Packer version. This command does not require a component or stack.

### Syntax

```shell
atmos packer version
```

### Examples

```shell
atmos packer version
```

---

## atmos packer source

Parent command for managing Packer component sources with just-in-time (JIT) vendoring.

### Syntax

```shell
atmos packer source <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `pull` | Download and vendor a component source |
| `describe` | Display source configuration for a component |
| `list` | List all components with source configured |
| `delete` | Remove a vendored component directory |

---

## atmos packer source pull

Vendor a Packer component source based on its `source` configuration. Downloads the component
from the specified URI and places it in the appropriate component directory.

### Syntax

```shell
atmos packer source pull <component> --stack <stack> [flags]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos component name |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack name. Also: `ATMOS_STACK` env var. |
| `--force` | `-f` | No | Force re-vendor even if the component directory already exists |
| `--identity` | `-i` | No | Identity to use for authentication when downloading from protected sources |

### Examples

```shell
# Basic vendor
atmos packer source pull ami-builder --stack dev

# Force re-vendor
atmos packer source pull ami-builder --stack dev --force

# With identity override
atmos packer source pull ami-builder --stack dev --identity admin
```

### Output

```text
Vendoring component 'ami-builder' from source...
Downloading from github.com/cloudposse/packer-templates//ami-builder?ref=1.0.0
Successfully vendored component to components/packer/ami-builder
```

---

## atmos packer source describe

Display the `source` configuration for a Packer component. The output matches the stack manifest
schema format.

### Syntax

```shell
atmos packer source describe <component> --stack <stack>
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos component name |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack name. Also: `ATMOS_STACK` env var. |

### Examples

```shell
atmos packer source describe ami-builder --stack dev
```

### Sample Output

```yaml
components:
  packer:
    ami-builder:
      source:
        uri: github.com/cloudposse/packer-templates//ami-builder
        version: 1.0.0
        included_paths:
          - "*.pkr.hcl"
          - "scripts/**"
        excluded_paths:
          - "*.md"
          - "tests/**"
```

---

## atmos packer source list

List all Packer components that have `source` configured. Shows which components can be vendored
using the source provisioner.

### Syntax

```shell
atmos packer source list [component] [flags]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | No | Filter results to a specific component name or folder |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | No | Filter by stack. When provided, omits the Stack column. Also: `ATMOS_STACK` env var. |
| `--format` | `-f` | No | Output format: `table`, `json`, `yaml`, `csv`, `tsv`. Default: `table`. Also: `ATMOS_FORMAT` env var. |

### Dynamic Columns

| Context | Columns Shown |
|---------|---------------|
| All stacks | Stack, Component, Folder*, URI, Version |
| Single stack (`--stack`) | Component, Folder*, URI, Version |

*Folder column only appears when any component uses `metadata.component` for a different folder name.

### Examples

```shell
# List all Packer sources across all stacks
atmos packer source list

# List sources in a specific stack
atmos packer source list --stack plat-ue2-dev

# List sources for a specific component across stacks
atmos packer source list ami-builder

# Output in JSON format
atmos packer source list --format json

# Output in YAML format
atmos packer source list --format yaml

# Output in CSV format (for piping)
atmos packer source list --format csv

# Output in TSV format (tab-separated)
atmos packer source list --format tsv
```

### Sample Output (All Stacks)

```text
STACK              COMPONENT       URI                                                    VERSION
plat-ue2-dev       ami-builder     github.com/cloudposse/packer-templates//ami-builder    1.0.0
plat-ue2-dev       docker-builder  github.com/cloudposse/packer-templates//docker         1.0.0
plat-ue2-prod      ami-builder     github.com/cloudposse/packer-templates//ami-builder    1.1.0
```

### Sample Output (Single Stack)

```text
COMPONENT       URI                                                    VERSION
ami-builder     github.com/cloudposse/packer-templates//ami-builder    1.0.0
docker-builder  github.com/cloudposse/packer-templates//docker         1.0.0
```

---

## atmos packer source delete

Delete the vendored source directory for a Packer component. Requires `--force` flag for safety.

### Syntax

```shell
atmos packer source delete <component> --stack <stack> --force
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos component name |

### Flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack name. Also: `ATMOS_STACK` env var. |
| `--force` | `-f` | Yes | Confirm deletion. Required for safety. |

### Safety Features

1. **Requires `--force`** -- Prevents accidental deletion.
2. **Only deletes source-managed components** -- Only works on components with `source` configured.
3. **Non-destructive on missing directories** -- Shows a warning if the directory does not exist.

### Examples

```shell
# Delete vendored component (requires --force)
atmos packer source delete ami-builder --stack dev --force

# Without --force flag (fails with error)
atmos packer source delete ami-builder --stack dev
# Error: --force flag is required
# Hint: Use --force to confirm deletion
```

### Sample Output

```text
Deleting directory: components/packer/ami-builder
Successfully deleted: components/packer/ami-builder
```

---

## Path-Based Component Resolution

All packer commands support using filesystem paths instead of component names.

### Supported Path Formats

| Format | Description |
|--------|-------------|
| `.` | Current directory |
| `./component` | Relative path from current directory |
| `../other-component` | Relative path to sibling directory |
| `/absolute/path/to/component` | Absolute path |

### Requirements

- Must be inside a component directory under the configured base path.
- Must specify `--stack` flag.
- Component must exist in the specified stack configuration.
- The component path must resolve to a unique component name. If multiple components in the stack
  reference the same path, use the explicit component name instead.

### Examples

```shell
# Navigate to component directory and use current directory
cd components/packer/aws/bastion
atmos packer validate . -s prod
atmos packer build . -s prod

# Use relative path from components/packer directory
cd components/packer
atmos packer init ./aws/bastion -s prod

# From project root with relative path
atmos packer build components/packer/aws/bastion -s prod

# Combine with other flags
cd components/packer/aws/bastion
atmos packer validate . -s prod --template main.pkr.hcl
atmos packer build . -s prod -t main.nonprod.pkr.hcl
atmos packer output . -s prod --query '.builds[0].artifact_id'
```

---

## Common Flag Summary

| Flag | Short | Applies To | Description |
|------|-------|------------|-------------|
| `--stack` | `-s` | All (except version) | Target Atmos stack |
| `--template` | `-t` | init, build, validate, inspect | Packer template file or directory |
| `--query` | `-q` | output | YQ expression for manifest parsing |
| `--force` | `-f` | source pull, source delete | Force operation |
| `--identity` | `-i` | source pull | Authentication identity override |
| `--format` | `-f` | source list | Output format (table/json/yaml/csv/tsv) |

---

## Source Configuration Reference

### String Format

```yaml
source: "github.com/cloudposse/packer-templates//ami-builder?ref=1.0.0"
```

### Map Format

```yaml
source:
  uri: github.com/cloudposse/packer-templates//ami-builder
  version: 1.0.0
  included_paths:
    - "*.pkr.hcl"
    - "*.pkr.json"
    - "scripts/**"
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
| `uri` | Go-getter compatible source URI. Supports git, s3, http, gcs, oci. |
| `version` | Version tag, branch, or commit. Appended as `?ref=<version>` for git sources. |
| `included_paths` | Glob patterns for files to include. Only matching files are copied. |
| `excluded_paths` | Glob patterns for files to exclude. Applied after included_paths. |
| `retry.max_attempts` | Maximum number of download retry attempts. |
| `retry.initial_delay` | Initial delay between retries. |
| `retry.max_delay` | Maximum delay between retries. |
| `retry.backoff_strategy` | Backoff strategy: `exponential`, `linear`, or `constant`. |
| `retry.multiplier` | Multiplier for exponential backoff. |
| `retry.random_jitter` | Add random jitter to retry delays. |
| `retry.max_elapsed_time` | Maximum total elapsed time for all retries. |

### Supported Protocols

| Protocol | URI Format |
|----------|------------|
| Git (GitHub shorthand) | `github.com/org/repo//path` |
| Git (HTTPS) | `git::https://github.com/org/repo.git//path` |
| Git (SSH) | `git::ssh://git@github.com/org/repo.git//path` |
| S3 | `s3::https://s3-us-east-1.amazonaws.com/bucket/path.tar.gz` |
| HTTP/HTTPS | `https://releases.example.com/templates/component.tar.gz` |
| OCI | `oci::registry.example.com/templates/component:v1.0.0` |

---

## Error Reference

### Missing Source Configuration

```text
Error: source not configured

Hint: Add source to the component configuration in your stack manifest

Example:
  components:
    packer:
      ami-builder:
        source:
          uri: github.com/cloudposse/packer-templates//ami-builder
          version: 1.0.0
```

### Component Directory Already Exists

```text
Error: component directory already exists

Hint: Use --force to overwrite the existing directory
```

### Download Failed

```text
Error: failed to download source

Cause: failed to clone repository: authentication required

Hint: Check credentials or use --identity to specify authentication
```

### Delete Without Force

```text
Error: --force flag is required

Explanation: Deletion requires --force flag for safety

Hint: Use --force to confirm deletion
```
