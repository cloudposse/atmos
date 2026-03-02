---
name: atmos-helmfile
description: "Helmfile orchestration: sync/apply/destroy/diff, Kubernetes deployments, varfile generation, EKS integration, source management"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Helmfile Orchestration

Atmos wraps the Helmfile CLI to provide stack-aware orchestration of Kubernetes deployments. Instead of
manually managing kubeconfig, variable files, and authentication for each Helmfile component, Atmos
resolves the full configuration from stack manifests and handles all of these concerns automatically.

## How Atmos Orchestrates Helmfile

When you run any `atmos helmfile` command, Atmos performs the following sequence:

1. **Resolves stack configuration** -- Reads and deep-merges all stack manifests to produce the fully resolved
   configuration for the target component in the target stack.
2. **Generates variable file** -- Writes a varfile containing all `vars` defined for the component in the stack.
3. **Configures EKS authentication** -- If `use_eks: true`, runs `aws eks update-kubeconfig` to generate
   kubeconfig from the EKS cluster and set up authentication.
4. **Executes the requested command** -- Runs `helmfile diff`, `apply`, `sync`, `destroy`, etc. with the
   generated varfile and any additional flags.

This means a single command like `atmos helmfile apply nginx-ingress -s ue2-dev` replaces what would normally
require multiple manual steps: configuring kubeconfig, writing variable files, and then running helmfile.

## Stack Configuration for Helmfile Components

Helmfile components are defined under the `components.helmfile` section in stack manifests:

```yaml
components:
  helmfile:
    nginx-ingress:
      metadata:
        type: real
        component: nginx-ingress

      settings: {}

      vars:
        installed: true
        namespace: ingress
        chart_version: "4.0.0"

      env:
        HELM_DEBUG: "true"
```

### Component Attributes

- **`vars`** -- Variables passed to Helmfile. Deep-merged and available to your Helmfile configuration.
- **`metadata`** -- Extends component functionality. Supports `type`, `component`, and `inherits` for
  inheritance chains.
- **`settings`** -- Free-form map for integration configuration.
- **`env`** -- Environment variables set when running Helmfile commands (e.g., `HELM_DEBUG`, `KUBECONFIG`).

### Component Inheritance

Use `metadata.inherits` to share configuration across components:

```yaml
components:
  helmfile:
    ingress-defaults:
      metadata:
        type: abstract
      vars:
        chart_version: "4.0.0"
        replica_count: 2

    nginx-ingress:
      metadata:
        type: real
        component: nginx-ingress
        inherits:
          - ingress-defaults
      vars:
        namespace: ingress
```

## Core Commands

### diff

Shows what changes would be made without applying them. This is the Helmfile equivalent of a dry-run.

```shell
atmos helmfile diff <component> -s <stack>
```

```shell
# Basic diff
atmos helmfile diff nginx-ingress -s ue2-dev

# Diff with stderr redirection
atmos helmfile diff echo-server -s tenant1-ue2-dev --redirect-stderr /dev/null
```

### apply

Applies Helmfile changes (install/upgrade charts).

```shell
atmos helmfile apply <component> -s <stack>
```

```shell
# Apply a component
atmos helmfile apply nginx-ingress -s ue2-dev

# Apply with stderr redirect
atmos helmfile apply echo-server -s tenant1-ue2-dev --redirect-stderr /dev/stdout
```

### sync

Synchronizes the desired state with the cluster. Installs missing releases, upgrades existing ones, and
removes releases that are no longer in the configuration.

```shell
atmos helmfile sync <component> -s <stack>
```

```shell
# Sync a component
atmos helmfile sync echo-server --stack tenant1-ue2-dev

# Sync with stderr redirect
atmos helmfile sync echo-server --stack tenant1-ue2-dev --redirect-stderr ./errors.txt
```

### destroy

Removes all releases managed by a component.

```shell
atmos helmfile destroy <component> -s <stack>
```

```shell
# Destroy a component
atmos helmfile destroy echo-server --stack=tenant1-ue2-dev

# Destroy with stderr redirect
atmos helmfile destroy echo-server --stack=tenant1-ue2-dev --redirect-stderr /dev/stdout
```

### deploy

Combines diff and apply in a single step.

```shell
atmos helmfile deploy <component> -s <stack>
```

```shell
atmos helmfile deploy nginx-ingress -s ue2-dev
```

## Variable File Generation

Atmos generates variable files from the `vars` section in the stack configuration. This happens
automatically before Helmfile commands, but can also be invoked manually:

```shell
atmos helmfile generate varfile <component> -s <stack>

# Output to a custom file
atmos helmfile generate varfile echo-server -s tenant1-ue2-dev -f vars.yaml

# With explicit stack flag
atmos helmfile generate varfile echo-server --stack tenant1-ue2-dev --file=vars.yaml
```

## Source Management (JIT Vendoring)

Helmfile components support just-in-time (JIT) vendoring through the `source` field. Instead of
pre-vendoring components or maintaining separate `component.yaml` files, declare the source inline
in stack configuration.

### Configuring a Source

Sources can be declared in two formats:

**String format (simple):**

```yaml
source: "github.com/cloudposse/helmfiles//releases/ingress-nginx?ref=1.0.0"
```

**Map format (full control):**

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
      vars:
        namespace: ingress-nginx
```

### Automatic Provisioning

Sources are automatically provisioned when running any Helmfile command. If a component has `source`
configured and the target directory does not exist, Atmos downloads the source before running Helmfile:

```shell
# Source is automatically provisioned on first use
atmos helmfile sync ingress-nginx --stack dev
# -> Auto-provisioning source for component 'ingress-nginx'
# -> Auto-provisioned source to components/helmfile/ingress-nginx
# -> Helmfile runs
```

### Source Commands

Explicit commands for fine-grained source management:

```shell
# Pull (vendor) a component source
atmos helmfile source pull ingress-nginx --stack dev

# Force re-vendor (overwrite existing)
atmos helmfile source pull ingress-nginx --stack dev --force

# Pull with identity override for private sources
atmos helmfile source pull ingress-nginx --stack dev --identity admin

# View source configuration
atmos helmfile source describe ingress-nginx --stack dev

# List all components with source configured
atmos helmfile source list --stack dev

# List across all stacks
atmos helmfile source list

# List in different output formats
atmos helmfile source list --format json

# Delete vendored source (requires --force)
atmos helmfile source delete ingress-nginx --stack dev --force
```

### Version Pinning per Environment

Override the source version per environment using stack inheritance:

```yaml
# stacks/catalog/ingress-nginx/defaults.yaml
components:
  helmfile:
    ingress-nginx/defaults:
      source:
        uri: github.com/cloudposse/helmfiles//releases/ingress-nginx
        version: 1.0.0

# stacks/dev.yaml
components:
  helmfile:
    ingress-nginx:
      metadata:
        inherits: [ingress-nginx/defaults]
      source:
        version: 1.1.0  # Override version for dev

# stacks/prod.yaml
components:
  helmfile:
    ingress-nginx:
      metadata:
        inherits: [ingress-nginx/defaults]
      source:
        version: 1.0.0  # Pin to stable version for prod
```

### Supported Source Protocols

The source provisioner uses go-getter and supports multiple protocols:

- **Git** -- `github.com/org/repo//path` or `git::https://github.com/org/repo.git//path`
- **S3** -- `s3::https://s3-us-east-1.amazonaws.com/bucket/path.tar.gz`
- **HTTP/HTTPS** -- `https://releases.example.com/helmfiles/component-1.0.0.tar.gz`
- **OCI** -- `oci::registry.example.com/helmfiles/component:v1.0.0`
- **GCS** -- Google Cloud Storage URIs

### Retry Configuration

Configure retries for transient network errors:

```yaml
source:
  uri: github.com/cloudposse/helmfiles//releases/ingress-nginx
  version: 1.0.0
  retry:
    max_attempts: 5
    initial_delay: 2s
    max_delay: 60s
    backoff_strategy: exponential
```

## EKS Integration

Atmos can automatically manage kubeconfig for Amazon EKS clusters before running Helmfile commands.

### Configuration in atmos.yaml

```yaml
components:
  helmfile:
    base_path: components/helmfile
    use_eks: true
    kubeconfig_path: /dev/shm
    cluster_name_template: "{{ .vars.namespace }}-{{ .vars.environment }}-{{ .vars.stage }}-eks"
```

### Configuration Options

- **`command`** -- Executable to run (default: `helmfile`). Env: `ATMOS_COMPONENTS_HELMFILE_COMMAND`.
- **`base_path`** -- Directory containing Helmfile components. Env: `ATMOS_COMPONENTS_HELMFILE_BASE_PATH`.
- **`use_eks`** -- Enable EKS integration (default: `false`). Env: `ATMOS_COMPONENTS_HELMFILE_USE_EKS`.
- **`kubeconfig_path`** -- Directory for kubeconfig files. Use `/dev/shm` for security.
  Env: `ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH`.
- **`cluster_name`** -- Explicit EKS cluster name. Env: `ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME`.
- **`cluster_name_template`** -- Go template for dynamic cluster names (recommended).
  Env: `ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_TEMPLATE`.

### Cluster Name Precedence

1. `--cluster-name` flag (highest priority)
2. `cluster_name` configuration
3. `cluster_name_template` expanded with Go templates
4. `cluster_name_pattern` expanded with token replacement (deprecated)

### Non-EKS Kubernetes Clusters

For non-EKS clusters (k3s, GKE, AKS, etc.), disable EKS integration and use existing kubeconfig:

```yaml
components:
  helmfile:
    base_path: components/helmfile
    use_eks: false  # Use existing KUBECONFIG
```

## Path-Based Component Resolution

You can use filesystem paths instead of component names:

```shell
# Navigate to component directory and use current directory
cd components/helmfile/echo-server
atmos helmfile diff . -s dev
atmos helmfile apply . -s dev

# Use relative path
cd components/helmfile
atmos helmfile sync ./echo-server -s prod

# From project root
atmos helmfile apply components/helmfile/echo-server -s dev

# Combine with other flags
cd components/helmfile/echo-server
atmos helmfile diff . -s dev --redirect-stderr /dev/null
atmos helmfile sync . -s dev --global-options="--no-color"
```

Path resolution only works when the component path resolves to a single unique component in the stack.
If multiple components reference the same path, use the explicit component name instead.

## Global Options

Pass global Helmfile options using the `--global-options` flag:

```shell
atmos helmfile apply nginx-ingress -s dev --global-options="--no-color --namespace=test"
```

Use double-dash `--` to separate Atmos flags from native Helmfile flags:

```shell
atmos helmfile sync echo-server -s dev -- --concurrency=1
```

## Common Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--stack` | `-s` | Target Atmos stack (required) |
| `--dry-run` | | Preview without executing |
| `--redirect-stderr` | | Redirect stderr to file or descriptor |
| `--global-options` | | Pass global options to Helmfile |
| `--cluster-name` | | Override EKS cluster name |
| `--identity` | | Override authentication identity |

## Debugging

### Describe Component

Use `atmos describe component` to see the fully resolved configuration:

```shell
atmos describe component nginx-ingress -s ue2-dev
```

This shows all merged vars, metadata, settings, and env for the component.

### Dry Run

Preview what Atmos will do without executing:

```shell
atmos helmfile apply nginx-ingress -s dev --dry-run
```

### Helm Debug Logging

Set `HELM_DEBUG` in the component env:

```yaml
components:
  helmfile:
    nginx-ingress:
      env:
        HELM_DEBUG: "true"
```

## Best Practices

1. **Use diff before apply.** Run `helmfile diff` first, review the output, then run `helmfile apply`
   to ensure exactly the reviewed changes are applied.

2. **Use deploy for combined operations.** The `deploy` command runs diff and apply in a single step.

3. **Store kubeconfig in `/dev/shm`.** When using EKS integration, use shared memory for security
   since files are not persisted to disk.

4. **Use `cluster_name_template` instead of `cluster_name_pattern`.** The Go template syntax is
   more powerful and the token replacement pattern is deprecated.

5. **Use source-based version pinning for multi-environment setups.** Override the `source.version`
   per environment to control which version is deployed to each stack.

6. **Use `atmos describe component`** to debug configuration resolution issues. It shows the fully
   merged result of all stack manifest inheritance.

7. **Leverage component inheritance** to share common configuration across Helmfile components
   and reduce duplication in stack manifests.

## Additional Resources

- For the complete list of all `atmos helmfile` subcommands, see [references/commands-reference.md](references/commands-reference.md)
