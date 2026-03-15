# {{.Config.project_name}}

Complete Atmos project created with `atmos init atmos`.

This project includes a fully configured `atmos.yaml` with:
- Terraform backend configuration
- Stack file organization
- Schema validation setup
- Template engine configuration
- Workflow support
- Vendoring configuration

## Directory Structure

```
{{.Config.project_name}}/
├── atmos.yaml              # Main configuration
├── components/             # Component definitions
│   ├── terraform/         # Terraform components
│   └── helmfile/          # Helmfile components
├── stacks/                # Stack configurations
├── schemas/               # Validation schemas
│   ├── jsonschema/       # JSON schemas
│   └── opa/              # OPA policies
├── workflows/             # Workflow definitions
└── vendor/               # Vendored components
```

## Next Steps

1. Add your Terraform components in `components/terraform/`
2. Create stack configurations in `stacks/`
3. Set up your S3 backend: `{{.Config.project_name}}-terraform-state`
4. Configure DynamoDB table: `{{.Config.project_name}}-terraform-state-lock`
5. Run `atmos terraform plan <component> -s <stack>` to get started
