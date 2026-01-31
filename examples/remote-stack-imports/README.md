# Remote Stack Imports Example

This example demonstrates how to import stack configurations from remote sources using [go-getter](https://github.com/hashicorp/go-getter) URL schemes.

## Overview

Atmos supports importing stack configurations from various remote sources:
- **HTTP/HTTPS URLs** - Raw files from web servers
- **Git repositories** - Using `git::` prefix or platform shorthand
- **S3 buckets** - Using `s3::` prefix
- **Google Cloud Storage** - Using `gcs::` prefix

## Example Structure

```
remote-stack-imports/
├── atmos.yaml                    # Atmos configuration
├── components/terraform/myapp/   # Simple mock component
└── stacks/
    ├── catalog/base.yaml         # Local base configuration
    └── deploy/demo.yaml          # Stack with remote imports
```

## Try It

```bash
cd examples/remote-stack-imports

# View the stack configuration (includes remote imports)
atmos describe stacks

# Describe the component
atmos describe component myapp -s demo
```

## Remote Import Examples

### HTTP/HTTPS URL

```yaml
import:
  - https://raw.githubusercontent.com/cloudposse/atmos/main/tests/fixtures/remote-imports/shared.yaml
```

### Git Repository

```yaml
import:
  # HTTPS with specific ref
  - git::https://github.com/acme/infrastructure.git//stacks/catalog/vpc?ref=v1.2.0

  # GitHub shorthand
  - github.com/acme/infrastructure//stacks/catalog/rds?ref=main
```

### S3 Bucket

```yaml
import:
  - s3::https://s3.amazonaws.com/my-bucket/stacks/catalog/vpc.yaml
```

## Best Practices

1. **Pin Versions** - Always use `?ref=` with a specific tag or commit SHA for Git imports
2. **Cache Considerations** - Remote imports are cached locally
3. **Authentication** - Configure credentials for private repositories via environment variables
4. **Fallback to Local** - Consider vendoring critical imports for offline access

## Learn More

- [Stack Imports Documentation](https://atmos.tools/stacks/imports)
- [go-getter URL Formats](https://github.com/hashicorp/go-getter#url-format)
