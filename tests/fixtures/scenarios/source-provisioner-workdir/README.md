# Source Provisioner + Workdir Integration Test Fixture

This test fixture demonstrates the integration between the source provisioner and workdir provisioner features.

## Overview

When both `source` and `provision.workdir.enabled` are configured for a component:

1. **Source Provisioner** (CLI-driven): Vendors remote source to `components/terraform/<component>/`
2. **Workdir Provisioner** (hook-based): Copies from local component to `.workdir/terraform/<stack>-<component>/`
3. **Terraform**: Runs in the isolated workdir

## Components

| Component | Source | Workdir | Description |
|-----------|--------|---------|-------------|
| `vpc-remote` | Yes | No | Remote source, no workdir isolation |
| `vpc-remote-workdir` | Yes | Yes | Remote source WITH workdir isolation |
| `mock-workdir` | No | Yes | Local component with workdir isolation |

## Manual Testing

### Test 1: Source Only (no workdir)

```bash
cd tests/fixtures/scenarios/source-provisioner-workdir

# Describe source configuration
atmos terraform source describe vpc-remote --stack dev

# Vendor the source
atmos terraform source pull vpc-remote --stack dev

# Verify vendored location
ls -la components/terraform/vpc-remote/

# Run terraform (directly in vendored directory)
atmos terraform plan vpc-remote -s dev --dry-run
```

### Test 2: Source + Workdir (combined)

```bash
# Describe source configuration
atmos terraform source describe vpc-remote-workdir --stack dev

# Vendor the source
atmos terraform source pull vpc-remote-workdir --stack dev

# Verify vendored location
ls -la components/terraform/vpc-remote-workdir/

# Run terraform (workdir provisioner will copy to .workdir/)
atmos terraform plan vpc-remote-workdir -s dev --dry-run

# Verify workdir was created
ls -la .workdir/terraform/dev-vpc-remote-workdir/
```

### Test 3: Local + Workdir (for comparison)

```bash
# No source pull needed - component is local

# Run terraform (workdir provisioner copies local component)
atmos terraform plan mock-workdir -s dev --dry-run

# Verify workdir was created
ls -la .workdir/terraform/dev-mock-workdir/
```

## Cleanup

```bash
# Clean vendored components
rm -rf components/terraform/vpc-remote/
rm -rf components/terraform/vpc-remote-workdir/

# Clean all workdirs
atmos terraform workdir clean --all

# Or manually
rm -rf .workdir/
```

## Directory Structure After Testing

```text
tests/fixtures/scenarios/source-provisioner-workdir/
├── atmos.yaml
├── .gitignore
├── README.md
├── components/terraform/
│   ├── mock/                          # Local component
│   │   └── main.tf
│   ├── vpc-remote/                    # Vendored by source pull (ignored)
│   │   └── *.tf
│   └── vpc-remote-workdir/            # Vendored by source pull (ignored)
│       └── *.tf
├── .workdir/terraform/                # Created by workdir provisioner (ignored)
│   ├── dev-vpc-remote-workdir/        # Isolated execution directory
│   │   ├── *.tf
│   │   └── .workdir-metadata.json
│   └── dev-mock-workdir/              # Isolated execution directory
│       ├── *.tf
│       └── .workdir-metadata.json
└── stacks/
    ├── catalog/
    │   ├── source-only.yaml           # Remote source, no workdir
    │   ├── source-with-workdir.yaml   # Remote source + workdir
    │   └── local-with-workdir.yaml    # Local component + workdir
    └── deploy/
        └── dev.yaml                   # Stack configuration
```
