---
name: atmos-vendoring
description: "Component vendoring: vendor.yaml manifests, pulling from Git/S3/HTTP/OCI/Terraform Registry"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Component Vendoring

Vendoring copies external components, stacks, and other artifacts into your repository. This gives you full control over when and how dependencies change, with visibility through `git diff`, an immutable audit trail, and the ability to apply emergency patches without waiting for upstream releases.

## Why Vendor

Terraform root modules must exist locally -- they cannot be pulled from remote sources at runtime the way child modules can. Vendoring makes this explicit: you copy the code once, commit it, and control when updates happen. This provides:

- **Visibility**: See actual code changes via `git diff`, not just version bumps.
- **Audit trail**: Every update is a commit with full history for compliance.
- **Emergency agility**: Patch vulnerabilities immediately without waiting for upstream.
- **Developer experience**: Full IDE navigation, grep across all code, better onboarding.
- **Deployment reliability**: No network dependencies during `terraform apply`.

## Types of Vendoring

Atmos supports two approaches:

1. **Vendor Configuration** (`vendor.yaml`): A centralized manifest listing all dependencies. This is the recommended approach.
2. **Component Manifest** (`component.yaml`): A per-component manifest placed inside the component directory. This is the legacy approach.

## vendor.yaml Manifest Format

The `vendor.yaml` file is a Kubernetes-style YAML configuration placed in the repository root (or the directory from which `atmos vendor pull` is executed):

```yaml
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: my-vendor-config
  description: Atmos vendoring manifest for ACME infrastructure
spec:
  imports:
    - "vendor/networking"
    - "vendor/security"

  sources:
    - component: "vpc"
      source: "github.com/cloudposse-terraform-components/aws-vpc.git?ref={{.Version}}"
      version: "1.398.0"
      targets:
        - "components/terraform/vpc"
      included_paths:
        - "**/*.tf"
        - "**/*.tfvars"
        - "**/*.md"
      excluded_paths:
        - "**/test/**"
      tags:
        - networking

    - component: "eks-cluster"
      source: "github.com/cloudposse-terraform-components/aws-eks-cluster.git?ref={{.Version}}"
      version: "2.15.0"
      targets:
        - "components/terraform/eks/cluster"
      tags:
        - compute
```

### Top-Level Fields

- `apiVersion`: Always `atmos/v1`.
- `kind`: Always `AtmosVendorConfig`.
- `metadata.name`: Optional name for the vendor configuration.
- `metadata.description`: Optional description.
- `spec.imports`: List of additional vendor manifests to import (supports hierarchical imports and glob patterns).
- `spec.sources`: List of source definitions for components and artifacts to vendor.

## Source Configuration

Each entry in `spec.sources` defines one component or artifact to vendor.

### Source Fields

```yaml
sources:
  - component: "vpc"
    source: "github.com/org/repo.git//path?ref={{.Version}}"
    version: "1.0.0"
    targets:
      - "components/terraform/vpc"
    included_paths:
      - "**/*.tf"
    excluded_paths:
      - "**/test/**"
    tags:
      - networking
    retry:
      max_attempts: 3
      initial_delay: 1s
      backoff_strategy: exponential
```

- `component` (string, optional): Component name used for `atmos vendor pull -c <component>` to vendor a single component. Also available as `{{ .Component }}` template variable.
- `source` (string, required): URL or path to the source. Supports Git, S3, HTTP/HTTPS, OCI, and local paths. Use `{{ .Version }}` template to inject the version.
- `version` (string, optional): Version identifier substituted into `{{ .Version }}` in source and targets.
- `targets` (list of strings, required): Local paths where files will be placed. Supports Go templates (`{{ .Component }}`, `{{ .Version }}`). Relative paths are resolved from the `vendor.yaml` location or `base_path`.
- `included_paths` (list of strings, optional): POSIX-style glob patterns for files to include. If not specified, all files are included.
- `excluded_paths` (list of strings, optional): POSIX-style glob patterns for files to exclude.
- `tags` (list of strings, optional): Tags for selective vendoring with `atmos vendor pull --tags <tag>`.
- `retry` (object, optional): Retry configuration for transient network errors.

### Template Parameters

The `source` and `targets` fields support Go templates with these variables:

- `{{ .Component }}`: Value of the `component` field.
- `{{ .Version }}`: Value of the `version` field.

Example with versioned targets:

```yaml
sources:
  - component: "vpc"
    source: "github.com/cloudposse-terraform-components/aws-vpc.git?ref={{.Version}}"
    version: "1.398.0"
    targets:
      - "components/terraform/{{ .Component }}/{{ .Version }}"
```

All Sprig template functions are available. For example, extracting major.minor version:

```yaml
targets:
  - "components/terraform/{{ .Component }}/{{ (first 2 (splitList \".\" .Version)) | join \".\" }}"
```

## Source Types

### Git Repositories

The most common source type. Supports GitHub, GitLab, Bitbucket, and any Git host:

```yaml
# GitHub (implicit HTTPS, recommended)
source: "github.com/cloudposse-terraform-components/aws-vpc.git?ref={{.Version}}"

# GitHub with subdirectory
source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}"

# Explicit Git protocol
source: "git::https://github.com/org/repo.git?ref={{.Version}}"

# SSH authentication
source: "git::ssh://git@github.com/org/private-repo.git?ref={{.Version}}"

# GitLab
source: "gitlab.com/group/project.git?ref={{.Version}}"

# Bitbucket
source: "bitbucket.org/owner/repo.git?ref={{.Version}}"
```

The `//` delimiter separates the repository URL from the subdirectory within the repository. For example, `repo.git//modules/vpc` extracts only the `modules/vpc` directory. Without `//`, Atmos downloads the entire repository root.

### OCI Registries

Pull artifacts from OCI-compatible container registries:

```yaml
# AWS ECR Public
source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"

# GitHub Container Registry
source: "oci://ghcr.io/cloudposse/components/vpc:{{.Version}}"

# Docker Hub
source: "oci://docker.io/library/nginx:alpine"
```

OCI authentication precedence:
1. Docker credentials from `~/.docker/config.json` (highest)
2. Environment variables (`GITHUB_TOKEN` + `GITHUB_ACTOR` for ghcr.io)
3. Anonymous (for public images)

### Amazon S3

```yaml
source: "s3::https://s3.amazonaws.com/acme-configs/components/vpc.tar.gz"
source: "s3::https://s3-us-west-2.amazonaws.com/bucket/path/component.tar.gz"
```

Uses AWS credentials from the environment or AWS config files.

### HTTP/HTTPS

```yaml
# Download and extract archive
source: "https://example.com/components/vpc.tar.gz"

# Download single file
source: "https://raw.githubusercontent.com/cloudposse/terraform-null-label/0.25.0/exports/context.tf"
```

### Local Paths

```yaml
# Relative to vendor.yaml location
source: "../shared-components/vpc"

# Absolute path
source: "/path/to/components/vpc"

# file:// URI
source: "file:///path/to/components/vpc"
```

## Authentication

### Automatic Token Injection

Atmos automatically injects tokens for private Git repositories:

| Platform | Environment Variables | Default Enabled |
|----------|----------------------|-----------------|
| GitHub | `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN` | Yes |
| GitLab | `ATMOS_GITLAB_TOKEN` or `GITLAB_TOKEN` | No |
| Bitbucket | `ATMOS_BITBUCKET_TOKEN` or `BITBUCKET_TOKEN` | No |

Enable GitLab/Bitbucket in `atmos.yaml`:

```yaml
settings:
  inject_gitlab_token: true
  inject_bitbucket_token: true
```

### SSH Authentication

```yaml
source: "git@github.com:owner/private-repo.git?ref=v1.0.0"
source: "git@github.com:owner/private-repo.git?ref=v1.0.0&sshkey=~/.ssh/custom_key"
```

## Include/Exclude Patterns

Use POSIX-style glob patterns to control which files are vendored:

```yaml
included_paths:
  - "**/*.tf"          # All Terraform files recursively
  - "**/*.tfvars"      # All tfvars files
  - "**/*.md"          # All markdown files

excluded_paths:
  - "**/test/**"       # Exclude test directories
  - "**/*.yaml"        # Exclude YAML files
  - "**/examples/**"   # Exclude examples
```

Glob pattern syntax:
- `*` matches any characters within a single path segment.
- `**` matches across multiple path segments recursively.
- `?` matches exactly one character.
- `[abc]` matches any single character in the set.
- `{a,b,c}` matches any of the comma-separated patterns.

If `included_paths` is not specified, all files are included (minus any `excluded_paths`).

## Imports in Vendor Manifests

Split the `vendor.yaml` into smaller files for maintainability:

```yaml
# vendor.yaml
apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  imports:
    - "vendor/networking"
    - "vendor/compute"
    - "vendor/security"
    - "vendor/**/*"           # Glob pattern: import all manifests recursively
```

Each imported file is a full `AtmosVendorConfig` manifest. Hierarchical imports are supported -- one manifest can import another, which imports another, etc. Import paths support glob patterns (`*`, `**`, `?`, `{a,b}`).

## Component Manifest (Legacy)

The legacy approach uses a `component.yaml` file inside the component directory:

```yaml
# components/terraform/vpc/component.yaml
apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: vpc-vendor-config
  description: Vendoring config for VPC component
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}
    version: 1.398.0
    included_paths:
      - "**/*.tf"
      - "**/*.md"
    excluded_paths:
      - "**/context.tf"
  mixins:
    - uri: https://raw.githubusercontent.com/cloudposse/terraform-null-label/0.25.0/exports/context.tf
      filename: context.tf
```

### Mixins (Legacy)

Mixins download additional files and overlay them on the vendored component. They are processed after the main source is downloaded, and they can overwrite source files with the same filename:

```yaml
spec:
  mixins:
    - uri: https://raw.githubusercontent.com/cloudposse/terraform-null-label/0.25.0/exports/context.tf
      filename: context.tf
    - uri: https://example.com/terraform/custom-providers.tf
      version: 1.0.0
      filename: custom-providers.tf
```

Mixin fields:
- `uri`: URL to download (supports all go-getter protocols).
- `filename`: Local filename in the component directory.
- `version`: Optional version for `{{ .Version }}` substitution in the URI.

## atmos vendor pull Command

```bash
# Vendor all sources from vendor.yaml
atmos vendor pull

# Vendor all sources (explicit flag)
atmos vendor pull --everything

# Vendor a specific component
atmos vendor pull -c vpc
atmos vendor pull --component eks-cluster

# Vendor by tags
atmos vendor pull --tags networking
atmos vendor pull --tags networking,compute
```

## Version Pinning

Always pin versions in your vendor manifest for reproducible builds:

```yaml
sources:
  - component: "vpc"
    source: "github.com/cloudposse-terraform-components/aws-vpc.git?ref={{.Version}}"
    version: "1.398.0"       # Pinned to specific tag
    targets:
      - "components/terraform/vpc"
```

For Git sources, use `?ref=` with a specific tag or commit SHA for reproducible builds. Branch names like `main` point to a moving target and should only be used intentionally for development workflows, not for production vendoring.

## Vendoring and Version Management Patterns

Vendoring works with several version management strategies:

### Single Version (Simplest)

```yaml
sources:
  - component: "vpc"
    version: "1.398.0"
    targets:
      - "components/terraform/vpc"
```

All environments use the same vendored version. Updates are atomic.

### Folder-Based Versioning

```yaml
sources:
  - component: "vpc"
    version: "1.398.0"
    targets:
      - "components/terraform/vpc/{{ .Version }}"
```

Multiple versions coexist. Stacks reference specific versions via `metadata.component`.

### Major.Minor Versioning

```yaml
sources:
  - component: "vpc"
    version: "1.398.0"
    targets:
      - "components/terraform/vpc/{{ (first 2 (splitList \".\" .Version)) | join \".\" }}"
```

Groups by major.minor version (e.g., `vpc/1.398/`).

## Best Practices

1. **Use vendor.yaml (not component.yaml)**: The centralized manifest is easier to maintain and provides a single view of all dependencies.
2. **Pin versions by default**: Use exact version tags or commit SHAs whenever possible. Use branch names only as an explicit exception when pinning is impractical.
3. **Review changes via git diff**: After running `atmos vendor pull`, review the diff before committing.
4. **Use tags for selective vendoring**: Tag sources by layer (networking, compute, security) for partial updates.
5. **Automate with CI/CD**: Set up GitHub Actions to periodically run `atmos vendor pull` and open PRs with changes.
6. **Include only what you need**: Use `included_paths` and `excluded_paths` to avoid vendoring test files, examples, and other unnecessary artifacts.
7. **Use retry for flaky networks**: Configure `retry` with exponential backoff for CI/CD environments.

## References

- [references/vendor-manifest.md](references/vendor-manifest.md) -- Complete vendor.yaml schema reference, all source type fields, URL syntax
