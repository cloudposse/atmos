# Workdir Provisioner Test Fixture

This fixture tests the workdir provisioner feature which enables isolated working directories for concurrent terraform operations.

## Features Tested

- `provision.workdir.enabled: true` - Component workdir isolation
- Multiple components with workdir enabled
- Multiple stacks using the same components
- Workdir CLI commands (list, show, describe, clean)

## Manual Testing

```bash
# Navigate to fixture
cd tests/fixtures/scenarios/workdir

# Build atmos
make build

# List stacks
../../../../build/atmos list stacks

# Describe a component (triggers workdir provisioning)
../../../../build/atmos describe component vpc -s dev

# Check workdir was created
ls -la .workdir/terraform/

# List workdirs
../../../../build/atmos terraform workdir list

# Show workdir details
../../../../build/atmos terraform workdir show vpc --stack dev

# Describe workdir as manifest
../../../../build/atmos terraform workdir describe vpc --stack dev

# Clean specific workdir
../../../../build/atmos terraform workdir clean vpc --stack dev

# Clean all workdirs
../../../../build/atmos terraform workdir clean --all
```

## Directory Structure

```
workdir/
├── atmos.yaml                 # Main config
├── .gitignore                 # Ignore .workdir/
├── components/
│   └── terraform/
│       ├── vpc/               # VPC component
│       │   └── main.tf
│       └── s3-bucket/         # S3 bucket component
│           └── main.tf
├── stacks/
│   ├── catalog/
│   │   └── workdir-defaults.yaml   # Shared defaults with workdir enabled
│   └── deploy/
│       ├── dev.yaml           # Dev stack
│       ├── staging.yaml       # Staging stack
│       └── prod.yaml          # Prod stack
└── .workdir/                  # Generated workdir (gitignored)
    └── terraform/
        ├── vpc/               # Isolated VPC execution
        └── s3-bucket/         # Isolated S3 execution
```
