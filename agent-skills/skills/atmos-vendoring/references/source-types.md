# Vendor Source Types

Detailed URL syntax for each `source:` type supported by `vendor.yaml` and `component.yaml`. See
the main `atmos-vendoring` skill for the overall manifest format and selector flags.

## Git Repositories

The most common source type. Supports GitHub, GitLab, Bitbucket, and any Git host:

```yaml
# GitHub (implicit HTTPS, recommended)
source: "github.com/cloudposse-terraform-components/aws-vpc.git?ref={{.Version}}"

# GitHub with subdirectory
# source: "github.com/org/terraform-components.git//modules/vpc?ref={{.Version}}"

# Explicit Git protocol
# source: "git::https://github.com/org/repo.git?ref={{.Version}}"

# SSH authentication
# source: "git::ssh://git@github.com/org/private-repo.git?ref={{.Version}}"

# GitLab
# source: "gitlab.com/group/project.git?ref={{.Version}}"

# Bitbucket
# source: "bitbucket.org/owner/repo.git?ref={{.Version}}"
```

The `//` delimiter separates the repository URL from the subdirectory within the repository. For example, `repo.git//modules/vpc` extracts only the `modules/vpc` directory. Without `//`, Atmos downloads the entire repository root.

## OCI Registries

Pull artifacts from OCI-compatible container registries:

```yaml
# AWS ECR Public
source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"

# GitHub Container Registry
# source: "oci://ghcr.io/cloudposse/components/vpc:{{.Version}}"

# Docker Hub
# source: "oci://docker.io/library/nginx:alpine"
```

OCI authentication precedence:
1. Docker credentials from `~/.docker/config.json` (highest)
2. Environment variables, for `ghcr.io` only -- token resolved as `ATMOS_GITHUB_TOKEN`, then
  `GITHUB_TOKEN`; username resolved as `ATMOS_GITHUB_USERNAME`, then `GITHUB_ACTOR`, then
  `GITHUB_USERNAME`
3. Anonymous (for public images)

## Amazon S3

```yaml
source: "s3::https://s3.amazonaws.com/acme-configs/components/vpc.tar.gz"
# source: "s3::https://s3-us-west-2.amazonaws.com/bucket/path/component.tar.gz"
```

Uses AWS credentials from the environment or AWS config files.

## Google Cloud Storage

```yaml
source: "gcs::https://www.googleapis.com/storage/v1/bucket/path/component.tar.gz"
```

Uses Google Cloud credentials from the environment (`GOOGLE_APPLICATION_CREDENTIALS`) or the
default application credentials configured via `gcloud auth application-default login`.

## HTTP/HTTPS

```yaml
# Download and extract archive
source: "https://example.com/components/vpc.tar.gz"

# Download single file
# source: "https://raw.githubusercontent.com/cloudposse/terraform-null-label/0.25.0/exports/context.tf"
```

## Local Paths

```yaml
# Relative to vendor.yaml location
source: "../shared-components/vpc"

# Absolute path
# source: "/path/to/components/vpc"

# file:// URI
# source: "file:///path/to/components/vpc"
```
