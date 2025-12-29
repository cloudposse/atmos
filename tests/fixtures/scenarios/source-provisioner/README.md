# Source Provisioner Test Fixture

This test fixture demonstrates the source provisioner feature for Just-in-Time (JIT) component vendoring.

## Overview

The source provisioner enables inline source declaration in component configuration:

```yaml
components:
  terraform:
    vpc:
      source: "github.com/org/repo//module?ref=v1.0.0"
```

This allows components to be vendored on-demand without requiring separate `vendor.yaml` or `component.yaml` files.

## Components

| Component | Source Configuration | Description |
|-----------|---------------------|-------------|
| `vpc-inline-ref` | URI with inline ref | Map form with version in URI via `?ref=` query param |
| `vpc-map` | Full source spec | Map form with included/excluded paths |
| `vpc-retry` | Retry configuration | Map form with retry options for transient failures |
| `vpc-no-source` | None | Component without source (for error testing) |

## Manual Testing

### Test 1: Source Describe

```bash
cd tests/fixtures/scenarios/source-provisioner

# Describe source with full spec (included/excluded paths)
atmos terraform source describe vpc-map --stack dev

# Describe source with URI containing inline ref
atmos terraform source describe vpc-inline-ref --stack dev

# Describe source with retry configuration
atmos terraform source describe vpc-retry --stack dev

# Test error case: component without source
atmos terraform source describe vpc-no-source --stack dev
# Expected: Error about missing source configuration
```

### Test 2: Source Pull

```bash
# Pull component source (vendors to components/terraform/vpc-map/)
atmos terraform source pull vpc-map --stack dev

# Verify the component was vendored
ls -la components/terraform/vpc-map/

# Force re-pull even if already exists
atmos terraform source pull vpc-map --stack dev --force
```

### Test 3: Source Delete

```bash
# Delete requires --force flag
atmos terraform source delete vpc-map --stack dev
# Expected: Error about missing --force flag

# Delete with --force
atmos terraform source delete vpc-map --stack dev --force

# Verify deletion
ls components/terraform/vpc-map/
# Expected: Directory not found
```

### Test 4: Source List

```bash
# List components with source configuration
atmos terraform source list --stack dev
# Note: Currently returns "not implemented"
```

### Test 5: Authentication (Private Repos)

```bash
# Set authentication for private repos
export GITHUB_TOKEN="your-token"

# Or use identity flag
atmos terraform source pull vpc-map --stack dev --identity my-identity
```

## Cleanup

```bash
# Delete vendored components
rm -rf components/terraform/vpc-map/
rm -rf components/terraform/vpc-inline-ref/
rm -rf components/terraform/vpc-retry/
```

## Directory Structure

```text
tests/fixtures/scenarios/source-provisioner/
├── atmos.yaml                          # Base Atmos configuration
├── .gitignore                          # Ignores vendored components
├── README.md                           # This file
├── components/terraform/
│   ├── mock/                           # Local mock component
│   │   └── main.tf
│   ├── vpc-map/                        # Vendored (ignored)
│   ├── vpc-inline-ref/                 # Vendored (ignored)
│   └── vpc-retry/                      # Vendored (ignored)
└── stacks/
    ├── catalog/
    │   ├── vpc-source-inline-ref.yaml  # Source with version inline via ?ref=
    │   ├── vpc-source-map.yaml         # Full source spec with paths
    │   ├── vpc-source-retry.yaml       # Source with retry configuration
    │   └── vpc-no-source.yaml          # Component without source
    └── deploy/
        └── dev.yaml                    # Stack that imports all catalog files
```

## Source Configuration Forms

### String Form (Simple)

```yaml
components:
  terraform:
    vpc:
      source: "github.com/cloudposse/terraform-null-label//exports?ref=0.25.0"
```

### Map Form (Full Spec)

```yaml
components:
  terraform:
    vpc:
      source:
        uri: "github.com/cloudposse/terraform-null-label//exports"
        version: "0.25.0"
        included_paths:
          - "*.tf"
        excluded_paths:
          - "*.md"
          - "examples/**"
```

### Map Form with Retry

```yaml
components:
  terraform:
    vpc:
      source:
        uri: "github.com/cloudposse/terraform-null-label//exports"
        version: "0.25.0"
        retry:
          max_attempts: 3
          initial_delay: "1s"
          max_delay: "10s"
```

## Integration Tests

Automated integration tests are in `tests/cli_source_provisioner_test.go`:

```bash
# Run all source provisioner tests
go test ./tests -run TestSourceProvisioner -v

# Run specific test
go test ./tests -run TestSourceProvisionerDescribe_Success -v
```
