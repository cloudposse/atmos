# Terraform Generate Files Test Fixture

This fixture tests the `atmos terraform generate files` command functionality.

## Structure

```text
terraform-generate-files/
├── atmos.yaml                    # Atmos configuration
├── components/
│   └── terraform/
│       ├── vpc/
│       │   └── main.tf           # VPC component
│       └── s3-bucket/
│           └── main.tf           # S3 bucket component
└── stacks/
    ├── catalog/
    │   └── generate-defaults.yaml  # Abstract components with generate sections
    └── deploy/
        ├── dev.yaml              # Dev stack with generate sections
        └── prod.yaml             # Prod stack with generate sections
```

## Test Scenarios

### File Type Generation
- **HCL (.tf)**: locals.tf with locals block
- **JSON (.json)**: metadata.json with component info
- **YAML (.yaml)**: config.yaml with bucket configuration
- **Markdown (.md)**: README.md with component documentation

### Inheritance
- Components inherit generate sections from abstract components in catalog
- Child components can extend or override inherited generate sections

### Commands Tested
```bash
# Generate files for single component
atmos terraform generate files vpc -s dev

# Dry-run mode
atmos terraform generate files vpc -s dev --dry-run

# Generate all
atmos terraform generate files --all

# Filter by stacks
atmos terraform generate files --all --stacks="dev"

# Filter by components
atmos terraform generate files --all --components="vpc"

# Clean generated files
atmos terraform generate files vpc -s dev --clean
```

## Generated Files

After running `atmos terraform generate files vpc -s dev`:

- `components/terraform/vpc/locals.tf` - HCL locals block
- `components/terraform/vpc/metadata.json` - JSON metadata
- `components/terraform/vpc/README.md` - Markdown documentation
