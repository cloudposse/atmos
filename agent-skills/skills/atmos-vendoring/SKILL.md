---
name: atmos-vendoring
description: "Component vendoring: vendor.yaml and component.yaml manifests, immutable vendor.lock.yaml receipts, pulling from Git/S3/HTTP/OCI/Terraform Registry, --stack/--labels/--tags selector composition, native vendor update, clean, diff, config, and reviewed local component copies"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: state-versioning
references:
  - references/component-updater.md
  - references/source-types.md
---

# Atmos Component Vendoring

For scheduled native Component Updater pull requests, scopes/groups, GitHub permissions, and CI summaries, use [references/component-updater.md](references/component-updater.md).

Vendoring copies external components, stacks, and other artifacts into your repository. This gives you full control over when and how dependencies change, with visibility through `git diff`, an immutable audit trail, and the ability to apply emergency patches without waiting for upstream releases.

Atmos records completed installs in the committed `vendor.lock.yaml` receipt. The receipt contains
credential-free declared and resolved sources, immutable artifact evidence, and the ordered
per-file materialization inventory. It applies to both centralized `vendor.yaml` sources and
legacy `component.yaml` sources and mixins.

## Why Vendor

Vendoring is the checked-in model for remote component code: you copy the code into the repository,
commit it, and control when updates happen. This provides:

- **Visibility**: See actual code changes via `git diff`, not just version bumps.
- **Audit trail**: Every update is a commit with full history for compliance.
- **Emergency agility**: Patch vulnerabilities immediately without waiting for upstream.
- **Developer experience**: Full IDE navigation, grep across all code, better onboarding.
- **Deployment reliability**: No network dependencies during `terraform apply`.

Atmos also supports component `source` provisioning for just-in-time fetching from stack
configuration. Use the `atmos-components` skill for that model. Prefer vendoring when the fetched
implementation should be reviewed and committed; prefer component source provisioning when the stack
configuration should declare the remote source and Atmos should fetch it on demand.

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

`source:` accepts Git (GitHub/GitLab/Bitbucket/SSH), OCI registries, Amazon S3, Google Cloud
Storage, HTTP/HTTPS, and local paths -- see
[references/source-types.md](references/source-types.md) for the full URL syntax and examples of
each.

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
    uri: github.com/cloudposse-terraform-components/aws-vpc.git?ref={{.Version}}
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

# Vendor by tags (vendor.yaml-declared source tags, matches ANY)
atmos vendor pull --tags networking
atmos vendor pull --tags networking,compute

# Vendor every component in a stack that has its own component.yaml
atmos vendor pull --stack plat-ue2-dev

# Vendor components whose stack metadata.labels match ALL pairs (matches --stack's resolution)
atmos vendor pull --labels tier=1,cost-center:platform

# Narrow a --stack/--labels selection by declared tags too
atmos vendor pull --stack plat-ue2-dev --tags networking

# Intentionally resolve mutable declared refs and replace their lock evidence
atmos vendor pull --refresh-lock

# Remove lock-owned files, preserving locally modified files by default
atmos vendor clean
atmos vendor clean --component vpc
atmos vendor clean --force
```

`vendor pull` reconciles the receipt as well as declared version pins: matching installed files
skip a download; missing or checksum-mismatched files are rehydrated from the recorded immutable
identity. `--refresh-lock` is the explicit mutable-ref refresh path. `vendor clean` only removes
lock-owned paths, reports modified-file conflicts, and requires `--force` to remove them. Do not
hand-delete a target directory or lock entry when a scoped clean/replay can preserve overlapping
source and mixin ownership.

### Selector Flags: --component, --stack, --labels, --tags

These four flags select which components a `vendor pull`/`diff`/`clean`/`update`/`verify` command
acts on. They compose as independent filters rather than being mutually exclusive selector "modes":

- `--component`/`-c`: command-specific cardinality. `vendor update` accepts repeated values and
  accumulates them; `pull`, `diff`, `clean`, and `verify` each accept only a single value. In every
  command, mutually exclusive with `--stack`/`--labels` (a stack-resolved set doesn't compose with
  one or more explicit targets). Composes with `--tags`.
- `--stack`/`-s`: every component declared in the stack (`vendor pull` narrows this further to
  components with their own `component.yaml` -- see below). Composes with `--labels` (narrows
  further) and `--tags`.
- `--labels`: filters the same stack-resolved component set `--stack` resolves, by each
  component's stack `metadata.labels` -- a *stack* concept, not a `vendor.yaml` concept (`vendor.yaml`
  sources have no labels field). Matches ALL the given comma-separated `key=value`/`key:value`
  pairs. Cannot combine with `--component`; composes with `--stack` and `--tags`.
- `--tags`: an independent filter over each candidate's declared `vendor.yaml` `tags:` (matches ANY
  of the given tags). Composes with `--component` or `--stack`/`--labels`, or stands on its own. A
  candidate resolved only through `--stack`/`--labels` (no matching `vendor.yaml` entry) has no tags
  to match and is excluded by any non-empty `--tags` filter -- the same way any filter excludes an
  entity missing the filtered attribute.

A selector with no eligible vendor target is always an explicit error, never a silent no-op or a
silent fall-through to "vendor everything."

For `vendor pull` only, `--stack`/`--labels` install from each resolved component's own
`component.yaml`, bypassing `vendor.yaml` entirely -- a component without one is silently skipped,
since not every stack component vendors this way. This is the one exception to the "always error"
rule above: if the stack resolves but none of its matched components has its own `component.yaml`,
`vendor pull` succeeds as an intentional no-op instead of erroring, since there was nothing for that
selector to install. A component declared in *both* places can have its `vendor.yaml`-driven install
(`-c <name>` or bare `--tags`) and its `--stack`/`--labels`-driven install disagree on content if the
two sources ever drift -- Atmos warns when it detects this, but doesn't reconcile the two
automatically. Prefer declaring a component in only one place.

`diff`/`clean`/`update`/`verify` do not have this exception: their `--stack`/`--labels` resolve
stack-declared component names directly, with no `component.yaml` required (they operate on already
vendored/lock-owned state, not on how it was installed), so an unmatched selector is always an error
for these four commands.

## Native Vendor Update and Diff

Use native `atmos vendor update` and `atmos vendor diff` before editing versions by hand.

```bash
# Dry run: show what Git-backed sources would update
atmos vendor update --check

# Update version fields in-place, preserving comments, anchors, and templates
atmos vendor update

# Update versions, then pull the changed sources
atmos vendor update --pull

# Scope updates
atmos vendor update --component vpc
atmos vendor update --tags networking,aws
atmos vendor update --stack plat-ue2-dev --labels tier=1
atmos vendor update --check --outdated

# Review upstream changes without a local checkout
atmos vendor diff --component vpc
atmos vendor diff -c vpc --from 1.0.0 --to 2.0.0
atmos vendor diff -c vpc --from 1.0.0 --to 2.0.0 --diff-file variables.tf
atmos vendor diff --stack plat-ue2-dev --tags networking
```

`update`/`diff` (and `clean`/`verify`) accept the same `--component`/`--stack`/`--labels`/`--tags`
selector composition described above.

`vendor update` follows imports and writes the manifest file that declares each source. It supports
Git-backed sources and reports skipped templated versions or non-Git sources. Use source-level
constraints (`constraints.version`, `excluded_versions`, `no_prereleases`) to define eligible
updates.

`vendor diff` compares Git refs for one component. `--from` defaults to the current pinned version,
and `--to` defaults to the latest tag. Use it for review before `vendor update --pull`.

## Vendor Config Editing

Use `atmos vendor config` for path-based, format-preserving edits to vendor manifests:

```bash
atmos vendor config get spec.sources[0].version
atmos vendor config set spec.sources[0].version v1.2.3
atmos vendor config delete spec.sources[0].tags
atmos vendor config format
atmos vendor config list 'spec.sources[*].version'
```

`atmos vendor get <component>` and `atmos vendor set <component> <version>` are component-name
aliases for common version lookups and edits.

## Version Pinning

Pin versions by default in your vendor manifest for reproducible builds:

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
5. **Use native update/diff**: Run `atmos vendor update --check` and `atmos vendor diff` before adopting a new version.
6. **Automate with CI/CD**: Set up GitHub Actions to run `atmos vendor update --check`, then update, pull, and open PRs when desired.
7. **Include only what you need**: Use `included_paths` and `excluded_paths` to avoid vendoring test files, examples, and other unnecessary artifacts.
8. **Use retry for flaky networks**: Configure `retry` with exponential backoff for CI/CD environments.
9. **Use Version Tracker for cross-surface versions**: If the same version feeds vendor manifests, CI workflows, images, or toolchain entries, route version policy to `atmos-version` and use vendoring to materialize reviewed source copies.

## References

- [references/vendor-manifest.md](references/vendor-manifest.md) -- Complete vendor.yaml schema reference, all source type fields, URL syntax
