# Vendor Manifest Schema Reference

Complete reference for the `vendor.yaml` manifest, all source type fields, include/exclude patterns, imports, and URL syntax.

## Full vendor.yaml Schema

```yaml
apiVersion: atmos/v1                    # Required. Always "atmos/v1"
kind: AtmosVendorConfig                 # Required. Always "AtmosVendorConfig"
metadata:                               # Optional
  name: "my-vendor-config"              # Optional. Descriptive name
  description: "Description here"       # Optional. Human-readable description
spec:
  imports:                              # Optional. List of additional vendor manifests
    - "vendor/networking"               # String path (relative to vendor.yaml)
    - "vendor/**/*"                     # Glob patterns supported

  sources:                              # Required. List of sources to vendor
    - component: "vpc"                  # Optional. Component identifier
      source: "..."                     # Required. Source URL or path
      version: "1.0.0"                  # Optional. Version string
      targets:                          # Required. Local destination paths
        - "components/terraform/vpc"
      included_paths:                   # Optional. Glob patterns for inclusion
        - "**/*.tf"
      excluded_paths:                   # Optional. Glob patterns for exclusion
        - "**/test/**"
      tags:                             # Optional. Tags for selective vendoring
        - networking
      retry:                            # Optional. Retry configuration
        max_attempts: 3
        initial_delay: 1s
        max_delay: 30s
        backoff_strategy: exponential
        multiplier: 2.0
        random_jitter: 0.1
        max_elapsed_time: 5m
```

## spec.sources Field Reference

### component

**Type**: string
**Required**: No

Identifies the component for selective vendoring. Used with `atmos vendor pull -c <component>`. Also available as the `{{ .Component }}` template variable in `source` and `targets`.

```yaml
sources:
  - component: "vpc"
    source: "github.com/org/aws-{{ .Component }}.git?ref={{ .Version }}"
    targets:
      - "components/terraform/{{ .Component }}"
```

### source

**Type**: string
**Required**: Yes

The URL or path to download the component from. Supports all go-getter protocols plus OCI.

**Supported schemes**:

| Scheme | Example |
|--------|---------|
| Implicit HTTPS | `github.com/org/repo.git?ref=v1.0` |
| Explicit HTTPS | `https://github.com/org/repo.git//path?ref=v1.0` |
| Git | `git::https://github.com/org/repo.git?ref=v1.0` |
| SSH | `git::ssh://git@github.com/org/repo.git?ref=v1.0` |
| SCP-style SSH | `git@github.com:org/repo.git?ref=v1.0` |
| OCI | `oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest` |
| S3 | `s3::https://s3.amazonaws.com/bucket/path.tar.gz` |
| GCS | `gcs::https://www.googleapis.com/storage/v1/bucket/path.tar.gz` |
| HTTP/HTTPS | `https://example.com/components.tar.gz` |
| Local relative | `../shared-components/vpc` |
| Local absolute | `/path/to/components/vpc` |
| file:// | `file:///path/to/components` |

**Subdirectory delimiter**: `//` separates the repository URL from the subdirectory to extract.

```yaml
# Extract only the modules/vpc subdirectory
source: "github.com/org/repo.git//modules/vpc?ref=v1.0"

# Root directory (explicit)
source: "github.com/org/repo.git//.?ref=v1.0"

# Root directory (implicit, no // needed for Git URLs)
source: "github.com/org/repo.git?ref=v1.0"
```

**Query parameters**:

| Parameter | Description |
|-----------|-------------|
| `ref=<value>` | Git reference: branch, tag, or commit SHA |
| `depth=<n>` | Git clone depth (Atmos defaults to `depth=1` for performance) |
| `sshkey=<path>` | Path to SSH private key |

### version

**Type**: string
**Required**: No

Version identifier substituted into `{{ .Version }}` placeholders in `source` and `targets`. Can be a semantic version tag, branch name, or commit SHA.

```yaml
sources:
  - component: "vpc"
    source: "github.com/org/repo.git?ref={{ .Version }}"
    version: "1.398.0"
```

### targets

**Type**: list of strings
**Required**: Yes

Local paths where vendored files are placed. Supports Go templates and Sprig functions.

```yaml
targets:
  # Simple path
  - "components/terraform/vpc"

  # Using template variables
  - "components/terraform/{{ .Component }}"

  # Versioned path
  - "components/terraform/{{ .Component }}/{{ .Version }}"

  # Major.minor versioned path using Sprig
  - "components/terraform/{{ .Component }}/{{ (first 2 (splitList \".\" .Version)) | join \".\" }}"
```

Paths can be absolute or relative (relative to `vendor.yaml` or `base_path`). Multiple targets are supported -- the source will be copied to all listed paths.

### included_paths

**Type**: list of strings
**Required**: No

POSIX-style glob patterns specifying which files to include. If not specified, all files are included (subject to `excluded_paths`).

```yaml
included_paths:
  - "**/*.tf"            # All .tf files recursively
  - "**/*.tfvars"        # All .tfvars files recursively
  - "**/*.md"            # All markdown files
  - "*.tf"               # Only .tf files in root (not recursive)
  - "modules/**/*.tf"    # .tf files in modules/ subdirectory
  - "**/*.{tf,md}"       # .tf and .md files (brace expansion)
```

### excluded_paths

**Type**: list of strings
**Required**: No

POSIX-style glob patterns specifying which files to exclude. Applied after `included_paths`.

```yaml
excluded_paths:
  - "**/test/**"         # Exclude test directories
  - "**/examples/**"     # Exclude examples
  - "**/*.yaml"          # Exclude YAML files
  - "**/*.yml"           # Exclude YML files
  - "**/context.tf"      # Exclude specific file
  - "**/.github/**"      # Exclude GitHub config
```

### tags

**Type**: list of strings
**Required**: No

Tags for selective vendoring. Use with `atmos vendor pull --tags <tag1>,<tag2>`.

```yaml
sources:
  - component: "vpc"
    tags:
      - networking
      - core
  - component: "eks-cluster"
    tags:
      - compute
      - kubernetes
```

```bash
atmos vendor pull --tags networking         # Vendors vpc only
atmos vendor pull --tags compute            # Vendors eks-cluster only
atmos vendor pull --tags networking,compute # Vendors both
```

### retry

**Type**: object
**Required**: No

Retry configuration for handling transient network errors. When not specified, no retries are performed.

```yaml
retry:
  max_attempts: 5              # Maximum retry attempts
  initial_delay: 2s            # Delay before first retry
  max_delay: 30s               # Maximum delay between retries
  backoff_strategy: exponential # exponential, linear, or constant
  multiplier: 2.0              # Multiplier for exponential/linear backoff
  random_jitter: 0.1           # Random jitter factor (0.0-1.0)
  max_elapsed_time: 5m         # Maximum total time for all retries
```

## spec.imports Reference

The `imports` field lists additional vendor manifests to merge into the main manifest. Supports hierarchical imports (imported manifests can import others).

```yaml
spec:
  imports:
    # Simple path (without extension, .yaml is assumed)
    - "vendor/networking"

    # With explicit extension
    - "vendor/security.yaml"

    # Glob patterns
    - "vendor/**/*"              # All manifests recursively
    - "layers/*.yaml"            # All YAML files in layers/
    - "vendor/{networking,security}"  # Specific manifests
```

Import paths are relative to the `vendor.yaml` file location. Each imported file must be a valid `AtmosVendorConfig` manifest with `apiVersion`, `kind`, and `spec` fields.

Processing order: Atmos processes the import chain depth-first, then processes all sources from all manifests in the order they are defined.

## Component Manifest Schema (Legacy)

The `component.yaml` file is placed inside a component directory (`components/terraform/<name>/component.yaml`).

```yaml
apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: "component-name-vendor-config"
  description: "Description"
spec:
  source:
    uri: "github.com/org/repo.git//modules/component?ref={{ .Version }}"
    version: "1.398.0"
    included_paths:
      - "**/*.tf"
    excluded_paths:
      - "**/context.tf"
  mixins:
    - uri: "https://raw.githubusercontent.com/org/repo/version/exports/context.tf"
      filename: "context.tf"
    - uri: "https://example.com/mixin.tf"
      version: "1.0.0"
      filename: "custom-mixin.tf"
```

### spec.source (Legacy)

- `uri`: Source URL (same protocols as `vendor.yaml` sources).
- `version`: Version for `{{ .Version }}` substitution.
- `included_paths`: Glob patterns for files to include.
- `excluded_paths`: Glob patterns for files to exclude.

### spec.mixins (Legacy)

List of additional files to download and overlay on the vendored component.

- `uri`: URL to download the mixin file from.
- `filename`: Local filename in the component directory. Overwrites source files with the same name.
- `version`: Optional version for `{{ .Version }}` substitution in the URI.

Mixins are processed after the main source, in list order. Common use case: replacing `context.tf` with a newer version from `terraform-null-label`.

## URL Syntax Quick Reference

### Git URL Format

```text
[scheme://]host/owner/repo.git[//subdirectory][?query-params]
```

Examples:

```text
github.com/org/repo.git?ref=v1.0.0
github.com/org/repo.git//modules/vpc?ref=v1.0.0
git::https://github.com/org/repo.git?ref=main
git::ssh://git@github.com/org/repo.git?ref=v1.0.0
git@github.com:org/repo.git?ref=v1.0.0
```

### OCI URL Format

```text
oci://registry/namespace/image:tag
```

Examples:

```text
oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest
oci://ghcr.io/org/components/vpc:v1.0.0
oci://docker.io/library/nginx:alpine
```

### Authentication Environment Variables

| Platform | Token Variable | Fallback | Username Variable |
|----------|---------------|----------|-------------------|
| GitHub | `ATMOS_GITHUB_TOKEN` | `GITHUB_TOKEN` | `x-access-token` (auto) |
| GitLab | `ATMOS_GITLAB_TOKEN` | `GITLAB_TOKEN` | `oauth2` (auto) |
| Bitbucket | `ATMOS_BITBUCKET_TOKEN` | `BITBUCKET_TOKEN` | `x-token-auth` (auto) |
| OCI (ghcr.io) | `GITHUB_TOKEN` | -- | `GITHUB_ACTOR` or `ATMOS_GITHUB_USERNAME` |

User-provided credentials in URLs always take precedence over automatic token injection.

## Glob Pattern Quick Reference

| Pattern | Meaning | Example Match |
|---------|---------|--------------|
| `*` | Any chars in one segment | `*.tf` matches `main.tf` |
| `**` | Any chars across segments | `**/*.tf` matches `dir/sub/main.tf` |
| `?` | Exactly one char | `file?.txt` matches `file1.txt` |
| `[abc]` | One char from set | `file[12].txt` matches `file1.txt` |
| `[a-z]` | One char from range | `file[a-c].txt` matches `filea.txt` |
| `{a,b}` | One of the alternatives | `*.{tf,md}` matches `main.tf` or `README.md` |
