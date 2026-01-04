# Source Provisioner + Workdir Integration Test Fixture

This test fixture demonstrates the integration between the source provisioner and workdir provisioner features for **Terraform**, **Helmfile**, and **Packer** components.

## Overview

When both `source` and `provision.workdir.enabled` are configured for a component:

1. **Source Provisioner**: Vendors remote source directly to `.workdir/<type>/<stack>-<component>/`
2. **Commands**: Run in the isolated workdir

When workdir is NOT enabled, source is vendored to `components/<type>/<component>/`.

## Prerequisites

### Remote Sources Used

This fixture uses the following public GitHub repositories as remote sources:

| Component Type | Repository | Version |
|----------------|------------|---------|
| Terraform | `github.com/cloudposse/terraform-null-label//exports` | `0.25.0` |
| Helmfile | `github.com/cloudposse-archives/helmfiles//releases/nginx-ingress` | `0.126.0` |
| Packer | `github.com/aws-samples/amazon-eks-custom-amis` | `main` |

### Authentication

All sources are public repositories and require no authentication for read access. If you need to test with private repositories, set the `GITHUB_TOKEN` environment variable or use the `--identity` flag.

### Environment

- **Atmos CLI**: Must be built and available (run `make build` from the repository root)
- **Network Access**: Required to download remote sources
- **Working Directory**: Tests should be run from `tests/fixtures/scenarios/source-provisioner-workdir/`

## Components

### Terraform

| Component | Source | Workdir | Description |
|-----------|--------|---------|-------------|
| `vpc-remote` | Yes | No | Remote source, no workdir isolation |
| `vpc-remote-workdir` | Yes | Yes | Remote source WITH workdir isolation |
| `mock-workdir` | No | Yes | Local component with workdir isolation |

### Helmfile

| Component | Source | Workdir | Description |
|-----------|--------|---------|-------------|
| `nginx-workdir` | Yes | Yes | Remote source WITH workdir isolation |

### Packer

| Component | Source | Workdir | Description |
|-----------|--------|---------|-------------|
| `ami-workdir` | Yes | Yes | Remote source WITH workdir isolation |

## Manual Testing

### Terraform: Source + Workdir

```bash
cd tests/fixtures/scenarios/source-provisioner-workdir

# Vendor the source (should go to .workdir/terraform/dev-vpc-remote-workdir/)
atmos terraform source pull vpc-remote-workdir --stack dev

# Verify workdir location
ls -la .workdir/terraform/dev-vpc-remote-workdir/

# Run terraform plan (uses same workdir)
atmos terraform plan vpc-remote-workdir -s dev
```

### Helmfile: Source + Workdir

```bash
# Vendor the source (should go to .workdir/helmfile/dev-nginx-workdir/)
atmos helmfile source pull nginx-workdir --stack dev

# Verify workdir location
ls -la .workdir/helmfile/dev-nginx-workdir/
```

### Packer: Source + Workdir

```bash
# Vendor the source (should go to .workdir/packer/dev-ami-workdir/)
atmos packer source pull ami-workdir --stack dev

# Verify workdir location
ls -la .workdir/packer/dev-ami-workdir/
```

### Terraform: Source Only (no workdir)

```bash
# Vendor the source (should go to components/terraform/vpc-remote/)
atmos terraform source pull vpc-remote --stack dev

# Verify component location
ls -la components/terraform/vpc-remote/
```

## Cleanup

```bash
# Clean all workdirs
rm -rf .workdir/

# Clean vendored components (source-only)
rm -rf components/terraform/vpc-remote/
```

## Directory Structure After Testing

```text
tests/fixtures/scenarios/source-provisioner-workdir/
├── atmos.yaml
├── .gitignore
├── README.md
├── components/
│   ├── terraform/
│   │   ├── mock/                          # Local component
│   │   │   └── main.tf
│   │   └── vpc-remote/                    # Vendored (source-only, ignored)
│   ├── helmfile/                          # Empty - workdir-enabled components go to .workdir/
│   └── packer/                            # Empty - workdir-enabled components go to .workdir/
├── .workdir/                              # Created by source provisioner when workdir enabled
│   ├── terraform/
│   │   └── dev-vpc-remote-workdir/
│   ├── helmfile/
│   │   └── dev-nginx-workdir/
│   └── packer/
│       └── dev-ami-workdir/
└── stacks/
    ├── catalog/
    │   ├── source-only.yaml               # Terraform: remote source, no workdir
    │   ├── source-with-workdir.yaml       # Terraform: remote source + workdir
    │   ├── local-with-workdir.yaml        # Terraform: local component + workdir
    │   ├── helmfile-source-with-workdir.yaml  # Helmfile: remote source + workdir
    │   └── packer-source-with-workdir.yaml    # Packer: remote source + workdir
    └── deploy/
        └── dev.yaml                       # Stack configuration
```
